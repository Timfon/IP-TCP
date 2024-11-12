package iptcpstack

import (
	"fmt"
	"math/rand"
	"net/netip"
	"time"
	"github.com/google/netstack/tcpip/header"
  "github.com/smallnest/ringbuffer"
)

type SocketStatus int

const (
	Listening    SocketStatus = 0
	SynSent      SocketStatus = 1
	SynReceived  SocketStatus = 2
	Established  SocketStatus = 3
)

type Socket struct {
	SID    int
	Conn   *VTCPConn
	Listen *VTCPListener
	Closed bool
}

type Window struct {
    recvBuffer     *ringbuffer.RingBuffer  // Buffer for receiving data
    sendBuffer     *ringbuffer.RingBuffer  // Buffer for sending data

    SendUna        uint32
    SendNxt        uint32
    SendWindowSize uint32
    SendLBW        uint32

    RecvNext       uint32
    RecvWindowSize uint32
    RecvLBR        uint32
    
    // Channel to signal data arrival
    DataAvailable chan struct{}
}
type VTCPConn struct {
	State      SocketStatus
	LocalAddr  netip.Addr
	LocalPort  uint16
	RemoteAddr netip.Addr
	RemotePort uint16
	SeqNum     uint32
	AckNum     uint32

	Window *Window
}

type VTCPListener struct {
	AcceptQueue chan *VTCPConn
	LocalPort   uint16
	Closed      bool

	// Info about previous packet
}

const (
	maxRetries    = 3
	retryTimeout  = 3 * time.Second
)

func NewWindow(size int) *Window {
    w := &Window{
        recvBuffer:     ringbuffer.New(int(size)),
        sendBuffer:     ringbuffer.New(int(size)),
        SendWindowSize: uint32(size),
        RecvWindowSize: uint32(size),
        DataAvailable:  make(chan struct{}, 1), // Buffer of 1 to prevent blocking on signal
    }
    return w
}

func (c *VTCPConn) VRead(buf []byte) (int, error) {
    if c.State != Established {
        return 0, fmt.Errorf("connection not established")
    }
    
    // Calculate available data using TCP sequence numbers
    availData := int(c.Window.RecvNext - c.Window.RecvLBR)
    fmt.Printf("Debug - Available data: %d (RecvNext: %d, RecvLBR: %d)\n", 
              availData, c.Window.RecvNext, c.Window.RecvLBR)
              
    if availData > 0 {
        // Read from receive buffer
        readLen := len(buf)
        if readLen > availData {
            readLen = availData
        }
        
        n, err := c.Window.recvBuffer.Read(buf[:readLen])
        if err != nil {
            return 0, fmt.Errorf("error reading from receive buffer: %v", err)
        }
        
        // Update TCP read pointer
        c.Window.RecvLBR += uint32(n)
        fmt.Printf("Debug - Read %d bytes: %q\n", n, buf[:n])
        return n, nil
    }
    
    // No data available, wait for more
    fmt.Println("Debug - No data available, waiting...")
    select {
    case <-c.Window.DataAvailable:
        fmt.Println("Debug - Received data notification")
        return c.VRead(buf)
    case <-time.After(30 * time.Second):
        return 0, fmt.Errorf("read timeout")
    }
}

func (c *VTCPConn) VWrite(data []byte, stack *IPStack, sock *Socket) (int, error) {
    if c.State != Established {
        return 0, fmt.Errorf("connection not established")
    }

    // Check available space in send window
    availSpace := int(c.Window.SendWindowSize - (c.Window.SendLBW - c.Window.SendUna))
	fmt.Errorf("flag")
    if availSpace <= 0 {
        return 0, fmt.Errorf("send buffer full")
    }
	fmt.Errorf("flag")

    writeLen := len(data)
    if writeLen > availSpace {
        writeLen = availSpace
    }

	fmt.Errorf("flag")
    // Write to send buffer
    n, err := c.Window.sendBuffer.Write(data[:writeLen])
    if err != nil {
        return 0, fmt.Errorf("failed to write to send buffer: %v", err)
    }

	fmt.Errorf("flag")
    c.Window.SendLBW += uint32(n)
    
    // Send the data

    err = stack.sendTCPPacket(sock, data[:writeLen], header.TCPFlagAck)
    if err != nil {
        return 0, fmt.Errorf("failed to send data: %v", err)
    }

    return n, nil
}

func (tcpStack *TCPStack) VConnect(addr netip.Addr, port uint16, ipStack *IPStack) (*VTCPConn, error) {
	// Find route to destination
	route, found, _ := ipStack.ForwardingTable.MatchPrefix(addr)
	if found == -1 {
		return nil, fmt.Errorf("no route to host %v", addr)
	}

	// Get local source IP based on route
	var localAddr netip.Addr
	if route.RoutingMode == 4 {
		localAddr = route.VirtualIP
	} else {
		iroute, _, _ := ipStack.ForwardingTable.MatchPrefix(route.VirtualIP)
		localAddr = iroute.VirtualIP
	}

	// Create new socket
	localPort := uint16(rand.Uint32() >> 16)
	seqNum := rand.Uint32()%100 * 1000
	conn := &VTCPConn{
		State:      SynSent,
		LocalAddr:  localAddr,
		LocalPort:  localPort,
		RemoteAddr: addr,
		RemotePort: port,
		SeqNum:     seqNum,
		AckNum:     0,
		Window: NewWindow(65535),
	}
	sock := &Socket{
		SID:  tcpStack.NextSocketID,
		Conn: conn,
	}
	tcpStack.NextSocketID++
	tcpStack.Sockets[sock.SID] = sock

	// Initial SYN send
	err := ipStack.sendTCPPacket(sock, []byte{}, header.TCPFlagSyn)
	if err != nil {
		delete(tcpStack.Sockets, sock.SID)
		return nil, fmt.Errorf("failed to send initial SYN packet: %v", err)
	}

	startTime := time.Now()
	retries := 0

	for {
		// Check if we've been trying too long
		if time.Since(startTime) > 30*time.Second {
			delete(tcpStack.Sockets, sock.SID)
			return nil, fmt.Errorf("connection timed out after 30 seconds")
		}

		// Check if connection established
		if sock.Conn.State == Established {
			return conn, nil
		}

		// Check if it's time to retry
		if time.Since(startTime) >= time.Duration(retries+1)*retryTimeout {
			if retries >= maxRetries {
				delete(tcpStack.Sockets, sock.SID)
				return nil, fmt.Errorf("connection failed after %d SYN retransmissions", maxRetries)
			}

			fmt.Printf("Retransmitting SYN (attempt %d/%d)\n", retries+1, maxRetries)
			err := ipStack.sendTCPPacket(sock, []byte{}, header.TCPFlagSyn)
			if err != nil {
				delete(tcpStack.Sockets, sock.SID)
				return nil, fmt.Errorf("failed to retransmit SYN packet: %v", err)
			}
			retries++
		}
	}
}

func (tcpStack *TCPStack) VListen(port uint16) (*VTCPListener, error) {
	l := &VTCPListener{
		AcceptQueue: make(chan *VTCPConn, 100),
		LocalPort:   port,
	}

	sock := &Socket{
		SID:    tcpStack.NextSocketID,
		Listen: l,
	}
	tcpStack.NextSocketID++
	tcpStack.Sockets[sock.SID] = sock
	return l, nil
}

func (l *VTCPListener) VAccept() (*VTCPConn, error) {
	if l.Closed {
		return nil, fmt.Errorf("Listener is closed")
	}

	conn, ok := <-l.AcceptQueue
	if !ok {
		return nil, fmt.Errorf("Listener is closed")
	}

	return conn, nil
}

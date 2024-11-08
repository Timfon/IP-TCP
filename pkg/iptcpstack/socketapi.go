package iptcpstack

import (
	"fmt"
	"math/rand"
	"net/netip"
	"time"
	"github.com/google/netstack/tcpip/header"
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
    // Send stuff
    SendBuf        []byte
    SendUna        uint32
    SendNxt        uint32
    SendWindowSize uint32
    SendLBW        uint32

    // Receive stuff
    RecvBuf        []byte
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
	SrcAddr netip.Addr
	DstAddr netip.Addr
	SrcPort uint16
	SeqNum  uint32
}

func NewWindow(size int) *Window {
    w := &Window{
        SendBuf:        make([]byte, size),
        RecvBuf:        make([]byte, size),
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

    // Wait for data with timeout
    timeout := time.After(30 * time.Second)

    for {
        // Check if data is available
        availData := int(c.Window.RecvNext - c.Window.RecvLBR)
        if availData > 0 {
            // Data available, process it
            readLen := len(buf)
            if readLen > availData {
                readLen = availData
            }

            // Copy data from receive buffer
            start := c.Window.RecvLBR % c.Window.RecvWindowSize
            if start+uint32(readLen) <= c.Window.RecvWindowSize {
                copy(buf, c.Window.RecvBuf[start:start+uint32(readLen)])
            } else {
                firstPart := c.Window.RecvWindowSize - start
                copy(buf, c.Window.RecvBuf[start:])
                copy(buf[firstPart:], c.Window.RecvBuf[:readLen-int(firstPart)])
            }

            c.Window.RecvLBR += uint32(readLen)
            return readLen, nil
        }

        // No data available, wait for notification or timeout
        select {
        case <-c.Window.DataAvailable:
            continue // Check for data again
        case <-timeout:
            return 0, fmt.Errorf("read timeout")
        }
    }
}

func (c *VTCPConn) VWrite(data []byte, stack *IPStack, sock *Socket) (int, error) {
    if c.State != Established {
        return 0, fmt.Errorf("connection not established")
    }

    availSpace := int(c.Window.SendWindowSize - (c.Window.SendLBW - c.Window.SendUna))
    if availSpace <= 0 {
        return 0, fmt.Errorf("send buffer full")
    }

    writeLen := len(data)
    if writeLen > availSpace {
        writeLen = availSpace
    }

    // Copy to send buffer
    start := c.Window.SendLBW % c.Window.SendWindowSize
    if start+uint32(writeLen) <= c.Window.SendWindowSize {
        copy(c.Window.SendBuf[start:], data[:writeLen])
    } else {
        firstPart := c.Window.SendWindowSize - start
        copy(c.Window.SendBuf[start:], data[:firstPart])
        copy(c.Window.SendBuf[:writeLen-int(firstPart)], data[firstPart:writeLen])
    }

    c.Window.SendLBW += uint32(writeLen)
    
    // Actually send the data using TCP
    err := stack.sendTCPPacket(sock, data[:writeLen], header.TCPFlagAck)
    if err != nil {
        return 0, fmt.Errorf("failed to send data: %v", err)
    }

    return writeLen, nil
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
	conn := &VTCPConn{
		State:      SynSent,
		LocalAddr:  localAddr,
		LocalPort:  localPort,
		RemoteAddr: addr,
		RemotePort: port,
		SeqNum:     uint32(time.Now().UnixNano()),
		AckNum:     0,
		Window:     NewWindow(65535),
	}
	sock := &Socket{
		SID:  tcpStack.NextSocketID,
		Conn: conn,
	}
	tcpStack.NextSocketID++
	tcpStack.Sockets[sock.SID] = sock

	const (
		maxRetries    = 3
		retryTimeout  = 3 * time.Second
	)

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

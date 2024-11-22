package iptcpstack

import (
	"fmt"
	"math/rand"
	"net/netip"
	"time"
	"github.com/google/netstack/tcpip/header"
  "github.com/smallnest/ringbuffer"
  "os"
  "io"
  "container/heap"
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
    RetransmissionQueue *RetransmissionQueue
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
  SID int
  ReceiveQueue *PriorityQueue
}

type VTCPListener struct {
	AcceptQueue chan *VTCPConn
	LocalPort   uint16
	Closed      bool

	// Info about previous packet
}

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
    if c == nil {
        return 0, fmt.Errorf("connection is nil")
    }
    if c.State != Established {
        return 0, fmt.Errorf("connection not established")
    }
    if c.Window == nil {
        return 0, fmt.Errorf("connection window is nil")
    }
    if c.Window.recvBuffer == nil {
        return 0, fmt.Errorf("receive buffer is nil")
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
        fmt.Printf("Debug - Read %d bytes\n", n)
        return n, nil
    }
    
    // No data available, wait for more
    fmt.Println("Debug - No data available, waiting...")
    select {
    case <-c.Window.DataAvailable:
        fmt.Println("Debug - Received data notification")
        return c.VRead(buf)  // Try reading again
    case <-time.After(5 * time.Second):  // Shorter timeout for individual reads
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

    // entry := &RetransmissionEntry{
    //   Data: data[:writeLen],
    //   SeqNum: c.SeqNum,
    //   SendTime: time.Now(),
    //   RTO: 1 * time.Second,
    // }
    // c.Window.RetransmissionQueue.Entries = append(c.Window.RetransmissionQueue.Entries, entry)

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
    SID: tcpStack.NextSocketID,
	}

  newq := make(PriorityQueue, 0)
  heap.Init(&newq)

  conn.ReceiveQueue = &newq

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

	// startTime := time.Now()
	// retries := 0
	// for {
	// 	// Check if we've been trying too long
	// 	if time.Since(startTime) > 30*time.Second {
	// 		delete(tcpStack.Sockets, sock.SID)
	// 		return nil, fmt.Errorf("connection timed out after 30 seconds")
	// 	}
	//
	// 	// Check if connection established
	// 	if sock.Conn.State == Established {
	// 		return conn, nil
	// 	}
	// 	// Check if it's time to retry
	// 	if time.Since(startTime) >= time.Duration(retries+1)*retryTimeout {
	// 		if retries >= maxRetries {
	// 			delete(tcpStack.Sockets, sock.SID)
	// 			return nil, fmt.Errorf("connection failed after %d SYN retransmissions", maxRetries)
	// 		}
	//
	// 		fmt.Printf("Retransmitting SYN (attempt %d/%d)\n", retries+1, maxRetries)
	// 		err := ipStack.sendTCPPacket(sock, []byte{}, header.TCPFlagSyn)
	// 		if err != nil {
	// 			delete(tcpStack.Sockets, sock.SID)
	// 			return nil, fmt.Errorf("failed to retransmit SYN packet: %v", err)
	// 		}
	// 		retries++
	// 	}
	// }
  return conn, nil
}

func (tcpStack *TCPStack) VListen(port uint16) (*VTCPListener, error) {
  if tcpStack.Listeners == nil {
        tcpStack.Listeners = make(map[uint16]*VTCPListener)
    }
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
  tcpStack.Listeners[port] = l
  //fmt.Println("TCPSTACK LISTENERS", tcpStack.Listeners)
	return l, nil
}

// Update VAccept to handle timeouts
func (l *VTCPListener) VAccept() (*VTCPConn, error) {
    if l.Closed {
        return nil, fmt.Errorf("Listener is closed")
    }

    // Wait for connection with timeout
    select {
    case conn, ok := <-l.AcceptQueue:
        if !ok {
            return nil, fmt.Errorf("Listener is closed")
        }
        fmt.Printf("Accepted connection, state: %d\n", conn.State)
        return conn, nil
    case <-time.After(30 * time.Second):
        return nil, fmt.Errorf("Accept timeout")
    }
}
func ACommand(port uint16, tcpstack *TCPStack){
    // Create listening socket
    listenConn, err := tcpstack.VListen(port)
    //fmt.Println(tcpstack.Listeners[port])
    if err != nil {
        fmt.Println(err)
        return
    }
    // Start a goroutine to continuously accept connections
    //fmt.Println("Acommand")
    go func() {
        for {
            //fmt.Println("Acommand1")
            _, err := listenConn.VAccept()
            //fmt.Println("Acommand2")
            if err != nil {
              // TODO: If the listener is closed, exit the goroutine
                fmt.Println(err)
                continue
            }
            // The connection is now established and ready for use by other REPL commands
            // We don't need to do anything else with it here
        }
    }()

}


//sendfile and receive file should do the whole c connect and accept!
// Update SendFile to wait for connection establishment
func SendFile(stack *IPStack, filepath string, destAddr netip.Addr, port uint16, tcpStack *TCPStack) (int, error) {
    file, err := os.Open(filepath)
    if err != nil {
        return 0, fmt.Errorf("failed to open file: %v", err)
    }
    defer file.Close()

    // Establish connection
    conn, err := tcpStack.VConnect(destAddr, port, stack)
    if err != nil {
        return 0, fmt.Errorf("failed to establish connection: %v", err)
    }

    // Wait for connection to be established
    startTime := time.Now()
    for time.Since(startTime) < 30*time.Second {
        if conn.State == Established {
            break
        }
        time.Sleep(100 * time.Millisecond)
    }
    
    if conn.State != Established {
        return 0, fmt.Errorf("connection failed to establish")
    }

    buf := make([]byte, 1024)
    totalBytes := 0

    for {
        n, err := file.Read(buf)
        if err == io.EOF {
            break
        }
        if err != nil {
            return totalBytes, fmt.Errorf("failed to read from file: %v", err)
        }

        written, err := conn.VWrite(buf[:n], stack, tcpStack.Sockets[conn.SID])
        if err != nil {
            return totalBytes, fmt.Errorf("failed to write to connection: %v", err)
        }
        totalBytes += written
        conn.SeqNum += uint32(written)
    }

    return totalBytes, nil
}

//should do the whole accept and receive File
func ReceiveFile(stack *IPStack, filepath string, port uint16, tcpStack *TCPStack) (int, error) {
    fmt.Println("1. Starting ReceiveFile")

    // Create listener
    listenConn, err := tcpStack.VListen(port)
    if err != nil {
        return 0, fmt.Errorf("failed to create listener: %v", err)
    }
    fmt.Println("2. Listener created on port:", port)
    
    // Create output file
    file, err := os.Create(filepath)
    if err != nil {
        return 0, fmt.Errorf("failed to create file: %v", err)
    }
    defer file.Close()
    fmt.Println("3. Output file created:", filepath)
    
    fmt.Println("4. Waiting for connection...")
    conn, err := listenConn.VAccept()
    if err != nil {
        return 0, fmt.Errorf("failed to accept connection: %v", err)
    }
    if conn == nil {
        return 0, fmt.Errorf("accepted connection is nil")
    }
    fmt.Println("5. Connection accepted")
    fmt.Printf("Connection state: %d, Window: %+v\n", conn.State, conn.Window)

    // Initialize the connection's window if it's nil
    if conn.Window == nil {
        conn.Window = NewWindow(65535)
    }

    buf := make([]byte, 1024)
    totalBytes := 0
    
    fmt.Println("6. Starting to receive data")
    
    // Read with timeout
    readDeadline := time.Now().Add(30 * time.Second)
    for time.Now().Before(readDeadline) {
        fmt.Printf("Before read - SeqNum: %d, AckNum: %d\n", conn.SeqNum, conn.AckNum)
        
        n, err := conn.VRead(buf)
        if err != nil {
            if err.Error() == "read timeout" {
                fmt.Println("Read timeout reached")
                break
            }
            return totalBytes, fmt.Errorf("failed to read from connection: %v", err)
        }
        if n == 0 {
            // No more data to read
            fmt.Println("No more data available")
            break
        }
        // Reset timeout on successful read
        readDeadline = time.Now().Add(30 * time.Second)
        fmt.Printf("Received %d bytes - SeqNum: %d, AckNum: %d\n", n, conn.SeqNum, conn.AckNum)
        
        written, err := file.Write(buf[:n])
        if err != nil {
            return totalBytes, fmt.Errorf("failed to write to file: %v", err)
        }
        totalBytes += written
        
        fmt.Printf("Progress: %d bytes received and written to file\n", totalBytes)
    }
    
    fmt.Printf("8. Transfer complete. Total bytes: %d\n", totalBytes)
    return totalBytes, nil
}

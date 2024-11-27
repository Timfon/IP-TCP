package iptcpstack

import (
	"container/heap"
	"fmt"
	"io"
	"math/rand"
	"net/netip"
	"os"
	"time"

	"github.com/google/netstack/tcpip/header"
	"github.com/smallnest/ringbuffer"
)

type SocketStatus int

const (
	Listening   SocketStatus = 0
	SynSent     SocketStatus = 1
	SynReceived SocketStatus = 2
	Established SocketStatus = 3
	FinWait1    SocketStatus = 4
	FinWait2    SocketStatus = 5
	CloseWait   SocketStatus = 6
	TimeWait    SocketStatus = 7
	LastAck     SocketStatus = 8

	TimeWaitDuration = 15 * time.Second
)

type Socket struct {
	SID    int
	Conn   *VTCPConn
	Listen *VTCPListener
	Closed bool
}

type Window struct {
	recvBuffer *ringbuffer.RingBuffer // Buffer for receiving data
	sendBuffer *ringbuffer.RingBuffer // Buffer for sending data

	SendUna        uint32
	SendNxt        uint32
	SendWindowSize uint32
	SendLBW        uint32

	RecvNext       uint32
	RecvWindowSize uint32
	RecvLBR        uint32
	ReadWindowSize uint32

	// Channel to signal data arrival
	DataAvailable       chan struct{}
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

	Window       *Window
	SID          int
	ReceiveQueue *PriorityQueue

	retransmissionStarted bool
}

type VTCPListener struct {
	AcceptQueue chan *VTCPConn
	LocalPort   uint16
	Closed      bool
	// Info about previous packet
}

func NewWindow(size int) *Window {
	w := &Window{
		recvBuffer:          ringbuffer.New(int(size)),
		sendBuffer:          ringbuffer.New(int(size)),
		SendWindowSize:      uint32(size),
		RecvWindowSize:      uint32(size),
		DataAvailable:       make(chan struct{}, 10), // Buffer of 10 to prevent blocking on signal
		SendUna:             0,
		SendLBW:             0,
		RetransmissionQueue: NewRetransmissionQueue(),
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
	//fmt.Printf("Debug - Available data: %d (RecvNext: %d, RecvLBR: %d)\n",
	//availData, c.Window.RecvNext, c.Window.RecvLBR)

	// Update receive window size based on buffer space
	c.Window.RecvWindowSize = uint32(c.Window.recvBuffer.Free())
	//fmt.Printf("Debug - Updated receive window size to %d\n", c.Window.RecvWindowSize)

	// Wait for data if none is available
	for availData == 0 {
		select {
		case <-c.Window.DataAvailable:
			availData = int(c.Window.RecvNext - c.Window.RecvLBR)
			//fmt.Printf("Debug - Received notification, now available: %d\n", availData)
		case <-time.After(5 * time.Second):
			if c.ReceiveQueue == nil || c.ReceiveQueue.Len() == 0 {
				return 0, fmt.Errorf("read timeout")
			}
			continue
		}
	}

	// Read data from buffer
	readLen := len(buf)
	if readLen > availData {
		readLen = availData
	}

	n, err := c.Window.recvBuffer.Read(buf[:readLen])
	if err != nil {
		return 0, fmt.Errorf("error reading from receive buffer: %v", err)
	}

	// Update TCP read pointer and window size
	c.Window.RecvLBR += uint32(n)
	//c.Window.RecvWindowSize = uint32(c.Window.recvBuffer.Free())
	//fmt.Printf("Debug - Read %d bytes, new window size: %d\n", n, c.Window.RecvWindowSize)
	return n, nil
}

func (w *Window) RemoveAckedData(bytesAcked uint32) {
	// Create a temporary buffer to discard the acknowledged bytes
	tmp := make([]byte, bytesAcked)
	w.sendBuffer.Read(tmp) // Remove the acknowledged data from send buffer
}

func (c *VTCPConn) VWrite(data []byte, stack *IPStack, sock *Socket, tcpstack *TCPStack) (int, error) {
	if c.State != Established {
		return 0, fmt.Errorf("connection not established")
	}

	//if !c.retransmissionStarted {
	//	go c.HandleRetransmission(stack, sock, tcpstack)
	//	c.retransmissionStarted = true
	//}

	currWritten := 0
	fmt.Println(c.Window.SendNxt)
	c.Window.SendLBW = c.Window.SendNxt + uint32(len(data))

	for c.Window.SendNxt < c.Window.SendLBW {
		// Get available space from both send buffer and receiver window
		sendBufferSpace := c.Window.sendBuffer.Free()
		receiverWindow := int(c.Window.ReadWindowSize)

		// Check for zero window condition
		if receiverWindow == 0 {
			fmt.Printf("Zero window detected, starting window probing\n")
			err := c.handleZeroWindow(stack, sock)
			if err != nil {
				return currWritten, fmt.Errorf("zero window probe failed: %v", err)
			}
			// After successful probe, get updated window size
			receiverWindow = int(c.Window.ReadWindowSize)
		}

		availableSpace := min(sendBufferSpace, receiverWindow)

		// If we have space to write
		if availableSpace > 0 {
			// Limit each write to MSS (512 in this case)
			writeLen := min(min(availableSpace, 512), len(data)-currWritten)

			// Write to send buffer
			n, err := c.Window.sendBuffer.Write(data[currWritten : currWritten+writeLen])
			if err != nil {
				return currWritten, fmt.Errorf("failed to write to send buffer: %v", err)
			}

			// Add to retransmission queue
			//c.Window.RetransmissionQueue.AddEntry(data[currWritten:currWritten+n], c.SeqNum)

			// Send the data
			err = stack.sendTCPPacket(sock, data[currWritten:currWritten+n], header.TCPFlagAck)
			if err != nil {
				return currWritten, fmt.Errorf("failed to send data: %v", err)
			}

			c.Window.SendNxt += uint32(n)
			currWritten += n
		} else {
			// No space available, wait before retrying
			// Use a shorter sleep when window is non-zero but full
			if receiverWindow > 0 {
				time.Sleep(2 * time.Millisecond)
			} else {
				// Use longer sleep when waiting for zero window probe response
				time.Sleep(10 * time.Millisecond)
			}
		}
	}
	return currWritten, nil
}

// Utility function to find minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *VTCPConn) VClose(stack *IPStack, sock *Socket) error {
	switch c.State {
	case Established:
		sock.Conn.Window.SendNxt = sock.Conn.Window.SendNxt
		err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagFin)
		if err != nil {
			return fmt.Errorf("failed to send FIN packet: %v", err)
		}
		c.State = FinWait1
		fmt.Println("Sent FIN, moving to FIN_WAIT_1")

	case CloseWait:
		// When in CLOSE_WAIT, send FIN and move to LAST_ACK
		sock.Conn.Window.SendNxt = sock.Conn.Window.SendNxt
		err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagFin)
		if err != nil {
			return fmt.Errorf("failed to send FIN packet: %v", err)
		}
		c.State = LastAck
		fmt.Println("Sent FIN, moving to LAST_ACK")

	default:
		return fmt.Errorf("cannot close connection in current state: %v", c.State)
	}

	return nil
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
	seqNum := rand.Uint32() >> 16
	conn := &VTCPConn{
		State:      SynSent,
		LocalAddr:  localAddr,
		LocalPort:  localPort,
		RemoteAddr: addr,
		RemotePort: port,
		SeqNum:     seqNum,
		AckNum:     0,
		Window:     NewWindow(65535),
		SID:        tcpStack.NextSocketID,
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
	return conn, nil
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
		return conn, nil
	}
}

func ACommand(port uint16, tcpstack *TCPStack) {
	// Create listening socket
	listenConn, err := tcpstack.VListen(port)
	if err != nil {
		fmt.Println(err)
		return
	}
	// Start a goroutine to continuously accept connections
	go func() {
		for {
			_, err := listenConn.VAccept()
			if err != nil {
				// TODO: If the listener is closed, exit the goroutine
				fmt.Println(err)
				continue
			}
		}
	}()
}

// sendfile and receive file should do the whole c connect and accept!
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

	buf := make([]byte, 512)
	totalBytes := 0

	for {
		n, err := file.Read(buf)
		if err == io.EOF {
			fmt.Printf("reached end of file, terminating send")
			break
		}
		if err != nil {
			return totalBytes, fmt.Errorf("failed to read from file: %v", err)
		}
		// Keep writing until all data from this read is sent
		bytesWritten := 0
		for bytesWritten < n {
			written, err := conn.VWrite(buf[bytesWritten:n], stack, tcpStack.Sockets[conn.SID], tcpStack)
			if err != nil {
				return totalBytes, fmt.Errorf("failed to write to connection: %v", err)
			}
			bytesWritten += written
			totalBytes += written
		}
		if tcpStack.Sockets[conn.SID].Listen != nil {
			tcpStack.Sockets[conn.SID].Listen.AcceptQueue <- conn
		}
	}

	conn.VClose(stack, tcpStack.Sockets[conn.SID])
	return totalBytes, nil
}

// should do the whole accept and receive File
func ReceiveFile(stack *IPStack, filepath string, port uint16, tcpStack *TCPStack) (int, error) {
	// Create listener
	listenConn, err := tcpStack.VListen(port)
	if err != nil {
		return 0, fmt.Errorf("failed to create listener: %v", err)
	}

	// Create output file
	file, err := os.Create(filepath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	conn, err := listenConn.VAccept()
	if err != nil {
		return 0, fmt.Errorf("failed to accept connection: %v", err)
	}
	if conn == nil {
		return 0, fmt.Errorf("accepted connection is nil")
	}

	// Initialize the connection's window if it's nil
	if conn.Window == nil {
		conn.Window = NewWindow(65535)
	}

	buf := make([]byte, 1024)
	totalBytes := 0

	for {
		//fmt.Printf("Before read - SeqNum: %d, AckNum: %d\n", conn.SeqNum, conn.AckNum)
		n, err := conn.VRead(buf)

		if err != nil {
			if err.Error() == "read timeout" {
				// Only break on timeout if we've received some data
				if totalBytes > 0 {
					break
				}
			}
			return totalBytes, fmt.Errorf("failed to read from connection: %v", err)
		}

		if n > 0 {
			//fmt.Printf("Received %d bytes - SeqNum: %d, AckNum: %d\n", n, conn.SeqNum, conn.AckNum)
			written, err := file.Write(buf[:n])
			if err != nil {
				return totalBytes, fmt.Errorf("failed to write to file: %v", err)
			}
			totalBytes += written
			//fmt.Printf("Progress: %d bytes received and written to file\n", totalBytes)

			// Don't break here - keep reading until timeout or error
		} else {
			// Only break on zero bytes if we've received some data already
			if totalBytes > 0 {
				time.Sleep(1 * time.Millisecond) // Small delay to check for more data
				select {
				case <-conn.Window.DataAvailable:
					continue // More data is available, keep reading
				default:
					break // No more data available
				}
			}
		}
	}

	fmt.Printf("Transfer complete. Total bytes: %d\n", totalBytes)
	conn.VClose(stack, tcpStack.Sockets[conn.SID])
	return totalBytes, nil
}

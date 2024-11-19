package iptcpstack

import (
	"fmt"
	"math/rand"
	"net/netip"
	"time"
	"github.com/google/netstack/tcpip/header"
  "github.com/smallnest/ringbuffer"
  "sync"
  "os"
  "io"
)

type RetransmissionEntry struct {
  Data []byte
  SeqNum uint32
  SendTime time.Time
  RTO time.Duration
}

type RetransmissionQueue struct {
  Entries []*RetransmissionEntry
  mutex sync.Mutex
  smoothRTT time.Duration
  rttAlpha float64
  rttBeta float64
}

//Initialize the retransmission queue
func NewRetransmissionQueue() *RetransmissionQueue {
  return &RetransmissionQueue{
    Entries: make([]*RetransmissionEntry, 0),
    smoothRTT: 1 * time.Second, // initial SRTT 1 sec
    rttAlpha: 0.125,
    rttBeta: 0.25,
  }
}

//Add a new entry to the retransmission queue
func (rq *RetransmissionQueue) AddEntry(data []byte, seqNum uint32) {
  rq.mutex.Lock()
  defer rq.mutex.Unlock()

  entry := &RetransmissionEntry{
    Data: data,
    SeqNum: seqNum,
    SendTime: time.Now(),
    RTO: rq.smoothRTT,
  }
  rq.Entries = append(rq.Entries, entry)
}

func (rq *RetransmissionQueue) RemoveAckedEntries(ackNum uint32) {
  rq.mutex.Lock()
  defer rq.mutex.Unlock()

  for i := 0; i < len(rq.Entries); i++ {
    if rq.Entries[i].SeqNum < ackNum {
      rq.Entries = append(rq.Entries[:i], rq.Entries[i+1:]...)
      i--
    }
  }
}
//Get Earliest
func (rq *RetransmissionQueue) GetEarliest() *RetransmissionEntry {
  rq.mutex.Lock()
  defer rq.mutex.Unlock()

  if len(rq.Entries) == 0 {
    return nil
  }

  return rq.Entries[0]
}


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

    // entry := &RetransmissionEntry{
    //   Data: data[:writeLen],
    //   SeqNum: c.SeqNum,
    //   SendTime: time.Now(),
    //   RTO: 1 * time.Second,
    // }
    // c.Window.RetransmissionQueue.Entries = append(c.Window.RetransmissionQueue.Entries, entry)

    return n, nil
}

//Handle the retransmission of packets
func (c *VTCPConn) HandleRetransmission(stack *IPStack, sock *Socket) {
  for {
    c.Window.RetransmissionQueue.mutex.Lock()
    for _, entry := range c.Window.RetransmissionQueue.Entries {
      if time.Since(entry.SendTime) > entry.RTO {
        fmt.Println("Retransmitting packet")
        stack.sendTCPPacket(sock, entry.Data, header.TCPFlagAck)
        entry.SendTime = time.Now()
        entry.RTO *= 2
      }
    }
    c.Window.RetransmissionQueue.mutex.Unlock()
    time.Sleep(1 * time.Second)
  }
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

//sendfile and receive file should do the whole c connect and accept!
func SendFile(stack *IPStack, filepath string, destAddr netip.Addr, port uint16, tcpStack *TCPStack)(int, error) {
    // Open the file
    file, err := os.Open(filepath)
    if err != nil {
      return 0, fmt.Errorf("failed to open file: %v", err)
    }
    defer file.Close()

    // Establish the TCP connection
    conn, err := tcpStack.VConnect(destAddr, port, stack)
    if err != nil {
      return 0, fmt.Errorf("failed to establish connection: %v", err)
    }
    //defer conn.VClose() // Close connection after file transfer

    buf := make([]byte, 1024) // Chunk size
    var totalBytes int // Track total bytes written

    for {
        // Read from file
        n, err := file.Read(buf)
        if err == io.EOF {
            break
        }
        if err != nil {
          return 0, fmt.Errorf("failed to read from file: %v", err)
        }

        // Write to the connection
        numBytes, err := conn.VWrite(buf[:n], stack, tcpStack.Sockets[conn.SID])
        if err != nil {
          return 0, fmt.Errorf("failed to write to connection: %v", err)
        }

        totalBytes += numBytes
    }
    return totalBytes, nil
}

//should do the whole accept and receive File
func ReceiveFile(stack *IPStack, filepath string, port uint16, tcpStack *TCPStack) (int, error) {
    // Create listening socket
    listenConn, err := tcpStack.VListen(port)
    if err != nil {
        return 0, fmt.Errorf("failed to create listener: %v", err)
    }
    // Create or truncate the output file
    file, err := os.Create(filepath)
    if err != nil {
        return 0, fmt.Errorf("failed to create file: %v", err)
    }
    defer file.Close()
    // Accept a connection
    conn, err := listenConn.VAccept()
    if err != nil {
        return 0, fmt.Errorf("failed to accept connection: %v", err)
    }
    // Read data in chunks and write to file
    buf := make([]byte, 1024)
    totalBytes := 0

    for {
        // Read from connection
        n, err := conn.VRead(buf)
        fmt.Println(buf[:n])
        if err != nil {
            if err.Error() == "read timeout" {
                // If we timeout, assume transfer is complete
                break
            }
            return totalBytes, fmt.Errorf("failed to read from connection: %v", err)
        }

        if n == 0 {
            // No more data to read
            break
        }
        // Write to file
        written, err := file.Write(buf[:n])
        if err != nil {
            return totalBytes, fmt.Errorf("failed to write to file: %v", err)
        }
        totalBytes += written
        // If we didn't fill the buffer, we might be done
        if n < len(buf) {
            break
        }
    }
    return totalBytes, nil
}

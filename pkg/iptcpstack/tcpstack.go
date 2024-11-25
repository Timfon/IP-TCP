package iptcpstack

import (
	"IP-TCP/pkg/lnxconfig"
  "net/netip"
  "IP-TCP/pkg/ipv4header"
  "IP-TCP/pkg/iptcp_utils"
  "fmt"
  "time"
  "github.com/google/netstack/tcpip/header"
  "math/rand"
  "container/heap"
)

type TCPStack struct {
  TcpRtoMin time.Duration
  TcpRtoMax time.Duration
  Sockets map[int]*Socket
  NextSocketID int
}

func InitializeTCP(config *lnxconfig.IPConfig) (*TCPStack, error){
  TcpStack := &TCPStack{
    TcpRtoMin: config.TcpRtoMin,
    TcpRtoMax: config.TcpRtoMax,
    Sockets: make(map[int]*Socket),
    NextSocketID: 0,
  }
  return TcpStack, nil
}

func (stack *TCPStack) FindSocket(localAddr netip.Addr, localPort uint16, remoteAddr netip.Addr, remotePort uint16) *Socket {
    // First check for exact connection match
    for _, sock := range stack.Sockets {
        if sock.Conn != nil && 
           sock.Conn.LocalPort == localPort &&
           sock.Conn.RemotePort == remotePort {
            return sock
        }
    }

    // If no connection found, check listening sockets
    for _, sock := range stack.Sockets {
        if sock.Listen != nil && sock.Listen.LocalPort == localPort {
            return sock
        }
    }

    return nil
}

func TCPPacketHandler(packet *Packet, args []interface{}) {
    stack := args[0].(*IPStack)
    tcpStack := args[1].(*TCPStack)
    hdr := packet.Header
    tcpHdr := iptcp_utils.ParseTCPHeader(packet.Body)
    sock := tcpStack.FindSocket(hdr.Dst, tcpHdr.DstPort, hdr.Src, tcpHdr.SrcPort)
    if sock == nil {
        fmt.Println("No matching socket found, dropping packet")
        fmt.Print("> ")
        return
    }
    // Handle listening socket case
    if sock.Listen != nil {
        if tcpHdr.Flags & header.TCPFlagSyn != 0 {
            handleSynReceived(sock, packet, tcpHdr, stack, tcpStack)
        }
        return 
    }

    // Only proceed if we have a connection
    if sock.Conn == nil {
        fmt.Println("Socket has no connection, dropping packet")
        return
    }

    // Now safe to check connection state
    switch sock.Conn.State {
    case 1:
        if tcpHdr.Flags & header.TCPFlagSyn != 0 && tcpHdr.Flags & header.TCPFlagAck != 0 {
            handleSynAckReceived(sock, packet, tcpHdr, stack)
        }
    case 2:
        if tcpHdr.Flags & header.TCPFlagAck != 0 {
            handleAckReceived(sock, packet, tcpHdr, stack, tcpStack)
        }
    case 3:
        if tcpHdr.Flags & header.TCPFlagAck != 0 {
            handleEstablished(sock, packet, tcpHdr, stack)
        }
        if tcpHdr.Flags & header.TCPFlagFin != 0 {
            finWaitTearDown(sock, packet, tcpHdr, stack)
        }
    case 4:
        if tcpHdr.Flags & header.TCPFlagAck != 0 {
            handleFinWait1(sock, packet, tcpHdr, stack)
        }
    case 5:
        if tcpHdr.Flags & header.TCPFlagAck != 0 {
            handleFinWait2(sock, packet, tcpHdr, stack)
        }
        if tcpHdr.Flags & header.TCPFlagFin != 0 {
            finWaitTearDown(sock, packet, tcpHdr, stack)
        }
    case 6:
        if tcpHdr.Flags & header.TCPFlagAck != 0 {
            handleCloseWait(sock, packet, tcpHdr, stack)
        }
    case 7:
        if tcpHdr.Flags & header.TCPFlagFin != 0 {
            handleTimeWait(sock, packet, tcpHdr, stack)
        }
    case 8:
        if tcpHdr.Flags & header.TCPFlagAck != 0 {
            recvTearDown(sock, packet, tcpHdr, stack, tcpStack)
        }
    }
}

func handleSynReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack, tcpstack *TCPStack) error {
    //add new socket to socket table
    l := sock.Listen
    seqNum := rand.Uint32() % 100 * 1000
    new_Connection := &VTCPConn{
        State: 2,
        LocalAddr: packet.Header.Dst,
        LocalPort: l.LocalPort,
        RemoteAddr: packet.Header.Src,
        RemotePort: tcpHdr.SrcPort,
        SeqNum: seqNum,
        AckNum: tcpHdr.SeqNum + 1,
        Window: NewWindow(65535),
        SID: tcpstack.NextSocketID,
    }
    // Initialize window tracking - for first data packet, we expect the original sequence number
    new_Connection.Window.RecvNext = tcpHdr.SeqNum
    new_Connection.Window.RecvLBR = tcpHdr.SeqNum

    newSocket := Socket{
        SID: tcpstack.NextSocketID,
        Conn: new_Connection,
    }
    tcpstack.Sockets[newSocket.SID] = &newSocket
    tcpstack.NextSocketID++
    // Send the packet
    fmt.Println("SYN received, sending SYN-ACK")
    err := stack.sendTCPPacket(&newSocket, []byte{}, header.TCPFlagAck | header.TCPFlagSyn)
    if err != nil {
        return fmt.Errorf("failed to send SYN-ACK packet: %v", err)
    }

    return nil
}

// Also modify handleSynAckReceived to initialize RecvNext
func handleSynAckReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) error {
    sock.Conn.State = 3
    sock.Conn.SeqNum = tcpHdr.AckNum
    sock.Conn.AckNum = tcpHdr.SeqNum + 1
    sock.Conn.Window.RecvNext = sock.Conn.AckNum // Add this line
    sock.Conn.Window.RecvLBR = tcpHdr.SeqNum + 1
    // fmt.Printf("Debug - Initialized RecvNext to %d\n", sock.Conn.Window.RecvNext)
    fmt.Println("SYN-ACK received, connection established")
    // Send ACK
    err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
    if err != nil {
        return fmt.Errorf("failed to send ACK packet: %v", err)
    }
    return nil
}

func handleAckReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack, tcpstack *TCPStack){
  fmt.Println("ACK received, connection established")
  sock.Conn.State = 3
  sock.Conn.Window.RecvNext = tcpHdr.SeqNum
  sock.Conn.Window.RecvLBR = tcpHdr.SeqNum
// Find the listening socket that created this connection
  for _, s := range tcpstack.Sockets {
      if s.Listen != nil && s.Listen.LocalPort == sock.Conn.LocalPort {
          // Place the established connection in the accept queue
          s.Listen.AcceptQueue <- sock.Conn
          break
      }
  }

    sock.Conn.Window.RetransmissionQueue = NewRetransmissionQueue()
    go sock.Conn.HandleRetransmission(stack, sock)

}

// Simplify handleEstablished to just handle in-order data
func handleEstablished(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) {
    payloadOffset := int(tcpHdr.DataOffset)
    payload := packet.Body[payloadOffset:]

    // Handle ACKs for sent data
    if tcpHdr.Flags&header.TCPFlagAck != 0 {
        if tcpHdr.AckNum > sock.Conn.Window.SendUna {
            newlyAcked := tcpHdr.AckNum - sock.Conn.Window.SendUna
            sock.Conn.Window.RemoveAckedData(newlyAcked)
            sock.Conn.Window.SendUna = tcpHdr.AckNum

            // Update RTT measurements only for non-retransmitted packets
            if sock.Conn.Window.RetransmissionQueue != nil {
                for _, entry := range sock.Conn.Window.RetransmissionQueue.Entries {
                    if entry.SeqNum + uint32(len(entry.Data)) == tcpHdr.AckNum {
                        // Only update RTT if this wasn't a retransmission
                        if time.Since(entry.SendTime) < entry.RTO {
                            measuredRTT := time.Since(entry.SendTime)
                            sock.Conn.Window.RetransmissionQueue.updateRTT(measuredRTT)
                        }
                        break
                    }
                }
                sock.Conn.Window.RetransmissionQueue.RemoveAckedEntries(tcpHdr.AckNum)
            }
        }
    }

    sock.Conn.SeqNum = tcpHdr.AckNum

    if len(payload) > 0 {
        // Check if this is a duplicate packet we've already processed
        if tcpHdr.SeqNum < sock.Conn.Window.RecvNext {
            // Send ACK for duplicate packet to help sender's flow control
            err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
            if err != nil {
                fmt.Printf("Error sending duplicate ACK: %v\n", err)
            }
            return
        }

        if tcpHdr.SeqNum == sock.Conn.Window.RecvNext {
            // Process in-order packet
            if err := processInOrderPacket(sock, payload, stack); err != nil {
                fmt.Printf("Error processing in-order packet: %v\n", err)
                return
            }

            // Try to process any buffered out-of-order packets
            processBufferedPackets(sock, stack)
        } else if tcpHdr.SeqNum > sock.Conn.Window.RecvNext {
            // Handle out-of-order packet
            if sock.Conn.ReceiveQueue == nil {
                sock.Conn.ReceiveQueue = &PriorityQueue{}
                heap.Init(sock.Conn.ReceiveQueue)
            }

            // Check if we already have this packet buffered
            isDuplicate := false
            for _, item := range *sock.Conn.ReceiveQueue {
                if item.priority == tcpHdr.SeqNum {
                    isDuplicate = true
                    break
                }
            }

            if !isDuplicate {
                fmt.Printf("Buffering out-of-order packet: seq=%d, expecting=%d\n", 
                    tcpHdr.SeqNum, sock.Conn.Window.RecvNext)
                entry := &Item{
                    value:    payload,
                    priority: tcpHdr.SeqNum,
                }
                heap.Push(sock.Conn.ReceiveQueue, entry)
            }

            // Send duplicate ACK to trigger fast retransmit
            err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
            if err != nil {
                fmt.Printf("Error sending duplicate ACK: %v\n", err)
            }
        }
    }
}

// Helper function to process in-order packets
func processInOrderPacket(sock *Socket, payload []byte, stack *IPStack) error {
    _, err := sock.Conn.Window.recvBuffer.Write(payload)
    if err != nil {
        return fmt.Errorf("error writing to receive buffer: %v", err)
    }

    sock.Conn.Window.RecvNext += uint32(len(payload))
    sock.Conn.AckNum = sock.Conn.Window.RecvNext

    // Signal data availability
    select {
    case sock.Conn.Window.DataAvailable <- struct{}{}:
        fmt.Printf("Processed in-order packet: seq=%d\n", sock.Conn.Window.RecvNext-uint32(len(payload)))
    default:
        fmt.Println("Channel already has signal, skipped")
    }

    // Send ACK
    return stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
}

// Helper function to process buffered out-of-order packets
func processBufferedPackets(sock *Socket, stack *IPStack) {
    if sock.Conn.ReceiveQueue == nil || sock.Conn.ReceiveQueue.Len() == 0 {
        return
    }

    for sock.Conn.ReceiveQueue.Len() > 0 {
        item := (*sock.Conn.ReceiveQueue)[0] // Peek at next packet
        if item.priority != sock.Conn.Window.RecvNext {
            break
        }

        // Remove and process the packet
        early := heap.Pop(sock.Conn.ReceiveQueue).(*Item)
        if err := processInOrderPacket(sock, early.value, stack); err != nil {
            fmt.Printf("Error processing buffered packet: %v\n", err)
            return
        }
    }
}


func handleFinWait1(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack){
    sock.Conn.State = 5
    sock.Conn.SeqNum = tcpHdr.AckNum
    sock.Conn.AckNum = tcpHdr.SeqNum + 1
    sock.Conn.Window.RecvNext = sock.Conn.AckNum // Add this line
    sock.Conn.Window.RecvLBR = tcpHdr.SeqNum + 1
}

func handleFinWait2(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack){
    //basically just accepts ack messages here

}

func finWaitTearDown(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) error{
    //takes a fin and sends an ACK
    sock.Conn.State = 7
    sock.Conn.SeqNum = tcpHdr.AckNum
    sock.Conn.AckNum = tcpHdr.SeqNum + 1
    sock.Conn.Window.RecvNext = sock.Conn.AckNum // Add this line
    sock.Conn.Window.RecvLBR = tcpHdr.SeqNum + 1
    
    // fmt.Printf("Debug - Initialized RecvNext to %d\n", sock.Conn.Window.RecvNext)
    fmt.Println("FIN received, entering Time Wait")
    
    // Send ACK
    err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
    if err != nil {
        return fmt.Errorf("failed to send ACK packet: %v", err)
    }
    return nil
}

func handleTimeWait(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) error{
    sock.Conn.SeqNum = tcpHdr.AckNum
    sock.Conn.AckNum = tcpHdr.SeqNum + 1
    sock.Conn.Window.RecvNext = sock.Conn.AckNum // Add this line
    sock.Conn.Window.RecvLBR = tcpHdr.SeqNum + 1
    
    // fmt.Printf("Debug - Initialized RecvNext to %d\n", sock.Conn.Window.RecvNext)
    fmt.Println("FIN received, Transmitting ACK For Confirmation and Closing")
    
    // Send ACK
    err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
    if err != nil {
        return fmt.Errorf("failed to send ACK packet: %v", err)
    }
    return nil
}

func handleCloseWait(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack){
    //literally just kinda stays there
    sock.Conn.SeqNum = tcpHdr.AckNum
    sock.Conn.Window.RecvNext = sock.Conn.AckNum // Add this line
    fmt.Println("ACK Received")

}

func recvTearDown(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack, tcpStack *TCPStack){
    //
    sock.Conn.SeqNum = tcpHdr.AckNum
    sock.Conn.Window.RecvNext = sock.Conn.AckNum // Add this line
    fmt.Println("Final ACK Received, deleting TCB")
    
    //prob
    delete(tcpStack.Sockets, sock.SID)

}

func (stack *IPStack) sendTCPPacket(sock *Socket, data []byte, flags uint8) error {
    var tcpHdr header.TCPFields
    var localAddr, remoteAddr netip.Addr
    if sock.Conn == nil {
        return fmt.Errorf("can't send via listen socket")
    }
    //fmt.Printf("Data %+v\n", data)
        // Normal connected socket - use Conn field
    tcpHdr = header.TCPFields{
        SrcPort:    sock.Conn.LocalPort,
        DstPort:    sock.Conn.RemotePort,
        SeqNum:     sock.Conn.SeqNum,
        AckNum:     sock.Conn.AckNum,
        DataOffset: 20,
        Flags:      flags,
        WindowSize: 65535,  // Default window size
    }
    localAddr = sock.Conn.LocalAddr
    remoteAddr = sock.Conn.RemoteAddr
    

    // Create TCP header bytes and compute checksum
    checksum := iptcp_utils.ComputeTCPChecksum(&tcpHdr, localAddr, remoteAddr, nil)
    tcpHdr.Checksum = checksum
    tcpHeaderBytes := make(header.TCP, iptcp_utils.TcpHeaderLen)
    tcp := header.TCP(tcpHeaderBytes)
    tcp.Encode(&tcpHdr)

    ipBytes := append(tcp, data...)

    hdr := ipv4header.IPv4Header{
        Version:  4,
        Len:     20,
        TOS:     0,
        TotalLen: ipv4header.HeaderLen + len(ipBytes),
        ID:      0,
        Flags:   0,
        FragOff: 0,
        TTL:     32,
        Protocol: 6,
        Checksum: 0,
        Src:     localAddr,
        Dst:     remoteAddr,
        Options: []byte{},
    }

    return SendIP(stack, &hdr, ipBytes)
}


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
}

// Simplify handleEstablished to just handle in-order data
func handleEstablished(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) {
    payloadOffset := int(tcpHdr.DataOffset)
    payload := packet.Body[payloadOffset:]
    // Handle ACKs for sent data
    if tcpHdr.Flags&header.TCPFlagAck != 0 {
        // Always update SendUna if we get a higher ACK number
        if tcpHdr.AckNum > sock.Conn.Window.SendUna {
            // Calculate how many new bytes were acknowledged
            newlyAcked := tcpHdr.AckNum - sock.Conn.Window.SendUna
            
            // Remove acknowledged data from send buffer
            sock.Conn.Window.RemoveAckedData(newlyAcked)
            
            // Update SendUna
            sock.Conn.Window.SendUna = tcpHdr.AckNum
            
            // If we have a retransmission queue, update RTT and clean up entries
            if sock.Conn.Window.RetransmissionQueue != nil {
                // Find the packet being acknowledged
                for _, entry := range sock.Conn.Window.RetransmissionQueue.Entries {
                    if entry.SeqNum + uint32(len(entry.Data)) == tcpHdr.AckNum {
                        measuredRTT := time.Since(entry.SendTime)
                        sock.Conn.Window.RetransmissionQueue.updateRTT(measuredRTT)
                        break
                    }
                }
                // Remove acknowledged packets
                sock.Conn.Window.RetransmissionQueue.RemoveAckedEntries(tcpHdr.AckNum)
            }
        }
    }

    if len(payload) > 0 {
        // fmt.Printf("Payload contents: %s\n", string(payload))
    }
    sock.Conn.SeqNum = tcpHdr.AckNum
    
    if len(payload) > 0 {
        if tcpHdr.SeqNum == sock.Conn.Window.RecvNext {
            // Write to receive buffer
            _, err := sock.Conn.Window.recvBuffer.Write(payload)
            if err != nil {
                fmt.Printf("Error writing to receive buffer: %v\n", err)
                return
            }
            // Update sequence tracking
            sock.Conn.Window.RecvNext += uint32(len(payload))
            sock.Conn.AckNum = sock.Conn.Window.RecvNext
            
            // Signal data availability
            select {
            case sock.Conn.Window.DataAvailable <- struct{}{}: 
                fmt.Println("Successfully signaled data availability")
            default: 
                fmt.Println("Channel already has signal, skipped")
            }

            // Check if ReceiveQueue exists and has items before processing
            if sock.Conn.ReceiveQueue != nil && sock.Conn.ReceiveQueue.Len() > 0 {
                for sock.Conn.ReceiveQueue.Len() > 0 {
                    early := heap.Pop(sock.Conn.ReceiveQueue).(*Item)
                    if early.priority == sock.Conn.Window.RecvNext {
                        n, err := sock.Conn.Window.recvBuffer.Write(early.value)
                        if err != nil {
                            fmt.Printf("Error writing to receive buffer: %v\n", err)
                            return
                        }
                        fmt.Printf("Debug - Wrote %d bytes from queue to receive buffer\n", n)
                        
                        sock.Conn.Window.RecvNext += uint32(len(early.value))
                        sock.Conn.AckNum = sock.Conn.Window.RecvNext
                    } else if early.priority > sock.Conn.Window.RecvNext {
                        heap.Push(sock.Conn.ReceiveQueue, early)
                        break
                    }
                }
            }

            // Send ACK
            err = stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
            if err != nil {
                fmt.Printf("Error sending ACK: %v\n", err)
            } else {
                fmt.Println("Successfully sent ACK")
            }

        } else {
            if sock.Conn.ReceiveQueue == nil {
                sock.Conn.ReceiveQueue = &PriorityQueue{}
                heap.Init(sock.Conn.ReceiveQueue)
            }
            fmt.Printf("Early Arrival packet - adding to queue")
            entry:= &Item{
                value: payload,
                priority: tcpHdr.SeqNum,  //sock.Conn.Window.RecvNext, // maybe use the actual sequence number, not recv next?
            }
            heap.Push(sock.Conn.ReceiveQueue, entry)
    }
} else {
    fmt.Println("Received return ACK")
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

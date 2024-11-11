package iptcpstack

import (
	"IP-TCP/pkg/lnxconfig"
  "net/netip"
  "IP-TCP/pkg/ipv4header"
  "IP-TCP/pkg/iptcp_utils"
  "fmt"
  "time"
  "github.com/google/netstack/tcpip/header"
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
            handleAckReceived(sock, packet, tcpHdr, stack)
        }
    case 3:
        handleEstablished(sock, packet, tcpHdr, stack)
    }
}

func handleSynReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack, tcpstack *TCPStack) error {
    //store previous packet data
    sock.Listen.SrcAddr = packet.Header.Src
    sock.Listen.DstAddr = packet.Header.Dst
    sock.Listen.SrcPort = tcpHdr.SrcPort
    sock.Listen.SeqNum = tcpHdr.SeqNum

    //add new socket to socket table
    l := sock.Listen

    new_Connection := &VTCPConn{
        State: 2,
        LocalAddr: packet.Header.Dst,
        LocalPort: l.LocalPort,
        RemoteAddr: packet.Header.Src,
        RemotePort: tcpHdr.SrcPort,
        SeqNum: uint32(time.Now().UnixNano()),
        AckNum: tcpHdr.SeqNum + 1,
        Window: NewWindow(65535),
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

    fmt.Printf("Debug - New connection initialized with RecvNext=%d, RecvLBR=%d, AckNum=%d\n",
              new_Connection.Window.RecvNext,
              new_Connection.Window.RecvLBR,
              new_Connection.AckNum)

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
    sock.Conn.AckNum = tcpHdr.SeqNum + 1
    sock.Conn.Window.RecvNext = tcpHdr.SeqNum + 1  // Add this line
    
    fmt.Printf("Debug - Initialized RecvNext to %d\n", sock.Conn.Window.RecvNext)
    fmt.Println("SYN-ACK received, connection established")
    
    // Send ACK
    err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
    if err != nil {
        return fmt.Errorf("failed to send SYN-ACK packet: %v", err)
    }
    return nil
}

func handleAckReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack){
  fmt.Println("ACK received, connection established")
  sock.Conn.State = 3
  //sock.Conn.AckNum = tcpHdr.SeqNum + 1
}

// Simplify handleEstablished to just handle in-order data
func handleEstablished(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) {
    fmt.Println("=== handleEstablished called ===")
    fmt.Printf("TCP Header: %+v\n", tcpHdr)
    
    payloadOffset := int(tcpHdr.DataOffset)
    payload := packet.Body[payloadOffset:]
    
    fmt.Printf("Payload offset: %d\n", payloadOffset)
    fmt.Printf("Payload length: %d\n", len(payload))
    if len(payload) > 0 {
        fmt.Printf("Payload contents: %s\n", string(payload))
    }
    
    if len(payload) > 0 {
        fmt.Printf("Debug - Current state before processing: RecvNext=%d, RecvLBR=%d, AckNum=%d\n",
                  sock.Conn.Window.RecvNext,
                  sock.Conn.Window.RecvLBR,
                  sock.Conn.AckNum)
                  
        if tcpHdr.SeqNum == sock.Conn.Window.RecvNext {
            fmt.Println("Debug - Processing packet")
            
            // Write to receive buffer
            n, err := sock.Conn.Window.recvBuffer.Write(payload)
            if err != nil {
                fmt.Printf("Error writing to receive buffer: %v\n", err)
                return
            }
            fmt.Printf("Debug - Wrote %d bytes to receive buffer\n", n)
            
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
            
            // Send ACK
            err = stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
            if err != nil {
                fmt.Printf("Error sending ACK: %v\n", err)
            } else {
                fmt.Println("Successfully sent ACK")
            }
            
            fmt.Printf("Debug - Updated state: RecvNext=%d, RecvLBR=%d, AckNum=%d\n",
                      sock.Conn.Window.RecvNext,
                      sock.Conn.Window.RecvLBR,
                      sock.Conn.AckNum)
        } else {
            fmt.Printf("Sequence number mismatch - Expected: %d, Got: %d\n",
                      sock.Conn.Window.RecvNext, tcpHdr.SeqNum)
        }
    }
    fmt.Println("=== handleEstablished finished ===\n")
}

func (stack *IPStack) sendTCPPacket(sock *Socket, data []byte, flags uint8) error {
    var tcpHdr header.TCPFields
    var localAddr, remoteAddr netip.Addr

    if sock.Listen != nil {
        // This is a listening socket - use Listen field
        tcpHdr = header.TCPFields{
            SrcPort:    sock.Listen.LocalPort,
            DstPort:    sock.Listen.SrcPort,  // Use info from last received packet
            SeqNum:     uint32(time.Now().UnixNano()),  // Generate new seq for SYN-ACK
            AckNum:     sock.Listen.SeqNum + 1,
            DataOffset: 20,
            Flags:      flags,
            WindowSize: 65535,  // Default window size
        }
        localAddr = sock.Listen.DstAddr
        remoteAddr = sock.Listen.SrcAddr
    } else {
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
    }

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
        TotalLen: ipv4header.HeaderLen + len(tcp),
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




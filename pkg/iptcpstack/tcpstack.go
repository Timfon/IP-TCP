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
    NextSocketID: 1,
  }
  return TcpStack, nil
}

func (stack *TCPStack) FindSocket(localAddr netip.Addr, localPort uint16, remoteAddr netip.Addr, remotePort uint16) *Socket{
  for _, sock := range stack.Sockets {
    conn := sock.Conn
    if conn.LocalAddr == localAddr &&
    conn.LocalPort == localPort &&
    conn.RemoteAddr == remoteAddr &&
    conn.RemotePort == remotePort {
      return sock
    }
  }

  //trye listening ports now
  for _, sock := range stack.Sockets {
    l := sock.Listen
    if l.LocalPort == localPort {
      return sock
    }
  }
  return nil
}

func TCPPacketHandler(packet *Packet, args []interface{}){
  //guaranteed that packet is a tcp packet
  //need to first parse IP
  //then parse TCP
  stack := args[0].(*IPStack)
  tcpStack := args[1].(*TCPStack)
  hdr := packet.Header


  tcpHdr := iptcp_utils.ParseTCPHeader(packet.Body)

  fmt.Println(tcpHdr)

  sock := tcpStack.FindSocket(hdr.Dst, tcpHdr.DstPort, hdr.Src, tcpHdr.SrcPort)
  if sock == nil {
    fmt.Println("No matching socket found, dropping packet")
    fmt.Print("> ")
    return
  }

  if sock.Listen != nil {
    if tcpHdr.Flags & header.TCPFlagSyn != 0 {
      handleSynReceived(sock, packet, tcpHdr, stack, tcpStack)
    }   
  }

  //handle the packet
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
      WindowSize: 65535,
      SendWindow: make([]byte, 0),
    }
    newSocket := Socket{
      SID: tcpstack.NextSocketID,
      Conn: new_Connection,
    }
    tcpstack.Sockets[newSocket.SID] = &newSocket
    tcpstack.NextSocketID++

    // Send the packet
    fmt.Println("SYN received, sending SYN-ACK")
    err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck | header.TCPFlagSyn)
    if err != nil {
        return fmt.Errorf("failed to send SYN-ACK packet: %v", err)
    }
    return nil
}

func handleSynAckReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) error{
  sock.Conn.State = 3
  sock.Conn.AckNum = tcpHdr.SeqNum + 1
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
  sock.Conn.AckNum = tcpHdr.SeqNum + 1
}

func handleEstablished(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack){
  // Send ACK
  _ = header.TCPFields{
    SrcPort:    sock.Conn.LocalPort,
    DstPort:    sock.Conn.RemotePort,
    SeqNum:     sock.Conn.SeqNum,
    AckNum:     sock.Conn.AckNum,
    Flags:      header.TCPFlagAck,
    WindowSize: sock.Conn.WindowSize,
  }
  // Create and send ACK packet
  //maybe:
  // ackChecksum := iptcp_utils.ComputeTCPChecksum(&ackHdr, sock.Conn.LocalAddr, sock.Conn.RemoteAddr, nil)
  // ackHdr.Checksum = ackChecksum
  // ackHeaderBytes := make(header.TCP, iptcp_utils.TcpHeaderLen)
  // ack := header.TCP(ackHeaderBytes)
  // ack.Encode(&ackHdr)
  // err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
  // if err != nil {
  //   fmt.Println("Error sending ACK packet")
  //   return
  // }
  stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
}

func (stack *IPStack) sendTCPPacket(sock *Socket, data []byte, flags uint8) error{
  // Create IP packet
  tcpHdr := header.TCPFields{
    SrcPort:    sock.Conn.LocalPort,
    DstPort:    sock.Conn.RemotePort,
    SeqNum:     sock.Conn.SeqNum,
    AckNum:     sock.Conn.AckNum,
    DataOffset: 20,  // TCP header size in bytes
    Flags:      flags,
    WindowSize: sock.Conn.WindowSize,
}

// Create TCP header bytes and compute checksum
  checksum := iptcp_utils.ComputeTCPChecksum(&tcpHdr, sock.Conn.LocalAddr, sock.Conn.RemoteAddr, nil)
  tcpHdr.Checksum = checksum

  tcpHeaderBytes := make(header.TCP, iptcp_utils.TcpHeaderLen)
  tcp := header.TCP(tcpHeaderBytes)
  tcp.Encode(&tcpHdr)

  ipBytes := append(tcp, data...)
  //src and dest should switch if flags are in handshake?
  hdr := ipv4header.IPv4Header{
            Version:  4,
            Len: 	20, // Header length is always 20 when no IP options
            TOS:      0,
            TotalLen: ipv4header.HeaderLen + len(tcp),//??data 
            ID:       0,
            Flags:    0,
            FragOff:  0,
            TTL:      32, // idk man
            Protocol: 6,
            Checksum: 0, // Should be 0 until checksum is computed
            Src:      sock.Conn.LocalAddr, // double check
            Dst:      sock.Conn.RemoteAddr, // double check
            Options:  []byte{},
        }
  // Send IP packet
  //fmt.Println(data)
  err := SendIP(stack, &hdr, ipBytes)
  if err != nil {
    fmt.Println("Error sending IP packet")
    return err
  }
  return nil
}




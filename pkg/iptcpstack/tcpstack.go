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
    if sock.LocalAddr == localAddr &&
    sock.LocalPort == localPort &&
    sock.RemoteAddr == remoteAddr &&
    sock.RemotePort == remotePort {
      return sock
    }
  }

  //trye listening ports now
  for _, sock := range stack.Sockets {
    if sock.State == 0 &&
    sock.LocalPort == localPort {
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

  //handle the packet
  switch sock.State {
  case 0:
    //2 2
    if tcpHdr.Flags & header.TCPFlagSyn != 0 {
      handleSynReceived(sock, packet, tcpHdr, stack, tcpStack)
    }
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
    tcpstack.Sockets[sock.SID + 1] = &Socket{
      SID: sock.SID + 1,
      State: 2,
      LocalAddr: packet.Header.Dst,
      LocalPort: sock.LocalPort,
      RemoteAddr: packet.Header.Src,
      RemotePort: tcpHdr.SrcPort,
      SeqNum: uint32(time.Now().UnixNano()),
      AckNum: tcpHdr.SeqNum + 1,
      WindowSize: 65535,
      SendWindow: make([]byte, 0),
    }
    fmt.Println(sock)
    // Create SYN-ACK header

    // Create IP header
    ipHdr := ipv4header.IPv4Header{
        Version:  4,
        Len:      20,
        TOS:      0,
        TotalLen: ipv4header.HeaderLen + len(tcpHeaderBytes),
        ID:       0,
        Flags:    0,
        FragOff:  0,
        TTL:      packet.Header.TTL,
        Protocol: 6,  // TCP protocol number
        Checksum: 0,
        Src:      packet.Header.Dst,
        Dst:      packet.Header.Src,
        Options:  []byte{},
    }
    // Send the packet
    err := SendIP(stack, &ipHdr, tcpHeaderBytes)
    if err != nil {
        return fmt.Errorf("failed to send SYN-ACK packet: %v", err)
    }
    return nil
}

func handleSynAckReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) error{
  sock.State = 3
  sock.AckNum = tcpHdr.SeqNum + 1
  fmt.Println("SYN-ACK received, connection established")

  // Send ACK
  ackHdr := header.TCPFields{
    SrcPort:    sock.LocalPort,
    DstPort:    sock.RemotePort,
    SeqNum:     sock.SeqNum,
    AckNum:     sock.AckNum,
    Flags:      header.TCPFlagAck,
    WindowSize: sock.WindowSize,
  }
  tcpHeaderBytes := make(header.TCP, iptcp_utils.TcpHeaderLen)
  tcp := header.TCP(tcpHeaderBytes)
  tcp.Encode(&ackHdr)

  // Create IP header
  ipHdr := ipv4header.IPv4Header{
      Version:  4,
      Len:      20,
      TOS:      0,
      TotalLen: ipv4header.HeaderLen + len(tcpHeaderBytes),
      ID:       0,
      Flags:    0,
      FragOff:  0,
      TTL:      packet.Header.TTL,
      Protocol: 6,  // TCP protocol number
      Checksum: 0,
      Src:      packet.Header.Dst,
      Dst:      packet.Header.Src,
      Options:  []byte{},
  }
  err := SendIP(stack, &ipHdr, tcpHeaderBytes)
  if err != nil {
      return fmt.Errorf("failed to send SYN-ACK packet: %v", err)
  }
  return nil
}

func handleAckReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack){
  fmt.Println("ACK received, connection established")
  sock.State = 3
  sock.AckNum = tcpHdr.SeqNum + 1
}

func handleEstablished(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack){
  // Send ACK
  ackHdr := header.TCPFields{
    SrcPort:    sock.LocalPort,
    DstPort:    sock.RemotePort,
    SeqNum:     sock.SeqNum,
    AckNum:     sock.AckNum,
    Flags:      header.TCPFlagAck,
    WindowSize: sock.WindowSize,
  }
  // Create and send ACK packet
  sendTCPPacket(sock, ackHdr, stack, packet)
}

func (stack *IPStack) sendTCPPacket(sock *Socket, packet *Packet, flags uint8){
  // Create IP packet
  
  data := packet.Body //??data
  tcpHdr := header.TCPFields{
    SrcPort:    sock.LocalPort,
    DstPort:    tcpHdr.remotePort,
    SeqNum:     sock.SeqNum,
    AckNum:     sock.AckNum,
    DataOffset: 20,  // TCP header size in bytes
    Flags:      flags,
    WindowSize: sock.WindowSize,
}

// Create TCP header bytes and compute checksum
  checksum := iptcp_utils.ComputeTCPChecksum(&synAckHdr, sock.LocalAddr, sock.RemoteAddr, nil)
  tcpHdr.Checksum = checksum

  tcpHeaderBytes := make(header.TCP, iptcp_utils.TcpHeaderLen)
  tcp := header.TCP(tcpHeaderBytes)
  tcp.Encode(&tcpHdr)
  hdr := ipv4header.IPv4Header{
            Version:  4,
            Len: 	20, // Header length is always 20 when no IP options
            TOS:      0,
            TotalLen: ipv4header.HeaderLen + len(tcp),//??data 
            ID:       0,
            Flags:    0,
            FragOff:  0,
            TTL:      32, // idk man
            Protocol: 0,
            Checksum: 0, // Should be 0 until checksum is computed
            Src:      packet.Header.Dst, // double check
            Dst:      packet.Header.Dst, // double check
            Options:  []byte{},
        }

  
  // Send IP packet
  //fmt.Println(data)
  SendIP(stack, &hdr, tcp)
}




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


type Socket struct {
  SID int
  State string
  LocalAddr netip.Addr
  LocalPort uint16  
  RemoteAddr netip.Addr
  RemotePort uint16
  SeqNum  uint32
  AckNum  uint32
  WindowSize uint16

  SendWindow []byte
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
    if sock.State == "ESTABLISHED" &&
    sock.LocalAddr == localAddr &&
    sock.LocalPort == localPort &&
    sock.RemoteAddr == remoteAddr &&
    sock.RemotePort == remotePort {
      return sock
    }
  }

  //trye listening ports now
  for _, sock := range stack.Sockets {
    if sock.State == "LISTEN" &&
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

  buffer := make([]byte, MAX_MESSAGE_SIZE) // max message size
  tcpHeaderAndData := buffer[hdr.Len:hdr.TotalLen]
  tcpHdr := iptcp_utils.ParseTCPHeader(tcpHeaderAndData)

  tcpPayload := tcpHeaderAndData[tcpHdr.DataOffset:]
  tcpChecksumFromHeader := tcpHdr.Checksum
  tcpHdr.Checksum = 0
  tcpComputedChecksum := iptcp_utils.ComputeTCPChecksum(&tcpHdr, hdr.Src, hdr.Dst, tcpPayload)
  if tcpComputedChecksum != tcpChecksumFromHeader {
    fmt.Println("Checksums do not match, dropping packet")
    fmt.Print("> ")
    return
  }

  //find the socket
  sock := tcpStack.FindSocket(hdr.Dst, tcpHdr.DstPort, hdr.Src, tcpHdr.SrcPort)
  if sock == nil {
    fmt.Println("No matching socket found, dropping packet")
    fmt.Print("> ")
    return
  }

  //handle the packet
  switch sock.State {
  case "LISTEN":
    //check if it is a SYN packet
    if tcpHdr.Flags & header.TCPFlagSyn != 0 {
      handleSynReceived(sock, packet, tcpHdr, stack)
    }
  case "SYN_SENT":
    if tcpHdr.Flags & header.TCPFlagSyn != 0 && tcpHdr.Flags & header.TCPFlagAck != 0 {
      handleSynAckReceived(sock, packet, tcpHdr, stack)
    }
  case "SYN_RECEIVED":
    if tcpHdr.Flags & header.TCPFlagAck != 0 {
      handleAckReceived(sock, packet, tcpHdr, stack)
    }
  case "ESTABLISHED":
    handleEstablished(sock, packet, tcpHdr, stack)
  }
}
func handleSynReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack){
  //create a new socket
  sock.State = "SYN_RECEIVED"
	sock.RemoteAddr = packet.Header.Src
	sock.RemotePort = tcpHdr.SrcPort
	sock.AckNum = tcpHdr.SeqNum + 1

	sock.SeqNum = uint32(time.Now().UnixNano())

	// Send SYN-ACK
	synAckHdr := header.TCPFields{
		SrcPort:    sock.LocalPort,
		DstPort:    sock.RemotePort,
		SeqNum:     sock.SeqNum,
		AckNum:     sock.AckNum,
		Flags:      header.TCPFlagSyn | header.TCPFlagAck,
		WindowSize: sock.WindowSize,
	}
	// Create and send SYN-ACK packet
	sendTCPPacket(sock, synAckHdr, stack, packet)
}

func handleSynAckReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack){
  sock.State = "ESTABLISHED"
  sock.AckNum = tcpHdr.SeqNum + 1

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

func handleAckReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack){
  sock.State = "ESTABLISHED"
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

func sendTCPPacket(sock *Socket, tcpHdr header.TCPFields, stack *IPStack, packet *Packet){
  // Create IP packet
  
  data := packet.Body //??data
 hdr := ipv4header.IPv4Header{
            Version:  4,
            Len: 	20, // Header length is always 20 when no IP options
            TOS:      0,
            TotalLen: ipv4header.HeaderLen + len(data),//??data 
            ID:       0,
            Flags:    0,
            FragOff:  0,
            TTL:      32, // idk man
            Protocol: 0,
            Checksum: 0, // Should be 0 until checksum is computed
            Src:      packet.Header.Src, // double check
            Dst:      packet.Header.Dst, // double check
            Options:  []byte{},
        }
  // Send IP packet
  fmt.Println(data)
  SendIP(stack, &hdr, data)
}




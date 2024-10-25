package iptcpstack

import (
	"IP-TCP/pkg/lnxconfig"
  "net/netip"
	"IP-TCP/pkg/socket"
  "IP-TCP/pkg/iptcp_utils"
  "fmt"
  "time"
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

  }
}
//
// func (stack *TCPStack) VListen(port uint16) (*socket.VTCPListener, error){
//
//
// }
//


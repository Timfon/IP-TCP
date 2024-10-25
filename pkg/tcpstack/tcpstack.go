package tcpstack

import (
	"IP-TCP/pkg/lnxconfig"
  "net/netip"
	"IP-TCP/pkg/socket"
  "IP-TCP/pkg/iptcp_utils"
  "IP-TCP/pkg/ipstack"
  "fmt"
  "time"
  "net"
)
type TCPStack struct {
  TcpRtoMin time.Duration
  TcpRtoMax time.Duration
}

type Socket struct {
  SID int
  State string
  LocalAddr netip.Addr
  LocalPort uint16  
  RemoteAddr netip.Addr
  RemotePort uint16
}

func InitializeTCP(config *lnxconfig.IPConfig) (*TCPStack, error){
  TcpStack := &TCPStack{
    TcpRtoMin: config.TcpRtoMin,
    TcpRtoMax: config.TcpRtoMax,
  }
  return TcpStack, nil
}

func TCPPacketHandler(packet *ipstack.Packet, args []interface{}){
  //guaranteed that packet is a tcp packet
  //need to first parse IP
  //then parse TCP
  hdr := packet.Header
  buffer := make([]byte, ipstack.MAX_MESSAGE_SIZE)
  stack := args[0].(*ipstack.IPStack)
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
  
}

func (stack *TCPStack) VListen(port uint16) (*socket.VTCPListener, error){


}



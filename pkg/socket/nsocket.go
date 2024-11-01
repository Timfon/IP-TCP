package socket

import (
	"net/netip"
  "fmt"
  "IP-TCP/pkg/iptcpstack"
  "IP-TCP/pkg/ipv4header"
  "IP-TCP/pkg/iptcp_utils"
	"github.com/google/netstack/tcpip/header"
  "time"
)

type VTCPConn struct {
  socket *iptcpstack.Socket
  tcpStack *iptcpstack.TCPStack
  readBuffer []byte
  closed bool
}

func VConnect(addr netip.Addr, port uint16, tcpStack *iptcpstack.TCPStack, ipStack *iptcpstack.IPStack) (*VTCPConn, error) {
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
    localPort := uint16(20000 + tcpStack.NextSocketID)
    sock := &iptcpstack.Socket{
        SID:        tcpStack.NextSocketID,
        State:      1,
        LocalAddr:  localAddr,
        LocalPort:  localPort,
        RemoteAddr: addr,
        RemotePort: port,
        SeqNum:     uint32(time.Now().UnixNano()),
        AckNum:     0,
        WindowSize: 65535,
        SendWindow: make([]byte, 0),
    }
    tcpStack.NextSocketID++
    //tcpStack.Sockets[sock.SID] = sock //I don't think we should be adding synsent to the sockettable

    // Create and send SYN packet
    synHdr := header.TCPFields{
        SrcPort:    sock.LocalPort,
        DstPort:    sock.RemotePort,
        SeqNum:     sock.SeqNum,
        AckNum:     0,
        DataOffset: 20, // TCP header size in bytes
        Flags:      header.TCPFlagSyn,
        WindowSize: sock.WindowSize,
        //checksum
    }

    // Create TCP header bytes
    checksum := iptcp_utils.ComputeTCPChecksum(&synHdr, localAddr, addr, nil)
    synHdr.Checksum = checksum
    tcpHeaderBytes := make(header.TCP, iptcp_utils.TcpHeaderLen)
    //fmt.Println(tcpHeaderBytes)
    tcp := header.TCP(tcpHeaderBytes)
    tcp.Encode(&synHdr)

    ipPacketPayload := make([]byte, 0, len(tcpHeaderBytes))
    ipPacketPayload = append(ipPacketPayload, tcpHeaderBytes...)
    // Send the packet
    ipHdr := ipv4header.IPv4Header{
      Version:  4,
            Len: 	20, // Header length is always 20 when no IP options
            TOS:      0,
            TotalLen: ipv4header.HeaderLen + len(tcpHeaderBytes),//??data 
            ID:       0,
            Flags:    0,
            FragOff:  0,
            TTL:      32, // idk man
            Protocol: 6,
            Checksum: 0, 
            Src:      localAddr,
            Dst:      addr,
            Options:  []byte{},
    }
    fmt.Println(ipHdr)

    err := iptcpstack.SendIP(ipStack, &ipHdr, tcpHeaderBytes)
    if err != nil {
        return nil, fmt.Errorf("failed to send SYN packet: %v", err)
    }
    // Create connection object
    conn := &VTCPConn{
        socket:     sock,
        tcpStack:   tcpStack,
        readBuffer: make([]byte, 0),
        closed:     false,
    }

    return conn, nil
}


func (c *VTCPConn) VClose() error {
  if c.closed {
    return fmt.Errorf("Connection is already closed")
  }

  c.closed = true
  delete(c.tcpStack.Sockets, c.socket.SID)
  return nil
}



// func (c *VTCPConn) VRead(buf []byte) (int, error){
//
// }
// //
// // func (c *VTCPConn) VWrite(data []byte) (int, error){
// //
// //
// // }

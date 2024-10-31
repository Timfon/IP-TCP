package socket

import (
	"net/netip"
  "fmt"
  "IP-TCP/pkg/iptcpstack"
  "time"
)

type VTCPConn struct {
  socket *iptcpstack.Socket
  tcpStack *iptcpstack.TCPStack
  readBuffer []byte
  closed bool
}

func VConnect(addr netip.Addr, port uint16, tcpStack *iptcpstack.TCPStack, ipStack *iptcpstack.IPStack) (*VTCPConn, error){
  //jank way to get the local route
  route, found, _ := ipStack.ForwardingTable.MatchPrefix(addr)
  if found == -1 {
    return nil, fmt.Errorf("no route to host %v", addr)
  }
  //fmt.Println(route)

  // Get the local source IP based on the route
  var localAddr netip.Addr
  if route.RoutingMode == 4 {
    localAddr = route.VirtualIP
  } else {
    // If route is through interface, get the interface's IP
    iroute, _, _ := ipStack.ForwardingTable.MatchPrefix(route.VirtualIP)
    localAddr = iroute.VirtualIP
  }
  localPort := uint16(20000 + tcpStack.NextSocketID)

  sock := &iptcpstack.Socket{
    SID: tcpStack.NextSocketID,
    State: "SYN_SENT",
    LocalAddr: localAddr,
    LocalPort: localPort,
    RemoteAddr: addr,
    RemotePort: port,
    SeqNum: uint32(time.Now().UnixNano()),
    AckNum: 0,
    WindowSize: 65535,
    SendWindow:  make([]byte, 0),
  }

  tcpStack.NextSocketID++
  tcpStack.Sockets[sock.SID] = sock
  conn := &VTCPConn{
    socket: sock,
    tcpStack: tcpStack,
    readBuffer: make([]byte, 0),
    closed: false,
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

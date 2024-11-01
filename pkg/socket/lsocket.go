package socket

import (
  "fmt"
  "net/netip"
  "IP-TCP/pkg/iptcpstack"
)

type VTCPListener struct {
  Socket *iptcpstack.Socket
  TcpStack *iptcpstack.TCPStack
  AcceptQueue chan *VTCPConn
  Closed bool
}

func VListen(port uint16, tcpStack *iptcpstack.TCPStack) (*VTCPListener, error){
  sock := &iptcpstack.Socket{
    SID: 0, // or should be next socket id?
    State: "LISTEN",
    LocalAddr: netip.IPv4Unspecified(),
    LocalPort: port,
    RemoteAddr: netip.IPv4Unspecified(),
    RemotePort: 0,
    SeqNum: 0,
    AckNum: 0,
    WindowSize: 65535, // maybe change later??
    SendWindow: make([]byte, 0), // maybe empty window, growing. 
  }
  tcpStack.NextSocketID++
  tcpStack.Sockets[sock.SID] = sock

  listener := &VTCPListener{
    Socket: sock,
    TcpStack: tcpStack,
    AcceptQueue: make(chan *VTCPConn, 100), //buffer of 100 pending connections idk
    Closed: false,
  }
  return listener, nil
}


func (l *VTCPListener) VAccept() (*VTCPConn, error){
  if l.Closed {
    return nil, fmt.Errorf("Listener is closed")
  }

  conn, ok := <-l.AcceptQueue
  fmt.Println(conn)
  if !ok {
    return nil, fmt.Errorf("Listener is closed")
  }

  return conn, nil
}

func (l *VTCPListener) VClose() error {
  if l.Closed {
    return fmt.Errorf("Listener is already closed")
  }

  l.Closed = true
  close(l.AcceptQueue)
  delete(l.TcpStack.Sockets, l.Socket.SID)
  return nil
}

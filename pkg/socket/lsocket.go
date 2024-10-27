package socket

import (
  "fmt"
  "net/netip"
  "IP-TCP/pkg/iptcpstack"
)

type VTCPListener struct {
  socket *iptcpstack.Socket
  tcpStack *iptcpstack.TCPStack
  acceptQueue chan *VTCPConn
  closed bool
}

func VListen(port uint16, tcpStack *iptcpstack.TCPStack) (*VTCPListener, error){
  sock := &iptcpstack.Socket{
    SID: tcpStack.NextSocketID,
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
    socket: sock,
    tcpStack: tcpStack,
    acceptQueue: make(chan *VTCPConn, 100), //buffer of 100 pending connections idk
    closed: false,
  }
  return listener, nil
}


func (l *VTCPListener) VAccept() (*VTCPConn, error){
  if l.closed {
    return nil, fmt.Errorf("Listener is closed")
  }

  conn, ok := <-l.acceptQueue
  if !ok {
    return nil, fmt.Errorf("Listener is closed")
  }

  return conn, nil
}

func (l *VTCPListener) VClose() error {
  if l.closed {
    return fmt.Errorf("Listener is already closed")
  }

  l.closed = true
  close(l.acceptQueue)
  delete(l.tcpStack.Sockets, l.socket.SID)
  return nil
}

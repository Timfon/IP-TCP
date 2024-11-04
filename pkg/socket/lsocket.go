package socket

import (
  "fmt"
  "net/netip"
  "IP-TCP/pkg/iptcpstack"
)

type VTCPListener struct {
  Socket *iptcpstack.Socket
  AcceptQueue chan *VTCPConn
  Closed bool
}

func (l *VTCPListener) VAccept() (*VTCPConn, error){
  if l.Closed {
    return nil, fmt.Errorf("Listener is closed")
  }

  conn, ok := <-l.AcceptQueue
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

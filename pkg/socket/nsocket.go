package socket

import (
	"net/netip"
  "fmt"
  "IP-TCP/pkg/iptcpstack"
  "IP-TCP/pkg/ipv4header"
  "IP-TCP/pkg/iptcp_utils"
	"github.com/google/netstack/tcpip/header"
  "time"
  "math/rand"
)

type VTCPConn struct {
  readBuffer []byte
  closed bool
}


func (c *VTCPConn) VClose() error {
  if c.closed {
    return fmt.Errorf("Connection is already closed")
  }
  c.closed = true
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

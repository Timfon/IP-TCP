package iptcpstack

import (
	"net/netip"
	//"IP-TCP/pkg/iptcp_utils"
	"math/rand"
    "fmt"
    "time"
    "github.com/google/netstack/tcpip/header"
)

type SocketStatus int

const (
  Listening SocketStatus = 0
  SynSent SocketStatus = 1
  SynReceived = 2
  Established = 3
)


type Socket struct {
  SID int
  Conn *VTCPConn 
  Listen *VTCPListener
  Closed bool
}

type VTCPConn struct {
    State SocketStatus
    LocalAddr netip.Addr
    LocalPort uint16  
    RemoteAddr netip.Addr
    RemotePort uint16
    SeqNum  uint32
    AckNum  uint32
    WindowSize uint16
    SendWindow []byte
    readBuffer []byte
  }

type VTCPListener struct {
    AcceptQueue chan *VTCPConn
    LocalPort uint16  
    Closed bool

    //info about previous packet
    SrcAddr netip.Addr
    DstAddr netip.Addr
    SrcPort uint16
    SeqNum  uint32
}
  

func (tcpStack *TCPStack) VConnect(addr netip.Addr, port uint16, ipStack *IPStack) (*VTCPConn, error) {
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
    localPort := uint16(rand.Uint32() >> 16)
    conn := &VTCPConn{
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

    sock:= &Socket{
        SID: tcpStack.NextSocketID,
        Conn: conn,
    }
    tcpStack.NextSocketID++
    //tcpStack.Sockets[sock.SID] = sock //I don't think we should be adding synsent to the sockettable
    tcpStack.Sockets[sock.SID] = sock


    // Create TCP header bytes
    err:= ipStack.sendTCPPacket(sock, []byte{}, header.TCPFlagSyn)
    if err != nil {
        return nil, fmt.Errorf("failed to send SYN packet: %v", err)
    }
    // Create connection object

    return conn, nil
}

func (tcpStack *TCPStack) VListen(port uint16) (*VTCPListener, error){
	l := &VTCPListener{
        AcceptQueue: make(chan *VTCPConn, 100),
        LocalPort: port,
    }

    sock := &Socket{
        SID: tcpStack.NextSocketID,
        Listen: l,
    }
	tcpStack.NextSocketID++
	tcpStack.Sockets[sock.SID] = sock
	return l, nil
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
  
  //deal with close later
  // func (l *VTCPListener) VClose() error {
  //   if l.Closed {
  //     return fmt.Errorf("Listener is already closed")
  //   }
  // 
  //   l.Closed = true
  //   close(l.AcceptQueue)
  //   delete(Sockets, l.SID)
  //   return nil
  // }
  //
  // func (c *VTCPConn) VClose() error {
  //   if c.closed {
  //     return fmt.Errorf("Connection is already closed")
  //   }
  //   c.closed = true
  //   return nil
  // }

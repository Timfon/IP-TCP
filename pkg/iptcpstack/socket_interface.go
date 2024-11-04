package iptcpstack

import {
	"net/netip"
	"IP-TCP/pkg/socket"
	"IP-TCP/pkg/iptcp_utils"
	"math/rand"
}

func ( tcpStack *TCPStack) VConnect(addr netip.Addr, port uint16, ipStack *IPStack) (*socket.VTCPConn, error) {
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
    sock := &socket.Socket{
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
    tcpStack.Sockets[sock.SID] = sock
    // Create and send SYN packet
	/*
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
    } */

    return conn, nil
}

func (tcpStack *iptcpstack.TCPStack) VNormal(port uint16) (*socket.VTCPConn, error){

}


func (tcpStack *iptcpstack.TCPStack) VListen(port uint16) (*socket.VTCPListener, error){
	sock := &iptcpstack.Socket{
	  SID: tcpStack.NextSocketID, // or should be next socket id?
	  State: 0,
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
  
	listener := &socket.VTCPListener{
	  Socket: sock,
	  AcceptQueue: make(chan *VTCPConn, 100), //buffer of 100 pending connections idk
	  Closed: false,
	}
	return listener, nil
  }
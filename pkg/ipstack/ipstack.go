package ipstack

import (
  "net/netip"
  "net"
  "time"
  "IP/pkg/lnxconfig"
  "IP/pkg/ipv4header"
  "github.com/google/netstack/tcpip/header"
  "fmt"
  "log"
)


type RoutingMode int

const (
	RoutingTypeNone   RoutingMode = 0
	RoutingTypeStatic RoutingMode = 1
	RoutingTypeRIP    RoutingMode = 2
)

type Route struct {
	RoutingMode RoutingMode
	Prefix netip.Prefix
	NextHop netip.Addr
	Cost int
}

type ForwardingTable struct {
	Routes []Route
}

func AddToForwardingTable(table *ForwardingTable, route Route) {
	table.Routes = append(table.Routes, route)
}

type Interface struct {
  Name string
  AssignedIP netip.Addr
  AssignedPrefix netip.Prefix
  UDPAddr netip.AddrPort
  UpOrDown bool
}

type IPStack struct {
 	Interfaces []Interface
	Neighbors  []lnxconfig.NeighborConfig
	RoutingMode lnxconfig.RoutingMode

	// ROUTERS ONLY:  Neighbors to send RIP packets
	RipNeighbors []netip.Addr

	// Manually-added routes ("route" directive, usually just for default on hosts)
	StaticRoutes map[netip.Prefix]netip.Addr

	OriginatingPrefixes []netip.Prefix // Unused, ignore.

	// ROUTERS ONLY:  Timing parameters for RIP updates
	RipPeriodicUpdateRate time.Duration
	RipTimeoutThreshold   time.Duration

	// HOSTS ONLY:  Timing parameters for TCP
	TcpRtoMin time.Duration
	TcpRtoMax time.Duration
	ForwardingTable ForwardingTable

	Default_Addr netip.Prefix
}

func InitializeStack(config *lnxconfig.IPConfig) (*IPStack, error){

  var ifaces []Interface
  for _, interfaceConfig := range config.Interfaces {
		iface := Interface{
			Name: interfaceConfig.Name,
			AssignedIP: interfaceConfig.AssignedIP,
			AssignedPrefix: interfaceConfig.AssignedPrefix,
			UDPAddr: interfaceConfig.UDPAddr,
			UpOrDown: true,
		}
		ifaces = append(ifaces, iface)
	}

  
  stack := &IPStack{
    Interfaces: ifaces,
    Neighbors: config.Neighbors,
    RoutingMode: config.RoutingMode,
    RipNeighbors: config.RipNeighbors,
    StaticRoutes: config.StaticRoutes,
    OriginatingPrefixes: config.OriginatingPrefixes,
    RipPeriodicUpdateRate: config.RipPeriodicUpdateRate,
    RipTimeoutThreshold: config.RipTimeoutThreshold,
    TcpRtoMin: config.TcpRtoMin,
    TcpRtoMax: config.TcpRtoMax,

	ForwardingTable: ForwardingTable{
		Routes: []Route{},
	},
	

}
  return stack, nil
}


func SendIP(src, dst netip.Addr, protocolNum uint8, data []byte, remoteAddr *net.UDPAddr, conn net.UDPConn) (error) { // does it really need src tho? and conn and remote addr?
	// Do we do the UDP stuff here or somewhere else?? 
	hdr := ipv4header.IPv4Header{
		Version:  4,
		Len: 	20, // Header length is always 20 when no IP options
		TOS:      0,
		TotalLen: ipv4header.HeaderLen + len(data),
		ID:       0,
		Flags:    0,
		FragOff:  0,
		TTL:      16, // idk man
		Protocol: int(protocolNum),
		Checksum: 0, // Should be 0 until checksum is computed
		Src:      src,
		Dst:      dst,
		Options:  []byte{},
	}
    // Assemble the header into a byte array
	headerBytes, err := hdr.Marshal()
	if err != nil {
		log.Fatalln("Error marshalling header:  ", err)
	}

	// Compute the checksum (see below)
	// Cast back to an int, which is what the Header structure expects
	hdr.Checksum = int(ComputeChecksum(headerBytes))

	headerBytes, err = hdr.Marshal()
	if err != nil {
		log.Fatalln("Error marshalling header:  ", err)
	}

	// Append header + message into one byte array
	bytesToSend := make([]byte, 0, len(headerBytes)+len(data))
	bytesToSend = append(bytesToSend, headerBytes...)
	bytesToSend = append(bytesToSend, []byte(data)...)

	// Send the message to the "link-layer" addr:port on UDP
	// FOr h1:  send to port 5002
	// ONE CALL TO WriteToUDP => 1 PACKET
	bytesWritten, err := conn.WriteToUDP(bytesToSend, remoteAddr)
	if err != nil {
		log.Panicln("Error writing to socket: ", err)
	}
	fmt.Printf("Sent %d bytes\n", bytesWritten)
	return nil
}


func ValidateChecksum(b []byte, fromHeader uint16) uint16 {
	checksum := header.Checksum(b, fromHeader)
	return checksum
}

func ComputeChecksum(b []byte) uint16 {
	checksum := header.Checksum(b, 0)

	// Invert the checksum value.  Why is this necessary?
	// This function returns the inverse of the checksum
	// on an initial computation.  While this may seem weird,
	// it makes it easier to use this same function
	// to validate the checksum on the receiving side.
	// See ValidateChecksum in the receiver file for details.
	checksumInv := checksum ^ 0xffff
	return checksumInv
}

func interfaceR(){

}

func matchPrefix(table *map[netip.Prefix]Interface, addr netip.Addr) (Interface, boolean, error){
	var longest_prefix Interface
	var b = 0
	for k, v := range table { 
		if k.contains(addr) {
			if k.Bits() > b {
				longest_prefix = v
				b = k.Bits()
			}
		}
	}
	if longest_prefix == nil{
		return nil, true, nil
	}
	return longest_prefix, false, nil	
}


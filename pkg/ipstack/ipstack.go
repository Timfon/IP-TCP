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
	NextHopUDP netip.AddrPort
	Cost int
}

//forwarding table is a slice/slice of routes
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
	Handlers map[uint8]HandlerFunc
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
	var routes []Route
	for _, interfaceConfig := range config.Interfaces {
		iface := Interface{
			Name: interfaceConfig.Name,
			AssignedIP: interfaceConfig.AssignedIP,
			AssignedPrefix: interfaceConfig.AssignedPrefix,
			UDPAddr: interfaceConfig.UDPAddr,
			UpOrDown: true,
		}

		route := Route{
			RoutingMode: RoutingTypeStatic,
			Prefix: interfaceConfig.AssignedPrefix,
			NextHop: interfaceConfig.AssignedIP,
			NextHopUDP: interfaceConfig.UDPAddr,
			Cost: -1,//change later
		}
		routes = append(routes, route)
		ifaces = append(ifaces, iface)
	}
  stack := &IPStack{
	  Handlers: make(map[uint8]HandlerFunc),
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
		Routes: routes,
	},
}
return stack, nil
}

//sending/receiving packets logic:

const (
	MaxMessageSize = 1400
)
type Packet struct {
	Header ipv4header.IPv4Header
	Body []byte
}
type HandlerFunc = func (*Packet, []interface{})
func (stack *IPStack) RegisterRecvHandler (protocolNum uint8, callbackFunc HandlerFunc) {
	stack.Handlers[protocolNum] = callbackFunc
}
func TestPacketHandler(packet *Packet, conn net.UDPConn) {
	fmt.Println("Test packet received")
}

// func RIPHandler(packet *Packet, args []interface{}) {
// 	fmt.Println("RIP packet received")
// }

//pass interface by reference?
func SendIP(iface Interface, dst netip.Addr, table *ForwardingTable, protocolNum uint8, data []byte) (error) { 
	// Do we do the UDP stuff here or somewhere else?? 
	// Turn the address string into a UDPAddr for the connection
	route, found, _ := MatchPrefix(table, dst)
	if !found {
		fmt.Println("No route found for packet")
		return nil
	}

	// bindAddrString := fmt.Sprintf(":%s", iface.UDPAddr.Port())
	// bindLocalAddr, err := net.ResolveUDPAddr("udp4", bindAddrString)
	// if err != nil {
	// 	log.Panicln("Error resolving address:  ", err)
	// }
	
	remoteAddrString := fmt.Sprintf("%s:%s", route.NextHopUDP.Addr(),route.NextHopUDP.Port())
	remoteAddr, err := net.ResolveUDPAddr("udp4", remoteAddrString)
	if err != nil {
		log.Panicln("Error resolving remote address:  ", err)
		return err	
	}

	conn, err := net.ListenUDP("udp4", remoteAddr)
	if err != nil {
		log.Panicln("Dial: ", err)
	}

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
		Src:      iface.AssignedIP, // double check
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

//maybe pass interface by reference
func ReceiveIP(addr *netip.AddrPort, table *ForwardingTable, iface Interface, stack *IPStack) (*Packet, *net.UDPAddr, error) {
    listenString := fmt.Sprintf(":%d", iface.UDPAddr.Port())
    listenAddr, err := net.ResolveUDPAddr("udp4", listenString)
    if err != nil {
        log.Fatalln("Error resolving UDP address: ", err)
    }
    
    // Create a new variable for the UDP connection
    conn, err := net.ListenUDP("udp4", listenAddr)
    if err != nil {
        log.Panicln("Could not bind to UDP port: ", err)
    }

	for {
		buffer := make([]byte, MaxMessageSize)

		_, sourceAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Panicln("Error reading from UDP socket ", err)
		}
		// Marshal the received byte array into a UDP header
		// NOTE:  This does not validate the checksum or check any fields
		// (You'll need to do this part yourself)
		hdr, err := ipv4header.ParseHeader(buffer)

		if err != nil {
			// What should you if the message fails to parse?
			// Your node should not crash or exit when you get a bad message.
			// Instead, simply drop the packet and return to processing.
			fmt.Println("Error parsing header", err)
			continue
		}
		headerSize := hdr.Len

		// Validate the checksum
		// The checksum is correct if the value we computed matches
		// the value stored in the header.
		// See ValudateChecksum for details.
		headerBytes := buffer[:headerSize]
		checksumFromHeader := uint16(hdr.Checksum)
		computedChecksum := ValidateChecksum(headerBytes, checksumFromHeader)

		if computedChecksum != checksumFromHeader {
			fmt.Println("Checksums do not match, dropping packet")
			continue
		}

		hdr.TTL = hdr.TTL - 1

		// Next, get the message, which starts after the header
		message := buffer[headerSize:]
		if hdr.Dst == addr.Addr() {
			if hdr.TTL == 0 {
				fmt.Println("TTL is 0, dropping packet")
				continue
			}

			packet := &Packet {
				Header: *hdr,
				Body: message,
			}

			if handler, exists := stack.Handlers[uint8(hdr.Protocol)]; exists {
				handler(packet, []interface{}{conn, sourceAddr})
			} else {
				fmt.Printf("No handler for protocol %d\n", hdr.Protocol)
			}
		} else {
			// Forward the packet
			route, found, _ := MatchPrefix(table, hdr.Dst)
			if !found {
				fmt.Println("No route found for packet")
				//what to do here?
				continue
			}
			fmt.Println("Forwarding packet to ", route.NextHop)
			SendIP(iface, route.NextHop, table, uint8(hdr.Protocol), message)
		}
		// Finally, print everything out
		// fmt.Printf("Received IP packet from %s\nHeader:  %v\nChecksum:  %s\nMessage:  %s\n",
		// 	sourceAddr.String(), hdr, checksumState, string(message))
	}

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


func MatchPrefix(table *ForwardingTable, addr netip.Addr) (Route, bool, error) {
	var longestRoute Route
	var longestPrefixLength = -1
	found := false
	
	for _, route := range table.Routes {
		if route.Prefix.Contains(addr) {
			prefixLength := route.Prefix.Bits() 
			if prefixLength > longestPrefixLength {
				longestRoute = route
				longestPrefixLength = prefixLength
				found = true
			}
		}
	}

	if !found {
		return Route{}, false, nil
	}
	return longestRoute, true, nil
}

func interfaceR(){

}

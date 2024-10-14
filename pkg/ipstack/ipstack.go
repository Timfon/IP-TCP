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
	RoutingTypeLocal  RoutingMode = 3
)



const (
	MAX_MESSAGE_SIZE = 1400
)

type Route struct {
	Iface Interface
	RoutingMode RoutingMode
	Prefix netip.Prefix
	VirtualIP netip.Addr
	Cost int
}

type Interface struct {
	Name string
	UdpSocket *net.UDPConn
	UpOrDown bool
  
  }

//forwarding table is a slice/slice of routes
type ForwardingTable struct {
	Routes []Route
}

func AddToForwardingTable(table *ForwardingTable, route Route) {
	table.Routes = append(table.Routes, route)
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

		udpPort := interfaceConfig.UDPAddr

		remoteAddr := net.UDPAddrFromAddrPort(udpPort)
		conn, err := net.ListenUDP("udp4", remoteAddr)
		if err != nil {
			log.Panicln("Dial: ", err)
		}

		iface := Interface{
			Name: interfaceConfig.Name,
			UdpSocket: conn,
			UpOrDown: true,
		}

		route := Route{
			Iface: iface,
			Prefix: interfaceConfig.AssignedPrefix,
			RoutingMode: RoutingTypeLocal,
			VirtualIP: interfaceConfig.AssignedIP,
			Cost: -1,//change later
		}
		routes = append(routes, route)
		ifaces = append(ifaces, iface)
	}

	for p, addr := range config.StaticRoutes {
		route := Route{
			RoutingMode: RoutingTypeStatic,
			Prefix: p,
			VirtualIP: addr,
			Cost: -1,
		}
		routes = append(routes, route)
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

//pass interface by reference?
func SendIP(dst netip.Addr, stack *IPStack, protocolNum uint8, data []byte) (error) { 
	// Do we do the UDP stuff here or somewhere else?? 
	// Turn the address string into a UDPAddr for the connection
	table:= stack.ForwardingTable
	route, found, _ := MatchPrefix(&table, dst)

	if !found {
		fmt.Println("No matching prefix found")
		return nil
	}
	var neighborUDP netip.AddrPort
	for _, neighbor := range stack.Neighbors {
		if neighbor.DestAddr == dst {
			neighborUDP = neighbor.UDPAddr
		}	
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
		Src:      route.VirtualIP, // double check
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
	bytesWritten, err := route.Iface.UdpSocket.WriteToUDP(bytesToSend,  net.UDPAddrFromAddrPort(neighborUDP))
	if err != nil {
		log.Panicln("Error writing to socket: ", err)
	}
	fmt.Printf("Sent %d bytes\n", bytesWritten)
	return nil
}

//maybe pass interface by reference
func ReceiveIP(route Route, stack *IPStack) (*Packet, *net.UDPAddr, error) {
    conn := route.Iface.UdpSocket
	addr := route.VirtualIP

	for {
		buffer := make([]byte, MAX_MESSAGE_SIZE)

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
		if hdr.Dst == addr {
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
			nroute, found, _ := MatchPrefix(&stack.ForwardingTable, hdr.Dst)
			if !found {
				fmt.Println("No route found for packet")
				//what to do here?
				continue
			}
			fmt.Println("Forwarding packet to ", route.VirtualIP)
			SendIP(nroute.VirtualIP, stack, uint8(hdr.Protocol), message)
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
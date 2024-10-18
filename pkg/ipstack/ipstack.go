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
  "sync"
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
	Iface *Interface
	RoutingMode RoutingMode
	Prefix netip.Prefix
	VirtualIP netip.Addr
	UpdateTime time.Time
	Cost uint32
}

type Interface struct {
	Name string
	UdpSocket *net.UDPConn
	UpOrDown bool
  }

//forwarding table is a slice/slice of routes
type ForwardingTable struct {
	Routes []Route
	Mu sync.Mutex
}


func (table *ForwardingTable) AddToForwardingTable(route Route) {
	table.Mu.Lock()
	defer table.Mu.Unlock()
	table.Routes = append(table.Routes, route)
}

func (table *ForwardingTable) MatchPrefix(addr netip.Addr) (Route, int, error) {
	table.Mu.Lock()
	defer table.Mu.Unlock()
	var longestRoute Route
	var longestPrefixLength = -1
	found := -1
	
	for i, route := range table.Routes {
		if route.Prefix.Contains(addr) {
			prefixLength := route.Prefix.Bits() 
			if prefixLength > longestPrefixLength {
				longestRoute = route
				longestPrefixLength = prefixLength
				found = i
			}
		}
	}

	return longestRoute, found, nil
}

type IPStack struct {
	Handlers map[uint8]HandlerFunc
 	Interfaces map[string]*Interface
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
	
	var ifaces map[string]*Interface
	ifaces = make(map[string]*Interface)
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
			Iface: &iface,
			Prefix: interfaceConfig.AssignedPrefix,
			RoutingMode: RoutingTypeLocal,
			VirtualIP: interfaceConfig.AssignedIP,
			Cost: 0,//change later
		}
		routes = append(routes, route)
		ifaces[iface.Name] = &iface
	}

	for p, addr := range config.StaticRoutes {
		route := Route{
			RoutingMode: RoutingTypeStatic,
			Prefix: p,
			VirtualIP: addr,
			Cost: 0, //higher cost for a static route??
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

//send over
/*
for _, rn := range config.RipNeighbors {

}*/
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

func TestPacketHandler(packet *Packet, args []interface{}) {
	srcIP := packet.Header.Src
	dstIP := packet.Header.Dst
	ttl := packet.Header.TTL
	data := string(packet.Body) 
	fmt.Printf("Received test packet: Src: %s, Dst: %s, TTL: %d, Data: %s\n", srcIP, dstIP, ttl, data)
	fmt.Print("> ")
}

//pass interface by reference?
func SendIP(stack *IPStack, header *ipv4header.IPv4Header, data []byte) (error) { 
	// Do we do the UDP stuff here or somewhere else?? 
	// Turn the address string into a UDPAddr for the connection
	dst:= header.Dst
	table:= stack.ForwardingTable

	route, found, _ := table.MatchPrefix(dst)
	if found == -1 {
		fmt.Println("No matching prefix found")
		return nil
	}

	var nextHop netip.AddrPort
	var conn *net.UDPConn

	if route.RoutingMode == RoutingTypeLocal {
		conn = route.Iface.UdpSocket
		if !route.Iface.UpOrDown{
			return nil
		}
		for _, n := range stack.Neighbors {
			if n.DestAddr == dst {
				nextHop = n.UDPAddr
			}
		}
	  } else {
		iroute, _, _ := table.MatchPrefix(route.VirtualIP)
		conn = iroute.Iface.UdpSocket
		if !iroute.Iface.UpOrDown{
			return nil
		}
		for _, n := range stack.Neighbors {
			if n.DestAddr == route.VirtualIP {
				nextHop = n.UDPAddr
			}
		}
	}

	if !nextHop.IsValid(){
		fmt.Println("No valid next hop port found")
		return nil
	}

	headerBytes, err := header.Marshal()

	header.Checksum = int(ComputeChecksum(headerBytes))
	if err != nil {
		log.Fatalln("Error marshalling header:  ", err)
	}
	headerBytes, err = header.Marshal()
	if err != nil {
		log.Fatalln("Error marshalling header:  ", err)
	}

	bytesToSend := make([]byte, 0, len(headerBytes)+len(data))
	bytesToSend = append(bytesToSend, headerBytes...)
	bytesToSend = append(bytesToSend, data...)
	
	bytesWritten, err := conn.WriteToUDP(bytesToSend,  net.UDPAddrFromAddrPort(nextHop))

	if err != nil {
		log.Panicln("Error writing to socket: ", err)
	}

	if header.Protocol == 0 {
		fmt.Printf("Sent %d bytes\n", bytesWritten)
	}
	return nil
}

//maybe pass interface by reference
func ReceiveIP(route Route, stack *IPStack) (*Packet, *net.UDPAddr, error) {
    conn := route.Iface.UdpSocket
	addr := route.VirtualIP

	for {
		buffer := make([]byte, MAX_MESSAGE_SIZE)
		_, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Panicln("Error reading from UDP socket ", err)
		}


		if !route.Iface.UpOrDown{
			continue
		}

		// Marshal the received byte array into a UDP header
		// NOTE:  This does not validate the checksum or check any fields
		// (You'll need to do this part yourself)
		hdr, err := ipv4header.ParseHeader(buffer)
		hdr.TTL = hdr.TTL - 1
		
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

		message := buffer[headerSize:]
		packet := &Packet {
			Header: *hdr,
			Body: message,
		}
		
		hdr.Checksum = 0
		computedChecksum := ValidateChecksum(headerBytes, checksumFromHeader)
		if computedChecksum != checksumFromHeader {
			fmt.Println("Checksums do not match, dropping packet")
			fmt.Print("> ")
			continue
		}

		// Next, get the message, which starts after the header
		if hdr.Dst == addr {
			if hdr.TTL == 0 {
				fmt.Println("TTL is 0, dropping packet")
				fmt.Print("> ")
				continue
			}
			if handler, exists := stack.Handlers[uint8(hdr.Protocol)]; exists {
				//for tcp, may need to change this code to pass in different parameters for rip, tcp, and test, although test doesn't need any parameters other than the packet. 
				handler(packet, []interface{}{stack})
			} else {
				fmt.Printf("No handler for protocol %d\n", hdr.Protocol)
			}
		} else {	
			SendIP(stack, hdr, message)
		}
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
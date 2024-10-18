package rippacket

import (
	"IP/pkg/ipstack"
	"IP/pkg/ipv4header"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
    "math/bits"
	"time"
)

type RIPEntry struct {
	Cost    uint32
	Address uint32
	Mask    uint32
}

type RIPMessage struct {
	Command    uint16 //1 for request, 2 for response
	NumEntries uint16
	Entries    []RIPEntry
}

// Function to serialize the RIP message to bytes
func SerializeRIPMessage(msg *RIPMessage) []byte {
	buffer := make([]byte, 4) // 4 bytes for Command and NumEntries
	binary.BigEndian.PutUint16(buffer[0:2], msg.Command)
	binary.BigEndian.PutUint16(buffer[2:4], msg.NumEntries)

	for _, entry := range msg.Entries {
		entryBytes := make([]byte, 12) // 12 bytes per entry
		binary.BigEndian.PutUint32(entryBytes[0:4], entry.Cost)
		binary.BigEndian.PutUint32(entryBytes[4:8], entry.Address)
		binary.BigEndian.PutUint32(entryBytes[8:12], entry.Mask)
		buffer = append(buffer, entryBytes...)
	}
	return buffer
}

func DeserializeRIPMessage(buffer []byte) *RIPMessage {
	msg := RIPMessage{}
	msg.Command = binary.BigEndian.Uint16(buffer[0:2])
	msg.NumEntries = binary.BigEndian.Uint16(buffer[2:4])
	buffer = buffer[4:]
	for i := 0; i < int(msg.NumEntries); i++ {
		entry := RIPEntry{}
		entry.Cost = binary.BigEndian.Uint32(buffer[0:4])
		entry.Address = binary.BigEndian.Uint32(buffer[4:8])
		entry.Mask = binary.BigEndian.Uint32(buffer[8:12])
		msg.Entries = append(msg.Entries, entry)
		buffer = buffer[12:]
	}
	return &msg
}

func SendRIPRequest(stack *ipstack.IPStack) {
	for _, RIPNeighbor := range stack.RipNeighbors {
		table := stack.ForwardingTable
		iroute, _, _ := table.MatchPrefix(RIPNeighbor)
		var srcIP = iroute.VirtualIP

		// Create a RIP request message
		ripRequest := RIPMessage{
			Command:    1,            // Command 1 for RIP request
			NumEntries: 0,            //may need to double check
			Entries:    []RIPEntry{}, // no entries for a request??
		}
		messageBytes := SerializeRIPMessage(&ripRequest)

		// Construct the IPv4 header
		hdr := ipv4header.IPv4Header{
			Version:  4,
			Len:      20, // Header length is always 20 when no IP options
			TOS:      0,
			TotalLen: ipv4header.HeaderLen + len(messageBytes),
			ID:       0,
			Flags:    0,
			FragOff:  0,
			TTL:      32,  // Time to live
			Protocol: 200, // Custom protocol number for RIP
			Checksum: 0,   // Will compute later
			Src:      srcIP,
			Dst:      RIPNeighbor,
			Options:  []byte{},
		}
		ipstack.SendIP(stack, &hdr, messageBytes)
	}
}

func SendRIPResponse(stack *ipstack.IPStack, routes []ipstack.Route) {
    for _, RIPNeighbor := range stack.RipNeighbors {
        iroute, _, _ := stack.ForwardingTable.MatchPrefix(RIPNeighbor)
        var srcIP = iroute.VirtualIP
        // Create a RIP response message
        ripResponse := RIPMessage{
            Command:    2, // Command 2 for RIP response
            NumEntries: 0, //change after populate entries
            Entries:    []RIPEntry{},
        }
        
        for _, route := range routes {
            addr := route.Prefix.Addr().String()
            mask := binary.BigEndian.Uint32(net.CIDRMask(route.Prefix.Bits(), 32))
            cost := route.Cost
            ip := net.ParseIP(addr).To4() // Convert string to net.IP and extract IPv4
            if ip == nil {
                // Handle error or unsupported cases for non-IPv4 addresses
                continue
            }
            // Convert the IPv4 address to uint32
            addrUint32 := binary.BigEndian.Uint32(ip)
            ripResponse.Entries = append(ripResponse.Entries, RIPEntry{
                Cost:    uint32(cost),
                Address: addrUint32,
                Mask:    mask,
            }) 
        }

        ripResponse.NumEntries = uint16(len(ripResponse.Entries))
        messageBytes := SerializeRIPMessage(&ripResponse)

        // Construct the IPv4 header
        hdr := ipv4header.IPv4Header{
            Version:  4,
            Len:      20, // Header length is always 20 when no IP options
            TOS:      0,
            TotalLen: ipv4header.HeaderLen + len(messageBytes),
            ID:       0,
            Flags:    0,
            FragOff:  0,
            TTL:      32,  // Time to live
            Protocol: 200, // Custom protocol number for RIP
            Checksum: 0,   // Will compute later
            Src:      srcIP,
            Dst:      RIPNeighbor,
            Options:  []byte{},
        }
        ipstack.SendIP(stack, &hdr, messageBytes)
    }
}

func CheckRouteTimeouts(stack *ipstack.IPStack) {
   
    for{
        now := time.Now()
        stack.ForwardingTable.Mu.Lock()
        //fmt.Println("Lock acquired")
        validRoutes := []ipstack.Route{}
        modifiedRoutes:= []ipstack.Route{}
        for _, route := range stack.ForwardingTable.Routes {
            if route.RoutingMode == 2 { 
                if now.Sub(route.UpdateTime) > 12 * time.Second && route.Cost < 16{
                    fmt.Println("Route timeout: ", route.Prefix)
                    // Route has expired, set cost to infinity and remove after triggering update
                    route.Cost = 16 // Set to infinity
                    modifiedRoutes = append(modifiedRoutes, route)
                }
            }
            validRoutes = append(validRoutes, route)
        }
        stack.ForwardingTable.Mu.Unlock()
        if len(modifiedRoutes) > 0{
            SendRIPResponse(stack, modifiedRoutes)
            stack.ForwardingTable.Mu.Lock()
            stack.ForwardingTable.Routes = validRoutes  
            stack.ForwardingTable.Mu.Unlock()
        }
        time.Sleep(500 * time.Millisecond)

    }
}

// a response to a RIP request, a triggered update, or a periodic update
func RipPacketHandler(packet *ipstack.Packet, args []interface{}) {
	msg := DeserializeRIPMessage(packet.Body)
	stack := args[0].(*ipstack.IPStack)

	if msg.Command == 1 { // or gets a triggered update, or a periodic update??
		// Send a response to the request
		stack.ForwardingTable.Mu.Lock()
		var ftable_copy = make([]ipstack.Route, len(stack.ForwardingTable.Routes))
		copy(ftable_copy, stack.ForwardingTable.Routes)
		stack.ForwardingTable.Mu.Unlock()

		SendRIPResponse(stack, ftable_copy)
		
        } else {
           
		UpdateForwardingTable(packet, stack)

	}
}

func SendPeriodicRIP(stack *ipstack.IPStack) {
    for {
        time.Sleep(5 * time.Second)
        var ftable_copy = make([]ipstack.Route, len(stack.ForwardingTable.Routes))
		stack.ForwardingTable.Mu.Lock()
		for i, v := range stack.ForwardingTable.Routes {
            ftable_copy[i] = v
        }
		stack.ForwardingTable.Mu.Unlock()
  
		SendRIPResponse(stack, ftable_copy)
	}
}

func UpdateForwardingTable(packet *ipstack.Packet, stack *ipstack.IPStack) {
    msg := DeserializeRIPMessage(packet.Body)
    srcAddr := packet.Header.Src    // N from Neighbor
    //dstAddr := packet.Header.Dst    // Receiver's IP address
    receiving_route, found, err := stack.ForwardingTable.MatchPrefix(srcAddr)

    //fmt.Println(nexthopVIP, srcAddr, dstAddr)
    if err != nil {
        fmt.Println("Error matching prefix: ", err)
    }
    if found == -1 {
        fmt.Println("No matching prefix found")
    }

    updatedRoutes := []ipstack.Route{}
    for _, entry := range msg.Entries {
        addrBytes := make([]byte, 4)
        binary.BigEndian.PutUint32(addrBytes, entry.Address)
        maskBytes := make([]byte, 4)
        binary.BigEndian.PutUint32(maskBytes, entry.Mask)
        maskBits := 0
        for _, b := range maskBytes {
            maskBits += bits.OnesCount8(b)
        }
        
        netipAddr, ok := netip.AddrFromSlice(addrBytes)
        if !ok {
            fmt.Println("Error converting to netip.Addr")
            continue
        }
		prefix := netip.PrefixFrom(netipAddr, maskBits)

        route, found, err := stack.ForwardingTable.MatchPrefix(netipAddr)
        if err != nil {
            fmt.Println("Error matching prefix: ", err)
            continue
        }

        totalCost := entry.Cost + 1
        stack.ForwardingTable.Mu.Lock()
        if found >= 0{
            c_old := route.Cost
            
            if totalCost < c_old {
                // Better route found
                route.Cost = totalCost
                route.VirtualIP = srcAddr
                route.UpdateTime = time.Now()
                updatedRoutes = append(updatedRoutes, route)
                stack.ForwardingTable.Routes[found] = route
            } else if totalCost == c_old && route.VirtualIP == srcAddr {
                // Refresh existing route
                route.UpdateTime = time.Now()
                stack.ForwardingTable.Routes[found] = route
            } else if totalCost > c_old && route.VirtualIP == srcAddr {
                route.Cost = totalCost
                route.UpdateTime = time.Now()
                updatedRoutes = append(updatedRoutes, route)
                stack.ForwardingTable.Routes[found] = route
            }

			//fmt.Println(route.Prefix, dstAddr, route.VirtualIP)
        } else {
			fmt.Println(receiving_route.Prefix, srcAddr, receiving_route.VirtualIP)
            newRoute := ipstack.Route{
                Iface:       receiving_route.Iface,
                Prefix:      prefix,
                Cost:        entry.Cost + 1,
                VirtualIP:   srcAddr, //route.virtualIP
                UpdateTime:  time.Now(),
                RoutingMode: ipstack.RoutingTypeRIP,
            }

            stack.ForwardingTable.Routes = append(stack.ForwardingTable.Routes, newRoute)


        }
        stack.ForwardingTable.Mu.Unlock()
    
    }

    if len(updatedRoutes) > 0 {
        SendRIPResponse(stack, updatedRoutes)
    }
}
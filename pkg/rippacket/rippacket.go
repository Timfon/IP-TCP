package rippacket

import (
    "IP/pkg/ipstack"
    "IP/pkg/ipv4header"
    "encoding/binary"
    "net"
    "fmt"
    "time"
    "net/netip"
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
        iroute, _, _ := ipstack.MatchPrefix(&stack.ForwardingTable, RIPNeighbor)
        var srcIP = iroute.VirtualIP

        // Create a RIP request message
        ripRequest := RIPMessage{
            Command:    1, // Command 1 for RIP request
            NumEntries: 0, //may need to double check
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
            TTL:      32, // Time to live
            Protocol: 200, // Custom protocol number for RIP
            Checksum: 0,   // Will compute later
            Src:      srcIP,
            Dst:      RIPNeighbor,
            Options:  []byte{},
        }
        ipstack.SendIP(stack, &hdr, messageBytes)
    }
}

func SendRIPResponse(stack *ipstack.IPStack){
    for _, RIPNeighbor := range stack.RipNeighbors {
        iroute, _, _ := ipstack.MatchPrefix(&stack.ForwardingTable, RIPNeighbor)
        var srcIP = iroute.VirtualIP

        // Create a RIP response message
        ripResponse := RIPMessage{
            Command:    2, // Command 2 for RIP response
            NumEntries: 0, //change after populate entries
            Entries:    []RIPEntry{},
        }

        // Add entries to the response message
        for _, route := range stack.ForwardingTable.Routes {
            if route.RoutingMode == ipstack.RoutingTypeRIP {
                ipv4Addr := route.Prefix.Addr().As4()
                ipv4Mask := route.Prefix.Masked().Addr().As4()

                entry := RIPEntry{
                    Cost:    uint32(route.Cost),
                    Address: binary.BigEndian.Uint32(ipv4Addr[:]), //the [:] turns the array into a slice lmao
                    Mask:    binary.BigEndian.Uint32(ipv4Mask[:]),
                }
                // Poison Reverse: Set cost to infinity if advertising back to the source
                //double check that  virtual IP is in fact the next hop
                if route.VirtualIP == RIPNeighbor {
                    entry.Cost = 16 // Cost of infinity
                }
                ripResponse.Entries = append(ripResponse.Entries, entry)
            }
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
            TTL:      32, // Time to live
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
    now := time.Now()
    validRoutes := []ipstack.Route{}
    for _, route := range stack.ForwardingTable.Routes {
        if now.Sub(route.UpdateTime) > 12*time.Second {
            fmt.Println("Route timeout: ", route.Prefix)
            // Route has expired, set cost to infinity and remove after triggering update
            route.Cost = 16 // Set to infinity
            SendRIPResponse(stack)
            continue
        }
        validRoutes = append(validRoutes, route)
    }
    stack.ForwardingTable.Routes = validRoutes  
}


//a response to a RIP request, a triggered update, or a periodic update
func RipPacketHandler(packet *ipstack.Packet, args []interface{}){
    msg := DeserializeRIPMessage(packet.Body)
    stack := args[0].(*ipstack.IPStack)

    if msg.Command == 1 { // or gets a triggered update, or a periodic update??
        // Send a response to the request
        SendRIPResponse(stack)
    } else {
        // Update the forwarding table
        UpdateForwardingTable(packet, stack)
    }
}

//TODO: Implement periodic RIP updates
func SendPeriodicRIP(stack *ipstack.IPStack){
    //RIPNeighbor is a netip.Addr
    for {

    }

}

func UpdateForwardingTable(packet *ipstack.Packet, stack *ipstack.IPStack) {
    msg := DeserializeRIPMessage(packet.Body)
    srcAddr := packet.Header.Src


    //Kinda jank way of getting the interface of the src address??
    iface := ipstack.Interface{}
    for _, route := range stack.ForwardingTable.Routes {
        if route.VirtualIP == srcAddr {
            iface = route.Iface
            break
        }
    }

    for _, entry := range msg.Entries {
        // Convert Address and Mask to netip.Addr
        addr := net.IPv4(byte(entry.Address>>24), byte(entry.Address>>16), byte(entry.Address>>8), byte(entry.Address))
        mask := net.IPv4Mask(byte(entry.Mask>>24), byte(entry.Mask>>16), byte(entry.Mask>>8), byte(entry.Mask))

        netipAddr, ok:= netip.AddrFromSlice(addr)
        if !ok {
            fmt.Println("Error converting to netip.Addr: octopus:")
            continue
        }

        _, maskBits := mask.Size()
        prefix := netip.PrefixFrom(netipAddr, maskBits)//used later if we need to add a new route?

        route, found, err := ipstack.MatchPrefix(&stack.ForwardingTable, netipAddr)
        if err != nil {
            fmt.Println("Error matching prefix: ", err)
            continue
        }

        if found {
            // Existing route
            c_old := route.Cost

            // Compare costs and update accordingly
            if entry.Cost < c_old {
                // Better route, update table
                route.Cost = entry.Cost
                route.VirtualIP = srcAddr // N = source address of the packet
                route.UpdateTime = time.Now()
            } else if entry.Cost > c_old {
                if route.VirtualIP == srcAddr {
                    // Topology has changed, higher cost from same neighbor
                    route.Cost = entry.Cost
                    route.UpdateTime = time.Now()
                }
                // Else: we ignore the update because the current route is better
            } else if entry.Cost == c_old && route.VirtualIP == srcAddr {
                //refresh the timeout
                route.UpdateTime = time.Now()
            }
        } else {
            // New route, add to forwarding table
            newRoute := ipstack.Route{
                Iface:    iface,
                Prefix:    prefix,
                Cost:      entry.Cost,
                VirtualIP:   srcAddr,
                UpdateTime: time.Now(),
                RoutingMode: ipstack.RoutingTypeRIP,
            }
            stack.ForwardingTable.Routes = append(stack.ForwardingTable.Routes, newRoute)
        }
    }
}

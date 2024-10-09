package ipstack

import (
  "net/netip"
  "time"
  "IP/pkg/lnxconfig"
)


type RoutingMode int

const (
	RoutingTypeNone   RoutingMode = 0
	RoutingTypeStatic RoutingMode = 1
	RoutingTypeRIP    RoutingMode = 2
)

type Interface struct {
  AssignedIP netip.Addr
  AssignedPrefix netip.Prefix
  UDPAddr netip.AddrPort
}

type Neighbor struct {
	DestAddr netip.Addr
	UDPAddr  netip.AddrPort
}
type IPStack struct {
 	Interfaces []Interface
	Neighbors  []Neighbor
	RoutingMode RoutingMode

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

	ForwardingTable map[netip.Prefix]Interface

	Default_Addr netip.Prefix
}

func initializeStack(config *IPConfig) (*IPStack, error){

	var ifaces []Interface
	for _, interface := range in config.Interfaces {
		iface := Interface{
			AssignedIP: interface.AssignedIP,
			AssignedPrefix: interface.AssignedPrefix,
			UPDAddr: interface.UDPAddr,
			UpOrDown: true
		}
		ifaces = append(ifaces, iface)
	}


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


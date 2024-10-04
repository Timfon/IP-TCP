package protocol

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
  Name string
  AssignedIP netip.Addr
  AssignedPrefix netip.Prefix
  UDPAddr netip.AddrPort
  UpOrDown bool

}

type Neighbor struct {
	DestAddr netip.Addr
	UDPAddr  netip.AddrPort
	InterfaceName string
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
			Name: interface.Name,
			AssignedIP: interface.AssignedIP,
			AssignedPrefix: interface.AssignedPrefix,
			UPDAddr: interface.UDPAddr,
			UpOrDown: true
		}
		ifaces = append(ifaces, iface)
	}


}


func interfaceR(){

}

func interfaceW(){

}
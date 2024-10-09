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

	ForwardingTable map[netip.Prefix]Interface

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
    ForwardingTable: make(map[netip.Prefix]Interface),
  }
  return stack, nil
}


func interfaceR(){

}

func interfaceW(){

}
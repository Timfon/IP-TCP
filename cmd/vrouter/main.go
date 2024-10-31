package main

import (
	"fmt"
	//"net/netip"
  "IP-TCP/pkg/lnxconfig"
  "IP-TCP/pkg/iptcpstack"
  "IP-TCP/pkg/repl"
	"os"
	"IP-TCP/pkg/rippacket"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Printf("Usage:  %s --config <lnx file>\n", os.Args[0])
		os.Exit(1)
	}
	fileName := os.Args[2]

	lnxConfig, err := lnxconfig.ParseConfig(fileName)
	if err != nil {
		panic(err)
	}

	// Goroutine for each interface
  stack, err := iptcpstack.InitializeStack(lnxConfig)
  tcp_stack, err := iptcpstack.InitializeTCP(lnxConfig)
  if err != nil {
	panic(err)
	  }
  //need to consult forwarding table to know the src of a packet interesting
  //hacky solution for now
  stack.RegisterRecvHandler(0, iptcpstack.TestPacketHandler)
  stack.RegisterRecvHandler(200, rippacket.RipPacketHandler)
  stack.RegisterRecvHandler(6, iptcpstack.TCPPacketHandler)

  go repl.StartRepl(stack, tcp_stack, "router")

  for _, routes := range stack.ForwardingTable.Routes{
	go iptcpstack.ReceiveIP(routes, stack, tcp_stack)
  }

  go rippacket.CheckRouteTimeouts(stack);

  go rippacket.SendRIPRequest(stack);

  go rippacket.SendPeriodicRIP(stack);

  

  //extra thread for sending periodic rip commands
  select{}
}

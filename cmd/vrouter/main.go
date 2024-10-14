package main

import (
	"fmt"
	//"net/netip"
  "IP/pkg/lnxconfig"
  "IP/pkg/ipstack"
  "IP/pkg/repl"
	"os"
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
  stack, err := ipstack.InitializeStack(lnxConfig)
  if err != nil {
	panic(err)
	  }
  //need to consult forwarding table to know the src of a packet interesting
  //hacky solution for now

  go repl.StartRepl(stack, "router")

  for _, routes := range stack.ForwardingTable.Routes{
	go ipstack.ReceiveIP(routes, stack)
  }
  select{}
}

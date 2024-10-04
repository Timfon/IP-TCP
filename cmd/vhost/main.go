package main

import (
  "fmt"
  "os"
	"IP/pkg/lnxconfig"
  "IP/pkg/ipstack"
  "net"
  "net/netip"
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
  
  //sets everything up
  stack, err := initializeStack(lnxConfig);
}









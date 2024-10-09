package main

import (
  "fmt"
  "os"
	"IP/pkg/lnxconfig"
  // "bufio"
  "IP/pkg/repl"
  // "net"
  "IP/pkg/ipstack"
  // "net/netip"
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
  var _ = lnxConfig
  
  //sets everything up
  stack, err := ipstack.InitializeStack(lnxConfig)
  fmt.Println(stack)
  go repl.StartRepl(lnxConfig)
  fmt.Println("hello")




}








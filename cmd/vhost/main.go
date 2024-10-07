package main

import (
  "fmt"
  "os"
	"IP/pkg/lnxconfig"
  "bufio"
  // "IP/pkg/ipstack"
  // "net"
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
  // stack, err := initializeStack(lnxConfig);

  reader := bufio.NewScanner(os.Stdin)
  for {
    fmt.Print("> ")
    if !reader.Scan() {
      break
    }
    input := reader.Text()
    if input == "li" {
      fmt.Println("Name "+ "Addr/Prefix " + "State")
      for _, iface := range lnxConfig.Interfaces {
        fmt.Println(iface.Name + " " + iface.AssignedPrefix.String() + " " + "UP") // change UP later to have the actual state of interface
      }
    }
  }
}









package main

import (
	"fmt"
	"IP/pkg/lnxconfig"
	"net/netip"
  "net"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Printf("Usage:  %s --config <lnx file>\n", os.Args[0])
		os.Exit(1)
	}
	fileName := os.Args[2]

	// Parse the file
	lnxConfig, err := lnxconfig.ParseConfig(fileName)
	if err != nil {
		panic(err)
	}

	// Demo:  print out the IP for each interface in this config
	for _, iface := range lnxConfig.Interfaces {
		prefixForm := netip.PrefixFrom(iface.AssignedIP, iface.AssignedPrefix.Bits())
		fmt.Printf("%s has IP %s\n", iface.Name, prefixForm.String())
    udpPort, err := netip.ParseAddrPort(iface.UDPAddr.String())
    if err != nil {
      fmt.Printf("Error parsing UDP address: %s\n", err)
    }
    fmt.Printf("UDP: %s\n", udpPort.String())
    bindLocalAddr, err := net.ResolveUDPAddr("udp4", udpPort.String())
    if err != nil {
      fmt.Printf("Error resolving UDP address: %s\n", err)
    }
    conn, err := net.ListenUDP("udp4", bindLocalAddr)
    if err != nil {
      fmt.Printf("Error listening on UDP address: %s\n", err)
    }
    bytesWritten, err := conn.WriteToUDP([]byte("Hello, world!"), bindLocalAddr)
    if err != nil {
      fmt.Printf("Error writing to UDP address: %s\n", err)
    }
    fmt.Printf("Wrote %d bytes to UDP address\n", bytesWritten)
	}
	//fmt.Printf("\n\nFull config:  %+v\n", lnxConfig)
}



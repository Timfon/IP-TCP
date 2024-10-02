package main

import (
	"fmt"
	"log"
	"net"
	"net/netip"
  "IP/pkg/lnxconfig"
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
	for _, iface := range lnxConfig.Interfaces {
		prefixForm := netip.PrefixFrom(iface.AssignedIP, iface.AssignedPrefix.Bits())
		fmt.Printf("%s has IP %s\n", iface.Name, prefixForm.String())

		udpPort, err := netip.ParseAddrPort(iface.UDPAddr.String())
		if err != nil {
			fmt.Printf("Error parsing UDP address: %s\n", err)
			continue
		}
		fmt.Printf("UDP: %s\n", udpPort.String())

		bindLocalAddr, err := net.ResolveUDPAddr("udp4", udpPort.String())
		if err != nil {
			fmt.Printf("Error resolving UDP address: %s\n", err)
			continue
		}

		conn, err := net.ListenUDP("udp4", bindLocalAddr)
		if err != nil {
			fmt.Printf("Error listening on UDP address: %s\n", err)
			continue
		}
		// Handle each UDP connection in a separate goroutine
		go handleUDPConnection(conn)
	}
  //prevents main from exiting
	select {}
}

// handleUDPConnection manages the UDP connection in a separate goroutine
func handleUDPConnection(conn *net.UDPConn) {
	defer conn.Close()

	buffer := make([]byte, 1024)
	for {
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Error reading from UDP connection: %s\n", err)
			return
		}
		fmt.Printf("Received %s from %s\n", string(buffer[:n]), addr)
		_, err = conn.WriteToUDP([]byte("Hello, vhost-> vrouter"), addr)
		if err != nil {
			fmt.Printf("Error writing to UDP address: %s\n", err)
		}
	}
}

package main

import (
	"fmt"
	"log"
	"net"
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
  fmt.Println(stack)
  repl.StartRepl(lnxConfig)
  fmt.Println("hello")

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

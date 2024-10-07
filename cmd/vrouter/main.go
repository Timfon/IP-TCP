package main

import (
	"fmt"
  "bufio"
	"log"
	"net"
	//"net/netip"
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

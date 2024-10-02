package main
import (
  "fmt"
  //"IP/pkg/protocol"
  "os"
	"IP/pkg/lnxconfig"
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
  udpPort, err := netip.ParseAddrPort(lnxConfig.Interfaces[0].UDPAddr.String())

  fmt.Printf("UDP: %s\n", udpPort.String())
  bindLocalAddr, err := net.ResolveUDPAddr("udp4", udpPort.String())
  if err != nil {
    fmt.Printf("Error resolving UDP address: %s\n", err)
  }
  conn, err := net.ListenUDP("udp4", bindLocalAddr)
  if err != nil {
    fmt.Printf("Error listening on UDP address: %s\n", err)
  }
  defer conn.Close()

  // ONE CALL TO WriteToUDP => 1 PACKET
  bytesWritten, err := conn.WriteToUDP([]byte("Hello, vhost-> vrouter"), bindLocalAddr)
  if err != nil {
    fmt.Printf("Error writing to UDP address: %s\n", err)
  }
  fmt.Printf("Wrote %d bytes to UDP address\n", bytesWritten)
}











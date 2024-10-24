package main
import (
  "fmt"
  "os"
	"IP-TCP/pkg/lnxconfig"
  // "bufio"
  "IP-TCP/pkg/repl"
  // "net"
  "IP-TCP/pkg/ipstack"
  // "net/netip"
  "IP-TCP/pkg/rippacket"

  "IP-TCP/pkg/tcpstack"
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
  ip_stack, err := ipstack.InitializeStack(lnxConfig)
  if err != nil {
    panic(err)
  }

  //tcp
  tcp_stack, err := tcpstack.InitializeTCP(lnxConfig)
  if err != nil {
    panic(err)
  }
  
  //IP-TCP Handlers
  ip_stack.RegisterRecvHandler(0, ipstack.TestPacketHandler)
  ip_stack.RegisterRecvHandler(200, rippacket.RipPacketHandler)

  //TCP Stuff here

  //Receive IP
  for _, route := range ip_stack.ForwardingTable.Routes{
    if route.RoutingMode == 3 {
      go ipstack.ReceiveIP(route, ip_stack)
    }
  }

  go repl.StartRepl(ip_stack, "host")

  

  select{}

}



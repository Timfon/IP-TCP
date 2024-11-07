package main
import (
  "fmt"
  "os"
	"IP-TCP/pkg/lnxconfig"
  "IP-TCP/pkg/repl"
  "IP-TCP/pkg/rippacket"
  "IP-TCP/pkg/iptcpstack"
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

  //tcp
  tcp_stack, err := iptcpstack.InitializeTCP(lnxConfig)
  if err != nil {
    panic(err)
  }

  ip_stack, err := iptcpstack.InitializeStack(lnxConfig, tcp_stack)
  if err != nil {
    panic(err)
  }
  
  //IP-TCP Handlers
  ip_stack.RegisterRecvHandler(0, iptcpstack.TestPacketHandler)
  ip_stack.RegisterRecvHandler(200, rippacket.RipPacketHandler)
  ip_stack.RegisterRecvHandler(6, iptcpstack.TCPPacketHandler)
  //TCP Stuff here

  //Receive IP
  for _, route := range ip_stack.ForwardingTable.Routes{
    if route.RoutingMode == 3 {
      go iptcpstack.ReceiveIP(route, ip_stack, tcp_stack)
    }
  }

  go repl.StartRepl(ip_stack, tcp_stack, "host")
  select{}
}



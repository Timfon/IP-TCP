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
  "IP/pkg/rippacket"
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
  stack, err := ipstack.InitializeStack(lnxConfig)
  if err != nil {
    panic(err)
  }

  stack.RegisterRecvHandler(0, ipstack.TestPacketHandler)
  stack.RegisterRecvHandler(200, rippacket.RipPacketHandler)

  go repl.StartRepl(stack, "host")

  for _, route := range stack.ForwardingTable.Routes{
    if route.RoutingMode == 3 {
      go ipstack.ReceiveIP(route, stack)
    }
  }

  select{}

}



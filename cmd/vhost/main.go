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
  //sets everything up
  stack, err := ipstack.InitializeStack(lnxConfig)
  if err != nil {
    panic(err)
  }
  go repl.StartRepl(stack, "host")

  for _, route := range stack.ForwardingTable.Routes{
    if route.Iface != (ipstack.Interface{}){
      go ipstack.ReceiveIP(route, stack)
    }
  }

  select{}

}



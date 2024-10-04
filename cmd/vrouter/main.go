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

	  //sets everything up
	  stack, err := initializeStack(lnxConfig);
}
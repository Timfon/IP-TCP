package repl
import (
  "fmt"
  "os"
  "IP/pkg/lnxconfig"
  "bufio"
)

func StartRepl(lnxConfig *lnxconfig.IPConfig) {
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
    } else if input == "ln" {
      fmt.Println("Iface " + "VIP " + "UDPAddr")
      for _, neighbor := range lnxConfig.Neighbors {
        fmt.Println(neighbor.InterfaceName + " " + neighbor.DestAddr.String() + " " + neighbor.UDPAddr.String())
      }
    }
  }
}


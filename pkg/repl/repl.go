package repl
import (
  "fmt"
  "os"
  "bufio"
  "strings"
  "IP/pkg/ipstack"
  "net/netip"
)

func StartRepl(stack *ipstack.IPStack, hostOrRouter string) {
  reader := bufio.NewScanner(os.Stdin)
  for {
      fmt.Print("> ")
      if !reader.Scan() {
          break
      }
      input := reader.Text()
      if input == "li" {
          fmt.Println("Name Addr/Prefix State")
          for _, iface := range stack.Interfaces {
              fmt.Println(iface.Name + " " + iface.AssignedPrefix.String() + " " + "UP") // change UP later to have the actual state of interface
          }
      } else if input == "ln" {
          fmt.Println("Iface VIP UDPAddr")
          for _, neighbor := range stack.Neighbors {
              fmt.Println(neighbor.InterfaceName + " " + neighbor.DestAddr.String() + " " + neighbor.UDPAddr.String())
          }
      } else if strings.HasPrefix(input, "send") {
          parts := strings.SplitN(input, " ", 3)
          if len(parts) != 3 {
              fmt.Println("Usage: send <addr> <message>")
              continue
          }
          
          destAddr, err := netip.ParseAddr(parts[1])
          if err != nil {
              fmt.Printf("Invalid IP address: %v\n", err)
              continue
          }
          messageBytes := []byte(parts[2])
          if hostOrRouter == "host" {
              ipstack.SendIP(stack.Interfaces[0], destAddr, &stack.ForwardingTable, 0, messageBytes)
          } else if hostOrRouter == "router" {
              route, found, err := ipstack.MatchPrefix(&stack.ForwardingTable, destAddr)
              if err != nil {
                  fmt.Printf("Error matching route: %v\n", err)
                  continue
              }
              if !found {
                  fmt.Println("No matching route found for destination")
                  continue
              }
              var outgoingIface *ipstack.Interface
              for i := range stack.Interfaces {
                  if stack.Interfaces[i].AssignedIP == route.NextHop{
                      outgoingIface = &stack.Interfaces[i]
                      break
                  }
              }
              if outgoingIface == nil {
                  fmt.Printf("Could not find interface")
                  continue
              }
              ipstack.SendIP(*outgoingIface, destAddr, &stack.ForwardingTable, 0, messageBytes)
          }
      } else if input == "lr" {
        fmt.Println("T   Prefix   Next Hop   Cost")
        for _, route := range stack.ForwardingTable.Routes {
            fmt.Println("S   " + route.Prefix.String() + "   " + route.NextHop.String() + "   " + "1") //change cost later
        }
      }
  }
}


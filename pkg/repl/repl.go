package repl
import (
  "fmt"
  "text/tabwriter"
  "os"
  "bufio"
  "strings"
  "IP/pkg/ipstack"
  "net/netip"
  "IP/pkg/ipv4header"
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
        w := tabwriter.NewWriter(os.Stdout, 1, 1, 3, ' ', 0)
          fmt.Fprintln(w, "Name\tAddr/Prefix\tState")
          stack.ForwardingTable.Mu.Lock()    
          for _, route := range stack.ForwardingTable.Routes {
                var ud = "down"
                var iface = route.Iface
                if iface != (ipstack.Interface{}) {
                    if iface.UpOrDown {
                        ud = "up"
                    }
                fmt.Fprintln(w, iface.Name + "\t" + route.Prefix.String() + "\t" + ud) 
             
                }// change UP later to have the actual state of interface
          }
          stack.ForwardingTable.Mu.Unlock()
          w.Flush()
      } else if input == "ln" {
        w := tabwriter.NewWriter(os.Stdout, 1, 1, 3, ' ', 0)
          fmt.Fprintln(w, "Iface\tVIP\tUDPAddr")
          for _, neighbor := range stack.Neighbors {
              fmt.Fprintln(w, neighbor.InterfaceName + "\t" + neighbor.DestAddr.String() + "\t" + neighbor.UDPAddr.String())
          }
          w.Flush()
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

        table := stack.ForwardingTable
        route, found, _ := table.MatchPrefix(destAddr)
          if !found {
              fmt.Println("No matching prefix found")
              continue
          }

          var srcIP netip.Addr
          if route.RoutingMode == 4{
            srcIP = route.VirtualIP
          } else {
            iroute, _, _ := table.MatchPrefix(route.VirtualIP)
            srcIP = iroute.VirtualIP
          }

          hdr := ipv4header.IPv4Header{
            Version:  4,
            Len: 	20, // Header length is always 20 when no IP options
            TOS:      0,
            TotalLen: ipv4header.HeaderLen + len(messageBytes),
            ID:       0,
            Flags:    0,
            FragOff:  0,
            TTL:      32, // idk man
            Protocol: 0,
            Checksum: 0, // Should be 0 until checksum is computed
            Src:      srcIP, // double check
            Dst:      destAddr,
            Options:  []byte{},
        }
        ipstack.SendIP(stack, &hdr, messageBytes)

      } else if input == "lr" {
        w := tabwriter.NewWriter(os.Stdout, 1, 1, 3, ' ', 0)
        fmt.Fprintln(w, "T\tPrefix\tNext Hop\tCost")
        stack.ForwardingTable.Mu.Lock()
        
        for _, route := range stack.ForwardingTable.Routes {
            switch route.RoutingMode {
            case 1:
                fmt.Fprintln(w, "S\t" + route.Prefix.String() + "\t" + route.VirtualIP.String() + "\t" + fmt.Sprint(route.Cost))
            case 2:
                fmt.Fprintln(w, "R\t" + route.Prefix.String() + "\t" + route.VirtualIP.String() + "\t" + fmt.Sprint(route.Cost))
            case 3:
                fmt.Fprintln(w, "L\t" + route.Prefix.String() + "\tLOCAL:" + route.Iface.Name + "\t" + fmt.Sprint(route.Cost))
            }
            //change cost later
        }
        stack.ForwardingTable.Mu.Unlock()
        w.Flush()
      }
     
  }
}


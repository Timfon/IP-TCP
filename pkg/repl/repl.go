package repl

import (
	"IP-TCP/pkg/iptcpstack"
	"IP-TCP/pkg/ipv4header"
	"bufio"
	"fmt"
	"net/netip"
	"os"
	"strings"
  "strconv"
	"text/tabwriter"
)

func StartRepl(stack *iptcpstack.IPStack, tcpstack *iptcpstack.TCPStack, hostOrRouter string) {
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

				if route.RoutingMode == 3 {
					var iface = route.Iface
					if iface.UpOrDown {
						ud = "up"
					}

					p := netip.PrefixFrom(route.VirtualIP, route.Prefix.Bits())
					fmt.Fprintln(w, iface.Name+"\t"+p.String()+"\t"+ud)

				} // change UP later to have the actual state of interface
			}
			stack.ForwardingTable.Mu.Unlock()
			w.Flush()
		} else if input == "ln" {
			w := tabwriter.NewWriter(os.Stdout, 1, 1, 3, ' ', 0)
			fmt.Fprintln(w, "Iface\tVIP\tUDPAddr")
			for _, neighbor := range stack.Neighbors {
				if stack.Interfaces[neighbor.InterfaceName].UpOrDown {
					fmt.Fprintln(w, neighbor.InterfaceName+"\t"+neighbor.DestAddr.String()+"\t"+neighbor.UDPAddr.String())
				}
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
			if found == -1 {
				fmt.Println("No matching prefix found")
				continue
			}

			var srcIP netip.Addr
			if route.RoutingMode == 4 {
				srcIP = route.VirtualIP
			} else {
				iroute, _, _ := table.MatchPrefix(route.VirtualIP)
				srcIP = iroute.VirtualIP
			}

			hdr := ipv4header.IPv4Header{
				Version:  4,
				Len:      20, // Header length is always 20 when no IP options
				TOS:      0,
				TotalLen: ipv4header.HeaderLen + len(messageBytes),
				ID:       0,
				Flags:    0,
				FragOff:  0,
				TTL:      32, // idk man
				Protocol: 0,
				Checksum: 0,     // Should be 0 until checksum is computed
				Src:      srcIP, // double check
				Dst:      destAddr,
				Options:  []byte{},
			}
			iptcpstack.SendIP(stack, &hdr, messageBytes)

		} else if input == "lr" {
			w := tabwriter.NewWriter(os.Stdout, 1, 1, 3, ' ', 0)
			fmt.Fprintln(w, "T\tPrefix\tNext Hop\tCost")
			stack.ForwardingTable.Mu.Lock()

			for _, route := range stack.ForwardingTable.Routes {
				switch route.RoutingMode {
				case 1:
					fmt.Fprintln(w, "S\t"+route.Prefix.String()+"\t"+route.VirtualIP.String()+"\t"+fmt.Sprint("-"))
				case 2:
					if route.Cost < 16 {
						fmt.Fprintln(w, "R\t"+route.Prefix.String()+"\t"+route.VirtualIP.String()+"\t"+fmt.Sprint(route.Cost))
					}
				case 3:
					fmt.Fprintln(w, "L\t"+route.Prefix.String()+"\tLOCAL:"+route.Iface.Name+"\t"+fmt.Sprint(route.Cost))
				}
				//change cost later
			}
			stack.ForwardingTable.Mu.Unlock()
			w.Flush()
		} else if strings.HasPrefix(input, "up") {
			parts := strings.SplitN(input, " ", 2)
			if len(parts) != 2 {
				fmt.Println("Usage: up <ifname>")
				continue
			}

			ifname := parts[1]

			if value, ok := stack.Interfaces[ifname]; ok {
				value.UpOrDown = true
				stack.Interfaces[ifname] = value
			}

		} else if strings.HasPrefix(input, "down") {
			parts := strings.SplitN(input, " ", 2)
			if len(parts) != 2 {
				fmt.Println("Usage: down <ifname>")
				continue
			}

			ifname := parts[1]

			if value, ok := stack.Interfaces[ifname]; ok {
				value.UpOrDown = false
				stack.Interfaces[ifname] = value
			}
		} else if strings.HasPrefix(input, "c") {
			parts := strings.SplitN(input, " ", 3)
			if len(parts) != 3 {
				fmt.Println("Usage: c <vip> <port>")
				continue
			}
			vip, err := netip.ParseAddr(parts[1])
			if err != nil {
				fmt.Printf("Invalid IP address: %v\n", err)
				continue
			}
      port, err := strconv.Atoi(parts[2])
      if err != nil {
        fmt.Printf("Invalid port number: %v\n", err)
        continue
      }
      _, err = tcpstack.VConnect(vip, uint16(port), stack)
      if err != nil {
        fmt.Println(err)
        continue
      }
      fmt.Println(tcpstack.Listeners)
		} else if strings.HasPrefix(input, "a") {
      parts := strings.SplitN(input, " ", 2)
      if len(parts) != 2 {
        fmt.Println("Usage: a <port>")
        continue
      }
      port, err := strconv.Atoi(parts[1])
      if err != nil {
        fmt.Printf("Invalid port number: %v\n", err)
        continue
      }
      iptcpstack.ACommand(uint16(port), tcpstack)
      fmt.Println(tcpstack.Listeners)
    } else if strings.HasPrefix(input, "sf") {
    parts := strings.SplitN(input, " ", 4)
    if len(parts) != 4 {
      fmt.Println("Usage: sf <file path> <addr> <port>")
      continue
    }
    filepath := parts[1]
    destAddr, err := netip.ParseAddr(parts[2])
    if err != nil {
      fmt.Printf("Invalid IP address: %v\n", err)
      continue
    }
    port, err := strconv.Atoi(parts[3])
    if err != nil {
      fmt.Printf("Invalid port number: %v\n", err)
      continue
    }
    numBytes, err := iptcpstack.SendFile(stack, filepath, destAddr, uint16(port), tcpstack)
    if err != nil {
      fmt.Println(err)
      continue
    }
    fmt.Printf("Sent %d bytes\n", numBytes)
  } else if strings.HasPrefix(input, "ls") {
      w := tabwriter.NewWriter(os.Stdout, 1, 1, 3, ' ', 0)
      fmt.Fprintln(w, "SID\tLAddr\tLPort\tRAddr\tRPort\tStatus")
      for _, sock := range tcpstack.Sockets {
          var sockState, laddr, raddr string
          var lport, rport uint16
          if sock.Conn == nil {
              // This is a listening socket
              sockState = "LISTEN"
              laddr = "0.0.0.0"    // or whatever your default listening address is
              lport = sock.Listen.LocalPort
              raddr = "*"
              rport = 0
          } else {
              // This is a connected socket
              switch sock.Conn.State {
              case 0:
                  sockState = "LISTEN"
              case 1:
                  sockState = "SYN-SENT"
              case 2:
                  sockState = "SYN-RECEIVED"
              case 3:
                  sockState = "ESTABLISHED"
              }
              laddr = sock.Conn.LocalAddr.String()
              lport = sock.Conn.LocalPort
              raddr = sock.Conn.RemoteAddr.String()
              rport = sock.Conn.RemotePort
          }

          fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%d\t%s\n", 
              sock.SID, 
              laddr, 
              lport, 
              raddr, 
              rport, 
              sockState)
      }
      w.Flush()
    } else if strings.HasPrefix(input, "s") {
      parts := strings.SplitN(input, " ", 3)
      if len(parts) != 3 {
          fmt.Println("Usage: s <sid> <message>")
          continue
      }
      sid, err := strconv.Atoi(parts[1])
      if err != nil {
          fmt.Printf("Invalid socket ID: %v\n", err)
          continue
      }
      messageBytes := []byte(parts[2])
      fmt.Println(int(sid))

      //this is so cursed wtf is this
	  conn := tcpstack.Sockets[int(sid)].Conn
	  if conn == nil {
		  continue
	  }
      bytesWritten, err := conn.VWrite(messageBytes, stack, tcpstack.Sockets[int(sid)])
      if err != nil {
        fmt.Println(err)
        continue
      }
      fmt.Printf("Wrote %d bytes\n", bytesWritten)
    } else if strings.HasPrefix(input, "rf") {
      parts := strings.SplitN(input, " ", 3)
      if len(parts) != 3 {
        fmt.Println("Usage: rf <dest_file> <port>")
        continue
      }
      destFile := parts[1]
      port, err := strconv.Atoi(parts[2])
      if err != nil {
        fmt.Printf("Invalid port number: %v\n", err)
        continue
      }
      go func() {
        numBytes, err := iptcpstack.ReceiveFile(stack, destFile, uint16(port), tcpstack)
        if err != nil {
          fmt.Println(err)
          return
        }
        fmt.Printf("Received %d bytes\n", numBytes)
      }()
      fmt.Println("File reception started in background")
  } else if strings.HasPrefix(input, "r") {
      parts := strings.SplitN(input, " ", 3)
      if len(parts) != 3 {
        fmt.Println("Usage: r <sid> <numBytes>")
        continue
      }
      sid, err := strconv.Atoi(parts[1])
      if err != nil {
        fmt.Printf("Invalid socket ID: %v\n", err)
        continue
      }
      numBytes, err := strconv.Atoi(parts[2])
      if err != nil {
        fmt.Printf("Invalid number of bytes: %v\n", err)
        continue
      }
      buf := make([]byte, numBytes)
      _, err = tcpstack.Sockets[int(sid)].Conn.VRead(buf)
      if err != nil {
        fmt.Println(err)
        continue
      }
      fmt.Println("Read", numBytes, "bytes: ", string(buf))
    } else if strings.HasPrefix(input, "v") {
		parts := strings.SplitN(input, " ", 2)
		if len(parts) != 2 {
			fmt.Println("Usage: sock <sid>")
			continue
		}
		sid, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Printf("Invalid socket ID: %v\n", err)
			continue
		}
		fmt.Println(tcpstack.Sockets[int(sid)].Conn)
		if tcpstack.Sockets[int(sid)].Conn != nil{
		fmt.Println(tcpstack.Sockets[int(sid)].Conn.Window)
		}
	}   
	}
}


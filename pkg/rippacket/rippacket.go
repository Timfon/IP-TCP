package rippacket

import (
	"fmt"
)
type RIPEntry struct {
    Cost    uint32
    Address uint32
    Mask    uint32
}

type RIPMessage struct {
    Command    uint16
    NumEntries uint16
    Entries    []RIPEntry
}

func RipPacketHandler(packet *Packet, args []interface{}){
	fmt.Println("rip done!")
}

//get udp address from neighbors
//


package iptcpstack

import (
	"IP-TCP/pkg/iptcp_utils"
	"IP-TCP/pkg/ipv4header"
	"IP-TCP/pkg/lnxconfig"
	"container/heap"
	"fmt"
	"math/rand"
	"net/netip"
	"sync"
	"time"

	"github.com/google/netstack/tcpip/header"
)

type TCPStack struct {
	TcpRtoMin    time.Duration
	TcpRtoMax    time.Duration
	Sockets      map[int]*Socket
	Mu           sync.Mutex
	NextSocketID int
}

func InitializeTCP(config *lnxconfig.IPConfig) (*TCPStack, error) {
	TcpStack := &TCPStack{
		TcpRtoMin:    config.TcpRtoMin,
		TcpRtoMax:    config.TcpRtoMax,
		Sockets:      make(map[int]*Socket),
		NextSocketID: 0,
	}
	return TcpStack, nil
}

func (stack *TCPStack) FindSocket(localAddr netip.Addr, localPort uint16, remoteAddr netip.Addr, remotePort uint16) *Socket {
	// First check for exact connection match
	stack.Mu.Lock()
	defer stack.Mu.Unlock()
	for _, sock := range stack.Sockets {
		if sock.Conn != nil &&
			sock.Conn.LocalPort == localPort &&
			sock.Conn.RemotePort == remotePort {
			return sock
		}
	}

	// If no connection found, check listening sockets
	for _, sock := range stack.Sockets {
		if sock.Listen != nil && sock.Listen.LocalPort == localPort {
			return sock
		}
	}

	return nil
}

func TCPPacketHandler(packet *Packet, args []interface{}) {
	stack := args[0].(*IPStack)
	tcpStack := args[1].(*TCPStack)
	hdr := packet.Header
	tcpHdr := iptcp_utils.ParseTCPHeader(packet.Body)

	tcpPayload := packet.Body[tcpHdr.DataOffset:]
	tcpChecksumFromHeader := tcpHdr.Checksum
	tcpHdr.Checksum = 0
	//fmt.Println(tcpHdr, hdr.Src, hdr.Dst, tcpPayload)
	tcpComputedChecksum := iptcp_utils.ComputeTCPChecksum(&tcpHdr, hdr.Src, hdr.Dst, tcpPayload)
	if tcpChecksumFromHeader != tcpComputedChecksum {
		fmt.Println("TCP Checksum mismatch, dropping packet")
		fmt.Print("> ")
		return
	}

	sock := tcpStack.FindSocket(hdr.Dst, tcpHdr.DstPort, hdr.Src, tcpHdr.SrcPort)
	if sock == nil {
		fmt.Println("No matching socket found, dropping packet")
		fmt.Print("> ")
		return
	}
	// Handle listening socket case
	if sock.Listen != nil {
		if tcpHdr.Flags&header.TCPFlagSyn != 0 {
			handleSynReceived(sock, packet, tcpHdr, stack, tcpStack)
		}
		return
	}

	// Only proceed if we have a connection
	if sock.Conn == nil {
		fmt.Println("Socket has no connection, dropping packet")
		return
	}

	// Now safe to check connection state
	switch sock.Conn.State {
	case 1:
		if tcpHdr.Flags&header.TCPFlagSyn != 0 && tcpHdr.Flags&header.TCPFlagAck != 0 {
			handleSynAckReceived(sock, packet, tcpHdr, stack, tcpStack)
		}
	case 2:
		if tcpHdr.Flags&header.TCPFlagAck != 0 {
			handleAckReceived(sock, packet, tcpHdr, stack, tcpStack)
		}
	case 3:
		if tcpHdr.Flags&header.TCPFlagAck != 0 {
			handleEstablished(sock, packet, tcpHdr, stack)
		}
		if tcpHdr.Flags&header.TCPFlagFin != 0 {
			finWaitTearDown(sock, packet, tcpHdr, stack)
		}
	case 4:
		if tcpHdr.Flags&header.TCPFlagAck != 0 {
			handleFinWait1(sock, packet, tcpHdr, stack)
		}
	case 5:
		if tcpHdr.Flags&header.TCPFlagAck != 0 {
			handleFinWait2(sock, packet, tcpHdr, stack)
		}
		if tcpHdr.Flags&header.TCPFlagFin != 0 {
			finWaitTearDown(sock, packet, tcpHdr, stack)
		}
	case 6:
		handleCloseWait(sock, packet, tcpHdr, stack)
	case 7:
		if tcpHdr.Flags&header.TCPFlagFin != 0 {
			handleTimeWait(sock, tcpStack)
		}
	case 8:
		if tcpHdr.Flags&header.TCPFlagAck != 0 {
			recvTearDown(sock, packet, tcpHdr, stack, tcpStack)
		}
	}
}

func handleSynReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack, tcpstack *TCPStack) error {
	//add new socket to socket table
	l := sock.Listen
	seqNum := rand.Uint32() >> 16
	new_Connection := &VTCPConn{
		State:      2,
		LocalAddr:  packet.Header.Dst,
		LocalPort:  l.LocalPort,
		RemoteAddr: packet.Header.Src,
		RemotePort: tcpHdr.SrcPort,
		SeqNum:     seqNum,
		AckNum:     tcpHdr.SeqNum + 1,
		Window:     NewWindow(65535),
		SID:        tcpstack.NextSocketID,
	}

	newq := make(PriorityQueue, 0)
	heap.Init(&newq)

	new_Connection.ReceiveQueue = &newq

	// Initialize window tracking - for first data packet, we expect the original sequence number
	new_Connection.Window.SendNxt = new_Connection.SeqNum
	new_Connection.Window.RecvNext = new_Connection.AckNum
	new_Connection.Window.RecvLBR = tcpHdr.SeqNum
	new_Connection.Window.ReadWindowSize = uint32(tcpHdr.WindowSize)

	newSocket := Socket{
		SID:  tcpstack.NextSocketID,
		Conn: new_Connection,
	}
	tcpstack.Sockets[newSocket.SID] = &newSocket
	tcpstack.NextSocketID++
	// Send the packet
	fmt.Println("SYN received, sending SYN-ACK")
	err := stack.sendTCPPacket(&newSocket, []byte{}, header.TCPFlagAck|header.TCPFlagSyn)
	if err != nil {
		return fmt.Errorf("failed to send SYN-ACK packet: %v", err)
	}

	return nil
}

// Also modify handleSynAckReceived to initialize RecvNext
func handleSynAckReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack, tcpstack *TCPStack) error {
	sock.Conn.State = 3
	sock.Conn.SeqNum = tcpHdr.AckNum
	sock.Conn.Window.ReadWindowSize = uint32(tcpHdr.WindowSize)
	sock.Conn.Window.SendNxt = tcpHdr.AckNum
	fmt.Printf("test %v\n",  sock.Conn.Window.ReadWindowSize)
	sock.Conn.Window.SendLBW = sock.Conn.Window.SendNxt + sock.Conn.Window.ReadWindowSize
	sock.Conn.Window.SendUna = tcpHdr.AckNum
	sock.Conn.AckNum = tcpHdr.SeqNum + 1
	sock.Conn.Window.RecvNext = sock.Conn.AckNum // Add this line
	sock.Conn.Window.RecvLBR = tcpHdr.SeqNum + 1

	
	// fmt.Printf("Debug - Initialized RecvNext to %d\n", sock.Conn.Window.RecvNext)
	fmt.Println("SYN-ACK received, connection established")
	// Send ACK
	err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
	if err != nil {
		return fmt.Errorf("failed to send ACK packet: %v", err)
	}

	sock.Conn.Window.RetransmissionQueue = NewRetransmissionQueue()
	go sock.Conn.HandleRetransmission(stack, sock, tcpstack)
	return nil
}

func handleAckReceived(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack, tcpstack *TCPStack) {
	fmt.Println("ACK received, connection established")
	sock.Conn.State = 3
	sock.Conn.Window.RecvNext = tcpHdr.SeqNum
	sock.Conn.Window.RecvLBR = tcpHdr.SeqNum
	sock.Conn.Window.SendNxt = tcpHdr.AckNum
	sock.Conn.Window.SendLBW = sock.Conn.Window.SendNxt + uint32(tcpHdr.WindowSize)
	sock.Conn.SeqNum = tcpHdr.AckNum
	sock.Conn.Window.SendUna = tcpHdr.AckNum
	// Find the listening socket that created this connection
	for _, s := range tcpstack.Sockets {
		if s.Listen != nil && s.Listen.LocalPort == sock.Conn.LocalPort {
			// Place the established connection in the accept queue
			s.Listen.AcceptQueue <- sock.Conn
			break
		}
	}

	sock.Conn.Window.RetransmissionQueue = NewRetransmissionQueue()
	go sock.Conn.HandleRetransmission(stack, sock, tcpstack)
}

func handleEstablished(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) {
	payloadOffset := int(tcpHdr.DataOffset)
	payload := packet.Body[payloadOffset:]

	sock.Conn.SeqNum = tcpHdr.AckNum
	if len(payload) > 0 {
		// Write to receive buffer
		if sock.Conn.Window.RecvNext == tcpHdr.SeqNum {
			err := processInOrderPacket(sock, payload, stack)
			if err != nil {
				fmt.Printf("Error sending ACK: %v\n", err)

			}
			processBufferedPackets(sock, stack)
		} else if tcpHdr.SeqNum == sock.Conn.Window.RecvNext-1 {
			already_read := sock.Conn.Window.RecvNext - tcpHdr.SeqNum
			payload = payload[already_read:]
			if len(payload) > 0 {
				err := processInOrderPacket(sock, payload, stack)
				if err != nil {
					fmt.Printf("Error sending ACK: %v\n", err)

				}
				processBufferedPackets(sock, stack)
			}
		} else if tcpHdr.SeqNum > sock.Conn.Window.RecvNext {
			fmt.Printf("Early Arrival packet - adding to queue, expected %v, got %v\n", sock.Conn.Window.RecvNext, tcpHdr.SeqNum)
			entry := &Item{
				value:    payload,
				priority: tcpHdr.SeqNum,
			}
			heap.Push(sock.Conn.ReceiveQueue, entry)

			stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
		}
		/* */
	} else {
		//fmt.Println("Windowsize tcp: ", tcpHdr.WindowSize)
		//fmt.Println("SendBuffer size: ", sock.Conn.Window.sendBuffer.Free())
		sock.Conn.Window.ReadWindowSize = uint32(tcpHdr.WindowSize)
		if tcpHdr.AckNum > sock.Conn.Window.SendUna {
			bytesAcked := tcpHdr.AckNum - sock.Conn.Window.SendUna
			// Actually remove the acknowledged data from send buffer
			discardBuf := make([]byte, bytesAcked)
			_, err := sock.Conn.Window.sendBuffer.Read(discardBuf)
			if err != nil {
				fmt.Printf("Error removing acked data from send buffer: %v\n", err)
			} else {
				//fmt.Printf("Removed %d acked bytes from send buffer\n", n)
			}
			// Update SendUna after successful removal
			sock.Conn.Window.SendUna = tcpHdr.AckNum
			sock.Conn.Window.RetransmissionQueue.RemoveAckedEntries(tcpHdr.AckNum)
			sock.Conn.Window.SendLBW = sock.Conn.Window.SendUna + uint32(tcpHdr.WindowSize)
		}
	}
}

// Helper function to process in-order packets
func processInOrderPacket(sock *Socket, payload []byte, stack *IPStack) error {
	// Write the payload to receive buffer

	written, err := sock.Conn.Window.recvBuffer.Write(payload)
	if err != nil {
		return fmt.Errorf("error writing to receive buffer: %v", err)
	}
	// Update sequence numbers

	sock.Conn.Window.RecvNext += uint32(written)
	sock.Conn.AckNum = sock.Conn.Window.RecvNext

	// Update receive window size
	availSpace := sock.Conn.Window.recvBuffer.Free()
	sock.Conn.Window.RecvWindowSize = uint32(availSpace)

	// Signal data availability - one signal per chunk of data
	dataChunks := (written + 1023) / 1024 // Signal for every 1KB of data

	for i := 0; i < dataChunks; i++ {
		select {
		case sock.Conn.Window.DataAvailable <- struct{}{}:
			//fmt.Printf("Sent data signal %d/%d for %d bytes\n", i+1, dataChunks, written)
		default:
			// Channel is full, which is fine - reader will get to it
		}
	}
	// Send ACK with updated window size
	return stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
}

// Helper function to process buffered out-of-order packets
func processBufferedPackets(sock *Socket, stack *IPStack) error {
	if sock.Conn.ReceiveQueue == nil || sock.Conn.ReceiveQueue.Len() == 0 {
		return nil
	}
	for sock.Conn.ReceiveQueue.Len() > 0 {
		item := heap.Pop(sock.Conn.ReceiveQueue).(*Item) // Peek at next packet

		// Check if this is the next expected packet
		if item.priority == sock.Conn.Window.RecvNext {
			// Remove and process the packet
			written, err := sock.Conn.Window.recvBuffer.Write(item.value)
			if err != nil {
				return fmt.Errorf("error writing to receive buffer: %v", err)
			}
			// Update sequence numbers

			sock.Conn.Window.RecvNext += uint32(written)
			sock.Conn.AckNum = sock.Conn.Window.RecvNext

			// Update receive window size
			availSpace := sock.Conn.Window.recvBuffer.Free()
			sock.Conn.Window.RecvWindowSize = uint32(availSpace)

		} else if item.priority == sock.Conn.Window.RecvNext-1 {
			already_read := sock.Conn.Window.RecvNext - item.priority
			payload := item.value[already_read:]
			if len(payload) > 0 {

				written, err := sock.Conn.Window.recvBuffer.Write(payload)
				if err != nil {
					return fmt.Errorf("error writing to receive buffer: %v", err)
				}
				// Update sequence numbers

				sock.Conn.Window.RecvNext += uint32(written)
				sock.Conn.AckNum = sock.Conn.Window.RecvNext

				// Update receive window size
				availSpace := sock.Conn.Window.recvBuffer.Free()
				sock.Conn.Window.RecvWindowSize = uint32(availSpace)

			}

		} else if item.priority > sock.Conn.Window.RecvNext {
			heap.Push(sock.Conn.ReceiveQueue, item)
			break
		}
	}
	return stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
}

func handleFinWait1(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) {
	if tcpHdr.Flags&header.TCPFlagAck != 0 {
		sock.Conn.State = FinWait2
		sock.Conn.SeqNum = tcpHdr.AckNum
		sock.Conn.AckNum = tcpHdr.SeqNum + 1
		sock.Conn.Window.RecvNext = sock.Conn.AckNum // Add this line
		sock.Conn.Window.RecvLBR = tcpHdr.SeqNum + 1
		fmt.Println("Received ACK for our FIN, moving to FIN_WAIT_2")
	}
	if tcpHdr.Flags&header.TCPFlagFin != 0 {
		sock.Conn.State = TimeWait
		sock.Conn.SeqNum = tcpHdr.AckNum
		sock.Conn.AckNum = tcpHdr.SeqNum + 1

		err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
		if err != nil {
			fmt.Printf("Error sending Ack in FIN_WAIT_1: %v\n", err)
			return
		}
		fmt.Println("Received FIN-ACK, moving to TIME_WAIT")

		go handleTimeWait(sock, stack.TcpStack)
	}
}

func handleFinWait2(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) {
	//basically just accepts ack messages here
	if tcpHdr.Flags&header.TCPFlagFin != 0 {
		// Move to TIME_WAIT
		sock.Conn.State = TimeWait
		sock.Conn.SeqNum = tcpHdr.AckNum
		sock.Conn.AckNum = tcpHdr.SeqNum + 1

		// Send ACK for the FIN
		err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
		if err != nil {
			fmt.Printf("Error sending ACK in FIN_WAIT_2: %v\n", err)
			return
		}
		fmt.Println("Received FIN, moving to TIME_WAIT")
		// Start TIME_WAIT timer
		go handleTimeWait(sock, stack.TcpStack)
	}

}

func finWaitTearDown(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) error {
	//takes a fin and sends an ACK
	if sock.Conn.State == Established {
		// Move to CLOSE_WAIT
		sock.Conn.State = CloseWait
		sock.Conn.SeqNum = tcpHdr.AckNum
		sock.Conn.AckNum = tcpHdr.SeqNum + 1
		sock.Conn.Window.RecvNext = sock.Conn.AckNum
		sock.Conn.Window.RecvLBR = tcpHdr.SeqNum + 1

		// First send ACK for the received FIN
		err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
		if err != nil {
			return fmt.Errorf("failed to send ACK packet: %v", err)
		}

		// Immediately send our FIN and move to LAST_ACK
		err = stack.sendTCPPacket(sock, []byte{}, header.TCPFlagFin)
		if err != nil {
			return fmt.Errorf("failed to send FIN packet: %v", err)
		}

		sock.Conn.State = LastAck
		fmt.Println("Received FIN, sent ACK+FIN, moving to LAST_ACK")

	} else {
		// Handle FIN in FIN_WAIT_2 state
		sock.Conn.State = TimeWait
		sock.Conn.SeqNum = tcpHdr.AckNum
		sock.Conn.AckNum = tcpHdr.SeqNum + 1
		sock.Conn.Window.RecvNext = sock.Conn.AckNum
		sock.Conn.Window.RecvLBR = tcpHdr.SeqNum + 1

		// Send ACK for the FIN
		err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagAck)
		if err != nil {
			return fmt.Errorf("failed to send ACK packet: %v", err)
		}
		fmt.Println("Received FIN in FIN_WAIT_2, moving to TIME_WAIT")

		// Start TIME_WAIT timer
		go handleTimeWait(sock, stack.TcpStack)
	}

	return nil
}

func handleTimeWait(sock *Socket, tcpStack *TCPStack) {
	timer := time.NewTimer(30 * time.Second) // Using 30 seconds instead of 4 minutes for testing
	<-timer.C

	// After timeout, mark as closed and cleanup
	if sock.Conn != nil && sock.Conn.State == TimeWait {
		fmt.Println("TIME_WAIT completed, connection CLOSED")
		sock.Conn.State = 0 // CLOSED

		// Find and delete the socket from TCPStack
		delete(tcpStack.Sockets, sock.SID)
	}
}

func handleCloseWait(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack) {
	// Update sequence numbers
	sock.Conn.SeqNum = tcpHdr.AckNum
	sock.Conn.Window.RecvNext = sock.Conn.AckNum
	// After receiving ACK in CLOSE_WAIT, we should send our own FIN
	// This transitions us to LAST_ACK state
	err := stack.sendTCPPacket(sock, []byte{}, header.TCPFlagFin)
	if err != nil {
		fmt.Printf("Error sending FIN in CLOSE_WAIT: %v\n", err)
		return
	}

	// Move to LAST_ACK state
	sock.Conn.State = LastAck
	fmt.Println("Sent FIN, moving to LAST_ACK state")
}

func recvTearDown(sock *Socket, packet *Packet, tcpHdr header.TCPFields, stack *IPStack, tcpStack *TCPStack) {
	fmt.Println("Final ACK received in LAST_ACK, deleting connection")

	tcpStack.Mu.Lock()
	delete(tcpStack.Sockets, sock.SID)
	tcpStack.Mu.Unlock()
}

func (stack *IPStack) sendTCPPacket(sock *Socket, data []byte, flags uint8, seqNum ...uint32) error {
	var tcpHdr header.TCPFields
	var localAddr, remoteAddr netip.Addr
	if sock.Conn == nil {
		return fmt.Errorf("can't send via listen socket")
	}
	// Calculate actual free space in receive buffer
	//fmt.Printf("Data %+v\n", data)
	// Normal connected socket - use Conn field

	tcpHdr = header.TCPFields{
		SrcPort:    sock.Conn.LocalPort,
		DstPort:    sock.Conn.RemotePort,
		SeqNum:     sock.Conn.Window.SendNxt,
		AckNum:     sock.Conn.Window.RecvNext,
		DataOffset: 20,
		Flags:      flags,
		WindowSize: (uint16)(sock.Conn.Window.recvBuffer.Free()), // Default window size
	}

	if len(seqNum) > 0 {
		tcpHdr.SeqNum = seqNum[0]
	}

	localAddr = sock.Conn.LocalAddr
	remoteAddr = sock.Conn.RemoteAddr

	// Create TCP header bytes and compute checksum
	checksum := iptcp_utils.ComputeTCPChecksum(&tcpHdr, localAddr, remoteAddr, data)
	tcpHdr.Checksum = checksum
	tcpHeaderBytes := make(header.TCP, iptcp_utils.TcpHeaderLen)
	tcp := header.TCP(tcpHeaderBytes)
	tcp.Encode(&tcpHdr)

	ipBytes := append(tcp, data...)

	hdr := ipv4header.IPv4Header{
		Version:  4,
		Len:      20,
		TOS:      0,
		TotalLen: ipv4header.HeaderLen + len(ipBytes),
		ID:       0,
		Flags:    0,
		FragOff:  0,
		TTL:      32,
		Protocol: 6,
		Checksum: 0,
		Src:      localAddr,
		Dst:      remoteAddr,
		Options:  []byte{},
	}
	return SendIP(stack, &hdr, ipBytes)
}

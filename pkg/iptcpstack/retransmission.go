package iptcpstack
import (
	"fmt"
	"sync"
	"time"
	"github.com/google/netstack/tcpip/header"
)

const (
	maxRetries   = 5
	retryTimeout = 3 * time.Second
)

type RetransmissionEntry struct {
	Data     []byte
	SeqNum   uint32
	SendTime time.Time
	Retries  uint32
	RTO     time.Duration
}

type RetransmissionQueue struct {
	Entries []*RetransmissionEntry
	mutex   sync.Mutex
	SRTT    time.Duration // Smoothed RTT
	alpha   float64       // Smoothing factor (typically 0.875)
	beta    float64       // RTO multiplier (typically 2.0)
	RTOMin  time.Duration // Minimum allowed RTO
	RTOMax  time.Duration // Maximum allowed RTO
	RTO time.Duration
}

// Initialize the retransmission queue
func NewRetransmissionQueue() *RetransmissionQueue {
	return &RetransmissionQueue{
		mutex:   sync.Mutex{},
		Entries: make([]*RetransmissionEntry, 0),
		SRTT:    1 * time.Second, // Initial SRTT guess
		alpha:   0.875,           // RFC793 recommended value (1 - 0.125)
		beta:    2.0,             // RTO multiplier
		RTOMin:  1 * time.Millisecond,
		RTOMax:  60 * time.Second,
		RTO:     250 * time.Millisecond, // Initial RTO guess
	}
}

// RFC793 RTT calculation
func (rq *RetransmissionQueue) updateRTT(measuredRTT time.Duration) {
	// SRTT = (α * SRTTLast) + (1 - α) * RTTMeasured
	rq.SRTT = time.Duration(float64(rq.SRTT)*rq.alpha +
		float64(measuredRTT)*(1-rq.alpha))
	// Calculate new RTO
	// RTO = max(RTOMin, min(β * SRTT, RTOMax))
	rq.RTO = time.Duration(float64(rq.SRTT) * rq.beta)

	if rq.RTO < rq.RTOMin {
		rq.RTO = rq.RTOMin
	}
	if rq.RTO > rq.RTOMax {
		rq.RTO = rq.RTOMax
	}
}

func (c *VTCPConn) handleZeroWindow(stack *IPStack, sock *Socket) error {
	fmt.Println("ZERO WINDOW CONDITION DETECTED")
	probeInterval := 1 * time.Second // Start with 1 second
	
	probe := []byte{0} // Just send a zero byte as probe
	peekBuf := make([]byte, 1)
	if n, err := c.Window.sendBuffer.Read(peekBuf); n == 1 && err == nil {
		// If we successfully read a byte, use it as probe and put it back
		probe[0] = peekBuf[0]
		c.Window.sendBuffer.Write(peekBuf)
	}
	for { // Limit max retries
		// Send 1-byte probe
		// Try to peek at first byte in send buffer if available
		err := stack.sendTCPPacket(sock, probe, header.TCPFlagAck)
		if err != nil {
			return fmt.Errorf("failed to send zero window probe: %v", err)
		}
		// Wait for response with exponential backoff
		time.Sleep(probeInterval)
		// Check if window has opened
		fmt.Println("Checking window size:", c.Window.ReadWindowSize)
		if c.Window.ReadWindowSize > 0 {
			return nil
		}
		// Exponential backoff for probe interval
	}
	return fmt.Errorf("zero window condition persisted after max retries")
}

func (c *VTCPConn) HandleRetransmission(stack *IPStack, sock *Socket, tcpStack *TCPStack) error {
    ticker := time.NewTicker(time.Millisecond * 100)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            if c.State != Established {
                return nil
            }

            c.Window.RetransmissionQueue.mutex.Lock()
            now := time.Now()

            newEntries := make([]*RetransmissionEntry, 0)
            for _, entry := range c.Window.RetransmissionQueue.Entries {
                // Check if packet is already fully acknowledged
                if entry.SeqNum+uint32(len(entry.Data)) <= c.Window.SendUna {
                    continue // Skip acknowledged packets
                }

                // Keep unacknowledged packets in the queue
                newEntries = append(newEntries, entry)

                timeSinceLastSend := now.Sub(entry.SendTime)
                if timeSinceLastSend >= entry.RTO {
                    // Double check it's still not acknowledged
                    if entry.SeqNum+uint32(len(entry.Data)) > c.Window.SendUna {
                        if entry.Retries >= maxRetries {
                            c.Window.RetransmissionQueue.mutex.Unlock()
                            tcpStack.Mu.Lock()
                            delete(tcpStack.Sockets, sock.SID)
                            tcpStack.Mu.Unlock()
                            return fmt.Errorf("connection timeout after %d retries", maxRetries)
                        }

                        fmt.Printf("Retransmitting unacked packet (Seq: %d, Retry: %d/%d, Last Acked: %d)\n",
                            entry.SeqNum, entry.Retries+1, maxRetries, c.Window.SendUna)

                        err := stack.sendTCPPacket(sock, entry.Data, header.TCPFlagAck, entry.SeqNum)
                        if err != nil {
                            c.Window.RetransmissionQueue.mutex.Unlock()
                            return fmt.Errorf("failed to retransmit packet: %v", err)
                        }

                        entry.Retries++
                        entry.SendTime = now
                        entry.RTO *= 2
                        if entry.RTO > c.Window.RetransmissionQueue.RTOMax {
                            entry.RTO = c.Window.RetransmissionQueue.RTOMax
                        }

                        time.Sleep(10 * time.Millisecond)
                    }
                }
            }

            // Update queue to only contain unacknowledged packets
            c.Window.RetransmissionQueue.Entries = newEntries
            c.Window.RetransmissionQueue.mutex.Unlock()
        }
    }
}

func (rq *RetransmissionQueue) AddEntry(data []byte, seqNum uint32) {
	rq.mutex.Lock()
	defer rq.mutex.Unlock()

	entry := &RetransmissionEntry{
		Data:     make([]byte, len(data)), // Make a copy of the data
		SeqNum:   seqNum,
		SendTime: time.Now(),
		RTO: rq.RTO,
	}
	copy(entry.Data, data)
	rq.Entries = append(rq.Entries, entry)
}

func (rq *RetransmissionQueue) RemoveAckedEntries(ackNum uint32) {
	rq.mutex.Lock()
	defer rq.mutex.Unlock()

	newEntries := make([]*RetransmissionEntry, 0)
	for _, entry := range rq.Entries {
		if entry.SeqNum+uint32(len(entry.Data)) > ackNum {
			newEntries = append(newEntries, entry)
		} else if entry.SeqNum+uint32(len(entry.Data)) == ackNum {
			if entry.Retries == 0 {
				// Only update RTT for packets that weren't retransmitted
				now := time.Now()
				measuredRTT := now.Sub(entry.SendTime)
				rq.updateRTT(measuredRTT)
			}
		}
	}
	rq.Entries = newEntries
}

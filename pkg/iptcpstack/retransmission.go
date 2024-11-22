package iptcpstack

import (
	"fmt"
	"time"
	"github.com/google/netstack/tcpip/header"
  "sync"
)

const (
	maxRetries    = 3
	retryTimeout  = 3 * time.Second
)

type RetransmissionEntry struct {
	Data []byte
	SeqNum uint32
	SendTime time.Time
	RTO time.Duration
  }
  
type RetransmissionQueue struct {
    Entries []*RetransmissionEntry
    mutex sync.Mutex
    SRTT time.Duration    // Smoothed RTT
    alpha float64         // Smoothing factor (typically 0.875)
    beta float64         // RTO multiplier (typically 2.0)
    RTOMin time.Duration  // Minimum allowed RTO
    RTOMax time.Duration  // Maximum allowed RTO
} 
  
  //Initialize the retransmission queue
 
func NewRetransmissionQueue() *RetransmissionQueue {
    return &RetransmissionQueue{
        Entries: make([]*RetransmissionEntry, 0),
        SRTT: 1 * time.Second,  // Initial SRTT guess
        alpha: 0.875,           // RFC793 recommended value (1 - 0.125)
        beta: 2.0,             // RTO multiplier
        RTOMin: 1 * time.Second,
        RTOMax: 60 * time.Second,
    }
} 

//RFC793 RTT calculation
func (rq *RetransmissionQueue) updateRTT(measuredRTT time.Duration) {
    rq.mutex.Lock()
    defer rq.mutex.Unlock()

    // SRTT = (α * SRTTLast) + (1 - α) * RTTMeasured
    rq.SRTT = time.Duration(float64(rq.SRTT)*rq.alpha + 
              float64(measuredRTT)*(1-rq.alpha))
    
    // Calculate new RTO
    // RTO = max(RTOMin, min(β * SRTT, RTOMax))
    proposedRTO := time.Duration(float64(rq.SRTT) * rq.beta)
    
    if proposedRTO < rq.RTOMin {
        proposedRTO = rq.RTOMin
    }
    if proposedRTO > rq.RTOMax {
        proposedRTO = rq.RTOMax
    }

    // Update RTO for all unacked packets
    for _, entry := range rq.Entries {
        entry.RTO = proposedRTO
    }
}

func (c *VTCPConn) HandleRetransmission(stack *IPStack, sock *Socket) {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            c.Window.RetransmissionQueue.mutex.Lock()
            now := time.Now()
            
            for i := 0; i < len(c.Window.RetransmissionQueue.Entries); i++ {
                entry := c.Window.RetransmissionQueue.Entries[i]
                if now.Sub(entry.SendTime) > entry.RTO {
                    fmt.Printf("Retransmitting packet seq=%d (RTO=%v)\n", 
                        entry.SeqNum, entry.RTO)
                    
                    // Send the packet
                    err := stack.sendTCPPacket(sock, entry.Data, header.TCPFlagAck)
                    if err != nil {
                        fmt.Printf("Failed to retransmit: %v\n", err)
                    }
                    entry.SendTime = now
                    // Double RTO for backoff, but keep within bounds
                    entry.RTO = time.Duration(float64(entry.RTO) * 2)
                    if entry.RTO > c.Window.RetransmissionQueue.RTOMax {
                        entry.RTO = c.Window.RetransmissionQueue.RTOMax
                    }
                }
            }
            c.Window.RetransmissionQueue.mutex.Unlock()
        }
    }
}

func (rq *RetransmissionQueue) AddEntry(data []byte, seqNum uint32) {
    rq.mutex.Lock()
    defer rq.mutex.Unlock()

    // Calculate current RTO based on SRTT
    currentRTO := time.Duration(float64(rq.SRTT) * rq.beta)
    if currentRTO < rq.RTOMin {
        currentRTO = rq.RTOMin
    }
    if currentRTO > rq.RTOMax {
        currentRTO = rq.RTOMax
    }

    entry := &RetransmissionEntry{
        Data: make([]byte, len(data)), // Make a copy of the data
        SeqNum: seqNum,
        SendTime: time.Now(),
        RTO: currentRTO,
    }
    copy(entry.Data, data)
    rq.Entries = append(rq.Entries, entry)
}

func (rq *RetransmissionQueue) RemoveAckedEntries(ackNum uint32) {
    rq.mutex.Lock()
    defer rq.mutex.Unlock()

    newEntries := make([]*RetransmissionEntry, 0)
    for _, entry := range rq.Entries {
        if entry.SeqNum + uint32(len(entry.Data)) > ackNum {
            newEntries = append(newEntries, entry)
        }
    }
    rq.Entries = newEntries
}

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
	smoothRTT time.Duration
	rttAlpha float64
	rttBeta float64
  }
  
  //Initialize the retransmission queue
  func NewRetransmissionQueue() *RetransmissionQueue {
	return &RetransmissionQueue{
	  Entries: make([]*RetransmissionEntry, 0),
	  smoothRTT: 1 * time.Second, // initial SRTT 1 sec
	  rttAlpha: 0.125,
	  rttBeta: 0.25,
	}
  }
  
  //Add a new entry to the retransmission queue
  func (rq *RetransmissionQueue) AddEntry(data []byte, seqNum uint32) {
	rq.mutex.Lock()
	defer rq.mutex.Unlock()
  
	entry := &RetransmissionEntry{
	  Data: data,
	  SeqNum: seqNum,
	  SendTime: time.Now(),
	  RTO: rq.smoothRTT,
	}
	rq.Entries = append(rq.Entries, entry)
  }
  
  func (rq *RetransmissionQueue) RemoveAckedEntries(ackNum uint32) {
	rq.mutex.Lock()
	defer rq.mutex.Unlock()
  
	for i := 0; i < len(rq.Entries); i++ {
	  if rq.Entries[i].SeqNum < ackNum {
		rq.Entries = append(rq.Entries[:i], rq.Entries[i+1:]...)
		i--
	  }
	}
  }
  //Get Earliest
func (rq *RetransmissionQueue) GetEarliest() *RetransmissionEntry {
	rq.mutex.Lock()
	defer rq.mutex.Unlock()
  
	if len(rq.Entries) == 0 {
	  return nil
	}
  
	return rq.Entries[0]
  }

  //Handle the retransmission of packets
func (c *VTCPConn) HandleRetransmission(stack *IPStack, sock *Socket) {
	for {
	  c.Window.RetransmissionQueue.mutex.Lock()
	  for _, entry := range c.Window.RetransmissionQueue.Entries {
		if time.Since(entry.SendTime) > entry.RTO {
		  fmt.Println("Retransmitting packet")
		  stack.sendTCPPacket(sock, entry.Data, header.TCPFlagAck)
		  entry.SendTime = time.Now()
		  entry.RTO *= 2
		}
	  }
	  c.Window.RetransmissionQueue.mutex.Unlock()
	  time.Sleep(1 * time.Second)
	}
  }
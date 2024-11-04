package socket


type SocketStatus int

const (
  Listening SocketStatus = 0
  SynSent SocketStatus = 1
  SynReceived = 2
  Established = 3
)


type Socket struct {
  SID int
  State SocketStatus
  LocalAddr netip.Addr
  LocalPort uint16  
  RemoteAddr netip.Addr
  RemotePort uint16
  SeqNum  uint32
  AckNum  uint32
  WindowSize uint16
  SendWindow []byte
}
# TCP Implementation
### Max Guo and Timothy Fong

## Key Data Structures

- **Connection Management:**
  - `Socket`: Wraps connection state and either holds a `VTCPConn` for established connections or `VTCPListener` for listening sockets
  - `Window`: Manages flow control with ring buffers for send/receive data and tracks sequence numbers
  - `RetransmissionQueue`: Handles reliable delivery with ordered entry tracking and RTO calculations

- **Buffering:**
  - Ring buffers for both send and receive windows
  - Priority queue for out-of-order packet handling
  - Retransmission queue for unacked data

## Threading Model
- **Main Thread**: Handles packet processing and state transitions
- **Retransmission Thread**: Per-connection goroutine monitoring unacked packets
- **Accept Thread**: For listening sockets, handles incoming connection requests
- **File Transfer**: Background goroutines for non-blocking file transfers

## Key Mechanisms
1. **Flow Control**: 
   - Window-based flow control using sliding window
   - Zero window probing to check for read buffer data availability

2. **Reliability**:
   - RTT-based retransmission timeouts
   - Retransmissions for unacked data
3. **Connection Management**:
   - TCP state machine
   - Connection teardown with FIN handling
   - TIME_WAIT state handling

## Potential Improvements
Our code structure could probably be greatly improved upon. Currently, a lot of our pointer arithmetic for the sliding window
is handled directly in VWrite and VRead

## Known Limitations
- Sending large files over lossy networks.
- Retransmission and 

## Testing
Wireshark
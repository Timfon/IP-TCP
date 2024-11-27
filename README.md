
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
1. Flow Control: 
   - Window-based flow control using sliding window
   - Zero window probing to check for read buffer data availability
2. Reliability:
   - RTT-based retransmission timeouts
   - Retransmissions for unacked data
3. Connection Management: 
   - TCP state machine
   - Connection teardown with FIN handling
   - TIME_WAIT state handling

## Potential Improvements
Our code structure could probably be greatly improved upon. Currently, a lot of our pointer arithmetic for the sliding window
is handled directly in VWrite and VRead, and it should probably be done in separate helper functions that interface directly with
the send and receive buffers. 

##Packet Capture:
The 3-way handshake
Frame 1, 2, and 3. 

One segment sent and acknowledged:
Frame 7 and 8.

One segment that is retransmitted:
Frame 27.

Connection teardown:
Frame 32, 33, 34, 35.

##Performance for sending 1MB on non lossy network:
Reference: 0.415s
Ours: 6.170110586s


## Known Limitations
- Retransmission sometimes deletes a socket when it shouldn't.
- Entering and exiting the zero window state repeatedly sometimes doesn't work. 

## Testing
Wireshark
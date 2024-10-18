# IP-IP-MAN
### Max Guo and Timothy Fong
### Hours Spent: > 1hr

Repo Link: https://github.com/brown-cs1680-f24/ip-ip-man

## Features
The IP program features a vhost and a vrouter program which share an ipstack package that they use to send packets in a network. Each router and host will have their own forwarding table, but the routers will continuously update their forwarding tables if they receive a route with a better cost from their RIP neighbors. The IP program also features disabling and enabling the interfaces on routers/hosts; the network will continuously find valid paths for packets to go through. Each host and router will have a command line interface, which has commands that allow the user to send packets to a specified virtual IP, see the node's interfaces, see the node's neighbors' interfaces, and see the current routes/costs.

## Example usage with loop:
- cd into the ip-ip-man directory
- make clean all
- ./util/vnet_generate nets/loop.json loop/
- ./util/vnet_run loop
- In h1, type "send 10.6.0.2 hello world!"
- h2 should print that it received the packet, with a TTL of 28.
- In r2, type "down if0"
- The routing table in r3 should update to show a higher cost, because packets can no longer go through the shortcut through r2.
- Sending a packet to 10.6.0.2 in h1 will now show a TTL of 27, because it had to go through more routers to reach its destination.
- Type "lr" on all the nodes to look at the cost of the routes in each of the forwarding tables.
- Type "li" to list the interfaces of a node.
- Type "ln" to list the neighbors of a node.

## Design Decisions
A list of Route structures was used to create the forwarding table for each node. The Route structure contains an interface, routing mode, Prefix, VirtualIP (the next hop), update time stamp, and cost. Each interface was also represented using a Go struct, containing its name, UDP socket connection, and an UpOrDown boolean. A thread was created for each interface; a separate thread was used to send periodic RIP updates, and another thread checks if any routes have expired.

A generic ReceiveIP() and SendIP() function was implemented in the ipstack, which both the vhost and vrouter can use. These functions handle any kind of packet and will map the protocol number of the packet to the appropriate handler function. 0 goes to TestPacketHandler(), and 200 goes to RIPPacketHandler. This map of protocol number to handler function is stored in the ipstack, and the vhost and vrouter must register the handler functions before being able to use them. This allows the IP program to be more extensible in the future, when TCP packets need to also be handled.

## Bugs
None identified during testing.

## Tests
- Wireshark

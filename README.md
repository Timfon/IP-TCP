# IP-IP-MAN
### Hours Spent: > 6hrs

## Max Guo and Timothy Fong
Repo Link: https://github.com/brown-cs1680-f24/ip-ip-man

## Features
The IP program features a vhost and a vrouter program which share an ipstack package that they use to send packets in a network. Each router and host will have their own forwarding table, but the routers will continuously update their forwarding tables if they receive a route with a better cost from their RIP neighbors. The IP program also features disabling and enabling the interfaces on routers/hosts; the network will continuously find valid paths for packets to go through. Each host and router will have a command line interface, which has commands that allow the user to send packets to a specified virtual IP, see the node's interfaces, see the node's neighbor's interfaces, and see the current routes/costs. 


## Example usage with loop:
    - cd into the ip-ip-man directory
    - make clean all
    - ./util/vnet_generate nets/loop.json loop/
    - ./util/vnet_run loop
    - In h1, type "send 10.6.0.2 hello world!"
    - h2 should print that it received the packet. 
    - Type "lr" in all the nodes to look at the cost of the routes in each of the forwarding tables. 



## Design Decisions
Design Decisions:
A list of Route structures was used to create the forwarding table for each node. The Route structure contains an interface, routing mode, Prefix, VirtualIP(the next hop), update time stamp, and cost. Each interface was also represented using a go struct, containing its name, udp socket connection, and an UpOrDown boolean. A go thread was created 


## Bugs
None

## Tests
    - Pass all tests
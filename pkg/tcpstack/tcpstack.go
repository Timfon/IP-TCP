package tcpstack

import {
	"IP-TCP/pkg/lnxconfig"
	"IP-TCP/pkg/socket"
}


type TCPStack struct {

}

func InitializeTCP(config *lnxconfig.IPConfig) (*TCPStack, error){

}


func (stack *TCPStack) VListen(port uint16) (*socket.VTCPListener, error){

}



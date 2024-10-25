package socket

import {
	"net/netip"
}

type VTCPConn struct {

}


func VConnect(addr netip.Addr, port int16) (VTCPConn, error){

}


func (*VTCPConn) VClose() error
{

}



func (*VTCPConn) VRead(buf []byte) (int, error){

}

func (*VTCPConn) VWrite(data []byte) (int, error){


}

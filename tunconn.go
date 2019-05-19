package svpn

import (
	"net"
	"github.com/songgao/water"
	"github.com/songgao/packets/ethernet"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	ethHeaderSize = 18
)

type TUNPacketConn struct {
	water.Interface
}

type ethernetAddr struct {
	net.HardwareAddr
}

func (ethernetAddr) Network() string {
	return "eth"
}

func (c *TUNPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = c.Interface.Read(p)
	if c.Interface.IsTAP() {
		if n >= ethHeaderSize {
			src := ethernetAddr{ethernet.Frame(p).Source()}
			return n, src, err
		}
	} else {
		// parse as ipv4 header
		v4Header, hErr := ipv4.ParseHeader(p[:n])
		if hErr == nil && v4Header != nil {
			return n, &net.IPAddr{IP: v4Header.Src, Zone: ""}, err
		}
		// parse as ipv6 header
		v6Header, hErr := ipv6.ParseHeader(p[:n])
		if hErr == nil && v6Header != nil {
			return n, &net.IPAddr{IP: v6Header.Src, Zone: ""}, err
		}
	}
	return n, nil, err
}

func (c *TUNPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {

}

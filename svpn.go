package svpn

import (
	"net"
)

// Network act as a TUN remote and implements net.PacketConn
type Network struct {
	// PacketSize is the size limit of packets that the VPN can transmit
	PacketSize uint
	// PublicAddr is the address for external network, can be dynamic, e.g. STUN
	PublicAddr    net.Addr
	// VPNAddr is the network address for inside the network, should be static
	VPNAddr       net.Addr
	// SignalBus is used for broadcasting presence and to negotiate address
	SignalBus  net.Conn
	// PacketConn is the underlying transport for packets
	PacketConn net.PacketConn
}

// Initializes S
func (vpn *Network) Initialize() error {
	return nil
}

package svpn

import (
	"net"
	"sync"
)

// NAT is a many-to-many network address translator
type NAT interface {
	// PublicAddrToPrivateAddrs translate a net.Addr to a list of net.Addr
	PublicAddrToPrivateAddrs(net.Addr) []net.Addr

	// PrivateAddrToPublicAddrs translate a net.Addr to a list of net.Addr
	PrivateAddrToPublicAddrs(net.Addr) []net.Addr
}

// NATPacketConn act as a router on top of a net.PacketConn
type NATPacketConn struct {
	// PacketConn is the underlying transport for packets
	net.PacketConn
	NAT NAT
	
	pendingReadFromBuf   []byte
	pendingReadFromAddrs []net.Addr
	pendingReadFromMutex sync.Mutex
}

// ReadFrom : See PacketConn.ReadFrom
func (c *NATPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	// first check if theres multicast pending, if yes, read remaining such packets
	c.pendingReadFromMutex.Lock()
	if len(c.pendingReadFromAddrs) > 0 {
		privateAddr := c.pendingReadFromAddrs[0]
		c.pendingReadFromAddrs = c.pendingReadFromAddrs[1:]
		c.pendingReadFromMutex.Unlock()
		n = copy(p, c.pendingReadFromBuf)
		return n, privateAddr, nil
	}
	c.pendingReadFromMutex.Unlock()
	// do actual read if there's no pending packets
	n, addr, err = c.PacketConn.ReadFrom(p)
	// do address translation
	privateAddrs := c.NAT.PublicAddrToPrivateAddrs(addr)
	if len(privateAddrs) == 0 {
		return n, nil, err
	}
	if len(privateAddrs) > 1 {
		c.pendingReadFromMutex.Lock()
		c.pendingReadFromAddrs = privateAddrs[1:]
		c.pendingReadFromMutex.Unlock()
	}
	return n, privateAddrs[0], err
}

// WriteTo : See PacketConn.WriteTo, this never fails
func (c *NATPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	publicAddrs := c.NAT.PrivateAddrToPublicAddrs(addr)
	for _, publicAddr := range publicAddrs {
		c.PacketConn.WriteTo(p, publicAddr)
	}
	return len(p), nil
}

type NATAddr struct {
	net.Addr
	Src net.Addr
}

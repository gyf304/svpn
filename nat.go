package svpn

import (
	"net"
	"sync"
	"fmt"
)

// NATEntry represents one entry in a NAT table
// Note that the entry is not just limited to IPv4 <-> IPv4
type NATEntry struct {
	Src net.Addr
	Dst net.Addr
}

// NAT is a many-to-many network address translator
type NAT interface {
	// TranslateInbound translate a net.Addr to a list of net.Addr
	TranslateInbound(net.Addr) []net.Addr

	// TranslateOutbound translate a net.Addr to a list of net.Addr
	TranslateOutbound(net.Addr) []net.Addr
}

// NATPacketConn act as a router on top of a net.PacketConn
type NATPacketConn struct {
	// PacketConn is the underlying transport for packets
	net.PacketConn
	NAT NAT

	pendingReadFromSrc   net.Addr
	pendingReadFromBuf   []byte
	pendingReadFromAddrs []net.Addr
	pendingReadFromMutex sync.Mutex
}

// ReadFromNAT is identical to ReadFrom but returns *NATAddr instead of net.Addr
func (c *NATPacketConn) ReadFromNAT(p []byte) (n int, naddr *NATAddr, err error) {
	// first check if theres multicast pending, if yes, read remaining such packets
	c.pendingReadFromMutex.Lock()
	if len(c.pendingReadFromAddrs) > 0 {
		privateAddr := c.pendingReadFromAddrs[0]
		c.pendingReadFromAddrs = c.pendingReadFromAddrs[1:]
		c.pendingReadFromMutex.Unlock()
		n = copy(p, c.pendingReadFromBuf)
		return n, &NATAddr{privateAddr, c.pendingReadFromSrc}, nil
	}
	c.pendingReadFromMutex.Unlock()
	// do actual read if there's no pending packets
	n, addr, err := c.PacketConn.ReadFrom(p)
	// do address translation
	privateAddrs := c.NAT.TranslateInbound(addr)
	if len(privateAddrs) == 0 {
		return n, nil, err
	}
	if len(privateAddrs) > 1 {
		c.pendingReadFromMutex.Lock()
		c.pendingReadFromAddrs = privateAddrs[1:]
		c.pendingReadFromSrc = addr
		c.pendingReadFromMutex.Unlock()
	}
	fmt.Println("Routing pkt from", naddr, "to", privateAddrs)
	return n, &NATAddr{privateAddrs[0], addr}, err
}

// ReadFrom : See PacketConn.ReadFrom
func (c *NATPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	return c.ReadFromNAT(p)
}

// WriteTo : See PacketConn.WriteTo, this never fails
func (c *NATPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	publicAddrs := c.NAT.TranslateOutbound(addr)
	for _, publicAddr := range publicAddrs {
		c.PacketConn.WriteTo(p, publicAddr)
	}
	// fmt.Println("Routing Pkt from", addr, "to", publicAddrs)
	return len(p), nil
}

// NATAddr expands net.Addr by including a Src
type NATAddr struct {
	net.Addr
	Src net.Addr
}

package svpn

import (
	"net"
	"github.com/gortc/stun"
	"fmt"
	"time"
)

// STUNUDP is a udp connection, but LocalAddr is now a Public Internet Address
type STUNUDP struct {
	*net.UDPConn
	StunServerAddr  *net.UDPAddr

	tickerStop      chan struct{}
	cachedAddr      *stunUDPAddr
}

// PublicAddr returns STUN address
func (c *STUNUDP) PublicAddr() net.Addr {
	return c.cachedAddr
}

func (c *STUNUDP) PublicAddrProxy() net.Addr {
	return (*stunUDPAddrProxy)(c)
}

func (c *STUNUDP) Initialize(keepAliveInterval time.Duration) error {
	c.SetKeepAliveInterval(keepAliveInterval)
	b := make([]byte, 4096)
	for {
		_, _, err := c.ReadFromUDP(b)
		if err != nil {
			return err
		}
		if c.cachedAddr != nil {
			return nil
		}
	}
}

func (c *STUNUDP) ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error) {
	var tmpB []byte
	if len(b) < 4096 {
		tmpB = make([]byte, 4096)
		n, addr, err = c.UDPConn.ReadFromUDP(tmpB)
	} else {
		n, addr, err = c.UDPConn.ReadFromUDP(b)
	}

	stunMessage := stun.Message{}
	if len(tmpB) == 0 {
		err = stun.Decode(b, &stunMessage)
	} else {
		err = stun.Decode(tmpB, &stunMessage)
	}
	if err == nil {
		xorAddr := stun.XORMappedAddress{}
		err = xorAddr.GetFrom(&stunMessage)
		if err == nil {
			c.cachedAddr = &stunUDPAddr{xorAddr.String()}
		}
	}
	if len(tmpB) > 0 {
		copy(b, tmpB)
	}
	return n, addr, err
}

func (c *STUNUDP) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	return c.ReadFromUDP(b)
}

func (c *STUNUDP) Read(b []byte) (n int, err error) {
	n, _, err = c.ReadFrom(b)
	return n, err
}

// SetKeepAliveInterval keeps the UDP mapping alive
func (c *STUNUDP) SetKeepAliveInterval(d time.Duration) {
	// set up sender
	if c.tickerStop != nil {
		close(c.tickerStop)
	}
	if d <= 0 {
		return
	}
	c.tickerStop = make(chan struct{})
	ticker := time.NewTicker(d)
	go func() {
		message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
		_, err := c.UDPConn.WriteToUDP(message.Raw, c.StunServerAddr)
		for {
			select {
			case <- ticker.C:
				message = stun.MustBuild(stun.TransactionID, stun.BindingRequest)
				_, err = c.UDPConn.WriteToUDP(message.Raw, c.StunServerAddr)
				if err != nil {
					fmt.Println(err)
				}
			case <- c.tickerStop:
				ticker.Stop()
				return
			}
		}
	}()
	// set up receiver
}

type stunUDPAddr struct {
	addr string
}

func (addr *stunUDPAddr) Network() string {
	return "udp"
}

func (addr *stunUDPAddr) String() string {
	if addr == nil {
		return ""
	}
	return addr.addr
}

type stunUDPAddrProxy STUNUDP

func (addr *stunUDPAddrProxy) Network() string {
	return "udp"
}

func (addr *stunUDPAddrProxy) String() string {
	return (*STUNUDP)(addr).cachedAddr.String()
}

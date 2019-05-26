package stunudp

import (
	"fmt"
	"net"
	"time"

	"github.com/gortc/stun"
)

// STUNUDPConn is a udp connection
type STUNUDPConn struct {
	*net.UDPConn
	StunServerAddr    *net.UDPAddr
	KeepAliveInterval time.Duration

	tickerStop chan struct{}
	cachedAddr *net.UDPAddr
}

// PublicAddr returns STUN address
func (c *STUNUDPConn) PublicAddr() net.Addr {
	return c.cachedAddr
}

func ListenSTUNUDP(network string, laddr *net.UDPAddr, stunAddr *net.UDPAddr) (*STUNUDPConn, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	return &STUNUDPConn{
		UDPConn: conn,
		StunServerAddr: stunAddr,
	}, nil
}

// Start
func (c *STUNUDPConn) Start() error {
	// set up sender
	if c.tickerStop != nil {
		close(c.tickerStop)
	}
	if c.KeepAliveInterval >= 0 {
		if c.KeepAliveInterval <= 10*time.Second {
			c.KeepAliveInterval = 10 * time.Second
		}
		c.tickerStop = make(chan struct{})
		ticker := time.NewTicker(c.KeepAliveInterval)
		go func() {
			message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
			_, err := c.UDPConn.WriteToUDP(message.Raw, c.StunServerAddr)
			for {
				select {
				case <-ticker.C:
					message = stun.MustBuild(stun.TransactionID, stun.BindingRequest)
					c.SetWriteDeadline(time.Now().Add(1 * time.Second))
					_, err = c.UDPConn.WriteToUDP(message.Raw, c.StunServerAddr)
					if err != nil {
						fmt.Println(err)
					}
				case <-c.tickerStop:
					ticker.Stop()
					return
				}
			}
		}()
	}
	_, _, err := c.ReadFromUDP(make([]byte, 4096))
	return err
}

// Stop stops everything
func (c *STUNUDPConn) Stop() error {
	if c.tickerStop != nil {
		close(c.tickerStop)
	}
	return nil
}

func (c *STUNUDPConn) ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error) {
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
			c.cachedAddr = &net.UDPAddr{IP: xorAddr.IP, Port: xorAddr.Port}
		}
	}
	if len(tmpB) > 0 {
		copy(b, tmpB)
	}
	return n, addr, err
}

func (c *STUNUDPConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	return c.ReadFromUDP(b)
}

func (c *STUNUDPConn) Read(b []byte) (n int, err error) {
	n, _, err = c.ReadFrom(b)
	return n, err
}

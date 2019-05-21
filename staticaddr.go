package svpn

import (
	"encoding/json"
	"net"
)

// staticAddr is a implementation of net.Addr
type _staticAddr struct {
	NetworkValue string `json:"network"`
	StringValue  string `json:"string"`
}

type staticAddr _staticAddr

func (addr *staticAddr) Network() string {
	if addr == nil {
		return ""
	}
	return addr.NetworkValue
}

func (addr *staticAddr) String() string {
	if addr == nil {
		return ""
	}
	return addr.StringValue
}

// MarshalText marshalls text into json
func (addr *staticAddr) MarshalText() (text []byte, err error) {
	return json.Marshal((*_staticAddr)(addr))
}

// UnmarshalText unmarshalls text from json
func (addr *staticAddr) UnmarshalText(text []byte) error {
	var xAddr _staticAddr
	err := json.Unmarshal(text, &xAddr)
	if err != nil {
		return err
	}
	*addr = staticAddr(xAddr)
	return nil
}

// staticAddrFromAddr creates staticAddr from any net.Addr
func staticAddrFromAddr(addr net.Addr) *staticAddr {
	return &staticAddr{addr.Network(), addr.String()}
}

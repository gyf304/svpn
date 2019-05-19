package svpn

import (
	"net"
	"encoding/json"
)

type StaticAddr struct {
	NetworkValue string `json:"network"`
	StringValue  string `json:"string"`
}

type staticAddr StaticAddr

func (addr *StaticAddr) Network() string {
	if addr == nil {
		return ""
	}
	return addr.NetworkValue
}

func (addr *StaticAddr) String() string {
	if addr == nil {
		return ""
	}
	return addr.StringValue
}

func (addr *StaticAddr) MarshallText() (text []byte, err error) {
	return json.Marshal(addr)
}

func (addr *StaticAddr) UnmarshalText(text []byte) error {
	var xAddr staticAddr
	err := json.Unmarshal(text, &xAddr)
	if err != nil {
		return err
	}
	*addr = StaticAddr(xAddr)
	return nil
}

func StaticAddrFromAddr(addr net.Addr) *StaticAddr {
	return &StaticAddr{addr.Network(), addr.String()}
}

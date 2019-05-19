package svpn

import (
	"net"
	"encoding/json"
)

type StaticAddr struct {
	NetworkValue string
	StringValue  string
}

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
	return json.Unmarshal(text, addr)
}

func StaticAddrFromAddr(addr net.Addr) *StaticAddr {
	return &StaticAddr{addr.Network(), addr.String()}
}

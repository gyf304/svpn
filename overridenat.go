package svpn

import "net"

type OverrideNAT struct {
	NAT
	OverrideNAT NAT
	OutboundOverride func (src net.Addr, dst []net.Addr) ([]net.Addr)
	InboundOverride func (src net.Addr, dst []net.Addr) ([]net.Addr)
}

func (s *OverrideNAT) TranslateInbound(addr net.Addr) []net.Addr {
	if s.InboundOverride == nil {
		return s.NAT.TranslateInbound(addr)
	}
	return s.InboundOverride(addr, s.NAT.TranslateInbound(addr))
}

func (s *OverrideNAT) TranslateOutbound(addr net.Addr) []net.Addr {
	if s.OutboundOverride == nil {
		return s.NAT.TranslateOutbound(addr)
	}
	return s.OutboundOverride(addr, s.NAT.TranslateInbound(addr))
}

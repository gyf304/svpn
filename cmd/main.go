package main

import (
	"time"
	"fmt"
	"net"
	"github.com/gyf304/svpn"
	"github.com/gyf304/svpn/mqttconn"
)

type ipv4ManagedNATOverride struct {
	IPNet           net.IPNet
	MNAT            svpn.ManagedNAT
}

func (ov *ipv4ManagedNATOverride) PublicAddrToPrivateAddrs(net.Addr) []net.Addr {
	return nil // don't override ingress address
}

func (ov *ipv4ManagedNATOverride) PrivateAddrToPublicAddrs(net.Addr) []net.Addr {
	return nil
}

func main() {
	// initialize STUN udp
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if (err != nil) {
		panic(err)
	}
	udpAddr, err := net.ResolveUDPAddr("udp4", "stun1.l.google.com:19302")
	if (err != nil) {
		panic(err)
	}
	fmt.Println(udpAddr)
	stunLn := &svpn.STUNUDP{
		UDPConn: ln,
		StunServerAddr: udpAddr,
	}
	stunLn.Initialize(10 * time.Second)
	// yo
	fmt.Println("IPAddr Remote: ", stunLn.PublicAddr())
	// initialize signalBus
	uri := "mqtt://broker.hivemq.com/test/room1234"
	signalBus, err := mqttconn.CreateMQTTConnFromURL(uri)
	if err != nil {
		panic(err)
	}
	// initialize ManagedNAT
	mNAT := &svpn.ManagedNAT{SignalBus: signalBus}
	// initial probe on ManagedNAT
	mNAT.Start()
	// wait 2 seconds to settle down
	time.Sleep(2 * time.Second)
	mNAT.PinMapping(&net.IPAddr{net.IPv4(192, 168, 8, 9), ""}, stunLn.PublicAddr())
	for {
		time.Sleep(5 * time.Second)
		fmt.Println(mNAT.GetOutboundMapping())
	}
}

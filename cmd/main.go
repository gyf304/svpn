package main

import (
	"bytes"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/songgao/packets/ethernet"
	"github.com/songgao/water"
	"github.com/gyf304/svpn"
	"github.com/gyf304/svpn/stunudp"
	"github.com/gyf304/go-mqttconn"
)

type ethAddr struct {
	net.HardwareAddr
}

func (ethAddr) Network() string {
	return "eth"
}

func parseMAC(s string) net.HardwareAddr {
	hardwareAddr, err := net.ParseMAC(s)
	if err != nil {
		panic(err)
	}
	return hardwareAddr
}

func main() {
	// --- parse flags ---
	stunServerAddrStrPtr := flag.String("stun-server", "stun1.l.google.com:19302", "stun server address")
	controlServerAddrStrPtr := flag.String("control-server", "mqtt://broker.hivemq.com/svpn/default-room", "control server address")
	flag.Parse()

	// --- initialize STUN udp ---
	stunServerAddr, err := net.ResolveUDPAddr("udp4", *stunServerAddrStrPtr)
	if err != nil {
		panic(err)
	}
	stunUDP, err := stunudp.ListenSTUNUDP("udp4", &net.UDPAddr{}, stunServerAddr)
	if err != nil {
		panic(err)
	}
	stunUDP.SetReadDeadline(time.Now().Add(3 * time.Second))
	stunUDP.Start()
	stunUDP.SetReadDeadline(time.Time{})
	publicAddr := stunUDP.PublicAddr()
	if publicAddr == nil {
		panic("cannot obtain public addr")
	}
	// finished initializing STUN UDP
	fmt.Println("IPAddr Remote: ", publicAddr)

	// --- initialize signalBus ---
	signalBus, err := mqttconn.DialMQTT(*controlServerAddrStrPtr)
	if err != nil {
		panic(err)
	}

	// --- start up TAP ---
	config := getPlatformWaterConfig()
	tap, err := water.New(config)
	if err != nil {
		panic(err)
	}
	ifce, err := net.InterfaceByName(tap.Name())
	if err != nil {
		panic(err)
	}

	// --- initialize ManagedNAT ---
	mNAT := &svpn.ManagedNAT{SignalBus: signalBus}
	// initial probe on ManagedNAT
	mNAT.Start()
	// wait 3 seconds to settle down
	time.Sleep(3 * time.Second)

	// --- setup local address mapping ---
	hardwareAddr := ifce.HardwareAddr
	boardcastAddr := parseMAC("ff:ff:ff:ff:ff:ff")
	mNAT.PinMapping(ethAddr{boardcastAddr}, publicAddr)
	mNAT.PinMapping(ethAddr{hardwareAddr}, publicAddr)

	// --- initialize NAT Overrides ---
	oNAT := &svpn.OverrideNAT{
		NAT: mNAT,
		// filter out own address, cuz that just won't work (unless router supports hairpin)
		OutboundOverride: func(src net.Addr, dsts []net.Addr) ([]net.Addr) {
			newDsts := make([]net.Addr, 0, len(dsts))
			for _, dst := range dsts {
				if src.String() != publicAddr.String() {
					newDsts = append(newDsts, dst)
				}
			}
			return newDsts
		},
	}

	// --- setup packetconn ---
	pConn := &svpn.NATPacketConn{PacketConn: stunUDP, NAT: oNAT}

	// --- setup piping ---
	go func() {
		p := make([]byte, 65536)
		for {
			n, err := tap.Read(p)
			if err != nil {
				panic(err)
			}
			dest := ethernet.Frame(p[:n]).Destination()
			// if addr is broadcast addr or if addr is local, send myself a copy
			if bytes.Equal(dest, boardcastAddr) || bytes.Equal(dest, hardwareAddr) {
				tap.Write(p[:n])
			}
			n, err = pConn.WriteTo(p[:n], ethAddr{dest})
			if err != nil {
				panic(err)
			}
		}
	}()
	go func() {
		p := make([]byte, 65536)
		for {
			n, _, err := pConn.ReadFrom(p)
			if err != nil {
				panic(err)
			}
			tap.Write(p[:n])
		}
	}()
	logTicker := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-logTicker.C:
				fmt.Println(mNAT.GetOutboundMapping())
			}
		}
	}()

	// --- setup blocking ---
	signalChan := make(chan os.Signal)
	signal.Notify(signalChan, os.Interrupt)
	fmt.Println("running")
	<-signalChan
	logTicker.Stop()
	fmt.Println("stopping")
	// mNAT.Stop()
}

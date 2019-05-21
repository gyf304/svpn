package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/gyf304/svpn"
	"github.com/gyf304/svpn/mqttconn"
	"github.com/songgao/water"
)

type ethAddr struct {
	net.HardwareAddr
}

func (ethAddr) Network() string {
	return "eth"
}

func parseMAC(s string) ethAddr {
	hardwareAddr, err := net.ParseMAC("FF:FF:FF:FF:FF:FF")
	if err != nil {
		panic(err)
	}
	return ethAddr{hardwareAddr}
}

func main() {
	// initialize STUN udp
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		panic(err)
	}
	udpAddr, err := net.ResolveUDPAddr("udp4", "stun1.l.google.com:19302")
	if err != nil {
		panic(err)
	}
	fmt.Println(udpAddr)
	stunLn := &svpn.STUNUDP{
		UDPConn:        ln,
		StunServerAddr: udpAddr,
	}
	stunLn.SetReadDeadline(time.Now().Add(3 * time.Second))
	stunLn.Start()
	stunLn.SetReadDeadline(time.Time{})
	publicAddr := stunLn.PublicAddr()
	if publicAddr == nil {
		panic("cannot obtain public addr")
	}

	fmt.Println("IPAddr Remote: ", publicAddr)
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
	// wait 3 seconds to settle down
	time.Sleep(3 * time.Second)
	mNAT.PinMapping(parseMAC("ff:ff:ff:ff:ff:ff"), publicAddr)
	// start up TAP
	tunTapConfig := water.Config{
		DeviceType: water.TAP,
		PlatformSpecificParams: {}
	}
	signalChan := make(chan os.Signal)
	workerStopped := make(chan struct{})
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer close(workerStopped)
		for {
			select {
			case <-ticker.C:
				fmt.Println(mNAT.GetOutboundMapping())
			case <-signalChan:
				ticker.Stop()
				fmt.Println("exit")
				return
			}
		}
	}()
	fmt.Println("waiting")
	<-workerStopped
	fmt.Println("stopping")
	mNAT.Stop()
}

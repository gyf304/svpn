package svpn

import (
	"fmt"
	"net"
	"encoding/base64"
	"sync"
	"strings"
)

// ManagedNAT handles peer discovery on a net.Conn signal bus
type ManagedNAT struct {
	SignalBus   net.Conn
	OverrideNAT NAT
	FallbackNAT NAT

	mapLock           sync.RWMutex
	publicToPrivate   map[StaticAddr]map[StaticAddr]bool // true if is pinned
	privateToPublic   map[StaticAddr]map[StaticAddr]bool
}

func generateAssocCmd(sPrivateAddr StaticAddr, sPublicAddr StaticAddr) string {
	privateAddrMarshalled, err := sPrivateAddr.MarshallText()
	if err != nil {
		panic(err)
	}
	publicAddrMarshalled, err := sPublicAddr.MarshallText()
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(
		"ASSOC %s %s", 
		base64.StdEncoding.EncodeToString(privateAddrMarshalled),
		base64.StdEncoding.EncodeToString(publicAddrMarshalled),
	)
}

// Start probes existing peers on network
func (mNAT *ManagedNAT) Start() error {
	go func() {
		b := make([]byte, 65536)
		for {
			n, err := mNAT.SignalBus.Read(b)
			if err != nil {
				return
			}
			mNAT.handleCommand(strings.Split(string(b[:n]), " ")...)
		}
	}()
	return mNAT.Probe()
}

// Probe probes existing peers on network
func (mNAT *ManagedNAT) Probe() error {
	_, err := mNAT.SignalBus.Write([]byte("PROBE"))
	return err
}

func (mNAT *ManagedNAT) addMapping(sPrivateAddr StaticAddr, sPublicAddr StaticAddr) {
	if mNAT.publicToPrivate == nil {
		mNAT.publicToPrivate = make(map[StaticAddr]map[StaticAddr]bool)
	}
	if mNAT.privateToPublic == nil {
		mNAT.privateToPublic = make(map[StaticAddr]map[StaticAddr]bool)
	}
	_, ok := mNAT.publicToPrivate[sPublicAddr]
	if !ok {
		mNAT.publicToPrivate[sPublicAddr] = make(map[StaticAddr]bool)
	}
	_, ok = mNAT.privateToPublic[sPrivateAddr]
	if !ok {
		mNAT.privateToPublic[sPrivateAddr] = make(map[StaticAddr]bool)
	}
	mNAT.privateToPublic[sPrivateAddr][sPublicAddr] = mNAT.privateToPublic[sPrivateAddr][sPublicAddr] || false
	mNAT.publicToPrivate[sPublicAddr][sPrivateAddr] = mNAT.privateToPublic[sPrivateAddr][sPublicAddr] || false
}

// AddMapping adds binding for privateAddr <-> publicAddr
func (mNAT *ManagedNAT) AddMapping(privateAddr net.Addr, publicAddr net.Addr) {
	mNAT.mapLock.Lock()
	defer mNAT.mapLock.Unlock()
	sPrivateAddr := *StaticAddrFromAddr(privateAddr)
	sPublicAddr  := *StaticAddrFromAddr(publicAddr)
	mNAT.addMapping(sPrivateAddr, sPublicAddr)
}

// DropMapping removes binding for privateAddr <-> publicAddr
func (mNAT *ManagedNAT) DropMapping(privateAddr net.Addr, publicAddr net.Addr) {
	mNAT.mapLock.Lock()
	defer mNAT.mapLock.Unlock()
	sPrivateAddr := *StaticAddrFromAddr(privateAddr)
	sPublicAddr  := *StaticAddrFromAddr(publicAddr)
	mNAT.dropMapping(sPrivateAddr, sPublicAddr)
}

func (mNAT *ManagedNAT) dropMapping(sPrivateAddr StaticAddr, sPublicAddr StaticAddr) {
	if mNAT.publicToPrivate != nil {
		_, ok := mNAT.publicToPrivate[sPublicAddr]
		if ok {
			if !mNAT.publicToPrivate[sPublicAddr][sPrivateAddr] {
				delete(mNAT.publicToPrivate[sPublicAddr], sPrivateAddr)
			}
			if len(mNAT.publicToPrivate[sPublicAddr]) == 0 {
				delete(mNAT.publicToPrivate, sPublicAddr)
			}
		}
	}
	if mNAT.privateToPublic != nil {
		_, ok := mNAT.privateToPublic[sPrivateAddr]
		if ok {
			if !mNAT.privateToPublic[sPublicAddr][sPrivateAddr] {
				delete(mNAT.privateToPublic[sPrivateAddr], sPublicAddr)
			}
			if len(mNAT.privateToPublic[sPrivateAddr]) == 0 {
				delete(mNAT.privateToPublic, sPrivateAddr)
			}
		}
	}
}

// PinMapping adds binding for privateAddr <-> publicAddr, also will reply on probe
func (mNAT *ManagedNAT) PinMapping(privateAddr net.Addr, publicAddr net.Addr) {
	mNAT.mapLock.Lock()
	defer mNAT.mapLock.Unlock()
	sPrivateAddr := *StaticAddrFromAddr(privateAddr)
	sPublicAddr  := *StaticAddrFromAddr(publicAddr)
	mNAT.addMapping(sPrivateAddr, sPublicAddr)
	mNAT.privateToPublic[sPrivateAddr][sPublicAddr] = true
	mNAT.publicToPrivate[sPublicAddr][sPrivateAddr] = true
	mNAT.SignalBus.Write([]byte(generateAssocCmd(sPrivateAddr, sPublicAddr)))
}

// UnpinMapping remove binding for privateAddr <-> publicAddr
func (mNAT *ManagedNAT) UnpinMapping(privateAddr net.Addr, publicAddr net.Addr) {
	mNAT.mapLock.Lock()
	defer mNAT.mapLock.Unlock()
	sPrivateAddr := *StaticAddrFromAddr(privateAddr)
	sPublicAddr  := *StaticAddrFromAddr(publicAddr)
	if mNAT.publicToPrivate != nil {
		_, ok := mNAT.publicToPrivate[sPublicAddr]
		if ok {
			mNAT.publicToPrivate[sPublicAddr][sPrivateAddr] = false
		}
	}
	if mNAT.privateToPublic != nil {
		_, ok := mNAT.privateToPublic[sPrivateAddr]
		if ok {
			mNAT.privateToPublic[sPrivateAddr][sPublicAddr] = false
		}
	}
	mNAT.dropMapping(sPrivateAddr, sPublicAddr)
}

func (mNAT *ManagedNAT) PublicAddrToPrivateAddrs(addr net.Addr) []net.Addr {
	mNAT.mapLock.Lock()
	defer mNAT.mapLock.Unlock()
	if mNAT.publicToPrivate == nil {
		if mNAT.FallbackNAT != nil {
			return mNAT.FallbackNAT.PublicAddrToPrivateAddrs(addr)
		}
		return nil
	}
	addrs, ok := mNAT.publicToPrivate[*StaticAddrFromAddr(addr)]
	if !ok {
		if mNAT.FallbackNAT != nil {
			return mNAT.FallbackNAT.PublicAddrToPrivateAddrs(addr)
		}
		return nil
	}
	addrsSlice := make([]net.Addr, 0, len(addrs))
	for k := range addrs {
		addrsSlice = append(addrsSlice, &k)
	}
	return addrsSlice
}

func (mNAT *ManagedNAT) PrivateAddrToPublicAddrs(addr net.Addr) []net.Addr {
	mNAT.mapLock.RLock()
	defer mNAT.mapLock.RUnlock()
	if mNAT.privateToPublic == nil {
		return nil
	}
	addrs, ok := mNAT.privateToPublic[*StaticAddrFromAddr(addr)]
	if !ok {
		return nil
	}
	addrsSlice := make([]net.Addr, 0, len(addrs))
	for k := range addrs {
		addrsSlice = append(addrsSlice, &k)
	}
	return addrsSlice
}

func (mNAT *ManagedNAT) handleCommand(cmd ...string) {
	if len(cmd) == 0 {
		return
	}
	fmt.Println(cmd)
	switch cmd[0] {
	case "PROBE":
		mNAT.mapLock.RLock()
		for privateAddr, publicAddrs := range mNAT.privateToPublic {
			for publicAddr, pinned := range publicAddrs {
				if pinned {
					mNAT.SignalBus.Write([]byte(generateAssocCmd(privateAddr, publicAddr)))
				}
			}
		}
		mNAT.mapLock.RUnlock()
	case "ASSOC":
		fallthrough
	case "DISAC":
		// ASSOC <private address> <public address>
		// DISAC <private address> <public address>
		if len(cmd) != 3 {
			break
		}
		privateAddrBytes, err1 := base64.StdEncoding.DecodeString(cmd[1])
		publicAddrBytes, err2  := base64.StdEncoding.DecodeString(cmd[2])
		if err1 != nil || err2 != nil {
			fmt.Println(err1, err2)
			break
		}
		fmt.Println(string(privateAddrBytes), string(publicAddrBytes))
		privateAddr := StaticAddr{}
		publicAddr  := StaticAddr{}
		err1 = (&privateAddr).UnmarshalText(privateAddrBytes)
		err2 = (&publicAddr).UnmarshalText(publicAddrBytes)
		fmt.Println(privateAddr, publicAddr)
		if err1 != nil || err2 != nil {
			fmt.Println(err1, err2)
			break
		}
		if cmd[0] == "ASSOC" {
			mNAT.AddMapping(&privateAddr, &publicAddr)
		} else if cmd[0] == "DISAC" {
			mNAT.DropMapping(&privateAddr, &publicAddr)
		}
	}
}

func (mNAT *ManagedNAT) getMapping(dir bool) []NATEntry {
	mNAT.mapLock.RLock()
	defer mNAT.mapLock.RUnlock()
	if mNAT.privateToPublic == nil {
		return make([]NATEntry, 0)
	}
	entries := make([]NATEntry, 0, len(mNAT.privateToPublic) * 2)
	for privateAddr, publicAddrs := range mNAT.privateToPublic {
		for publicAddr := range publicAddrs {
			if dir {
				entries = append(entries, NATEntry{Src: &publicAddr, Dst: &privateAddr})
			} else {
				entries = append(entries, NATEntry{Src: &privateAddr, Dst: &publicAddr})
			}
		}
	}
	return entries
}

func (mNAT *ManagedNAT) GetOutboundMapping() []NATEntry {
	return mNAT.getMapping(false)
}

func (mNAT *ManagedNAT) GetInboundMapping() []NATEntry {
	return mNAT.getMapping(true)
}

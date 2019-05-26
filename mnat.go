package svpn

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// ManagedNAT handles peer discovery on a net.Conn signal bus
type ManagedNAT struct {
	SignalBus     net.Conn
	ProbeInterval time.Duration

	mapLock         sync.RWMutex
	publicToPrivate map[staticAddr]map[staticAddr]bool // true if is pinned
	privateToPublic map[staticAddr]map[staticAddr]bool

	readerClosed chan struct{}
	proberClosed chan struct{}
	closing      bool
}

func generateAddrPairCmd(cmd string, sPrivateAddr *staticAddr, sPublicAddr *staticAddr) string {
	privateAddrMarshalled, err := sPrivateAddr.MarshalText()
	if err != nil {
		panic(err)
	}
	publicAddrMarshalled, err := sPublicAddr.MarshalText()
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(
		"%s %s %s",
		cmd,
		base64.StdEncoding.EncodeToString(privateAddrMarshalled),
		base64.StdEncoding.EncodeToString(publicAddrMarshalled),
	)
}

// Start probes existing peers on network
func (mNAT *ManagedNAT) Start() error {
	mNAT.closing = false
	go func() {
		mNAT.readerClosed = make(chan struct{})
		defer close(mNAT.readerClosed)
		b := make([]byte, 65536)
		for {
			n, err := mNAT.SignalBus.Read(b)
			if err != nil {
				if !mNAT.closing && os.IsTimeout(err) {
					continue
				}
				return
			}
			mNAT.handleCommand(strings.Split(string(b[:n]), " ")...)
		}
	}()
	if mNAT.ProbeInterval >= 0 {
		interval := mNAT.ProbeInterval
		if interval < 10*time.Second {
			interval = 10 * time.Second
		}
		go func() {
			mNAT.proberClosed = make(chan struct{})
			defer close(mNAT.proberClosed)
			for {
				mNAT.Probe()
				time.Sleep(interval)
				if mNAT.closing {
					return
				}
			}
		}()
	}
	return mNAT.Probe()
}

// Stop Stops any activity on the signal bus
func (mNAT *ManagedNAT) Stop() error {
	mNAT.closing = true
	if mNAT.proberClosed != nil {
		<-mNAT.proberClosed
	}
	if mNAT.readerClosed != nil {
		<-mNAT.readerClosed
	}
	pinnedPairs := mNAT.getMapping(false, true)
	for _, pair := range pinnedPairs {
		mNAT.SignalBus.Write([]byte(generateAddrPairCmd("DISAC", pair.Src, pair.Dst)))
	}
	mNAT.closing = false
	return nil
}

// Probe probes existing peers on network
func (mNAT *ManagedNAT) Probe() error {
	_, err := mNAT.SignalBus.Write([]byte("PROBE"))
	return err
}

func (mNAT *ManagedNAT) addMapping(sPrivateAddr staticAddr, sPublicAddr staticAddr) {
	if mNAT.publicToPrivate == nil {
		mNAT.publicToPrivate = make(map[staticAddr]map[staticAddr]bool)
	}
	if mNAT.privateToPublic == nil {
		mNAT.privateToPublic = make(map[staticAddr]map[staticAddr]bool)
	}
	_, ok := mNAT.publicToPrivate[sPublicAddr]
	if !ok {
		mNAT.publicToPrivate[sPublicAddr] = make(map[staticAddr]bool)
	}
	_, ok = mNAT.privateToPublic[sPrivateAddr]
	if !ok {
		mNAT.privateToPublic[sPrivateAddr] = make(map[staticAddr]bool)
	}
	mNAT.privateToPublic[sPrivateAddr][sPublicAddr] = mNAT.privateToPublic[sPrivateAddr][sPublicAddr] || false
	mNAT.publicToPrivate[sPublicAddr][sPrivateAddr] = mNAT.privateToPublic[sPrivateAddr][sPublicAddr] || false
}

// AddMapping adds binding for privateAddr <-> publicAddr
func (mNAT *ManagedNAT) AddMapping(privateAddr net.Addr, publicAddr net.Addr) {
	mNAT.mapLock.Lock()
	defer mNAT.mapLock.Unlock()
	sPrivateAddr := *staticAddrFromAddr(privateAddr)
	sPublicAddr := *staticAddrFromAddr(publicAddr)
	mNAT.addMapping(sPrivateAddr, sPublicAddr)
}

// DropMapping removes binding for privateAddr <-> publicAddr
func (mNAT *ManagedNAT) DropMapping(privateAddr net.Addr, publicAddr net.Addr) {
	mNAT.mapLock.Lock()
	defer mNAT.mapLock.Unlock()
	sPrivateAddr := *staticAddrFromAddr(privateAddr)
	sPublicAddr := *staticAddrFromAddr(publicAddr)
	mNAT.dropMapping(sPrivateAddr, sPublicAddr)
}

func (mNAT *ManagedNAT) dropMapping(sPrivateAddr staticAddr, sPublicAddr staticAddr) {
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
	sPrivateAddr := *staticAddrFromAddr(privateAddr)
	sPublicAddr := *staticAddrFromAddr(publicAddr)
	mNAT.addMapping(sPrivateAddr, sPublicAddr)
	mNAT.privateToPublic[sPrivateAddr][sPublicAddr] = true
	mNAT.publicToPrivate[sPublicAddr][sPrivateAddr] = true
	mNAT.SignalBus.Write([]byte(generateAddrPairCmd("ASSOC", &sPrivateAddr, &sPublicAddr)))
}

// UnpinMapping remove binding for privateAddr <-> publicAddr
func (mNAT *ManagedNAT) UnpinMapping(privateAddr net.Addr, publicAddr net.Addr) {
	mNAT.mapLock.Lock()
	defer mNAT.mapLock.Unlock()
	sPrivateAddr := *staticAddrFromAddr(privateAddr)
	sPublicAddr := *staticAddrFromAddr(publicAddr)
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

func (mNAT *ManagedNAT) TranslateInbound(addr net.Addr) []net.Addr {
	if mNAT.publicToPrivate != nil {
		mNAT.mapLock.RLock()
		defer mNAT.mapLock.RUnlock()
		addrs := mNAT.publicToPrivate[*staticAddrFromAddr(addr)]
		addrsSlice := make([]net.Addr, 0, len(addrs))
		for k := range addrs {
			addrsSlice = append(addrsSlice, staticAddrFromAddr(&k))
		}
		return addrsSlice
	}
	return nil
}

func (mNAT *ManagedNAT) TranslateOutbound(addr net.Addr) []net.Addr {
	if mNAT.privateToPublic != nil {
		mNAT.mapLock.RLock()
		defer mNAT.mapLock.RUnlock()
		addrs := mNAT.privateToPublic[*staticAddrFromAddr(addr)]
		addrsSlice := make([]net.Addr, 0, len(addrs))
		for k := range addrs {
			addrsSlice = append(addrsSlice, staticAddrFromAddr(&k))
		}
		return addrsSlice
	}
	return nil
}

func (mNAT *ManagedNAT) handleCommand(cmd ...string) {
	if len(cmd) == 0 {
		return
	}
	switch cmd[0] {
	case "PROBE":
		mNAT.mapLock.RLock()
		for privateAddr, publicAddrs := range mNAT.privateToPublic {
			for publicAddr, pinned := range publicAddrs {
				if pinned {
					mNAT.SignalBus.Write([]byte(generateAddrPairCmd("ASSOC", &privateAddr, &publicAddr)))
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
		publicAddrBytes, err2 := base64.StdEncoding.DecodeString(cmd[2])
		if err1 != nil || err2 != nil {
			break
		}
		privateAddr := staticAddr{}
		publicAddr := staticAddr{}
		err1 = (&privateAddr).UnmarshalText(privateAddrBytes)
		err2 = (&publicAddr).UnmarshalText(publicAddrBytes)
		if err1 != nil || err2 != nil {
			break
		}
		if cmd[0] == "ASSOC" {
			mNAT.AddMapping(&privateAddr, &publicAddr)
		} else if cmd[0] == "DISAC" {
			mNAT.DropMapping(&privateAddr, &publicAddr)
		}
	}
}

type staticNATEntry struct {
	Src *staticAddr
	Dst *staticAddr
}

func (mNAT *ManagedNAT) getMapping(dir bool, onlyPinned bool) []staticNATEntry {
	mNAT.mapLock.RLock()
	defer mNAT.mapLock.RUnlock()
	if mNAT.privateToPublic == nil {
		return make([]staticNATEntry, 0)
	}
	entries := make([]staticNATEntry, 0, len(mNAT.privateToPublic)*2)
	for privateAddr, publicAddrs := range mNAT.privateToPublic {
		for publicAddr, pinned := range publicAddrs {
			if !onlyPinned || pinned {
				if dir {
					entries = append(entries, staticNATEntry{
						staticAddrFromAddr(&publicAddr),
						staticAddrFromAddr(&privateAddr),
					})
				} else {
					fmt.Println(privateAddr, publicAddr)
					entries = append(entries, staticNATEntry{
						staticAddrFromAddr(&privateAddr),
						staticAddrFromAddr(&publicAddr),
					})
				}
			}
		}
	}
	return entries
}

func staticNATEntriesToNATEntries(sEntries []staticNATEntry) []NATEntry {
	entries := make([]NATEntry, len(sEntries))
	for i, v := range sEntries {
		entries[i] = NATEntry{v.Src, v.Dst}
	}
	return entries
}

func (mNAT *ManagedNAT) GetOutboundMapping() []NATEntry {
	return staticNATEntriesToNATEntries(mNAT.getMapping(false, false))
}

func (mNAT *ManagedNAT) GetInboundMapping() []NATEntry {
	return staticNATEntriesToNATEntries(mNAT.getMapping(true, false))
}

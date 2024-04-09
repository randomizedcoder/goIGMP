package goIGMP

import (
	"fmt"
	"log"
	"net"
	"net/netip"

	"golang.org/x/net/ipv4"
)

const (
	protocolIGMP = "ip4:2"
)

func (r IGMPReporter) openUnicastPacketConn(interf side) (c net.PacketConn) {
	var (
		err     error
		localIP netip.Addr
		ok      bool
	)

	debugLog(r.debugLevel > 10, fmt.Sprintf("openPacketUnicastConnection(%s)", interf))

	if localIP, ok = r.NetAddr[interf]; !ok {
		log.Fatalf("openUnicastPacketConn(%s) interface IP lookup error", interf)
	}

	c, err = net.ListenPacket(protocolIGMP, localIP.String())
	if err != nil {
		log.Fatal(fmt.Sprintf("openUnicastPacketConn(%s) ListenPacket(%s,%s) err:", interf, protocolIGMP, localIP.String()), err)
	}

	debugLog(r.debugLevel > 10, fmt.Sprintf("openPacketUnicastConnection(%s) open on IP:%s", interf, localIP.String()))

	return c
}

func (r IGMPReporter) createPacketConns(interf side) {

	debugLog(r.debugLevel > 10, fmt.Sprintf("createPacketConns(%s)", interf))

	if r.anyCon[interf] == nil {
		r.anyCon[interf] = make(map[netip.Addr]net.PacketConn)
	}
	if r.mConIGMP[interf] == nil {
		r.mConIGMP[interf] = make(map[netip.Addr]*ipv4.PacketConn)
	}
	for _, g := range r.multicastGroups {
		if r.anyCon[interf][r.mapIPtoNetAddr[g]] == nil {
			r.anyCon[interf][r.mapIPtoNetAddr[g]], r.mConIGMP[interf][r.mapIPtoNetAddr[g]] = r.openPacketMulticastPacketConn(interf, r.mapIPtoNetAddr[g])
			debugLog(r.debugLevel > 10, fmt.Sprintf("createPacketConns(%s) group:%s", interf, r.mapIPtoNetAddr[g]))
		}
	}

	if r.debugLevel > 10 {
		for key, val := range r.anyCon {
			debugLog(r.debugLevel > 10, fmt.Sprintf("createPacketConns(%s) IntName Key: %v, Value: %v", interf, key, val))
		}
	}
}

// openPacketMulticastConnection opens:
// - IGMP socket
// - Sets up control message to recieve src, dst, interface
// - Joins on multicast group
// https://pkg.go.dev/golang.org/x/net/ipv4#hdr-Multicasting
func (r IGMPReporter) openPacketMulticastPacketConn(interf side, destinationIP netip.Addr) (c net.PacketConn, p *ipv4.PacketConn) {
	var (
		err error
	)

	debugLog(r.debugLevel > 100, fmt.Sprintf("openPacketMulticastConnection(%s) destinationIP:%s", interf, destinationIP))

	if !destinationIP.IsMulticast() {
		log.Fatalf("openPacketMulticastPacketConn(%s) !destinationIP.IsMulticast()", interf)
	}

	// This line fails when not running as root in the container.  Weird!! TODO Investigate
	// inspired by https://godoc.org/golang.org/x/net/ipv4#example-RawConn--AdvertisingOSPFHello
	c, err = net.ListenPacket(protocolIGMP, "0.0.0.0")
	if err != nil {
		log.Fatal(fmt.Sprintf("openPacketMulticastPacketConn(%s) ListenPacket(%s, \"0.0.0.0\") err:", interf, protocolIGMP), err)
		//log.Print(fmt.Sprintf("openPacketMulticastPacketConn(%s) ListenPacket(%s, \"0.0.0.0\") err:", interf, protocolIGMP), err)
	}

	if _, ok := r.NetIF[interf]; !ok {
		debugLog(r.debugLevel > 10, fmt.Sprintf("openPacketMulticastConnection(%s) !r.NetIF[%s]", interf, interf))
		r.NetIF[interf], r.NetIP[interf], r.NetAddr[interf] = r.getInterfaceHandle(interf)
	}

	p = ipv4.NewPacketConn(c)

	//---------------
	// Control message

	if err := p.SetControlMessage(ipv4.FlagSrc, true); err != nil {
		log.Fatal(fmt.Sprintf("openPacketMulticastPacketConn(%s) SetControlMessage err:", interf), err)
	}

	if err := p.SetControlMessage(ipv4.FlagDst, true); err != nil {
		log.Fatal(fmt.Sprintf("openPacketMulticastPacketConn(%s) SetControlMessage err:", interf), err)
	}

	if err := p.SetControlMessage(ipv4.FlagInterface, true); err != nil {
		log.Fatal(fmt.Sprintf("openPacketMulticastPacketConn(%s) SetControlMessage err:", interf), err)
	}

	debugLog(r.debugLevel > 10, fmt.Sprintf("openPacketMulticastPacketConn(%s) set FlagSrc, FlagDst, FlagInterface", interf))

	//---------------
	// Join multicast

	joinIP := r.mapIPtoNetIP[r.mapNetAddrtoIP[destinationIP]]

	if err := p.JoinGroup(r.NetIF[interf], &net.UDPAddr{IP: joinIP}); err != nil {
		log.Fatal(fmt.Sprintf("openPacketMulticastPacketConn(%s) JoinGroup err:", interf), err)
	}
	debugLog(r.debugLevel > 10, fmt.Sprintf("openPacketMulticastPacketConn(%s) joined:%s", interf, destinationIP))

	return c, p
}

func (r IGMPReporter) openRawConnection(interf side) (raw *ipv4.RawConn) {

	debugLog(r.debugLevel > 100, fmt.Sprintf("openRawConnection(%s)", interf))

	// inspired by https://godoc.org/golang.org/x/net/ipv4#example-RawConn--AdvertisingOSPFHello
	c, err := net.ListenPacket(protocolIGMP, "0.0.0.0")
	if err != nil {
		log.Fatal(fmt.Sprintf("openRawConnection(%s) ListenPacket err:", interf), err)
	}

	var netIF *net.Interface
	var ok bool
	if netIF, ok = r.NetIF[interf]; !ok {
		debugLog(r.debugLevel > 10, fmt.Sprintf("openRawConnection(%s) !r.NetIF[%s]", interf, interf))
		netIF, _, _ = r.getInterfaceHandle(interf)
		r.NetIF[interf] = netIF
	}

	raw, err = ipv4.NewRawConn(c)
	if err != nil {
		log.Fatal("openRawConnection() NewRawConn err:", err)
	}

	if err := raw.SetMulticastInterface(netIF); err != nil {
		log.Fatal(err)
	}

	if err := raw.SetMulticastTTL(igmpTTLCst); err != nil {
		log.Fatal("openRawConnection() SetMulticastTTL err:", err)
	}
	debugLog(r.debugLevel > 10, fmt.Sprintf("openRawConnection(%s) SetMulticastInterface and SetMulticastTTL:%d set", interf, igmpTTLCst))

	if r.conf.Testing.MulticastLoopback {
		if err := raw.SetMulticastLoopback(true); err != nil {
			log.Fatal("openRawConnection() SetMulticastLoopback err:", err)
		}
		debugLog(r.debugLevel > 10, fmt.Sprintf("openRawConnection(%s) SetMulticastLoopback set", interf))

	}

	return raw
}

// getInterfaceHandle takes name and returns a point to the interface struct, and the local IP
func (r IGMPReporter) getInterfaceHandle(interf side) (netIF *net.Interface, netIP net.IP, netaddr netip.Addr) {
	var (
		err error
	)

	debugLog(r.debugLevel > 100, fmt.Sprintf("getInterfaceHandle(%s)", interf))

	netIF, err = net.InterfaceByName(r.IntName[interf])
	if err != nil {
		log.Fatal(fmt.Sprintf("getInterfaceHandle(%s) InterfaceByName err:", r.IntName[interf]), err)
	}
	debugLog(r.debugLevel > 10, fmt.Sprintf("getInterfaceHandle(%s) netIF:%v", interf, netIF))

	addrs, err := netIF.Addrs()
	if err != nil {
		log.Fatal(fmt.Sprintf("getInterfaceHandle(%s) Addrs err:", interf), err)
	}

forLoop:
	for _, addr := range addrs {
		debugLog(r.debugLevel > 10, fmt.Sprintf("getInterfaceHandle(%s) addr:%s", interf, addr))
		switch v := addr.(type) {
		case *net.IPAddr:
			netIP = v.IP
			break forLoop
		case *net.IPNet:
			netIP = v.IP
			break forLoop
		default:
			debugLog(r.debugLevel > 10, fmt.Sprintf("getInterfaceHandle(%s) some strange addr:%s", interf, addr))
			continue
		}
	}

	debugLog(r.debugLevel > 10, fmt.Sprintf("getInterfaceHandle(%s) netIP:%s", interf, netIP))

	netaddr, err = r.netip2Addr(netIP)
	if err != nil {
		log.Fatal("getInterfaceHandle mapNetAddrtoIP err:", err)
	}

	return netIF, netIP, netaddr
}

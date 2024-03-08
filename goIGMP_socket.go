package goIGMP

import (
	"fmt"
	"log"
	"net"

	"golang.org/x/net/ipv4"
)

func (r IGMPReporter) openRawConnection() (c net.PacketConn, raw *ipv4.RawConn) {
	var (
		err error
	)

	debugLog(r.debugLevel > 10, "openRawConnection()")

	// inspired by https://godoc.org/golang.org/x/net/ipv4#example-RawConn--AdvertisingOSPFHello
	c, err = net.ListenPacket("ip4:2", "0.0.0.0") // IGMP
	if err != nil {
		log.Fatal("openRawConnection, err:", err)
	}

	raw, err = ipv4.NewRawConn(c)
	if err != nil {
		log.Fatal("selfQuery() NewRawConn err:", err)
	}

	if err := raw.SetMulticastLoopback(true); err != nil {
		log.Fatal("selfQuery() SetMulticastLoopback err:", err)
	}

	if err := raw.SetMulticastTTL(igmpTTLCst); err != nil {
		log.Fatal("selfQuery() SetMulticastTTL err:", err)
	}

	return c, raw
}

// getInterfaceHandle takes name and returns a point to the interface struct, and the local IP
func (r IGMPReporter) getInterfaceHandle(intName string) (netIF *net.Interface, netIP net.IP) {
	var (
		err error
	)

	netIF, err = net.InterfaceByName(intName)
	if err != nil {
		log.Fatal("getInterfaceHandle() InterfaceByName err:", err)
	}

	addrs, err := netIF.Addrs()
	if err != nil {
		log.Fatal("getInterfaceHandle(), err:", err)
	}
	for _, addr := range addrs {
		switch v := addr.(type) {
		case *net.IPAddr:
			netIP = v.IP
		case *net.IPNet:
			netIP = v.IP
		default:
			debugLog(r.debugLevel > 10, fmt.Sprint("getInterfaceHandle() some strange addr:", addr))
			continue
		}
	}

	return netIF, netIP
}

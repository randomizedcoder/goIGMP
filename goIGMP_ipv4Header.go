package goIGMP

import (
	"net"
	"sync"

	"golang.org/x/net/ipv4"
)

type multicastDestination int

const (
	dscpCst = 0xc0 // DSCP CS6 Network control
	//https://en.wikipedia.org/wiki/Differentiated_services

	ttlCst = 1

	allZerosQuad  = "0.0.0.0"
	allHostsQuad  = "224.0.0.1"
	IGMPHostsQuad = "224.0.0.22"

	allZerosHosts multicastDestination = 0  // 0.0.0.0
	allHosts      multicastDestination = 1  // 224.0.0.1
	IGMPHosts     multicastDestination = 22 // 224.0.0.22
	// https://en.wikipedia.org/wiki/Multicast_address
)

var (
	once                 sync.Once
	multicastNetIP       map[multicastDestination]net.IP
	igmpIPProtocolNumber = 2
)

func (r IGMPReporter) ipv4Header(payloadLength int, multicastDst multicastDestination) (iph *ipv4.Header) {

	once.Do(func() {
		setupMulticastNetIPMap()
	})

	iph = &ipv4.Header{
		Version:  ipv4.Version,
		Len:      ipv4.HeaderLen,
		TOS:      dscpCst,
		TotalLen: ipv4.HeaderLen + payloadLength,
		TTL:      ttlCst,
		Protocol: igmpIPProtocolNumber,
		Dst:      multicastNetIP[multicastDst],
		Options:  []byte{0x94, 0x04, 0x0, 0x0}, //router alert: https://tools.ietf.org/html/rfc2113
	}

	return iph
}

// Setup the multicast destination to Net.IP map
// This is the only place that we write to map[multicastDestination]net.IP
func setupMulticastNetIPMap() {
	multicastNetIP = make(map[multicastDestination]net.IP)

	multicastNetIP[allZerosHosts] = net.ParseIP(allZerosQuad).To4()
	multicastNetIP[allHosts] = net.ParseIP(allHostsQuad).To4()
	multicastNetIP[IGMPHosts] = net.ParseIP(IGMPHostsQuad).To4()
}

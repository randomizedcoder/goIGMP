package goIGMP

import (
	"golang.org/x/net/ipv4"
)

const (
	dscpCst = 0xc0 // DSCP CS6 Network control
	//https://en.wikipedia.org/wiki/Differentiated_services
	ttlCst               = 1
	igmpIPProtocolNumber = 2
)

func (r IGMPReporter) ipv4Header(payloadLength int, multicastDst destIP) (iph *ipv4.Header) {

	iph = &ipv4.Header{
		Version:  ipv4.Version,
		Len:      ipv4.HeaderLen,
		TOS:      dscpCst,
		TotalLen: ipv4.HeaderLen + payloadLength,
		TTL:      ttlCst,
		Protocol: igmpIPProtocolNumber,
		Dst:      r.mapIPtoNetIP[multicastDst],
		Options:  []byte{0x94, 0x04, 0x0, 0x0}, //router alert: https://tools.ietf.org/html/rfc2113
	}

	return iph
}

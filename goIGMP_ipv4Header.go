package goIGMP

import (
	"fmt"
	"log"
	"net"

	"golang.org/x/net/ipv4"
)

const (
	//https://en.wikipedia.org/wiki/Differentiated_services
	dscpCst              = 0xc0 // DSCP CS6 Network control
	ttlCst               = 1
	igmpIPProtocolNumber = 2
)

func (r IGMPReporter) ipv4Header(payloadLength int, dest destIP) (iph *ipv4.Header) {

	iph = &ipv4.Header{
		Version:  ipv4.Version,
		Len:      ipv4.HeaderLen,
		TOS:      dscpCst,
		TotalLen: ipv4.HeaderLen + payloadLength,
		TTL:      ttlCst,
		Protocol: igmpIPProtocolNumber,
		Dst:      r.destinationNetIP(dest),
		Options:  []byte{0x94, 0x04, 0x0, 0x0}, //router alert: https://tools.ietf.org/html/rfc2113
	}

	return iph
}

func (r IGMPReporter) ipv4HeaderNetIP(payloadLength int, dest net.IP) (iph *ipv4.Header) {

	iph = &ipv4.Header{
		Version:  ipv4.Version,
		Len:      ipv4.HeaderLen,
		TOS:      dscpCst,
		TotalLen: ipv4.HeaderLen + payloadLength,
		TTL:      ttlCst,
		Protocol: igmpIPProtocolNumber,
		Dst:      dest,
		Options:  []byte{0x94, 0x04, 0x0, 0x0}, //router alert: https://tools.ietf.org/html/rfc2113
	}

	return iph
}

// destinationNetIP returns the unicast destination of the querier in the case of QueryHost
// this really a hack to get it to send unicast
func (r IGMPReporter) destinationNetIP(dest destIP) (netIP net.IP) {
	if dest == QueryHost {
		debugLog(r.debugLevel > 10, fmt.Sprintf("destinationNetIP QueryHost r.querierSourceIP:%s", r.querierSourceIP.String()))
		var err error
		if !r.querierSourceIP.IsValid() {
			debugLog(r.debugLevel > 10, "destinationNetIP !r.querierSourceIP.IsValid(), using unicastDst")
			netIP, err = r.addr2NetIP(r.unicastDst)
			if err != nil {
				log.Fatal("destinationNetIP err")
			}
			return netIP
		}
		netIP, err = r.addr2NetIP(r.querierSourceIP)
		if err != nil {
			log.Fatal("destinationNetIP err")
		}
		debugLog(r.debugLevel > 10, "destinationNetIP using querierSourceIP.IsUnspecified")
		return netIP
	}

	netIP = r.mapIPtoNetIP[dest]

	return netIP
}

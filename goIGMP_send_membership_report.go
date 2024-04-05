package goIGMP

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"time"

	"github.com/randomizedcoder/gopacket"
	"github.com/randomizedcoder/gopacket/layers"
)

const (
	MaxResponseTimeCst = 10 * time.Second
)

func (r IGMPReporter) sendMembershipReport(interf side, membershipItems []MembershipItem) {
	// const (
	// 	// TODO this will be replaced by a dynamic list of the groups we are joined too
	// 	allowGroupAddressCst = "232.0.0.1"
	// )

	startTime := time.Now()
	defer func() {
		r.pH.WithLabelValues("sendMembershipReport", "start", "complete").Observe(time.Since(startTime).Seconds())
	}()
	r.pC.WithLabelValues("sendMembershipReport", "start", "count").Inc()

	debugLog(r.debugLevel > 10, fmt.Sprintf("sendMembershipReport(%s)", interf))

	for i, membershipItem := range membershipItems {

		debugLog(r.debugLevel > 10, fmt.Sprintf("sendMembershipReport(%s) i:%d, membershipItem:%v", interf, i, membershipItem))

		buffer := gopacket.NewSerializeBuffer()
		options := gopacket.SerializeOptions{
			ComputeChecksums: true,
			FixLengths:       true,
		}

		g, errN := addr2NetIP(membershipItem.Group)
		if errN != nil {
			log.Fatalf("addr2NetIP(%s) err:%v", membershipItem.Group, errN)
		}

		igmp := layers.IGMPv1or2{
			Type:            layers.IGMPMembershipReportV2,
			MaxResponseTime: MaxResponseTimeCst,
			GroupAddress:    g,
			Version:         2,
		}

		//err := gopacket.SerializeLayers(buffer, options, r.pbp.ethernetLayer, r.pbp.ipLayer, igmp)
		err := gopacket.SerializeLayers(buffer, options, igmp)
		if err != nil {
			log.Fatal(fmt.Sprintf("sendMembershipReport(%s) SerializeLayers err:", interf), err)
		}

		igmpPayload := buffer.Bytes()
		//iph := r.ipv4Header(len(igmpPayload), IGMPHosts)
		iph := r.ipv4Header(len(igmpPayload), g)

		var dest destIP
		if r.conf.UnicastMembershipReports {
			debugLog(r.debugLevel > 10, fmt.Sprintf("sendMembershipReport(%s) UnicastMembershipReports dest = QueryHost", interf))
			dest = QueryHost
		} else {
			dest = IGMPHosts
		}

		if r.debugLevel > 10 {
			debugLog(r.debugLevel > 10, fmt.Sprintf("sendMembershipReport(%s) iph:%v", interf, iph))
		}

		errSWD := r.conRaw[interf].SetWriteDeadline(time.Now().Add(writeDeadlineCst))
		if errSWD != nil {
			log.Fatal(fmt.Sprintf("sendMembershipReport(%s) SetWriteDeadline errSWD:", interf), errSWD)
		}
		if errW := r.conRaw[interf].WriteTo(iph, igmpPayload, r.ContMsg[interf]); errW != nil {
			log.Fatal(fmt.Sprintf("sendMembershipReport(%s) WriteTo errW:", interf), errW)
		}

		debugLog(r.debugLevel > 10, fmt.Sprintf("sendMembershipReport(%s) WriteTo success! len(igmpPayload):%d", interf, len(igmpPayload)))
	}

}

// netip2Addr
// https://djosephsen.github.io/posts/ipnet/
func netip2Addr(ip net.IP) (netip.Addr, error) {

	//debugLog(m.debugLevel > 100, fmt.Sprintf("netip2Addr() ip:%s, multicast:%t", ip.String(), ip.IsMulticast()))

	if addr, ok := netip.AddrFromSlice(ip); ok {
		return addr, nil
	}
	return netip.Addr{}, errors.New("invalid IP")
}

func addr2NetIP(addr netip.Addr) (net.IP, error) {
	if addr.IsValid() {
		return addr.AsSlice(), nil
	}
	return net.IP{}, errors.New("invalid ip")
}

// IGMPv3 stuff here

// https://github.com/randomizedcoder/gopacket/blob/master/layers/igmp.go#L162
//groupRecords := []IGMPv2GroupRecord{}

// for _, mItem := range membershipItems {
// 	debugLog(r.debugLevel > 10, fmt.Sprintf("NewIGMPReporter() mItem:%v", mItem))

// 	s, err := r.addr2NetIP(mItem.Source)
// 	if err != nil {
// 		log.Fatal("s addr2NetIP err:", err)
// 	}
// 	ss := []net.IP{s}

// 	g, err := r.addr2NetIP(mItem.Group)
// 	if err != nil {
// 		log.Fatal("g addr2NetIP err:", err)
// 	}

// 	groupRecord := IGMPv3GroupRecord{
// 		Type:             IGMPv3GroupRecordType(layers.IGMPAllow),
// 		NumberOfSources:  uint16(len(ss)),
// 		SourceAddresses:  ss,
// 		MulticastAddress: g,
// 		// AuxDataLen:       0, // this should always be 0 as per IGMPv3 spec.
// 		// AuxData:          0, // NOT USED
// 	}
// 	groupRecords = append(groupRecords, groupRecord)
// }

// // https://github.com/randomizedcoder/gopacket/blob/master/layers/igmp.go#L224
// // Except that we created our own IGMP to add serialize functions
// // TODO look at just adding serialize functions
// igmp := &IGMP{
// 	Type:                 layers.IGMPMembershipReportV3,
// 	Version:              3,
// 	GroupAddress:         r.mapIPtoNetIP[IGMPHosts],
// 	NumberOfGroupRecords: uint16(len(groupRecords)),
// 	GroupRecords:         groupRecords,
// }

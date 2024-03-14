package goIGMP

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var (
	bytePool = sync.Pool{
		New: func() interface{} {
			b := make([]byte, maxIGMPPacketRecieveBytesCst)
			return &b
		},
	}
)

func (r IGMPReporter) recvIGMP(wg *sync.WaitGroup, interf side, g destIP) {

	defer wg.Done()

	debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s started", interf, r.mapIPtoNetAddr[g]))

	for loops := 0; ; loops++ {

		debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d", interf, r.mapIPtoNetAddr[g], loops))

		err := r.mConIGMP[interf][r.mapIPtoNetAddr[g]].SetReadDeadline(time.Now().Add(readDeadlineCst))
		if err != nil {
			log.Fatal(fmt.Sprintf("recvIGMP(%s) g:%s loops:%d SetReadDeadline err:", interf, r.mapIPtoNetAddr[g], loops), err)
		}

		//buf := make([]byte, maxIGMPPacketRecieveBytesCst)
		buf := bytePool.Get().(*[]byte)
		n, cm, src, err := r.mConIGMP[interf][r.mapIPtoNetAddr[g]].ReadFrom(*buf)
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s ReadFrom timeout", interf, r.mapIPtoNetAddr[g]))
				continue
			}
		}

		//------------------
		// Validate incoming interface is correct
		// https://pkg.go.dev/golang.org/x/net/ipv4#ControlMessage
		// https://pkg.go.dev/net#Interface

		if r.NetIFIndex[cm.IfIndex] != interf {
			debugLog(r.debugLevel > 100, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d r.NetIFIndex[%d]:%s != interf:%s. Packet not for our interface. Ignoring",
				interf, r.mapIPtoNetAddr[g], loops, cm.IfIndex, r.NetIFIndex[cm.IfIndex], interf))
			continue
		}

		debugLog(r.debugLevel > 100, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d n:%d, cm:%s, src:%s", interf, r.mapIPtoNetAddr[g], loops, n, cm, src))

		if r.debugLevel > 100 {
			if !cm.Dst.IsMulticast() {
				debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d not multicast. ??! warning", interf, r.mapIPtoNetAddr[g], loops))
			}
		}

		//------------------
		// Validate destination IP is correct
		dstAddr, err := r.netip2Addr(cm.Dst)
		if err != nil {
			log.Fatal(fmt.Sprintf("recvIGMP(%s) g:%s loops:%d mapNetAddrtoIP err:", interf, r.mapIPtoNetAddr[g], loops), err)
		}

		if dstAddr != r.mapIPtoNetAddr[g] {
			debugLog(r.debugLevel > 100, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d Packet not for our multicast group. Ignoring", interf, r.mapIPtoNetAddr[g], loops))
			continue
		}

		//------------------
		// Validate this is IGMP and it's the correct type of IGMP

		// https://pkg.go.dev/github.com/tsg/gopacket#hdr-Basic_Usage
		// https://github.com/google/gopacket/blob/master/layers/igmp.go#L224
		packet := gopacket.NewPacket(*buf, layers.LayerTypeIGMP, gopacket.Default)

		igmpLayer := packet.Layer(layers.LayerTypeIGMP)
		if igmpLayer == nil {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d This isn't deserializing to IGMP.  Ignoring", interf, r.mapIPtoNetAddr[g], loops))
			continue
		}

		igmp, _ := igmpLayer.(*layers.IGMP)

		//debugLog(r.debugLevel > 10, fmt.Sprint(igmp.Type))

		igmpT, ok := r.mapIPtoIGMPType[g][igmp.Type]
		if !ok {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d Packet is not of a valid IGMP type for this group. Ignoring", interf, r.mapIPtoNetAddr[g], loops))
			continue
		}

		switch igmpT {

		case igmpTypeQuery:
			if r.conf.QueryNotify {
				select {
				case r.QueryNotifyCh <- struct{}{}:
					debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d QueryNotifyCh <- struct{}{}", interf, r.mapIPtoNetAddr[g], loops))
				default:
					debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d QueryNotifyCh failed.  Channel full?  Is something reading from the channel?", interf, r.mapIPtoNetAddr[g], loops))
				}
			}
		case igmpTypeMembershipReport:

			mitems := r.groupRecordsToMembershipItem(igmp.GroupRecords)

			if r.conf.MembershipReportsFromNetwork {
				select {
				case r.MembershipReportFromNetworkCh <- mitems:
					debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d MembershipReportFromNetworkCh", interf, r.mapIPtoNetAddr[g], loops))
				default:
					debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d MembershipReportFromNetworkCh failed.  Channel full?  Is something reading from the channel?", interf, r.mapIPtoNetAddr[g], loops))
				}
			}

		default:
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d This shouldn't happen.  Bug?", interf, r.mapIPtoNetAddr[g], loops))
		}

		if r.proxyIt(interf) {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d proxying to:%s", interf, r.mapIPtoNetAddr[g], loops, r.IntOutName[interf]))

			// We actually just send buf completely
			r.proxy(r.IntOutName[interf], g, buf)

			bytePool.Put(buf)
		}

	}
}

func (r IGMPReporter) proxyIt(interf side) (proxyIt bool) {
	switch interf {
	case OUT:
		if r.conf.ProxyOutToIn {
			proxyIt = true
		}
	case IN:
		if r.conf.ProxyInToOut {
			proxyIt = true
		}
	}
	return proxyIt
}

func (r IGMPReporter) groupRecordsToMembershipItem(groupRecords []layers.IGMPv3GroupRecord) (mitems []membershipItem) {

	debugLog(r.debugLevel > 100, "groupRecordsToMembershipItem()")

	for _, gr := range groupRecords {
		var g membershipItem
		for _, sa := range gr.SourceAddresses {
			na, err := r.netip2Addr(sa)
			if err != nil {
				log.Fatal("recvIGMP netip2Addr(sa) err", err)
			}
			g.Sources = append(g.Sources, na)
		}

		na, err := r.netip2Addr(gr.MulticastAddress)
		if err != nil {
			log.Fatal("recvIGMP netip2Addr(sa) err", err)
		}
		g.Group = na

		mitems = append(mitems, g)
	}

	debugLog(r.debugLevel > 10, fmt.Sprintf("groupRecordsToMembershipItem() len(mitems):%d", len(mitems)))

	return mitems
}

func (r IGMPReporter) recvUnicastIGMP(wg *sync.WaitGroup, interf side) {
	var (
		//err     error
		localIP netip.Addr
		ok      bool
	)

	defer wg.Done()

	if localIP, ok = r.NetAddr[interf]; !ok {
		log.Fatalf("recvUnicastIGMP(%s) interface IP lookup error", interf)
	}

	debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s started", interf, localIP))

	for loops := 0; ; loops++ {

		debugLog(r.debugLevel > 100, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d", interf, localIP, loops))

		err := r.uCon[IN].SetReadDeadline(time.Now().Add(readDeadlineCst))
		if err != nil {
			log.Fatal(fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s  loops:%d SetReadDeadline err:", interf, localIP, loops), err)
		}

		//buf := make([]byte, maxIGMPPacketRecieveBytesCst)
		buf := bytePool.Get().(*[]byte)
		n, addr, err := r.uCon[IN].ReadFrom(*buf)
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s ReadFrom timeout", interf, localIP))
				continue
			}
		}

		debugLog(r.debugLevel > 100, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d n:%d, addr:%s", interf, localIP, loops, n, addr))

		//------------------
		// Validate this is IGMP and it's the correct type of IGMP

		// https://pkg.go.dev/github.com/tsg/gopacket#hdr-Basic_Usage
		// https://github.com/google/gopacket/blob/master/layers/igmp.go#L224
		packet := gopacket.NewPacket(*buf, layers.LayerTypeIGMP, gopacket.Default)

		igmpLayer := packet.Layer(layers.LayerTypeIGMP)
		if igmpLayer == nil {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d This isn't deserializing to IGMP.  Ignoring", interf, localIP, loops))
			continue
		}

		igmp, _ := igmpLayer.(*layers.IGMP)

		_, ok := r.mapUnicastIGMPTypes[igmp.Type]
		if !ok {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d Packet is not of a valid IGMP type for this interface. Ingnoring", interf, localIP, loops))
			continue
		}

		debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d proxying to:%s", interf, localIP, loops, r.IntOutName[IN]))

		// We actually just send buf completely
		r.proxy(r.IntOutName[interf], IGMPHosts, buf)

		bytePool.Put(buf)

	}
}

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

		loopStartTime := time.Now()
		r.pCrecvIGMP.WithLabelValues("loop", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Inc()

		debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d", interf, r.mapIPtoNetAddr[g], loops))

		err := r.mConIGMP[interf][r.mapIPtoNetAddr[g]].SetReadDeadline(time.Now().Add(readDeadlineCst))
		if err != nil {
			log.Fatal(fmt.Sprintf("recvIGMP(%s) g:%s loops:%d SetReadDeadline err:", interf, r.mapIPtoNetAddr[g], loops), err)
		}

		buf := bytePool.Get().(*[]byte)
		n, cm, src, err := r.mConIGMP[interf][r.mapIPtoNetAddr[g]].ReadFrom(*buf)
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s ReadFrom timeout", interf, r.mapIPtoNetAddr[g]))
				r.pCrecvIGMP.WithLabelValues("timeout", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Inc()
				continue
			}
			r.pCrecvIGMP.WithLabelValues("ReadFrom", interf.String(), r.mapIPtoNetAddr[g].String(), "error").Inc()
		}
		packetStartTime := time.Now()
		r.pCrecvIGMP.WithLabelValues("n", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Add(float64(n))

		//------------------
		// Validate incoming interface is correct
		// https://pkg.go.dev/golang.org/x/net/ipv4#ControlMessage
		// https://pkg.go.dev/net#Interface

		if r.NetIFIndex[cm.IfIndex] != interf {
			debugLog(r.debugLevel > 100, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d r.NetIFIndex[%d]:%s != interf:%s. Packet not for our interface. Ignoring",
				interf, r.mapIPtoNetAddr[g], loops, cm.IfIndex, r.NetIFIndex[cm.IfIndex], interf))
			r.pCrecvIGMP.WithLabelValues("interf", interf.String(), r.mapIPtoNetAddr[g].String(), "ignore").Inc()
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
			r.pCrecvIGMP.WithLabelValues("netip2Addr", interf.String(), r.mapIPtoNetAddr[g].String(), "error").Inc()
		}

		if dstAddr != r.mapIPtoNetAddr[g] {
			debugLog(r.debugLevel > 100, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d Packet not for our multicast group. Ignoring", interf, r.mapIPtoNetAddr[g], loops))
			r.pCrecvIGMP.WithLabelValues("dstAddr", interf.String(), r.mapIPtoNetAddr[g].String(), "ignore").Inc()
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
			r.pCrecvIGMP.WithLabelValues("deserializing", interf.String(), r.mapIPtoNetAddr[g].String(), "error").Inc()
			continue
		}

		igmp, _ := igmpLayer.(*layers.IGMP)

		//debugLog(r.debugLevel > 10, fmt.Sprint(igmp.Type))

		igmpT, ok := r.mapIPtoIGMPType[g][igmp.Type]
		if !ok {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d Packet is not of a valid IGMP type for this group. Ignoring", interf, r.mapIPtoNetAddr[g], loops))
			r.pCrecvIGMP.WithLabelValues("igmpT", interf.String(), r.mapIPtoNetAddr[g].String(), "error").Inc()
			continue
		}

		switch igmpT {

		case igmpTypeQuery:
			r.pCrecvIGMP.WithLabelValues("igmpTypeQuery", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Inc()

			srcIP, err := r.netip2Addr(cm.Src)
			if err != nil {
				r.pCrecvIGMP.WithLabelValues("srcNetip2Addr", interf.String(), r.mapIPtoNetAddr[g].String(), "error").Inc()
			}
			r.querierSourceIP = srcIP

			if r.conf.QueryNotify {
				select {
				case r.QueryNotifyCh <- struct{}{}:
					r.pCrecvIGMP.WithLabelValues("QueryNotifyCh", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Inc()
					debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d QueryNotifyCh <- struct{}{}", interf, r.mapIPtoNetAddr[g], loops))
				default:
					r.pCrecvIGMP.WithLabelValues("QueryNotifyCh", interf.String(), r.mapIPtoNetAddr[g].String(), "error").Inc()
					debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d QueryNotifyCh failed.  Channel full?  Is something reading from the channel?", interf, r.mapIPtoNetAddr[g], loops))
				}
			}
		case igmpTypeMembershipReport:
			r.pCrecvIGMP.WithLabelValues("igmpTypeMembershipReport", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Inc()

			mitems := r.groupRecordsToMembershipItem(igmp.GroupRecords)

			if r.conf.MembershipReportsFromNetwork {
				select {
				case r.MembershipReportFromNetworkCh <- mitems:
					r.pCrecvIGMP.WithLabelValues("MembershipReportFromNetworkCh", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Inc()
					debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d MembershipReportFromNetworkCh", interf, r.mapIPtoNetAddr[g], loops))
				default:
					r.pCrecvIGMP.WithLabelValues("MembershipReportFromNetworkCh", interf.String(), r.mapIPtoNetAddr[g].String(), "error").Inc()
					debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d MembershipReportFromNetworkCh failed.  Channel full?  Is something reading from the channel?", interf, r.mapIPtoNetAddr[g], loops))
				}
			}

		default:
			r.pCrecvIGMP.WithLabelValues("WrongType", interf.String(), r.mapIPtoNetAddr[g].String(), "error").Inc()
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d This shouldn't happen.  Bug?", interf, r.mapIPtoNetAddr[g], loops))
		}

		if r.proxyIt(interf) {
			r.pCrecvIGMP.WithLabelValues("proxyIt", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Inc()
			r.pCrecvIGMP.WithLabelValues("proxyIt", interf.String(), r.mapIPtoNetAddr[g].String(), "bytes").Add(float64(len(*buf)))
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d proxying to:%s", interf, r.mapIPtoNetAddr[g], loops, r.IntOutName[interf]))

			// We actually just send buf completely
			r.proxy(r.IntOutName[interf], g, buf)

			bytePool.Put(buf)
		}

		r.pHrecvIGMP.WithLabelValues("sincePacketStartTime", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Observe(time.Since(packetStartTime).Seconds())
		r.pHrecvIGMP.WithLabelValues("sinceLoopStartTime", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Observe(time.Since(loopStartTime).Seconds())

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

	startTime := time.Now()
	defer func() {
		r.pH.WithLabelValues("groupRecordsToMembershipItem", "start", "complete").Observe(time.Since(startTime).Seconds())
	}()
	r.pC.WithLabelValues("groupRecordsToMembershipItem", "start", "count").Inc()

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

		loopStartTime := time.Now()
		r.pC.WithLabelValues("recvUnicastIGMP", "loops", "counter").Inc()

		debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d", interf, localIP, loops))

		err := r.uCon[IN].SetReadDeadline(time.Now().Add(readDeadlineCst))
		if err != nil {
			log.Fatal(fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s  loops:%d SetReadDeadline err:", interf, localIP, loops), err)
		}

		buf := bytePool.Get().(*[]byte)
		n, addr, err := r.uCon[IN].ReadFrom(*buf)
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s ReadFrom timeout", interf, localIP))
				r.pC.WithLabelValues("recvUnicastIGMP", "timeout", "counter").Inc()
				continue
			}
		}
		packetStartTime := time.Now()

		debugLog(r.debugLevel > 100, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d n:%d, addr:%s", interf, localIP, loops, n, addr))
		r.pC.WithLabelValues("recvUnicastIGMP", "n", "counter").Add(float64(n))

		//------------------
		// Validate this is IGMP and it's the correct type of IGMP

		// https://pkg.go.dev/github.com/tsg/gopacket#hdr-Basic_Usage
		// https://github.com/google/gopacket/blob/master/layers/igmp.go#L224
		packet := gopacket.NewPacket(*buf, layers.LayerTypeIGMP, gopacket.Default)

		igmpLayer := packet.Layer(layers.LayerTypeIGMP)
		if igmpLayer == nil {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d This isn't deserializing to IGMP.  Ignoring", interf, localIP, loops))
			r.pC.WithLabelValues("recvUnicastIGMP", "deserializing", "error").Inc()
			continue
		}

		igmp, _ := igmpLayer.(*layers.IGMP)

		_, ok := r.mapUnicastIGMPTypes[igmp.Type]
		if !ok {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d Packet is not of a valid IGMP type for this interface. Ingnoring", interf, localIP, loops))
			r.pC.WithLabelValues("recvUnicastIGMP", "igmpType", "error").Inc()
			continue
		}

		debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d n:%d, addr:%s proxying to:%s", interf, localIP, loops, n, addr, r.IntOutName[IN]))

		var dest destIP
		if r.conf.UnicastMembershipReports {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d UnicastMembershipReports dest = QueryHost", interf, localIP, loops))
			dest = QueryHost
		} else {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d dest = IGMPHosts", interf, localIP, loops))
			dest = IGMPHosts
		}
		// We actually just send buf completely
		r.proxy(r.IntOutName[interf], dest, buf)

		bytePool.Put(buf)

		r.pH.WithLabelValues("recvUnicastIGMP", "sincePacketStartTime", "counter").Observe(time.Since(packetStartTime).Seconds())
		r.pH.WithLabelValues("recvUnicastIGMP", "sinceLoopStartTime", "counter").Observe(time.Since(loopStartTime).Seconds())

	}
}

package goIGMP

import (
	"context"
	"fmt"
	"log"
	"net"
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

func (r IGMPReporter) recvIGMP(wg *sync.WaitGroup, ctx context.Context, interf side, g destIP) {

	defer wg.Done()

	debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s started", interf, r.mapIPtoNetAddr[g]))

forLoop:
	for loops := 0; ; loops++ {

		select {
		case <-ctx.Done():
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d ctx.Done()", interf, r.mapIPtoNetAddr[g], loops))
			break forLoop
		default:
			debugLog(r.debugLevel > 100, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d ctx is not cancelled", interf, r.mapIPtoNetAddr[g], loops))
		}

		loopStartTime := time.Now()
		r.pCrecvIGMP.WithLabelValues("loop", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Inc()

		debugLog(r.debugLevel > 100, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d", interf, r.mapIPtoNetAddr[g], loops))

		err := r.mConIGMP[interf][r.mapIPtoNetAddr[g]].SetReadDeadline(time.Now().Add(r.conf.SocketReadDeadLine))
		if err != nil {
			log.Fatal(fmt.Sprintf("recvIGMP(%s) g:%s loops:%d SetReadDeadline err:", interf, r.mapIPtoNetAddr[g], loops), err)
		}

		buf := bytePool.Get().(*[]byte)
		n, cm, src, err := r.mConIGMP[interf][r.mapIPtoNetAddr[g]].ReadFrom(*buf)
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s ReadFrom timeout", interf, r.mapIPtoNetAddr[g]))
				r.pCrecvIGMP.WithLabelValues("timeout", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Inc()
				bytePool.Put(buf)
				continue
			}
			r.pCrecvIGMP.WithLabelValues("ReadFrom", interf.String(), r.mapIPtoNetAddr[g].String(), "error").Inc()
			bytePool.Put(buf)
			continue
		}
		packetStartTime := time.Now()
		r.pCrecvIGMP.WithLabelValues("n", interf.String(), r.mapIPtoNetAddr[g].String(), "counter").Add(float64(n))

		//------------------
		// Ignore traffic on the non-active outside interface
		if r.AltOutExists {
			if r.ignoreOnNonActiveOutOrAltInterface(&interf) {
				debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d ignoring on non active outside interface", interf, r.mapIPtoNetAddr[g], loops))
				bytePool.Put(buf)
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
			r.pCrecvIGMP.WithLabelValues("interf", interf.String(), r.mapIPtoNetAddr[g].String(), "ignore").Inc()
			bytePool.Put(buf)
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
			bytePool.Put(buf)
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
			bytePool.Put(buf)
			continue
		}

		igmp, _ := igmpLayer.(*layers.IGMP)

		//debugLog(r.debugLevel > 10, fmt.Sprint(igmp.Type))

		igmpT, ok := r.mapIPtoIGMPType[g][igmp.Type]
		if !ok {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d Packet is not of a valid IGMP type for this group. Ignoring", interf, r.mapIPtoNetAddr[g], loops))
			r.pCrecvIGMP.WithLabelValues("igmpT", interf.String(), r.mapIPtoNetAddr[g].String(), "error").Inc()
			bytePool.Put(buf)
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

			out, ok := r.IntOutName.Load(interf)
			if !ok {
				debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) Load !ok", interf))
				r.pC.WithLabelValues("recvIGMP", "Load", "error").Inc()
				bytePool.Put(buf)
				continue
			}
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) g:%s loops:%d proxying to:%s", interf, r.mapIPtoNetAddr[g], loops, out.(side)))

			r.proxy(out.(side), g, buf)

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

func (r IGMPReporter) ignoreOnNonActiveOutOrAltInterface(interf *side) (ignore bool) {
	if r.OutsideInterfaces[*interf] {

		out, ok := r.IntOutName.Load(IN)
		if !ok {
			debugLog(r.debugLevel > 10, fmt.Sprintf("ignoreOnNonActiveOutOrAltInterface(%s) Load !ok", *interf))
			r.pC.WithLabelValues("ignoreOnNonActiveOutOrAltInterface", "Load", "error").Inc()
			ignore = true
			return ignore
		}

		if *interf != out.(side) {
			ignore = true
			r.pC.WithLabelValues("ignoreOnNonActiveOutOrAltInterface", "ignore", "count").Inc()
			debugLog(r.debugLevel > 100, fmt.Sprintf("ignoreOnNonActiveOutOrAltInterface(%s) ignoring non-active outside interface", *interf))
		}
	}

	return ignore
}

// groupRecordsToMembershipItem converts the real IGMP packet group memberships
// into the internal representatino as a list of []membershipItem
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

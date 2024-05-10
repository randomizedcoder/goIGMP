package goIGMP

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/randomizedcoder/gopacket"
	"github.com/randomizedcoder/gopacket/layers"
)

func (r IGMPReporter) recvUnicastIGMP(wg *sync.WaitGroup, ctx context.Context, interf side) {
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

forLoop:
	for loops := 0; ; loops++ {

		select {
		case <-ctx.Done():
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP(%s) loops:%d ctx.Done()", interf, loops))
			break forLoop
		default:
			debugLog(r.debugLevel > 1000, fmt.Sprintf("recvIGMP(%s) loops:%d ctx is not cancelled", interf, loops))
		}

		loopStartTime := time.Now()
		r.pC.WithLabelValues("recvUnicastIGMP", "loops", "counter").Inc()

		debugLog(r.debugLevel > 1000, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d", interf, localIP, loops))

		err := r.uCon[IN].SetReadDeadline(time.Now().Add(r.conf.SocketReadDeadLine))
		if err != nil {
			log.Fatal(fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d SetReadDeadline err:", interf, localIP, loops), err)
		}

		buf := bytePool.Get().(*[]byte)
		n, addr, err := r.uCon[IN].ReadFrom(*buf)
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d ReadFrom timeout", interf, localIP, loops))
				r.pC.WithLabelValues("recvUnicastIGMP", "timeout", "counter").Inc()
				bytePool.Put(buf)
				continue
			}
		}
		packetStartTime := time.Now()

		debugLog(r.debugLevel > 1000, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d n:%d, addr:%s", interf, localIP, loops, n, addr))
		r.pC.WithLabelValues("recvUnicastIGMP", "n", "counter").Add(float64(n))

		//------------------
		// Validate this is IGMP and it's the correct type of IGMP

		// type IGMPType uint8

		// const (
		// 	IGMPMembershipQuery    IGMPType = 0x11 // General or group specific query
		// 	IGMPMembershipReportV1 IGMPType = 0x12 // Version 1 Membership Report
		// 	IGMPMembershipReportV2 IGMPType = 0x16 // Version 2 Membership Report
		// 	IGMPLeaveGroup         IGMPType = 0x17 // Leave Group
		// 	IGMPMembershipReportV3 IGMPType = 0x22 // Version 3 Membership Report
		// )
		// https://github.com/randomizedcoder/gopacket/blob/master/layers/igmp.go#L18C1-L27C2

		igmpType := layers.IGMPType((*buf)[0])
		debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d type:%s", interf, localIP, loops, igmpType))
		r.pC.WithLabelValues("recvUnicastIGMP", igmpType.String(), "count").Inc()

		// https://pkg.go.dev/github.com/tsg/gopacket#hdr-Basic_Usage
		// https://github.com/randomizedcoder/gopacket/blob/master/layers/igmp.go#L224
		packet := gopacket.NewPacket(*buf, layers.LayerTypeIGMP, gopacket.Default)

		igmpLayer := packet.Layer(layers.LayerTypeIGMP)
		if igmpLayer == nil {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d This isn't deserializing to IGMP.  Ignoring", interf, localIP, loops))
			r.pC.WithLabelValues("recvUnicastIGMP", "deserializing", "error").Inc()
			bytePool.Put(buf)
			continue
		}

		// outside interface can change between ethernet/GRE
		o, ok := r.IntOutName.Load(interf)
		if !ok {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) Load !ok", interf))
			r.pC.WithLabelValues("recvUnicastIGMP", "Load", "error").Inc()
			bytePool.Put(buf)
			continue
		}
		out, ok := o.(side)
		if !ok {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d o.(side) type cast error", interf, localIP, loops))
			r.pC.WithLabelValues("recvUnicastIGMP", "typeCast", "error").Inc()
			continue
		}

		// For type1/2 we need to decode to find the group address
		switch igmpType {

		//case layers.IGMPMembershipQuery:
		//TODO implment this

		case layers.IGMPMembershipReportV1:
			r.sendIGMPv1or2(interf, loops, out, igmpLayer, buf)

		case layers.IGMPMembershipReportV2:
			r.sendIGMPv1or2(interf, loops, out, igmpLayer, buf)

		case layers.IGMPMembershipReportV3:
			r.sendIGMPv3(interf, loops, out, buf)

		case layers.IGMPLeaveGroup:
			r.sendIGMPLeave(interf, loops, out, buf)

		default:
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d unexpected igmp.Type", interf, localIP, loops))
			r.pC.WithLabelValues("recvUnicastIGMP", "unexpectedIgmpType", "error").Inc()
		}

		r.pH.WithLabelValues("recvUnicastIGMP", "sincePacketStartTime", "counter").Observe(time.Since(packetStartTime).Seconds())
		r.pH.WithLabelValues("recvUnicastIGMP", "sinceLoopStartTime", "counter").Observe(time.Since(loopStartTime).Seconds())

	}
}

// sendIGMPv1or2 needs to send to the multicast destination, so it decodes the payload to find the group
func (r IGMPReporter) sendIGMPv1or2(interf side, loops int, out side, igmpLayer gopacket.Layer, buf *[]byte) {

	igmpv1or2, ok := igmpLayer.(*layers.IGMPv1or2)
	if !ok {
		debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) loops:%d sendIGMPv1or2 igmpLayer.(*layers.IGMPv1or2) type cast error", interf, loops))
	}

	debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) loops:%d sendIGMPv1or2 proxyUniToMultiv1or2 to:%s", interf, loops, out))

	r.proxyUniToMultiv1or2(out, igmpv1or2.GroupAddress, buf)
}

// sendIGMPv3 is more simple, and just sends to the IGMPv3 destination 224.0.0.22
func (r IGMPReporter) sendIGMPv3(interf side, loops int, out side, buf *[]byte) {

	var dest destIP
	if r.conf.UnicastMembershipReports {
		debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) loops:%d sendIGMPv3 UnicastMembershipReports dest = QueryHost", interf, loops))
		dest = QueryHost
	} else {
		debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) loops:%d sendIGMPv3 dest = IGMPHosts", interf, loops))
		dest = IGMPHosts
	}

	debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) loops:%d sendIGMPv3 proxying to:%s", interf, loops, out))

	r.proxy(out, dest, buf)

	bytePool.Put(buf)

}

// sendIGMPv1or2 needs to send to the multicast destination, so it decodes the payload to find the group
func (r IGMPReporter) sendIGMPLeave(interf side, loops int, out side, buf *[]byte) {

	debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) loops:%d sendIGMPLeave proxyUniToMultiv1or2 to:%s", interf, loops, out))

	r.proxyUniToMultiv1or2(out, net.IPv4allrouter, buf)
}

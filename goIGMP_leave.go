package goIGMP

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/randomizedcoder/gopacket"
	"github.com/randomizedcoder/gopacket/layers"
)

func (r IGMPReporter) leaveToNetworkWorker(wg *sync.WaitGroup, ctx context.Context) {

	defer wg.Done()

	debugLog(r.debugLevel > 10, "leaveToNetworkWorker() start")

forLoop:
	for loops := 0; ; loops++ {

		startTime := time.Now()
		r.pC.WithLabelValues("leaveToNetworkWorker", "loops", "count").Inc()

		var groups []MembershipItem
		select {

		case groups = <-r.LeaveToNetworkCh:
			debugLog(r.debugLevel > 10, fmt.Sprintf("leaveToNetworkWorker() loops:%d groups:%v", loops, groups))

		case <-ctx.Done():
			debugLog(r.debugLevel > 10, "leaveToNetworkWorker ctx.Done()")
			break forLoop

		}

		r.sendLeave(OUT, groups)

		r.pH.WithLabelValues("leaveToNetworkWorker", "loop", "complete").Observe(time.Since(startTime).Seconds())
	}

	debugLog(r.debugLevel > 10, "leaveToNetworkWorker() complete")
}

func (r IGMPReporter) sendLeave(interf side, membershipItems []MembershipItem) {

	startTime := time.Now()
	defer func() {
		r.pH.WithLabelValues("sendLeave", "start", "complete").Observe(time.Since(startTime).Seconds())
	}()
	r.pC.WithLabelValues("sendLeave", "start", "count").Inc()

	debugLog(r.debugLevel > 10, fmt.Sprintf("sendLeave(%s)", interf))

	for i, membershipItem := range membershipItems {

		debugLog(r.debugLevel > 10, fmt.Sprintf("sendLeave(%s) i:%d, membershipItem:%v", interf, i, membershipItem))

		buffer := gopacket.NewSerializeBuffer()
		options := gopacket.SerializeOptions{
			ComputeChecksums: true,
			FixLengths:       true,
		}

		g, errN := addr2NetIP(membershipItem.Group)
		if errN != nil {
			log.Fatalf("sendLeave(%s) addr2NetIP(%s) err:%v", interf, membershipItem.Group, errN)
		}

		igmp := layers.IGMPv1or2{
			Type:            layers.IGMPLeaveGroup,
			MaxResponseTime: MaxResponseTimeCst,
			GroupAddress:    g,
			Version:         2,
		}

		//err := gopacket.SerializeLayers(buffer, options, r.pbp.ethernetLayer, r.pbp.ipLayer, igmp)
		err := gopacket.SerializeLayers(buffer, options, &igmp)
		if err != nil {
			log.Fatal(fmt.Sprintf("sendLeave(%s) SerializeLayers err:", interf), err)
		}

		igmpPayload := buffer.Bytes()

		var dest destIP
		if r.conf.UnicastMembershipReports {
			debugLog(r.debugLevel > 10, fmt.Sprintf("sendLeave(%s) UnicastMembershipReports dest = QueryHost", interf))
			dest = QueryHost
		} else {
			dest = allRouters
		}

		iph := r.ipv4Header(len(igmpPayload), dest)

		if r.debugLevel > 10 {
			debugLog(r.debugLevel > 10, fmt.Sprintf("sendLeave(%s) iph:%v", interf, iph))
		}

		errSWD := r.conRaw[interf].SetWriteDeadline(time.Now().Add(writeDeadlineCst))
		if errSWD != nil {
			log.Fatal(fmt.Sprintf("sendLeave(%s) SetWriteDeadline errSWD:", interf), errSWD)
		}
		if errW := r.conRaw[interf].WriteTo(iph, igmpPayload, r.ContMsg[interf]); errW != nil {
			log.Fatal(fmt.Sprintf("sendLeave(%s) WriteTo errW:", interf), errW)
		}

		debugLog(r.debugLevel > 10, fmt.Sprintf("sendLeave(%s) WriteTo success! len(igmpPayload):%d", interf, len(igmpPayload)))
	}

}

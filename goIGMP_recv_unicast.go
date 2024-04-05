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
			debugLog(r.debugLevel > 100, fmt.Sprintf("recvIGMP(%s) loops:%d ctx is not cancelled", interf, loops))
		}

		loopStartTime := time.Now()
		r.pC.WithLabelValues("recvUnicastIGMP", "loops", "counter").Inc()

		debugLog(r.debugLevel > 100, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d", interf, localIP, loops))

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

		debugLog(r.debugLevel > 100, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d n:%d, addr:%s", interf, localIP, loops, n, addr))
		r.pC.WithLabelValues("recvUnicastIGMP", "n", "counter").Add(float64(n))

		//------------------
		// Validate this is IGMP and it's the correct type of IGMP

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

		igmp, _ := igmpLayer.(*layers.IGMP)

		_, ok := r.mapUnicastIGMPTypes[igmp.Type]
		if !ok {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d Packet is not of a valid IGMP type for this interface. Ingnoring", interf, localIP, loops))
			r.pC.WithLabelValues("recvUnicastIGMP", "igmpType", "error").Inc()
			bytePool.Put(buf)
			continue
		}

		var dest destIP
		if r.conf.UnicastMembershipReports {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d UnicastMembershipReports dest = QueryHost", interf, localIP, loops))
			dest = QueryHost
		} else {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d dest = IGMPHosts", interf, localIP, loops))
			dest = IGMPHosts
		}

		out, ok := r.IntOutName.Load(interf)
		if !ok {
			debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) Load !ok", interf))
			r.pC.WithLabelValues("recvUnicastIGMP", "Load", "error").Inc()
			bytePool.Put(buf)
			continue
		}
		debugLog(r.debugLevel > 10, fmt.Sprintf("recvUnicastIGMP(%s) localIP:%s loops:%d proxying to:%s", interf, localIP, loops, out.(side)))

		r.proxy(out.(side), dest, buf)

		bytePool.Put(buf)

		r.pH.WithLabelValues("recvUnicastIGMP", "sincePacketStartTime", "counter").Observe(time.Since(packetStartTime).Seconds())
		r.pH.WithLabelValues("recvUnicastIGMP", "sinceLoopStartTime", "counter").Observe(time.Since(loopStartTime).Seconds())

	}
}

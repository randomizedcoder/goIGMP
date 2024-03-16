package goIGMP

import (
	"fmt"
	"log"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// See also
// https://pkg.go.dev/golang.org/x/net@v0.22.0/ipv4#RawConn
// https://pkg.go.dev/golang.org/x/net@v0.22.0/ipv4#example-RawConn-AdvertisingOSPFHello

func (r IGMPReporter) selfQuery(interf side) {
	const (
		minQueryDurationCst         = 1 * time.Second
		igmpQueryMaxResponseTimeCst = 10 * time.Second
	)

	startTime := time.Now()
	defer func() {
		r.pH.WithLabelValues("selfQuery", "start", "complete").Observe(time.Since(startTime).Seconds())
	}()
	r.pC.WithLabelValues("selfQuery", "start", "count").Inc()

	debugLog(r.debugLevel > 10, fmt.Sprintf("selfQuery(%s) - don't do this at home folks", interf))

	if r.TimerDuration[QUERY] < minQueryDurationCst {
		debugLog(r.debugLevel > 10, fmt.Sprintf("selfQuery(%s) - queryTime < minQueryDurationCst", interf))
		return
	}

	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	// https://github.com/google/gopacket/blob/master/layers/igmp.go#L224
	// Except that we created our own IGMP to add serialize functions
	// TODO look at just adding serialize functions
	igmp := &IGMP{
		Type:         layers.IGMPMembershipQuery,
		Version:      3,
		GroupAddress: r.mapIPtoNetIP[allZerosHosts],
		//GroupAddress:    net.ParseIP("232.1.1.1"), There is a bug.  This turns out as 232.1.0.0 currently
		MaxResponseTime: igmpQueryMaxResponseTimeCst,
	}

	err := gopacket.SerializeLayers(buffer, options, igmp)
	if err != nil {
		log.Fatal(fmt.Sprintf("selfQuery(%s) SerializeLayers err:", interf), err)
	}

	igmpPayload := buffer.Bytes()
	iph := r.ipv4Header(len(igmpPayload), IGMPHosts)

	t := time.NewTicker(r.TimerDuration[QUERY])
	defer t.Stop()

	for loops := 0; ; loops++ {

		loopStartTime := time.Now()
		r.pC.WithLabelValues("selfQuery", "loops", "count").Inc()

		debugLog(r.debugLevel > 10, fmt.Sprintf("selfQuery(%s) loops:%d", interf, loops))

		<-t.C

		debugLog(r.debugLevel > 10, fmt.Sprintf("selfQuery(%s) tick loops:%d", interf, loops))

		err := r.conRaw[interf].SetWriteDeadline(time.Now().Add(writeDeadlineCst))
		if err != nil {
			log.Fatal(fmt.Sprintf("selfQuery(%s) SetWriteDeadline err:", interf), err)
		}

		if err := r.conRaw[interf].WriteTo(iph, igmpPayload, r.ContMsg[interf]); err != nil {
			log.Fatal(fmt.Sprintf("selfQuery(%s) WriteTo err:", interf), err)
		}

		debugLog(r.debugLevel > 10, fmt.Sprintf("selfQuery(%s) - WriteTo success, len(igmpPayload):%d", interf, len(igmpPayload)))
		r.pH.WithLabelValues("selfQuery", "loopStartTime", "complete").Observe(time.Since(loopStartTime).Seconds())
	}
}

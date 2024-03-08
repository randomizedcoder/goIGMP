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

func (r IGMPReporter) selfQuery() {
	const (
		minQueryDurationCst         = 1 * time.Second
		igmpQueryMaxResponseTimeCst = 10 * time.Second
	)

	debugLog(r.debugLevel > 10, "selfQuery() - don't do this at home folks")

	if r.queryTime < minQueryDurationCst {
		debugLog(r.debugLevel > 10, "selfQuery() - queryTime < minQueryDurationCst")
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
		GroupAddress: multicastNetIP[allZerosHosts],
		//GroupAddress:    net.ParseIP("232.1.1.1"), There is a bug.  This turns out as 232.1.0.0 currently
		MaxResponseTime: igmpQueryMaxResponseTimeCst,
	}

	err := gopacket.SerializeLayers(buffer, options, igmp)
	if err != nil {
		log.Fatal("sendMembershipReport() SerializeLayers err:", err)
	}

	igmpPayload := buffer.Bytes()
	iph := r.ipv4Header(len(igmpPayload), IGMPHosts)

	t := time.NewTicker(r.queryTime)
	defer t.Stop()

	for loops := 0; ; loops++ {

		debugLog(r.debugLevel > 10, fmt.Sprintf("selfQuery() loops:%d", loops))

		<-t.C

		debugLog(r.debugLevel > 10, fmt.Sprintf("selfQuery() tick loops:%d", loops))

		err := r.raw.SetWriteDeadline(time.Now().Add(writeDeadlineCst))
		if err != nil {
			log.Fatal("selfQuery() SetWriteDeadline err:", err)
		}

		if err := r.raw.WriteTo(iph, igmpPayload, r.cm); err != nil {
			log.Fatal("selfQuery() err:", err)
		}

		debugLog(r.debugLevel > 10, "selfQuery() WriteTo success!")
	}
}

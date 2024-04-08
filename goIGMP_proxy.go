package goIGMP

import (
	"fmt"
	"log"
	"net"
	"time"
)

func (r IGMPReporter) proxy(interf side, dest destIP, buf *[]byte) {

	startTime := time.Now()
	defer func() {
		r.pH.WithLabelValues("proxy", "start", "complete").Observe(time.Since(startTime).Seconds())
	}()
	r.pC.WithLabelValues("proxy", "start", "count").Inc()

	debugLog(r.debugLevel > 100, fmt.Sprintf("proxy:%s dest:%s", interf, r.mapIPtoNetAddr[dest]))

	iph := r.ipv4Header(len(*buf), dest)

	err := r.conRaw[interf].SetWriteDeadline(time.Now().Add(writeDeadlineCst))
	if err != nil {
		log.Fatal(fmt.Sprintf("proxy(%s) SetWriteDeadline err:", interf), err)
	}

	if errW := r.conRaw[interf].WriteTo(iph, *buf, r.ContMsg[interf]); errW != nil {
		log.Fatal(fmt.Sprintf("proxy(%s) WriteTo errW:", interf), errW)
	}

	debugLog(r.debugLevel > 10, fmt.Sprintf("proxy(%s) WriteTo success! len(payload):%d", interf, len(*buf)))
}

func (r IGMPReporter) proxyUniToMultiv1or2(interf side, dest net.IP, buf *[]byte) {

	startTime := time.Now()
	defer func() {
		r.pH.WithLabelValues("proxyUniToMultiv1or2", "start", "complete").Observe(time.Since(startTime).Seconds())
	}()
	r.pC.WithLabelValues("proxyUniToMultiv1or2", "start", "count").Inc()

	debugLog(r.debugLevel > 100, fmt.Sprintf("proxyUniToMultiv1or2:%s dest:%s", interf, dest.String()))

	iph := r.ipv4HeaderNetIP(len(*buf), dest)

	err := r.conRaw[interf].SetWriteDeadline(time.Now().Add(writeDeadlineCst))
	if err != nil {
		log.Fatal(fmt.Sprintf("proxyUniToMultiv1or2(%s) SetWriteDeadline err:", interf), err)
	}

	if errW := r.conRaw[interf].WriteTo(iph, *buf, r.ContMsg[interf]); errW != nil {
		log.Fatal(fmt.Sprintf("proxyUniToMultiv1or2(%s) WriteTo errW:", interf), errW)
	}

	debugLog(r.debugLevel > 10, fmt.Sprintf("proxyUniToMultiv1or2(%s) WriteTo success! len(payload):%d", interf, len(*buf)))
}

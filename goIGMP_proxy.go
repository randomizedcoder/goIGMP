package goIGMP

import (
	"fmt"
	"log"
	"time"
)

func (r IGMPReporter) proxy(interf side, dest destIP, buf *[]byte) {

	debugLog(r.debugLevel > 100, fmt.Sprintf("proxy:%s dest:%s", interf, r.mapIPtoNetAddr[dest]))

	iph := r.ipv4Header(len(*buf), dest)

	err := r.conRaw[interf].SetWriteDeadline(time.Now().Add(writeDeadlineCst))
	if err != nil {
		log.Fatal(fmt.Sprintf("sendMembershipReport(%s) SetWriteDeadline err:", interf), err)
	}

	if errW := r.conRaw[interf].WriteTo(iph, *buf, r.ContMsg[interf]); errW != nil {
		log.Fatal(fmt.Sprintf("sendMembershipReport(%s) WriteTo errW:", interf), errW)
	}

	debugLog(r.debugLevel > 10, fmt.Sprintf("proxy(%s) WriteTo success! len(payload):%d", interf, len(*buf)))
}

package goIGMP

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"
)

// hackReadIGMPMemershipReportPayload reads the IGMP membership paylaod from a file
// the payload was extracted from pcap
// this needs to be replaced by correct serializtion in the gopacket library
func (r IGMPReporter) hackReadIGMPMemershipReportPayload(filename string) (payload []byte) {
	const ()

	var err error
	payload, err = os.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	if r.debugLevel > 10 {
		// https://pkg.go.dev/encoding/hex#Encode

		dst := make([]byte, hex.EncodedLen(len(payload)))
		n := hex.Encode(dst, payload)

		log.Printf("hackReadIGMPMemershipReportPayload() n:%d", n)
		log.Printf("hackReadIGMPMemershipReportPayload() hex:%s", dst)
	}

	return payload
}

func (r IGMPReporter) sendMembershipReport(interf side, membershipItems []membershipItem) {
	// const (
	// 	// TODO this will be replaced by a dynamic list of the groups we are joined too
	// 	allowGroupAddressCst = "232.0.0.1"
	// )

	startTime := time.Now()
	defer func() {
		r.pH.WithLabelValues("sendMembershipReport", "start", "complete").Observe(time.Since(startTime).Seconds())
	}()
	r.pC.WithLabelValues("sendMembershipReport", "start", "count").Inc()

	debugLog(r.debugLevel > 10, fmt.Sprintf("sendMembershipReport(%s)", interf))

	debugLog(r.debugLevel > 10, fmt.Sprintf("sendMembershipReport() should send based on membershipItems, but doesn't yet!:%v", membershipItems))

	// buffer := gopacket.NewSerializeBuffer()
	// options := gopacket.SerializeOptions{
	// 	ComputeChecksums: true,
	// 	FixLengths:       true,
	// }

	// // https://github.com/google/gopacket/blob/master/layers/igmp.go#L162
	// groupRecords := []IGMPv3GroupRecord{}

	// for _, mItem := range membershipItems {
	// 	debugLog(r.debugLevel > 10, fmt.Sprintf("NewIGMPReporter() mItem:%v", mItem))

	// 	s, err := r.addr2NetIP(mItem.Source)
	// 	if err != nil {
	// 		log.Fatal("s addr2NetIP err:", err)
	// 	}
	// 	ss := []net.IP{s}

	// 	g, err := r.addr2NetIP(mItem.Group)
	// 	if err != nil {
	// 		log.Fatal("g addr2NetIP err:", err)
	// 	}

	// 	groupRecord := IGMPv3GroupRecord{
	// 		Type:             IGMPv3GroupRecordType(layers.IGMPAllow),
	// 		NumberOfSources:  uint16(len(ss)),
	// 		SourceAddresses:  ss,
	// 		MulticastAddress: g,
	// 		// AuxDataLen:       0, // this should always be 0 as per IGMPv3 spec.
	// 		// AuxData:          0, // NOT USED
	// 	}
	// 	groupRecords = append(groupRecords, groupRecord)
	// }

	// // https://github.com/google/gopacket/blob/master/layers/igmp.go#L224
	// // Except that we created our own IGMP to add serialize functions
	// // TODO look at just adding serialize functions
	// igmp := &IGMP{
	// 	Type:                 layers.IGMPMembershipReportV3,
	// 	Version:              3,
	// 	GroupAddress:         r.mapIPtoNetIP[IGMPHosts],
	// 	NumberOfGroupRecords: uint16(len(groupRecords)),
	// 	GroupRecords:         groupRecords,
	// }

	// //err := gopacket.SerializeLayers(buffer, options, r.pbp.ethernetLayer, r.pbp.ipLayer, igmp)
	// err := gopacket.SerializeLayers(buffer, options, igmp)
	// if err != nil {
	// 	log.Fatal(fmt.Sprintf("sendMembershipReport(%s) SerializeLayers err:", interf), err)
	// }

	// igmpPayload := buffer.Bytes()
	//iph := r.ipv4Header(len(igmpPayload), IGMPHosts)

	var dest destIP
	if r.conf.UnicastMembershipReports {
		debugLog(r.debugLevel > 10, fmt.Sprintf("sendMembershipReport(%s) UnicastMembershipReports dest = QueryHost", interf))
		dest = QueryHost
	} else {
		dest = IGMPHosts
	}

	iph := r.ipv4Header(len(r.membershipReportPayloadHack), dest)

	if r.debugLevel > 10 {
		debugLog(r.debugLevel > 10, fmt.Sprintf("sendMembershipReport(%s) iph:%v", interf, iph))
	}

	errSWD := r.conRaw[interf].SetWriteDeadline(time.Now().Add(writeDeadlineCst))
	if errSWD != nil {
		log.Fatal(fmt.Sprintf("sendMembershipReport(%s) SetWriteDeadline errSWD:", interf), errSWD)
	}
	if errW := r.conRaw[interf].WriteTo(iph, r.membershipReportPayloadHack, r.ContMsg[interf]); errW != nil {
		log.Fatal(fmt.Sprintf("sendMembershipReport(%s) WriteTo errW:", interf), errW)
	}

	debugLog(r.debugLevel > 10, fmt.Sprintf("sendMembershipReport(%s) WriteTo success! len(igmpPayload):%d", interf, len(r.membershipReportPayloadHack)))

}

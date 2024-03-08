package goIGMP

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

func (r IGMPReporter) sendMembershipReport() {
	const (
		// TODO this will be replaced by a dynamic list of the groups we are joined too
		allowGroupAddressCst = "232.0.0.1"
	)

	debugLog(r.debugLevel > 10, "sendMembershipReport()")

	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	// https://github.com/google/gopacket/blob/master/layers/igmp.go#L162
	groupRecords := []IGMPv3GroupRecord{}
	groupRecord := IGMPv3GroupRecord{
		Type:             IGMPv3GroupRecordType(layers.IGMPAllow),
		NumberOfSources:  0,
		MulticastAddress: net.ParseIP(allowGroupAddressCst),
		AuxDataLen:       0, // this should always be 0 as per IGMPv3 spec.
		AuxData:          0, // NOT USED
	}
	groupRecords = append(groupRecords, groupRecord)

	// https://github.com/google/gopacket/blob/master/layers/igmp.go#L224
	// Except that we created our own IGMP to add serialize functions
	// TODO look at just adding serialize functions
	igmp := &IGMP{
		Type:                 layers.IGMPMembershipReportV3,
		Version:              3,
		GroupAddress:         multicastNetIP[IGMPHosts],
		NumberOfGroupRecords: uint16(len(groupRecords)),
		GroupRecords:         groupRecords,
	}

	//err := gopacket.SerializeLayers(buffer, options, r.pbp.ethernetLayer, r.pbp.ipLayer, igmp)
	err := gopacket.SerializeLayers(buffer, options, igmp)
	if err != nil {
		log.Fatal("sendMembershipReport() SerializeLayers err:", err)
	}

	igmpPayload := buffer.Bytes()

	iph := r.ipv4Header(len(igmpPayload), IGMPHosts)

	errSWD := r.raw.SetWriteDeadline(time.Now().Add(writeDeadlineCst))
	if errSWD != nil {
		log.Fatal("sendMembershipReport() SetWriteDeadline errSWD:", errSWD)
	}
	if errW := r.raw.WriteTo(iph, igmpPayload, r.cm); errW != nil {
		log.Fatal("sendMembershipReport() errW:", errW)
	}

	debugLog(r.debugLevel > 10, "selfQuery() WriteTo success!")

	debugLog(r.debugLevel > 10, fmt.Sprintf("sendMembershipReport() WriteTo:%d", len(igmpPayload)))
}

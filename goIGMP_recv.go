package goIGMP

import (
	"fmt"
	"log"
	"net"
	"time"
)

func (r IGMPReporter) recvIGMP() {

	debugLog(r.debugLevel > 10, "recvIGMP()")

	for loops := 0; ; loops++ {

		debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP() loops:%d", loops))

		buf := make([]byte, maxIGMPPacketRecieveBytesCst)
		err := r.c.SetReadDeadline(time.Now().Add(readDeadlineCst))
		if err != nil {
			log.Fatal("recvIGMP() SetReadDeadline err:", err)
		}

		n, addr, err := r.c.ReadFrom(buf)
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				debugLog(r.debugLevel > 10, "recvIGMP() ReadFrom timeout")
				continue
			}
		}

		debugLog(r.debugLevel > 10, fmt.Sprintf("recvIGMP() n:%d, addr:%s", n, addr))

		// // https://pkg.go.dev/github.com/tsg/gopacket#hdr-Basic_Usage
		// // https://github.com/google/gopacket/blob/master/layers/igmp.go#L224
		// packet := gopacket.NewPacket(buf, layers.LayerTypeIGMP, gopacket.Default)

		// if igmpLayer := packet.Layer(layers.LayerTypeIGMP); igmpLayer != nil {
		// 	igmp, _ := igmpLayer.(*layers.IGMP)
		// 	debugLog(r.debugLevel > 10, igmp)
		// 	//debugLog(r.debugLevel > 10, fmt.Sprintf("IGMP igmp.Type:%v igmp.SourceAddress:%v", igmp.Type, igmp.SourceAddresses))
		// }

		r.readCh <- struct{}{}

	}
}

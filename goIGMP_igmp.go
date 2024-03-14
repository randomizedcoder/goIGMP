package goIGMP

// This is mostly the gopacket igmp, with extra serialization layers added
// https://github.com/google/gopacket/blob/master/layers/igmp.go
// https://github.com/google/gopacket/blob/master/layers/igmp_test.go

// TODO look at just adding serialize functions to the gopacket types

// Borrowed from here
// https://github.com/opencord/bbsim/blob/v1.16.3/internal/bbsim/responders/igmp/igmp.go

import (
	"encoding/binary"
	"log"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

const (
	howLargeCanThisReallyGet = 500 // can probably be smaller
	IGMPv2SizeCst            = 8
	IGMPv3BaseSizeCst        = 24
)

type IGMP struct {
	layers.BaseLayer
	Type                    layers.IGMPType
	MaxResponseTime         time.Duration
	Checksum                uint16
	GroupAddress            net.IP
	SupressRouterProcessing bool
	RobustnessValue         uint8
	IntervalTime            time.Duration
	SourceAddresses         []net.IP
	NumberOfGroupRecords    uint16
	NumberOfSources         uint16
	GroupRecords            []IGMPv3GroupRecord
	Version                 uint8 // IGMP protocol version
}

// IGMPv3GroupRecord stores individual group records for a V3 Membership Report message.
type IGMPv3GroupRecord struct {
	Type             IGMPv3GroupRecordType
	AuxDataLen       uint8 // this should always be 0 as per IGMPv3 spec.
	NumberOfSources  uint16
	MulticastAddress net.IP
	SourceAddresses  []net.IP
	AuxData          uint32 // NOT USED
}

type IGMPv3GroupRecordType uint8

const (
	IGMPIsIn  IGMPv3GroupRecordType = 0x01 // Type MODE_IS_INCLUDE, source addresses x
	IGMPIsEx  IGMPv3GroupRecordType = 0x02 // Type MODE_IS_EXCLUDE, source addresses x
	IGMPToIn  IGMPv3GroupRecordType = 0x03 // Type CHANGE_TO_INCLUDE_MODE, source addresses x
	IGMPToEx  IGMPv3GroupRecordType = 0x04 // Type CHANGE_TO_EXCLUDE_MODE, source addresses x
	IGMPAllow IGMPv3GroupRecordType = 0x05 // Type ALLOW_NEW_SOURCES, source addresses x
	IGMPBlock IGMPv3GroupRecordType = 0x06 // Type BLOCK_OLD_SOURCES, source addresses x
)

func (i IGMPv3GroupRecordType) String() string {
	switch i {
	case IGMPIsIn:
		return "MODE_IS_INCLUDE"
	case IGMPIsEx:
		return "MODE_IS_EXCLUDE"
	case IGMPToIn:
		return "CHANGE_TO_INCLUDE_MODE"
	case IGMPToEx:
		return "CHANGE_TO_EXCLUDE_MODE"
	case IGMPAllow:
		return "ALLOW_NEW_SOURCES"
	case IGMPBlock:
		return "BLOCK_OLD_SOURCES"
	default:
		return ""
	}
}

// SerializeTo writes the serialized form of this layer into the
// SerializationBuffer, implementing gopacket.SerializableLayer.
// See the docs for gopacket.SerializableLayer for more info.
func (igmp *IGMP) SerializeTo(b gopacket.SerializeBuffer, opts gopacket.SerializeOptions) error {

	switch igmp.Version {
	case 2:
		return igmp.SerializeToIGMPv2(b, opts)
	case 3:
		return igmp.SerializeToIGMPv3(b, opts)
	default:
		log.Fatal("SerializeTo2 IGMP types must be 2 or 3")
	}

	return nil
}

// SerializeToIGMPv2
// 0                   1                   2                   3
// 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |      Type     | Max Resp Time |           Checksum            |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                         Group Address                         |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
func (igmp *IGMP) SerializeToIGMPv2(b gopacket.SerializeBuffer, opts gopacket.SerializeOptions) error {

	data, err := b.PrependBytes(IGMPv2SizeCst)
	if err != nil {
		return err
	}

	data[0] = byte(igmp.Type)
	data[1] = byte(int(float64(igmp.MaxResponseTime.Seconds()) * 10))
	data[2] = 0 //checksum byte1
	data[3] = 0 //checksum byte2
	copy(data[4:8], igmp.GroupAddress.To4())
	if opts.ComputeChecksums {
		igmp.Checksum = tcpipChecksum(data, 0)
		binary.BigEndian.PutUint16(data[2:4], igmp.Checksum)
	}

	return nil
}

// SerializeToIGMPv3
// 0                   1                   2                   3
// 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |  Type = 0x11  | Max Resp Code |           Checksum            |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                         Group Address                         |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// | Resv  |S| QRV |     QQIC      |     Number of Sources (N)     |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                       Source Address [1]                      |
// +-                                                             -+
// |                       Source Address [2]                      |
// +-                              .                              -+
// .                               .                               .
// .                               .                               .
// +-                                                             -+
// |                       Source Address [N]                      |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
func (igmp *IGMP) SerializeToIGMPv3(b gopacket.SerializeBuffer, opts gopacket.SerializeOptions) error {

	data, err := b.PrependBytes(IGMPv3BaseSizeCst + len(igmp.SourceAddresses))
	if err != nil {
		return err
	}

	data[0] = byte(igmp.Type)
	data[1] = byte(int(float64(igmp.MaxResponseTime.Seconds()) * 10))
	data[2] = 0                              //checksum byte1
	data[3] = 0                              //checksum byte2
	copy(data[4:8], igmp.GroupAddress.To4()) // bug is here.  TODO FIX ME!
	binary.BigEndian.PutUint16(data[6:8], igmp.NumberOfGroupRecords)
	j := 8
	for i := uint16(0); i < igmp.NumberOfGroupRecords; i++ {
		data[j] = byte(igmp.GroupRecords[i].Type)
		data[j+1] = byte(0)
		binary.BigEndian.PutUint16(data[j+2:j+4], igmp.GroupRecords[i].NumberOfSources)
		copy(data[j+4:j+8], igmp.GroupRecords[i].MulticastAddress.To4())
		j = j + 8
		for m := uint16(0); m < igmp.GroupRecords[i].NumberOfSources; m++ {
			copy(data[j:(j+4)], igmp.GroupRecords[i].SourceAddresses[m].To4())
			j = j + 4
		}
	}
	if opts.ComputeChecksums {
		igmp.Checksum = tcpipChecksum(data, 0)
		binary.BigEndian.PutUint16(data[2:4], igmp.Checksum)
	}

	return nil
}

func (i *IGMP) LayerType() gopacket.LayerType { return layers.LayerTypeIGMP }
func (i *IGMP) LayerContents() []byte         { return i.Contents }
func (i *IGMP) LayerPayload() []byte          { return i.Payload }

// Please note. I found two (2) slightly different checksum methods
// TODO confirm which is better/correct

// Calculate the TCP/IP checksum defined in rfc1071.
func tcpipChecksum(data []byte, csum uint32) uint16 {
	length := len(data) - 1
	for i := 0; i < length; i += 2 {
		csum += uint32(data[i]) << 8
		csum += uint32(data[i+1])
	}
	if len(data)%2 == 1 {
		csum += uint32(data[length]) << 8
	}
	for csum > 0xffff {
		csum = (csum & 0xffff) + (csum >> 16)
	}
	return ^uint16((csum >> 16) + csum)
}

// // Calculate the TCP/IP checksum defined in rfc1071.  The passed-in csum is any
// // initial checksum data that's already been computed.
// func tcpipChecksum(data []byte, csum uint32) uint16 {
// 	// to handle odd lengths, we loop to length - 1, incrementing by 2, then
// 	// handle the last byte specifically by checking against the original
// 	// length.
// 	length := len(data) - 1
// 	for i := 0; i < length; i += 2 {
// 		// For our test packet, doing this manually is about 25% faster
// 		// (740 ns vs. 1000ns) than doing it by calling binary.BigEndian.Uint16.
// 		csum += uint32(data[i]) << 8
// 		csum += uint32(data[i+1])
// 	}
// 	if len(data)%2 == 1 {
// 		csum += uint32(data[length]) << 8
// 	}
// 	for csum > 0xffff {
// 		csum = (csum >> 16) + (csum & 0xffff)
// 	}
// 	return ^uint16(csum)
// }

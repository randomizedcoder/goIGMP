package goIGMP

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/net/ipv4"
)

const (
	writeDeadlineCst = 5 * time.Second // writes should be fast

	igmpTTLCst = 1

	maxIGMPPacketRecieveBytesCst = 200

	// IN  = "inside"
	// OUT = "outside"
	OUT       side = 1
	IN        side = 2
	ALTOUT    side = 3
	OUTSTR         = "outside"
	ALTOUTSTR      = "altOutside"
	INSTR          = "inside"

	OutOrAltKey int = 1

	// localMembership  membershipType = 1
	// remoteMembership membershipType = 2

	TTL        ttlType = 1
	GRATUITOUS ttlType = 2
	QUERY      ttlType = 3

	allZerosQuad  = "0.0.0.0"
	allHostsQuad  = "224.0.0.1"
	IGMPHostsQuad = "224.0.0.22"

	allZerosHosts destIP = 0   // 0.0.0.0
	allHosts      destIP = 1   // 224.0.0.1
	IGMPHosts     destIP = 22  // 224.0.0.22
	QueryHost     destIP = 666 // In this case we use the query source IP
	// https://en.wikipedia.org/wiki/Multicast_address

	igmpTypeQuery            igmpType = 1
	igmpTypeMembershipReport igmpType = 2

	quantileError    = 0.05
	summaryVecMaxAge = 5 * time.Minute
)

type side int
type destIP int
type ttlType int
type igmpType int

func (s side) String() string {
	switch s {
	case OUT:
		return OUTSTR
	case IN:
		return INSTR
	case ALTOUT:
		return ALTOUTSTR
	default:
		return ""
	}
}

// type membershipType int

type Config struct {
	InIntName                    string
	OutIntName                   string
	AltOutIntName                string
	UnicastDst                   string
	ProxyOutToIn                 bool
	ProxyInToOut                 bool
	UnicastProxyInToOut          bool
	QueryNotify                  bool
	MembershipReportsFromNetwork bool
	MembershipReportsToNetwork   bool
	UnicastMembershipReports     bool
	SocketReadDeadLine           time.Duration
	ChannelSize                  int
	Gratuitous                   time.Duration
	QueryTime                    time.Duration
	HackPayloadFilename          string
	DebugLevel                   int
	Testing                      TestingOptions
}

type TestingOptions struct {
	MulticastLoopback       bool
	ConnectQueryToReport    bool
	MembershipReportsReader bool
}

func (c Config) String() string {
	return "Config " +
		fmt.Sprintf("InIntName:%s, ", c.InIntName) +
		fmt.Sprintf("OutIntName:%s, ", c.OutIntName) +
		fmt.Sprintf("AltOutIntName:%s, ", c.AltOutIntName) +
		fmt.Sprintf("UnicastDst:%s, ", c.UnicastDst) +
		fmt.Sprintf("ProxyOutToIn:%t, ", c.ProxyOutToIn) +
		fmt.Sprintf("ProxyInToOut:%t, ", c.ProxyInToOut) +
		fmt.Sprintf("UnicastProxyInToOut:%t, ", c.UnicastProxyInToOut) +
		fmt.Sprintf("QueryNotify:%t, ", c.QueryNotify) +
		fmt.Sprintf("MembershipReportsFromNetwork:%t, ", c.MembershipReportsFromNetwork) +
		fmt.Sprintf("MembershipReportsToNetwork:%t, ", c.MembershipReportsToNetwork) +
		fmt.Sprintf("UnicastMembershipReports:%t, ", c.UnicastMembershipReports) +
		fmt.Sprintf("Testing.MulticastLoopback:%t, ", c.Testing.MulticastLoopback) +
		fmt.Sprintf("Testing.ConnectQueryToReport:%t, ", c.Testing.ConnectQueryToReport) +
		fmt.Sprintf("Testing.MembershipReportsReader:%t, ", c.Testing.MembershipReportsReader) +
		fmt.Sprintf("ChannelSize:%d,", c.ChannelSize)
}

type IGMPReporter struct {
	conf Config

	IntName    map[side]string
	IntOutName *sync.Map
	Interfaces []side

	AltOutExists      bool
	OutsideInterfaces map[side]bool

	TimerDuration map[ttlType]time.Duration

	NetIF      map[side]*net.Interface
	NetIFIndex map[int]side
	NetIP      map[side]net.IP
	NetAddr    map[side]netip.Addr

	multicastGroups []destIP
	uCon            map[side]net.PacketConn
	// Each multicast socket has anyCon listening on 0.0.0.0,
	// and mConIGMP has the join for the particular group 224.0.0.1 or 224.0.0.22
	anyCon   map[side]map[netip.Addr]net.PacketConn
	mConIGMP map[side]map[netip.Addr]*ipv4.PacketConn
	// Raw for sending
	conRaw map[side]*ipv4.RawConn

	ContMsg map[side]*ipv4.ControlMessage

	membershipReportPayloadHack []byte

	QueryNotifyCh                 chan struct{}
	MembershipReportFromNetworkCh chan []membershipItem
	MembershipReportToNetworkCh   chan []membershipItem
	OutInterfaceSelectorCh        chan side

	//membership map[membershipType]*btree.BTreeG[membershipItem]

	mapIPtoNetIP   map[destIP]net.IP
	mapIPtoNetAddr map[destIP]netip.Addr
	//mapNetIPtoIP   map[net.IP]destIP - you can't use net.IP as a key, so use netip.Addr
	mapNetAddrtoIP      map[netip.Addr]destIP
	mapIPtoIGMPType     map[destIP]map[layers.IGMPType]igmpType
	mapUnicastIGMPTypes map[layers.IGMPType]bool

	querierSourceIP netip.Addr
	unicastDst      netip.Addr

	pC         *prometheus.CounterVec
	pH         *prometheus.SummaryVec
	pCrecvIGMP *prometheus.CounterVec
	pHrecvIGMP *prometheus.SummaryVec
	pG         prometheus.Gauge

	WG *sync.WaitGroup

	debugLevel int
}

// NewIGMPReporter
func NewIGMPReporter(conf Config) *IGMPReporter {

	r := new(IGMPReporter)

	r.conf = conf

	r.debugLevel = conf.DebugLevel

	if r.debugLevel > 10 {
		debugLog(r.debugLevel > 10, fmt.Sprintf("NewIGMPReporter() r.conf:%s", r.conf))
	}

	r.pC = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "counters",
			Name:      "goIGMP",
			Help:      "goIGMP counters",
		},
		[]string{"function", "variable", "type"},
	)
	r.pH = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Subsystem: "histrograms",
			Name:      "goIGMP",
			Help:      "goIGMP historgrams",
			Objectives: map[float64]float64{
				0.1:  quantileError,
				0.5:  quantileError,
				0.9:  quantileError,
				0.99: quantileError,
			},
			MaxAge: summaryVecMaxAge,
		},
		[]string{"function", "variable", "type"},
	)

	r.pCrecvIGMP = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "counters",
			Name:      "recvIGMP",
			Help:      "recvIGMP counters",
		},
		[]string{"function", "interface", "group", "type"},
	)
	r.pHrecvIGMP = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Subsystem: "histrograms",
			Name:      "recvIGMP",
			Help:      "recvIGMP historgrams",
			Objectives: map[float64]float64{
				0.1:  quantileError,
				0.5:  quantileError,
				0.9:  quantileError,
				0.99: quantileError,
			},
			MaxAge: summaryVecMaxAge,
		},
		[]string{"function", "interface", "group", "type"},
	)
	r.pG = promauto.NewGauge(prometheus.GaugeOpts{
		Subsystem: "guage",
		Name:      "outInterfaceSelector",
		Help:      "outInterfaceSelector gauge",
	})

	r.IntName = make(map[side]string)
	r.IntName[IN] = r.conf.InIntName
	r.IntName[OUT] = r.conf.OutIntName
	r.Interfaces = []side{IN, OUT}

	var m sync.Map
	m.Store(OUT, IN)
	m.Store(IN, OUT)
	r.IntOutName = &m

	if len(r.conf.AltOutIntName) > 0 {
		r.AltOutExists = true

		r.IntName[ALTOUT] = r.conf.AltOutIntName
		r.Interfaces = append(r.Interfaces, ALTOUT)

		r.OutInterfaceSelectorCh = make(chan side, r.conf.ChannelSize)

		r.OutsideInterfaces = make(map[side]bool)
		r.OutsideInterfaces[OUT] = true
		r.OutsideInterfaces[ALTOUT] = true
	}

	if r.debugLevel > 10 {
		for key, val := range r.IntName {
			debugLog(r.debugLevel > 10, fmt.Sprintf("NewIGMPReporter() IntName Key: %v, Value: %v", key, val))
		}
		for key, val := range r.IntName {
			debugLog(r.debugLevel > 10, fmt.Sprintf("NewIGMPReporter() IntOutName Key: %v, Value: %v", key, val))
		}
	}

	r.unicastDst = netip.MustParseAddr(r.conf.UnicastDst)

	r.TimerDuration = make(map[ttlType]time.Duration)
	r.TimerDuration[GRATUITOUS] = conf.Gratuitous
	r.TimerDuration[QUERY] = conf.QueryTime

	r.NetIF = make(map[side]*net.Interface)
	r.NetIFIndex = make(map[int]side)
	r.NetIP = make(map[side]net.IP)
	r.NetAddr = make(map[side]netip.Addr)

	r.multicastGroups = []destIP{allHosts, IGMPHosts}
	r.uCon = make(map[side]net.PacketConn)
	r.anyCon = make(map[side]map[netip.Addr]net.PacketConn)
	r.mConIGMP = make(map[side]map[netip.Addr]*ipv4.PacketConn)
	r.conRaw = make(map[side]*ipv4.RawConn)

	r.ContMsg = make(map[side]*ipv4.ControlMessage)

	r.membershipReportPayloadHack = r.hackReadIGMPMemershipReportPayload(r.conf.HackPayloadFilename)

	r.QueryNotifyCh = make(chan struct{}, r.conf.ChannelSize)
	r.MembershipReportFromNetworkCh = make(chan []membershipItem, r.conf.ChannelSize)
	r.MembershipReportToNetworkCh = make(chan []membershipItem, r.conf.ChannelSize)

	r.mapIPtoNetIP, r.mapIPtoNetAddr, r.mapNetAddrtoIP, r.mapIPtoIGMPType = r.makeIPMaps()

	if r.debugLevel > 10 {
		for key, val := range r.mapIPtoNetIP {
			debugLog(r.debugLevel > 10, fmt.Sprintf("NewIGMPReporter() mapIPtoNetIP Key: %v, Value: %v", key, val))
		}
		for key, val := range r.mapIPtoNetAddr {
			debugLog(r.debugLevel > 10, fmt.Sprintf("NewIGMPReporter() mapIPtoNetAddr Key: %v, Value: %v", key, val))
		}
		for key, val := range r.mapNetAddrtoIP {
			debugLog(r.debugLevel > 10, fmt.Sprintf("NewIGMPReporter() mapNetAddrtoIP Key: %v, Value: %v", key, val))
		}
		for key, val := range r.mapIPtoIGMPType {
			debugLog(r.debugLevel > 10, fmt.Sprintf("NewIGMPReporter() mapIPtoIGMPType Key: %v, Value: %v", key, val))
		}
	}

	for _, i := range r.Interfaces {
		r.NetIF[i], r.NetIP[i], r.NetAddr[i] = r.getInterfaceHandle(i)
		r.NetIFIndex[r.NetIF[i].Index] = i
		r.ContMsg[i] = &ipv4.ControlMessage{IfIndex: r.NetIF[i].Index}
	}

	debugLog(r.debugLevel > 10, "NewIGMPReporter() Opening sockets")

	if r.conf.UnicastProxyInToOut {
		debugLog(r.debugLevel > 10, "NewIGMPReporter() UnicastProxyInToOut")

		r.uCon[IN] = r.openUnicastPacketConn(IN)

		if r.conRaw[OUT] == nil {
			r.conRaw[OUT] = r.openRawConnection(OUT)
		}

		r.mapUnicastIGMPTypes = make(map[layers.IGMPType]bool)
		r.mapUnicastIGMPTypes[layers.IGMPMembershipReportV3] = true
		r.mapUnicastIGMPTypes[layers.IGMPMembershipReportV2] = true
		r.mapUnicastIGMPTypes[layers.IGMPMembershipReportV1] = true
	}

	if r.conf.QueryNotify || r.conf.MembershipReportsFromNetwork {
		debugLog(r.debugLevel > 10, "NewIGMPReporter() r.conf.QueryNotify || r.conf.MembershipReportsFromNetwork")

		r.createPacketConns(OUT)
	}

	if r.conf.ProxyOutToIn {
		debugLog(r.debugLevel > 10, "NewIGMPReporter() ProxyOutToIn")

		r.createPacketConns(OUT)

		if r.conRaw[IN] == nil {
			r.conRaw[IN] = r.openRawConnection(IN)
		}

		if r.AltOutExists {
			debugLog(r.debugLevel > 10, "NewIGMPReporter() ProxyOutToIn with alternative output")
			r.createPacketConns(ALTOUT)
		}
	}

	if r.conf.ProxyInToOut || r.conf.MembershipReportsToNetwork {
		debugLog(r.debugLevel > 10, "NewIGMPReporter() r.conf.ProxyInToOut || r.conf.MembershipReportsToNetwork")

		r.createPacketConns(IN)

		if r.conRaw[OUT] == nil {
			debugLog(r.debugLevel > 10, "openRawConnection(OUT)")
			r.conRaw[OUT] = r.openRawConnection(OUT)
		}

		if r.AltOutExists {
			debugLog(r.debugLevel > 10, "openRawConnection(ALTOUT)")
			r.createPacketConns(ALTOUT)
		}
	}

	var wg sync.WaitGroup
	r.WG = &wg

	debugLog(r.debugLevel > 10, "NewIGMPReporter() setup complete")

	if r.debugLevel > 10 {
		for key, val := range r.NetIFIndex {
			debugLog(r.debugLevel > 10, fmt.Sprintf("NewIGMPReporter() NetIFIndex Key: %v, Value: %v", key, val))
		}
	}

	return r
}

func (r IGMPReporter) Run(ctx context.Context) {

	debugLog(r.debugLevel > 10, "IGMPReporter.Run()")

	var added int

	if r.conf.ProxyOutToIn || r.conf.QueryNotify || r.conf.MembershipReportsFromNetwork {
		for _, g := range r.multicastGroups {
			r.WG.Add(1)
			go r.recvIGMP(r.WG, ctx, OUT, g)
			debugLog(r.debugLevel > 10, fmt.Sprintf("IGMPReporter.Run() recvIGMP OUT started, g:%s", r.mapIPtoNetAddr[g]))
			added++
		}

		if r.AltOutExists {
			for _, g := range r.multicastGroups {
				r.WG.Add(1)
				go r.recvIGMP(r.WG, ctx, ALTOUT, g)
				debugLog(r.debugLevel > 10, fmt.Sprintf("IGMPReporter.Run() recvIGMP ALTOUT started, g:%s", r.mapIPtoNetAddr[g]))
				added++
			}

			r.WG.Add(1)
			go r.outInterfaceSelector(r.WG)
			debugLog(r.debugLevel > 10, "IGMPReporter.Run() outInterfaceSelector started()")
			added++
		}
	}

	if r.conf.ProxyInToOut {
		for _, g := range r.multicastGroups {
			r.WG.Add(1)
			go r.recvIGMP(r.WG, ctx, IN, g)
			debugLog(r.debugLevel > 10, fmt.Sprintf("IGMPReporter.Run() recvIGMP IN started, g:%s", r.mapIPtoNetAddr[g]))
			added++
		}
	}

	if r.conf.UnicastProxyInToOut {
		r.WG.Add(1)
		go r.recvUnicastIGMP(r.WG, ctx, IN)
		debugLog(r.debugLevel > 10, "IGMPReporter.Run() UnicastProxyInToOut started")
		added++
	}

	if r.conf.MembershipReportsToNetwork {
		r.WG.Add(1)
		go r.readMembershipReportToNetworkCh(r.WG, ctx)
		debugLog(r.debugLevel > 10, "IGMPReporter.Run() readMembershipReportToNetworkCh started")
		added++
	}

	if r.conf.Testing.ConnectQueryToReport {
		r.WG.Add(1)
		go r.connectQueryToReport(r.WG, ctx)
		debugLog(r.debugLevel > 10, "IGMPReporter.Run() connectQueryToReport started")
		added++
	}

	if r.conf.Testing.MembershipReportsReader {
		r.WG.Add(1)
		go r.testingReadMembershipReportsFromNetwork(r.WG, ctx)
		debugLog(r.debugLevel > 10, "IGMPReporter.Run() testingReadMembershipReportsFromNetwork started")
		added++
	}

	r.WG.Wait()

	if added == 0 {
		debugLog(r.debugLevel > 10, "IGMPReporter.Run() added == 0.  Recommend enabling some features")
	} else {
		debugLog(r.debugLevel > 10, fmt.Sprintf("IGMPReporter.Run() complete (added:%d)", added))
	}

}

func (r IGMPReporter) RunSelfQuery() {
	for _, in := range r.Interfaces {
		go r.selfQuery(in)
	}
}

// makeIPMaps builds maps for translating IPs
//
// "Compared to the net.IP type, Addr type takes less memory, is immutable,
// and is comparable (supports == and being a map key). "
// https://pkg.go.dev/net/netip#pkg-types
func (r IGMPReporter) makeIPMaps() (
	mapIPtoNetIP map[destIP]net.IP,
	mapIPtoNetAddr map[destIP]netip.Addr,
	mapNetAddrtoIP map[netip.Addr]destIP,
	mapIPtoIGMPType map[destIP]map[layers.IGMPType]igmpType) {

	mapIPtoNetIP = make(map[destIP]net.IP)
	mapIPtoNetAddr = make(map[destIP]netip.Addr)
	mapNetAddrtoIP = make(map[netip.Addr]destIP)
	mapIPtoIGMPType = make(map[destIP]map[layers.IGMPType]igmpType)

	mapIPtoNetIP[allZerosHosts] = net.ParseIP(allZerosQuad).To4()
	mapIPtoNetIP[allHosts] = net.ParseIP(allHostsQuad).To4()
	mapIPtoNetIP[IGMPHosts] = net.ParseIP(IGMPHostsQuad).To4()

	debugLog(r.debugLevel > 100, fmt.Sprintf("makeIPMaps() mapIPtoNetIP[allZerosHosts]:%s", mapIPtoNetIP[allZerosHosts]))
	az, err := r.netip2Addr(mapIPtoNetIP[allZerosHosts])
	if err != nil {
		log.Fatal("makeIPMaps() netip2Addr allZerosHosts err:", err)
	}

	debugLog(r.debugLevel > 100, fmt.Sprintf("makeIPMaps() mapIPtoNetIP[allHosts]:%s", mapIPtoNetIP[allHosts]))
	ah, err := r.netip2Addr(mapIPtoNetIP[allHosts])
	if err != nil {
		log.Fatal("makeIPMaps() netip2Addr allHosts err:", err)
	}

	debugLog(r.debugLevel > 100, fmt.Sprintf("makeIPMaps() mapIPtoNetIP[IGMPHosts]:%s", mapIPtoNetIP[IGMPHosts]))
	ih, err := r.netip2Addr(mapIPtoNetIP[IGMPHosts])
	if err != nil {
		log.Fatal("makeIPMaps() netip2Addr IGMPHosts err:", err)
	}

	mapIPtoNetAddr[allZerosHosts] = az
	mapIPtoNetAddr[allHosts] = ah
	mapIPtoNetAddr[IGMPHosts] = ih

	mapNetAddrtoIP[az] = allZerosHosts
	mapNetAddrtoIP[ah] = allHosts
	mapNetAddrtoIP[ih] = IGMPHosts

	// true means query, false means membershipReport, doesn't exist means type doesn't match
	mapIPtoIGMPType[allHosts] = make(map[layers.IGMPType]igmpType)
	mapIPtoIGMPType[allHosts][layers.IGMPMembershipQuery] = igmpTypeQuery
	mapIPtoIGMPType[IGMPHosts] = make(map[layers.IGMPType]igmpType)
	mapIPtoIGMPType[IGMPHosts][layers.IGMPMembershipReportV3] = igmpTypeMembershipReport
	mapIPtoIGMPType[IGMPHosts][layers.IGMPMembershipReportV2] = igmpTypeMembershipReport
	mapIPtoIGMPType[IGMPHosts][layers.IGMPMembershipReportV1] = igmpTypeMembershipReport

	return mapIPtoNetIP, mapIPtoNetAddr, mapNetAddrtoIP, mapIPtoIGMPType
}

// netip2Addr
// https://djosephsen.github.io/posts/ipnet/
func (r IGMPReporter) netip2Addr(ip net.IP) (netip.Addr, error) {

	debugLog(r.debugLevel > 100, fmt.Sprintf("netip2Addr() ip:%s, multicast:%t", ip.String(), ip.IsMulticast()))

	if addr, ok := netip.AddrFromSlice(ip); ok {
		return addr, nil
	}
	return netip.Addr{}, errors.New("invalid IP")
}

// addr2NetIP safely convert a netip.Addr to net.IP
/* trunk-ignore(golangci-lint/unused) */
func (r IGMPReporter) addr2NetIP(addr netip.Addr) (net.IP, error) {
	if addr.IsValid() {
		return addr.AsSlice(), nil
	}
	return net.IP{}, errors.New("invalid ip")
}

package goIGMP

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/net/ipv4"
)

const (
	writeDeadlineCst = 5 * time.Second // writes should be fast
	readDeadlineCst  = 2 * time.Minute // IGMP queries come in slowly

	readChLengthCst  = 100
	firstChLengthCst = 10

	igmpTTLCst = 1

	maxIGMPPacketRecieveBytesCst = 200
)

type IGMPReporter struct {
	inter      string
	//ttl        time.Duration
	gratuitous time.Duration

	queryTime time.Duration

	netIF *net.Interface
	netIP net.IP
	cm    *ipv4.ControlMessage

	c   net.PacketConn
	raw *ipv4.RawConn

	readCh chan struct{}

	debugLevel int
}

// NewIGMPReporter
func NewIGMPReporter(
	intName string,
	//ttl time.Duration,
	gratuitous time.Duration,
	queryTime time.Duration,
	debugLevel int) *IGMPReporter {

	r := new(IGMPReporter)

	r.debugLevel = debugLevel

	r.inter = intName
	//r.ttl = ttl
	r.gratuitous = gratuitous
	r.queryTime = queryTime

	r.netIF, r.netIP = r.getInterfaceHandle(intName)
	r.c, r.raw = r.openRawConnection()

	r.cm = &ipv4.ControlMessage{IfIndex: r.netIF.Index}

	r.readCh = make(chan struct{}, readChLengthCst)

	debugLog(r.debugLevel > 10, "NewIGMPReporter() setup complete")

	return r
}

func (r IGMPReporter) Run() {

	debugLog(r.debugLevel > 10, "IGMPReporter.Run()")

	go r.selfQuery()

	go r.recvIGMP()

	g := r.setupGratuitousTicker()

	first := setupFirstChannel()

	for loops := 0; ; loops++ {

		debugLog(r.debugLevel > 10, fmt.Sprintf("IGMPReporter.Run() loops:%d", loops))

		select {

		case <-first:
			debugLog(r.debugLevel > 10, "IGMPReporter.Run() first time send membership immediately")
			r.sendMembershipReport()

		case <-r.readCh:
			debugLog(r.debugLevel > 10, "IGMPReporter.Run() IGMP packet recieved")
			r.sendMembershipReport()

		case <-g.C:
			debugLog(r.debugLevel > 10, "IGMPReporter.Run() gratuitous tick")
			r.sendMembershipReport()
		}
	}
}

func (r IGMPReporter) RunSelfQuery() {
	r.selfQuery()
}

func setupFirstChannel() (first chan struct{}) {
	first = make(chan struct{}, firstChLengthCst)
	first <- struct{}{}
	return first
}

// setupGratuitousTicker starts the gratuitous ticker, if we need too
// if gratuitous duration is zero (0), this won't tick
func (r IGMPReporter) setupGratuitousTicker() (g *time.Ticker) {
	if r.gratuitous == 0 {
		return g
	}
	g = time.NewTicker(r.gratuitous)
	return g
}

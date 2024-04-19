package mcast2hls

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/randomizedcoder/goIGMP"
)

const (
	goIGMPChannelSizeCst = 10

	logModulusCst = 100
)

// membershipReporter generates gratuitious membership reports
// and responds  to membership queries
// gratuitious membership reports is based on the gratuitiousMembershipReportFrequency
// To do this, it has a channel, over which it sends the memberships,
// and the goIGMP module does the actual IGMP work ( including opening the sockets, etc )
// Using singletonCh to make sure only one instance of gratuitiousReporter is running
func (m *Mcast2HLS) membershipReporter(ctx context.Context, wg *sync.WaitGroup) {

	defer wg.Done()

	startTime := time.Now()
	defer func() {
		m.pH.WithLabelValues("membershipReporter", "start", "complete").Observe(time.Since(startTime).Seconds())
	}()
	m.pC.WithLabelValues("membershipReporter", "start", "count").Inc()

	m.debugLog(m.debugLevel > 100, "membershipReporter start")

	r := m.createGoIGMPReporter()

	w := new(sync.WaitGroup)

	w.Add(1)
	go r.Run(ctx, w)
	m.debugLog(m.debugLevel > 10, "membershipReporter started goIGMP")

	singletonCh := make(chan struct{}, 1) // deliberately length = 1

	for i, keepLooping := 0, true; keepLooping; i++ {
		m.pC.WithLabelValues("membershipReporter", "loops", "count").Inc()

		if i%logModulusCst == 0 {
			m.debugLog(m.debugLevel > 10, fmt.Sprintf("membershipReporter i:%d", i))
		}

		loopStartTime := time.Now()

		select {

		case <-m.mRptNotifyCh:
			m.pH.WithLabelValues("membershipReporter", "notify", "count").Observe(time.Since(loopStartTime).Seconds())
			m.pC.WithLabelValues("membershipReporter", "notify", "count").Inc()
			debugLog(m.debugLevel > 10,
				fmt.Sprintf("membershipReporter i:%d, mRptNotifyCh, checking gratuitiousReporter is started", i))

			select {
			case singletonCh <- struct{}{}:
				// We were able to send on the channel, so this means gratuitiousReporter
				// was not running so we start it
				go m.gratuitiousReporter(ctx, singletonCh, r.MembershipReportToNetworkCh, r.LeaveToNetworkCh)
				m.pC.WithLabelValues("membershipReporter", "startGReporter", "count").Inc()
				debugLog(m.debugLevel > 10,
					fmt.Sprintf("membershipReporter i:%d, starting gratuitiousReporter", i))
			default:
				// non-blocking
				// If we hit this path, then gratuitiousReporter is already running, so we don't start it
				//
				// To be honest, I think this should be the only path.  Just doing this is case.
				// The risk is that http_manifest_tracking could get x2 requests at very similar times
				// Maybe in the race for the m.localStreams.Range they could both get zero length
				// and so they would both send
				// Might be able to take this extra channel out
				m.pC.WithLabelValues("membershipReporter", "alreadyGReporter", "count").Inc()
				debugLog(m.debugLevel > 10,
					fmt.Sprintf("membershipReporter i:%d, mRptNotifyCh, gratuitiousReporter is already running", i))
			}

		case <-r.QueryNotifyCh:
			m.pH.WithLabelValues("membershipReporter", "QueryNotifyCh", "count").Observe(time.Since(loopStartTime).Seconds())
			m.pC.WithLabelValues("membershipReporter", "QueryNotifyCh", "count").Inc()
			// debugLog(m.debugLevel > 10,
			// 	fmt.Sprintf("membershipReporter i:%d, <-r.QueryNotifyCh, ignoring for now", i))

			groups := m.buildMembershipItems()
			if len(groups) > 0 {
				debugLog(m.debugLevel > 10,
					fmt.Sprintf("membershipReporter i:%d, <-r.QueryNotifyCh, responded with groups:%d", i, len(groups)))
				m.sendMemberShipReport(groups, r.MembershipReportToNetworkCh)
			} else {
				debugLog(m.debugLevel > 10,
					fmt.Sprintf("membershipReporter i:%d, <-r.QueryNotifyCh, but there are no local viewers", i))
			}

		case <-r.MembershipReportFromNetworkCh:
			// TODO Ignoring these for now, but it would be nice to look at them
			m.pH.WithLabelValues("membershipReporter", "memRptFromNet", "count").Observe(time.Since(loopStartTime).Seconds())
			m.pC.WithLabelValues("membershipReporter", "memRptFromNet", "count").Inc()
			if i%logModulusCst == 0 {
				debugLog(m.debugLevel > 10,
					fmt.Sprintf("membershipReporter i:%d, memRptFromNet, ignoring this for now. ", i))
			}

		case <-ctx.Done():
			m.pH.WithLabelValues("membershipReporter", "Done", "count").Observe(time.Since(startTime).Seconds())
			m.pC.WithLabelValues("membershipReporter", "Done", "count").Inc()
			debugLog(m.debugLevel > 10,
				fmt.Sprintf("membershipReporter i:%d, ctx.Done(), time.Since(startTime).Seconds():%.2fs",
					i, time.Since(startTime).Seconds()))
			keepLooping = false
		}
	}

	m.debugLog(m.debugLevel > 10, "membershipReporter w.Wait() for goIGMPReporter")
	w.Wait()

	m.debugLog(m.debugLevel > 10, "membershipReporter complete")
}

// gratuitiousReporter is run as a goroutine to send group memberships to the goIGMP worker
// every gratuitious report frequency
// using singletonCh to make sure only one instance of gratuitiousReporter is running
func (m *Mcast2HLS) gratuitiousReporter(ctx context.Context, singletonCh <-chan struct{}, MembershipReportToNetworkCh chan<- []goIGMP.MembershipItem, LeaveToNetworkCh chan<- []goIGMP.MembershipItem) {

	startTime := time.Now()
	defer func() {
		m.pH.WithLabelValues("gratuitiousReporter", "start", "complete").Observe(time.Since(startTime).Seconds())
	}()
	m.pC.WithLabelValues("gratuitiousReporter", "start", "count").Inc()

	m.debugLog(m.debugLevel > 1000, "gratuitiousReporter start")

	var previousGroups []goIGMP.MembershipItem

	ticker := time.NewTicker(m.Config.GratuitiousMembershipReportFrequency)

	for i, keepLooping := 0, true; keepLooping; i++ {
		m.pC.WithLabelValues("gratuitiousReporter", "loops", "count").Inc()

		select {
		case <-ticker.C:
			m.pC.WithLabelValues("gratuitiousReporter", "tick", "count").Inc()
			if m.debugLevel > 10 {
				tickTime := time.Now()
				m.debugLog(m.debugLevel > 1000, fmt.Sprintf("gratuitiousReporter startTime:%s tickTime:%s", startTime, tickTime))
			}

			groups := m.buildMembershipItems()

			m.findAndLeaveGroups(groups, previousGroups, LeaveToNetworkCh)

			if len(groups) > 0 {
				m.sendMemberShipReport(groups, MembershipReportToNetworkCh)
				previousGroups = groups
				continue
			}

			m.debugLog(m.debugLevel > 10, fmt.Sprintf("gratuitiousReporter startTime:%s len(groups) == 0, all done here.", startTime))

			keepLooping = false

		case <-ctx.Done():
			m.pH.WithLabelValues("gratuitiousReporter", "Done", "count").Observe(time.Since(startTime).Seconds())
			m.pC.WithLabelValues("gratuitiousReporter", "Done", "count").Inc()
			m.debugLog(m.debugLevel > 10,
				fmt.Sprintf("gratuitiousReporter, i:%d,  <-ctx.Done() time.Since(startTime).Seconds():%.2fs",
					i, time.Since(startTime).Seconds()))
			keepLooping = false
		}

	}
	// read on the channel, which allows a new instance to be started, after we return after this line
	<-singletonCh
	m.debugLog(m.debugLevel > 10, fmt.Sprintf("gratuitiousReporter startTime:%s <-singletonCh", startTime))

	//gratuitiousReporter goes away...
}

// buildMembershipItems() converts the localStreams sync.Map to a list/slice
// Keeping this function as simple and small as possible
// TODO probably makes sense to create the slice with a larger capacity.
// Not sure the best way to do this, so do it later if required
func (m *Mcast2HLS) buildMembershipItems() (groups []goIGMP.MembershipItem) {

	m.pC.WithLabelValues("buildMembershipItems", "start", "count").Inc()

	m.localStreams.Range(func(key, value any) bool {
		m.debugLog(m.debugLevel > 1000,
			fmt.Sprintf("buildMembershipItems stream:%s MembershipItem:%v\n", key, value))

		lStream, ok := value.(*localStream)
		if !ok {
			m.pC.WithLabelValues("buildMembershipItems", "typeCast", "error").Inc()
			m.debugLog(m.debugLevel > 10, "buildMembershipItems, typeCast error")
			return false
		}

		groups = append(groups, lStream.memItem)
		return true
	})

	m.debugLog(m.debugLevel > 1000,
		fmt.Sprintf("buildMembershipItems len(groups):%d", len(groups)))

	return groups
}

// findGroupsToLeave compares the current and previous groups to find any groups that have been left
//
//	type MembershipItem struct {
//	    Sources []netip.Addr
//	    Group   netip.Addr
//	}
func (m *Mcast2HLS) findGroupsToLeave(current []goIGMP.MembershipItem, previous []goIGMP.MembershipItem) (leaves []goIGMP.MembershipItem) {

	cgMap := make(map[netip.Addr]struct{}, len(current))

	for _, item := range current {
		cgMap[item.Group] = struct{}{}
	}

	for _, item := range previous {
		if _, found := cgMap[item.Group]; !found {
			leaves = append(leaves, item)
		}
	}

	return leaves
}

func (m *Mcast2HLS) createGoIGMPReporter() (r *goIGMP.IGMPReporter) {

	r = goIGMP.NewIGMPReporter(goIGMP.Config{
		OutIntName:                   m.Config.Interface, // the only one that matters in this case
		InIntName:                    m.Config.Interface,
		UnicastDst:                   m.Config.UnicastIGMPDestinationIPStr,
		QueryNotify:                  true,
		MembershipReportsFromNetwork: true,
		MembershipReportsToNetwork:   true,
		UnicastMembershipReports:     true,
		LeaveToNetwork:               true,
		SocketReadDeadLine:           multicastSocketReadDealineCst,
		ChannelSize:                  goIGMPChannelSizeCst,
		DebugLevel:                   m.Config.IGMPDebugLevel,
	})

	return r
}

// sendMemberShipReport sends on the MembershipReportToNetworkCh
// First we try a non-blocking send let's us detect if the channel is too small, or if
// for whatever reason we are slow to send the IGMP messages
func (m *Mcast2HLS) sendMemberShipReport(
	groups []goIGMP.MembershipItem,
	MembershipReportToNetworkCh chan<- []goIGMP.MembershipItem) {

	m.debugLog(m.debugLevel > 10,
		fmt.Sprintf("mcast2hls sendMemberShipReport(), MembershipReportToNetworkCh <- groups:%v", groups))

	select {

	case MembershipReportToNetworkCh <- groups:
		m.pC.WithLabelValues("sendMemberShipReport", "membershipCh", "count").Inc()
		return

	default:
		// non-blocking
		m.pC.WithLabelValues("sendMemberShipReport", "membershipChBlocked", "error").Inc()

	}
	// blocking send
	MembershipReportToNetworkCh <- groups
}

// sendLeaves sends on the MembershipReportToNetworkCh
// First we try a non-blocking send let's us detect if the channel is too small, or if
// for whatever reason we are slow to send the IGMP messages
func (m *Mcast2HLS) sendLeaves(
	groups []goIGMP.MembershipItem,
	LeaveToNetworkCh chan<- []goIGMP.MembershipItem) {

	m.debugLog(m.debugLevel > 10,
		fmt.Sprintf("mcast2hls sendLeaves(), sendLeaves <- groups:%v", groups))

	select {

	case LeaveToNetworkCh <- groups:
		m.pC.WithLabelValues("sendLeaves", "leaveCh", "count").Inc()
		return

	default:
		// non-blocking
		m.pC.WithLabelValues("sendLeaves", "leaveChBlocked", "error").Inc()

	}
	// blocking send
	LeaveToNetworkCh <- groups
}

// findAndLeaveGroups finds the groups to leave and then sends those groups on the the channel,
// so goIGMP can send the actual leave
func (m *Mcast2HLS) findAndLeaveGroups(
	groups []goIGMP.MembershipItem,
	previousGroups []goIGMP.MembershipItem,
	LeaveToNetworkCh chan<- []goIGMP.MembershipItem) {

	leaves := m.findGroupsToLeave(groups, previousGroups)

	m.debugLog(m.debugLevel > 10,
		fmt.Sprintf("findAndLeaveGroups len(groups),%d, previousGroups:%d, len(leaves):%d",
			len(groups), len(previousGroups), len(leaves)))

	m.debugLog(m.debugLevel > 10,
		fmt.Sprintf("findAndLeaveGroups groups,%v, previousGroups:%v, leaves:%v",
			groups, previousGroups, leaves))

	m.sendLeaves(leaves, LeaveToNetworkCh)
}

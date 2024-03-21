package goIGMP

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"time"
)

type membershipItem struct {
	Sources []netip.Addr
	Group   netip.Addr
	//LastSeenTime time.Time
}

func (r IGMPReporter) readMembershipReportToNetworkCh(wg *sync.WaitGroup, ctx context.Context) {

	defer wg.Done()

	debugLog(r.debugLevel > 10, "readMembershipReportFromWebServerCh()")

forLoop:
	for loops := 0; ; loops++ {

		startTime := time.Now()
		r.pC.WithLabelValues("readMembershipReportToNetworkCh", "loops", "count").Inc()

		var groups []membershipItem
		select {
		case groups = <-r.MembershipReportToNetworkCh:
			debugLog(r.debugLevel > 10, fmt.Sprintf("readMembershipReportToNetworkCh() loops:%d groups:%v", loops, groups))
		case <-ctx.Done():
			debugLog(r.debugLevel > 10, "readMembershipReportToNetworkCh ctx.Done()")
			break forLoop
		}

		r.sendMembershipReport(OUT, groups)

		r.pH.WithLabelValues("readMembershipReportToNetworkCh", "loop", "complete").Observe(time.Since(startTime).Seconds())
	}
}

func (r IGMPReporter) testingReadMembershipReportsFromNetwork(wg *sync.WaitGroup, ctx context.Context) {

	defer wg.Done()

	debugLog(r.debugLevel > 10, "testingReadMembershipReportsFromNetwork()")

forLoop:
	for loops := 0; ; loops++ {

		startTime := time.Now()
		r.pC.WithLabelValues("testingReadMembershipReportsFromNetwork", "loops", "count").Inc()

		var groups []membershipItem
		select {
		case groups = <-r.MembershipReportFromNetworkCh:
			debugLog(r.debugLevel > 10, fmt.Sprintf("testingReadMembershipReportsFromNetwork() loops:%d groups:%v", loops, groups))
		case <-ctx.Done():
			debugLog(r.debugLevel > 10, "testingReadMembershipReportsFromNetwork ctx.Done()")
			break forLoop
		}

		r.pH.WithLabelValues("testingReadMembershipReportsFromNetwork", "loop", "complete").Observe(time.Since(startTime).Seconds())
	}
}

// connectQueryToReport is for testing
// it reads from the query channel and will generate membership reports
func (r IGMPReporter) connectQueryToReport(wg *sync.WaitGroup, ctx context.Context) {

	defer wg.Done()

	debugLog(r.debugLevel > 10, "connectQueryToReport()")

	var mi []membershipItem

forLoop:
	for loops := 0; ; loops++ {

		startTime := time.Now()
		r.pC.WithLabelValues("connectQueryToReport", "loops", "count").Inc()

		select {
		case <-r.QueryNotifyCh:
		case <-ctx.Done():
			debugLog(r.debugLevel > 10, "connectQueryToReport ctx.Done()")
			break forLoop
		}

		debugLog(r.debugLevel > 10, fmt.Sprintf("connectQueryToReport() loops:%d <-r.QueryNotifyCh, calling r.sendMembershipReport(OUT, mi)", loops))

		r.sendMembershipReport(OUT, mi)

		r.pH.WithLabelValues("connectQueryToReport", "loop", "complete").Observe(time.Since(startTime).Seconds())

	}

}

//"github.com/google/btree"
// const (
// 	BtreeDegreeCst = 3
// )

// func (r IGMPReporter) makeMembershipMapAndBtree() {
// 	r.membership = make(map[membershipType]*btree.BTreeG[membershipItem])

// 	r.membership[localMembership] = btree.NewG[membershipItem](BtreeDegreeCst)
// 	r.membership[remoteMembership] =

// 		btree.NewG[uint16](degree)

// }

// type membershipItemConstraint interface {
// 	membershipItem
// }

// func Less[T membershipItemConstraint]() btree.LessFunc[T] {
// 	return func(a, b T) bool { return a.LastSeenTime < b.LastSeenTime }
// }

// func membershipItemLess[T membershipItemConstraint](a, b membershipItemConstraint) bool {
// 	if a.LastSeenTime < b.LastSeenTime {
// 		return true
// 	}
// 	return false
// }

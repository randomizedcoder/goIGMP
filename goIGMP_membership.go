package goIGMP

import (
	"fmt"
	"net/netip"
	"sync"
)

type membershipItem struct {
	Sources []netip.Addr
	Group   netip.Addr
	//LastSeenTime time.Time
}

func (r IGMPReporter) readMembershipReportToNetworkCh(wg *sync.WaitGroup) {

	defer wg.Done()

	debugLog(r.debugLevel > 10, "readMembershipReportFromWebServerCh()")

	for loops := 0; ; loops++ {

		groups := <-r.MembershipReportToNetworkCh

		debugLog(r.debugLevel > 10, fmt.Sprintf("readMembershipReportToNetworkCh() loops:%d groups:%v", loops, groups))

		r.sendMembershipReport(OUT, groups)

	}

}

// connectQueryToReport is for testing
// it reads from the query channel and will generate membership reports
func (r IGMPReporter) connectQueryToReport(wg *sync.WaitGroup) {

	defer wg.Done()

	debugLog(r.debugLevel > 10, "connectQueryToReport()")

	var mi []membershipItem

	for loops := 0; ; loops++ {

		<-r.QueryNotifyCh

		debugLog(r.debugLevel > 10, fmt.Sprintf("connectQueryToReport() loops:%d <-r.QueryNotifyCh, calling r.sendMembershipReport(OUT, mi)", loops))

		r.sendMembershipReport(OUT, mi)

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

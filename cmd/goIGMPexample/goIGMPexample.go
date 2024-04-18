package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/randomizedcoder/goIGMP"
	"github.com/vishvananda/netlink"
)

const (
	debugLevelCst = 11

	signalChannelSize = 10

	promListenCst           = ":7500"
	promPathCst             = "/metrics"
	promMaxRequestsInFlight = 10
	promEnableOpenMetrics   = true

	inNameCst     = "lo"
	outNameCst    = "eth0"
	altNameCst    = ""
	unicastDstCst = "10.99.0.1"

	ProxyOutToInCst                 = true
	ProxyInToOutCst                 = false
	UnicastProxyInToOutCst          = true
	QueryNotifyCst                  = false
	MembershipReportsFromNetworkCst = false
	MembershipReportsToNetworkCst   = false
	UnicastMembershipReportsCst     = false
	ConnectQueryToReportCst         = false
	LeaveToNetworkCst               = true

	channelSizeCst = 10

	readDeadlineCst = 10 * time.Second // IGMP queries come in infrequently

	//joinTTLCst    = 60 * time.Second
	gratuitousCst = 600 * time.Second // These should be longer!!
	selfQueryCst  = 5 * time.Second
	loopbackCst   = false

	cancelSleepTime = 5 * time.Second
)

var (
	// Passed by "go build -ldflags" for the show version
	commit string
	date   string

	//debugLevel int
)

func main() {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go initSignalHandler(cancel)

	version := flag.Bool("version", false, "version")

	// https://pkg.go.dev/net#Listen
	promListen := flag.String("promListen", promListenCst, "Prometheus http listening socket")
	promPath := flag.String("promPath", promPathCst, "Prometheus http path")
	// curl -s http://[::1]:9111/metrics 2>&1 | grep -v "#"
	// curl -s http://127.0.0.1:9111/metrics 2>&1 | grep -v "#"

	inName := flag.String("inName", inNameCst, "inside interface to listen & send on")
	outName := flag.String("outName", outNameCst, "outside interface to listen & send on")
	altName := flag.String("altName", altNameCst, "alternative outside interface to listen & send on. leave blank for none")

	unicastDst := flag.String("unicastDst", unicastDstCst, "Fallback unicast destination for the unicast membership reports")

	proxyOutIn := flag.Bool("proxyOutIn", false, "Proxy IGMP from the outside to the inside")
	proxyInOut := flag.Bool("proxyInOut", false, "Proxy IGMP from the inside to the outside")
	// proxyOutIn := flag.Bool("proxyOutIn", ProxyOutToInCst, "Proxy IGMP from the outside to the inside")
	// proxyInOut := flag.Bool("proxyInOut", ProxyInToOutCst, "Proxy IGMP from the inside to the outside")
	//unicastProxyInToOut := flag.Bool("unicastProxyInToOut", UnicastProxyInToOutCst, "Proxy unicast IGMP from the inside to outside multicast")
	unicastProxyInToOut := flag.Bool("unicastProxyInToOut", false, "Proxy unicast IGMP from the inside to outside multicast")
	queryNotify := flag.Bool("queryNotify", false, "Listen for IGMP queries and notify on QueryNotifyCh")
	//queryNotify := flag.Bool("queryNotify", QueryNotifyCst, "Listen for IGMP queries and notify on QueryNotifyCh")
	membershipReportsFromNetwork := flag.Bool("membershipReportsFromNetwork", false, "Listen for IGMP membership reports and notify on MembershipReportFromNetworkCh")
	//membershipReportsFromNetwork := flag.Bool("membershipReportsFromNetwork", MembershipReportsFromNetworkCst, "Listen for IGMP membership reports and notify on MembershipReportFromNetworkCh")
	membershipReportsToNetwork := flag.Bool("membershipReportsToNetwork", false, "Read from MembershipReportToNetworkCh and send IGMP membership reports")
	//membershipReportsToNetwork := flag.Bool("membershipReportsToNetwork", MembershipReportsToNetworkCst, "Read from MembershipReportToNetworkCh and send IGMP membership reports")
	unicastMembershipReports := flag.Bool("unicastMembershipReports", false, "Send IGMP membership reports as unicast")
	//unicastMembershipReports := flag.Bool("unicastMembershipReports", UnicastMembershipReportsCst, "Send IGMP membership reports as unicast")
	connectQueryToReport := flag.Bool("connectQueryToReport", false, "Testing Option. Connect the query notify channel to the membership report channel.  This is for testing only.")
	//connectQueryToReport := flag.Bool("connectQueryToReport", ConnectQueryToReportCst, "Connect the query notify channel to the membership report channel.  This is for testing only.")
	membershipReportsReader := flag.Bool("membershipReportsReader", false, "Testing Option. Start a goroutine to read the membership report channel to stop is getting full and blocking.")
	leaveToNetwork := flag.Bool("leaveToNetwork", false, "LeaveToNetwork channel and sender")

	channelSize := flag.Int("channelSize", channelSizeCst, "channel size")

	readDeadline := flag.Duration("readDeadline", readDeadlineCst, "readDeadline sets the socket read deadline.  This impacts how quickly an IGMPReporter will detect context.Cancel and shutdown")

	gratuitous := flag.Duration("gratuitous", gratuitousCst, "gratuitous duration to send gratuitous reports")

	selfQuery := flag.Duration("selfQuery", selfQueryCst, "self query")

	mloopback := flag.Bool("mloopback", loopbackCst, "Enable loopback on the multicast send sockets")

	dl := flag.Int("dl", debugLevelCst, "nasty debugLevel")

	flag.Parse()

	if *version {
		fmt.Println("commit:", commit, "\tdate(UTC):", date)
		os.Exit(0)
	}

	go initPromHandler(*promPath, *promListen)

	testing := &goIGMP.TestingOptions{
		MulticastLoopback:       *mloopback,
		ConnectQueryToReport:    *connectQueryToReport,
		MembershipReportsReader: *membershipReportsReader,
	}

	if *outName == "" {
		di, err := getDefaultRouteInterface(ctx)
		if err != nil {
			log.Fatal("getDefaultRouteInterface err:", err)
		}
		outName = &di
	}

	conf := &goIGMP.Config{
		InIntName:                    *inName,
		OutIntName:                   *outName,
		AltOutIntName:                *altName,
		UnicastDst:                   *unicastDst,
		ProxyOutToIn:                 *proxyOutIn,
		ProxyInToOut:                 *proxyInOut,
		UnicastProxyInToOut:          *unicastProxyInToOut,
		QueryNotify:                  *queryNotify,
		MembershipReportsFromNetwork: *membershipReportsFromNetwork,
		MembershipReportsToNetwork:   *membershipReportsToNetwork,
		UnicastMembershipReports:     *unicastMembershipReports,
		LeaveToNetwork:               *leaveToNetwork,
		SocketReadDeadLine:           *readDeadline,
		ChannelSize:                  *channelSize,
		Gratuitous:                   *gratuitous,
		QueryTime:                    *selfQuery,
		DebugLevel:                   *dl,
		Testing:                      *testing,
	}

	r := goIGMP.NewIGMPReporter(*conf)

	log.Println("goIGMPExample.go r created")

	w := new(sync.WaitGroup)

	w.Add(1)
	r.Run(ctx, w)

	log.Println("goIGMPExample.go w.Wait()")
	w.Wait()

	log.Println("goIGMPExample.go all done bye")
}

// initSignalHandler sets up signal handling for the process, and
// will call cancel() when recieved
func initSignalHandler(cancel context.CancelFunc) {
	c := make(chan os.Signal, signalChannelSize)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c
	log.Printf("Signal caught, closing application")
	cancel()

	log.Printf("Signal caught, sleeping to allow goroutines to close")
	time.Sleep(cancelSleepTime)

	log.Printf("Sleep complete, goodbye! exit(0)")

	os.Exit(0)
}

// initPromHandler starts the prom handler with error checking
func initPromHandler(promPath string, promListen string) {
	// https: //pkg.go.dev/github.com/prometheus/client_golang/prometheus/promhttp?tab=doc#HandlerOpts
	http.Handle(promPath, promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			EnableOpenMetrics:   promEnableOpenMetrics,
			MaxRequestsInFlight: promMaxRequestsInFlight,
		},
	))
	go func() {
		err := http.ListenAndServe(promListen, nil)
		if err != nil {
			log.Fatal("prometheus error", err)
		}
	}()
}

// getDefaultRouteInterface find the default route interface name
func getDefaultRouteInterface(ctx context.Context) (string, error) {

	log.Println("getDefaultRouteInterface start")

	routes, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
	if err != nil {
		return "", err
	}

	for _, route := range routes {
		log.Printf("route:%s", route)
		if route.Dst == nil {
			link, err := netlink.LinkByIndex(route.LinkIndex)
			if err != nil {
				return "", err
			}
			return link.Attrs().Name, nil
		}
	}

	return "", fmt.Errorf("default route not found")
}

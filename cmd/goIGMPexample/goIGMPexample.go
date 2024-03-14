package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/randomizedcoder/goIGMP"
)

const (
	debugLevelCst = 11

	signalChannelSize = 10

	promListenCst           = ":7500"
	promPathCst             = "/metrics"
	promMaxRequestsInFlight = 10
	promEnableOpenMetrics   = true

	inNameCst  = "br-siden"
	outNameCst = "enp1s0"

	ProxyOutToInCst                 = true
	ProxyInToOutCst                 = false
	UnicastProxyInToOutCst          = true
	QueryNotifyCst                  = false
	MembershipReportsFromNetworkCst = false
	MembershipReportsToNetworkCst   = false
	UnicastMembershipReportsCst     = false
	ConnectQueryToReportCst         = false

	hackFilenameCst = "../../pcaps/ipmpv3_membership_report_s_172.17.200.10_g_232_0_0_1.payload"

	channelSizeCst = 10

	//joinTTLCst    = 60 * time.Second
	gratuitousCst = 600 * time.Second // These should be longer!!
	selfQueryCst  = 5 * time.Second
	loopbackCst   = false
)

var (
	// Passed by "go build -ldflags" for the show version
	commit string
	date   string

	//debugLevel int
)

func main() {

	_, cancel := context.WithCancel(context.Background())
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

	proxyOutIn := flag.Bool("proxyOutIn", false, "Proxy IGMP from the outside to the inside")
	proxyInOut := flag.Bool("proxyInOut", false, "Proxy IGMP from the inside to the outside")
	// proxyOutIn := flag.Bool("proxyOutIn", ProxyOutToInCst, "Proxy IGMP from the outside to the inside")
	// proxyInOut := flag.Bool("proxyInOut", ProxyInToOutCst, "Proxy IGMP from the inside to the outside")
	//unicastProxyInToOut := flag.Bool("unicastProxyInOut", UnicastProxyInToOutCst, "Proxy unicast IGMP from the inside to outside multicast")
	unicastProxyInToOut := flag.Bool("unicastProxyInOut", false, "Proxy unicast IGMP from the inside to outside multicast")
	queryNotify := flag.Bool("queryNotify", false, "Listen for IGMP queries and notify on QueryNotifyCh")
	//queryNotify := flag.Bool("queryNotify", QueryNotifyCst, "Listen for IGMP queries and notify on QueryNotifyCh")
	membershipReportsFromNetwork := flag.Bool("membershipReportsFromNetwork", false, "Listen for IGMP membership reports and notify on MembershipReportFromNetworkCh")
	//membershipReportsFromNetwork := flag.Bool("membershipReportsFromNetwork", MembershipReportsFromNetworkCst, "Listen for IGMP membership reports and notify on MembershipReportFromNetworkCh")
	membershipReportsToNetwork := flag.Bool("membershipReportsToNetwork", false, "Read from MembershipReportToNetworkCh and send IGMP membership reports")
	//membershipReportsToNetwork := flag.Bool("membershipReportsToNetwork", MembershipReportsToNetworkCst, "Read from MembershipReportToNetworkCh and send IGMP membership reports")
	unicastMembershipReports := flag.Bool("unicastMembershipReports", false, "Send IGMP membership reports as unicast")
	//unicastMembershipReports := flag.Bool("unicastMembershipReports", UnicastMembershipReportsCst, "Send IGMP membership reports as unicast")
	connectQueryToReport := flag.Bool("connectQueryToReport", false, "Connect the query notify channel to the membership report channel.  This is for testing only.")
	//connectQueryToReport := flag.Bool("connectQueryToReport", ConnectQueryToReportCst, "Connect the query notify channel to the membership report channel.  This is for testing only.")

	filename := flag.String("filename", hackFilenameCst, "filename of file with igmp membership payload")

	channelSize := flag.Int("channelSize", channelSizeCst, "channel size")

	gratuitous := flag.Duration("gratuitous", gratuitousCst, "gratuitous duration to send gratuitous reports")

	selfQuery := flag.Duration("selfQuery", selfQueryCst, "self query")

	loopback := flag.Bool("loopback", loopbackCst, "Enable loopback on the multicast send sockets")

	dl := flag.Int("dl", debugLevelCst, "nasty debugLevel")

	flag.Parse()

	if *version {
		fmt.Println("commit:", commit, "\tdate(UTC):", date)
		os.Exit(0)
	}

	go initPromHandler(*promPath, *promListen)

	conf := &goIGMP.Config{
		InIntName:                    *inName,
		OutIntName:                   *outName,
		ProxyOutToIn:                 *proxyOutIn,
		ProxyInToOut:                 *proxyInOut,
		UnicastProxyInToOut:          *unicastProxyInToOut,
		QueryNotify:                  *queryNotify,
		MembershipReportsFromNetwork: *membershipReportsFromNetwork,
		MembershipReportsToNetwork:   *membershipReportsToNetwork,
		UnicastMembershipReports:     *unicastMembershipReports,
		ConnectQueryToReport:         *connectQueryToReport,
		ChannelSize:                  *channelSize,
		Gratuitous:                   *gratuitous,
		QueryTime:                    *selfQuery,
		Loopback:                     *loopback,
		HackPayloadFilename:          *filename,
		DebugLevel:                   *dl,
	}

	r := goIGMP.NewIGMPReporter(*conf)

	log.Println("goIGMPExample.go r created")

	r.Run()
}

// initSignalHandler sets up signal handling for the process, and
// will call cancel() when recieved
func initSignalHandler(cancel context.CancelFunc) {
	c := make(chan os.Signal, signalChannelSize)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c
	log.Printf("Signal caught, closing application")
	cancel()
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

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

	promListenCst           = ":9901"
	promPathCst             = "/metrics"
	promMaxRequestsInFlight = 10
	promEnableOpenMetrics   = true

	intNameCst = "wlp0s20f3"
	//joinTTLCst    = 60 * time.Second
	gratuitousCst = 10 * time.Second // These should be longer!!
	selfQueryCst  = 5 * time.Second
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

	intName := flag.String("intName", intNameCst, "interface to listen & send on")
	//ttl := flag.Duration("ttl", joinTTLCst, "join ttl")
	gratuitous := flag.Duration("gratuitous", gratuitousCst, "gratuitous duration to send gratuitous reports")

	selfQuery := flag.Duration("selfQuery", selfQueryCst, "self query")

	dl := flag.Int("dl", debugLevelCst, "nasty debugLevel")

	flag.Parse()

	if *version {
		fmt.Println("commit:", commit, "\tdate(UTC):", date)
		os.Exit(0)
	}

	go initPromHandler(*promPath, *promListen)

	r := goIGMP.NewIGMPReporter(*intName, *gratuitous, *selfQuery, *dl)

	log.Println("r created")

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

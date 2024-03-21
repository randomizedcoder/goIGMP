package goIGMP

import (
	"fmt"
	"sync"
	"time"
)

func (r IGMPReporter) outInterfaceSelector(wg *sync.WaitGroup) {

	defer wg.Done()

	debugLog(r.debugLevel > 10, "outInterfaceSelector()")

	for loops := 0; ; loops++ {

		startTime := time.Now()
		r.pC.WithLabelValues("outInterfaceSelector", "loops", "count").Inc()

		outInt := <-r.OutInterfaceSelectorCh

		r.IntOutName.Store(IN, outInt)

		r.pC.WithLabelValues("outInterfaceSelector", outInt.String(), "count").Inc()
		r.pG.Set(float64(outInt))

		debugLog(r.debugLevel > 10, fmt.Sprintf("outInterfaceSelector() loops:%d outInt:%v", loops, outInt))

		r.pH.WithLabelValues("outInterfaceSelector", "loop", "complete").Observe(time.Since(startTime).Seconds())
	}
}

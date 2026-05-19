package metrics

import (
	"fmt"
	"sync/atomic"
	"time"
)

type Counters struct {
	Started         atomic.Int64
	Succeeded       atomic.Int64
	Failed          atomic.Int64
	Active          atomic.Int64
	TotalDurationMs atomic.Int64
}

var Global Counters

func Summary() string {
	started := Global.Started.Load()
	succeeded := Global.Succeeded.Load()
	failed := Global.Failed.Load()
	active := Global.Active.Load()
	totalMs := Global.TotalDurationMs.Load()

	var avgDur string
	completed := succeeded + failed
	if completed > 0 {
		avg := time.Duration(totalMs/completed) * time.Millisecond
		avgDur = avg.Round(time.Second).String()
	} else {
		avgDur = "n/a"
	}

	return fmt.Sprintf(
		"Active jobs: %d\nStarted total: %d\nSucceeded: %d  Failed: %d\nAvg duration: %s",
		active, started, succeeded, failed, avgDur,
	)
}

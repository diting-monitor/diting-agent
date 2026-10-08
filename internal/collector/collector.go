package collector

import (
	"sync"
	"time"

	protocol "github.com/diting-monitor/diting-protocol"
	"github.com/shirou/gopsutil/v4/cpu"
)

// Collector serves as the central facade for hardware and system metrics collection.
// It maintains state across sample intervals to compute differential rates
// (e.g. CPU percentages, network throughput, and disk I/O rates).
type Collector struct {
	mu sync.Mutex // guards concurrent access to differential sampling state and cached metadata

	// Cached static host hardware and operating system metadata.
	cachedMeta *protocol.MetaParams

	// Baseline CPU times from the previous tick for differential computation.
	prevCPUTimes cpu.TimesStat
	isCPUWarm    bool

	// Baseline network counters and timestamp for throughput delta calculation.
	prevNetInBytes  uint64
	prevNetOutBytes uint64
	prevNetDropIn   uint64
	prevNetDropOut  uint64
	prevNetErrIn    uint64
	prevNetTime     time.Time
	isNetWarm       bool // flags whether the initial cold-start baseline has been captured

	// Baseline disk I/O counters and timestamp for I/O rate delta calculation.
	prevDiskReadBytes  uint64
	prevDiskWriteBytes uint64
	prevDiskTime       time.Time
	isDiskIOWarm       bool // flags whether the initial disk I/O baseline has been captured
}

// New creates and initializes a new Collector instance.
func New() *Collector {
	now := time.Now()
	return &Collector{
		prevNetTime:  now,
		prevDiskTime: now,
	}
}

// calcRate computes the per-second rate of change between two cumulative counter values.
// It returns 0 if a counter reset or rollover occurs (cur < prev) or if timeDelta is non-positive.
func calcRate(cur, prev uint64, timeDelta float64) uint64 {
	if cur >= prev && timeDelta > 0 {
		return uint64(float64(cur-prev) / timeDelta)
	}
	return 0
}

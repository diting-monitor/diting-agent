package collector

import (
	"time"

	protocol "github.com/diting-monitor/diting-protocol"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

// getMetrics collects current dynamic performance metrics across hardware subsystems.
func (c *Collector) getMetrics() (protocol.MetricsPayload, error) {
	metrics := protocol.MetricsPayload{}

	// 1. CPU utilization via exact time-slice differential computation.
	if cpuPct, err := c.GetCPUPercent(); err == nil {
		metrics.CPUPercent = cpuPct
	}

	// 2. Physical memory usage and swap (with container cgroup quota awareness).
	if vmem, err := mem.VirtualMemory(); err == nil {
		metrics.MemoryUsed = vmem.Used
	}
	if _, cgroupUsage, ok := getCgroupMemory(); ok && cgroupUsage > 0 {
		metrics.MemoryUsed = cgroupUsage
	}
	if smem, err := mem.SwapMemory(); err == nil {
		metrics.SwapUsed = smem.Used
	}

	// 3. Physical disk aggregated usage.
	if dStat, err := GetDiskUsage(); err == nil {
		metrics.DiskUsed = dStat.Used
	}

	// 4. System load averages (1m, 5m, 15m).
	if lAvg, err := load.Avg(); err == nil {
		metrics.Load1 = lAvg.Load1
		metrics.Load5 = lAvg.Load5
		metrics.Load15 = lAvg.Load15
	}

	// 5. Network interface throughput rates and cumulative bytes.
	if netStats, err := c.GetNetMetrics(); err == nil {
		metrics.NetInBytes = netStats.NetInBytes
		metrics.NetOutBytes = netStats.NetOutBytes
		metrics.NetInRate = netStats.NetInRate
		metrics.NetOutRate = netStats.NetOutRate
	}

	// 6. Running process count and active TCP/UDP socket connections.
	metrics.ProcessCount = GetProcessCount()
	connStats := GetConnectionsCount()
	metrics.TCPCount = connStats.TCPCount
	metrics.UDPCount = connStats.UDPCount

	return metrics, nil
}

// CollectMetrics collects and packages dynamic performance metrics for agent.metrics.
func (c *Collector) CollectMetrics() (*protocol.MetricsParams, error) {
	metrics, err := c.getMetrics()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	params := &protocol.MetricsParams{
		Timestamp: now.UnixMilli(),
		Metrics:   metrics,
	}

	return params, nil
}

// GetCPUPercent calculates exact overall CPU utilization percentage (0.0 ~ 100.0) via differential time slices.
func (c *Collector) GetCPUPercent() (float64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	times, err := cpu.Times(false)
	if err != nil || len(times) == 0 {
		return 0, err
	}
	cur := times[0]

	if !c.isCPUWarm {
		c.prevCPUTimes = cur
		c.isCPUWarm = true
		return 0, nil
	}

	prev := c.prevCPUTimes
	c.prevCPUTimes = cur

	deltaTotal := cpuTotalTime(cur) - cpuTotalTime(prev)
	if deltaTotal <= 0 {
		return 0, nil
	}

	deltaIdle := cur.Idle - prev.Idle

	// Non-idle time ratio represents overall CPU busy percentage.
	busy := deltaTotal - deltaIdle
	if busy < 0 {
		busy = 0
	}
	return min(100.0, max(0.0, (busy/deltaTotal)*100.0)), nil
}

// cpuTotalTime aggregates CPU tick states into total ticks.
// Note: Guest and GuestNice are already included in User and Nice in Linux /proc/stat,
// so they are omitted here to prevent double-counting.
func cpuTotalTime(t cpu.TimesStat) float64 {
	return t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
}


package collector

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/diting-monitor/diting-agent/internal/version"
	"github.com/shirou/gopsutil/v4/disk"
)

func TestCollectMeta(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil Collector pointer")
	}

	info, err := c.CollectMeta()
	if err != nil {
		t.Fatalf("CollectMeta failed: %v", err)
	}

	if info.Version != version.Current {
		t.Errorf("Version mismatch: expected %q, got %q", version.Current, info.Version)
	}

	// Verify that subsequent calls return the cached metadata pointer.
	cachedInfo, err := c.CollectMeta()
	if err != nil {
		t.Fatalf("Secondary CollectMeta call failed: %v", err)
	}
	if cachedInfo != info {
		t.Errorf("Metadata cache miss: expected identical pointer on subsequent calls")
	}

	t.Logf("Host OS: %s (%s)", info.OS, info.Kernel)
	t.Logf("Hostname: %s", info.Hostname)
	t.Logf("Arch / CPU: %s / %s (%d cores)", info.Arch, info.CPUModel, info.CPUCores)
	t.Logf("Topology: %d physical cores / %d logical cores", info.CPUPhysicalCores, info.CPUCores)
	t.Logf("Physical memory: %d MB", info.MemoryTotal/(1024*1024))
	t.Logf("Total disk: %d GB", info.DiskTotal/(1024*1024*1024))
	t.Logf("Boot time: %d", info.BootTime)

	if info.Hostname == "" {
		t.Error("Hostname should not be empty")
	}
	if info.BootTime <= 0 {
		t.Error("BootTime must be greater than 0")
	}
	if info.CPUCores <= 0 {
		t.Errorf("Invalid CPUCores: %d", info.CPUCores)
	}
	if info.CPUPhysicalCores <= 0 {
		t.Errorf("Invalid CPUPhysicalCores: %d", info.CPUPhysicalCores)
	}
	if info.CPUPhysicalCores > info.CPUCores {
		t.Errorf("Physical cores (%d) should not exceed logical cores (%d)", info.CPUPhysicalCores, info.CPUCores)
	}
	if info.MemoryTotal <= 0 {
		t.Errorf("Invalid MemoryTotal: %d", info.MemoryTotal)
	}
}

func TestNetMetricsAndDeltaRate(t *testing.T) {
	c := New()

	// 1. Initial cold-start sample (rates must be 0)
	net1, err := c.GetNetMetrics()
	if err != nil {
		t.Fatalf("Cold-start net collection failed: %v", err)
	}
	if net1.NetInRate != 0 || net1.NetOutRate != 0 {
		t.Errorf("Cold-start rates must be 0, got in=%d, out=%d", net1.NetInRate, net1.NetOutRate)
	}
	t.Logf("Cold-start baseline: in=%d B, out=%d B", net1.NetInBytes, net1.NetOutBytes)

	// Wait 200ms before taking second sample to verify differential rate calculation
	time.Sleep(200 * time.Millisecond)

	net2, err := c.GetNetMetrics()
	if err != nil {
		t.Fatalf("Warm net collection failed: %v", err)
	}
	t.Logf("Warm sample: inRate=%d B/s, outRate=%d B/s, drops(in/out)=%d/%d drop/s, errs=%d err/s",
		net2.NetInRate, net2.NetOutRate, net2.NetDropInRate, net2.NetDropOutRate, net2.NetErrInRate)

	if net2.NetInBytes < net1.NetInBytes {
		t.Error("Cumulative net in bytes should not decrease")
	}
	if net2.NetOutBytes < net1.NetOutBytes {
		t.Error("Cumulative net out bytes should not decrease")
	}
}

func TestCollectMetrics(t *testing.T) {
	c := New()

	report, err := c.CollectMetrics()
	if err != nil {
		t.Fatalf("CollectMetrics failed: %v", err)
	}

	t.Logf("CPU utilization: %.2f%%", report.Metrics.CPUPercent)
	t.Logf("Memory used: %d MB", report.Metrics.MemoryUsed/(1024*1024))
	t.Logf("Disk used: %d GB", report.Metrics.DiskUsed/(1024*1024*1024))
	t.Logf("Load average: 1m=%.2f, 5m=%.2f, 15m=%.2f",
		report.Metrics.Load1, report.Metrics.Load5, report.Metrics.Load15)
	t.Logf("Process count: %d", report.Metrics.ProcessCount)
	t.Logf("Active sockets: TCP=%d, UDP=%d", report.Metrics.TCPCount, report.Metrics.UDPCount)
	t.Logf("Network throughput: in=%d B/s, out=%d B/s", report.Metrics.NetInRate, report.Metrics.NetOutRate)

	if report.Timestamp <= 0 {
		t.Error("Report timestamp must be greater than 0")
	}
	if report.Metrics.MemoryUsed <= 0 {
		t.Error("Memory used should not be 0")
	}
	if report.Metrics.ProcessCount <= 0 {
		t.Error("Process count should be positive")
	}
}

func TestDiskUsageAndIOMetrics(t *testing.T) {
	// 1. Disk usage aggregation and inode testing
	dStat, err := GetDiskUsage()
	if err != nil {
		t.Fatalf("GetDiskUsage failed: %v", err)
	}
	t.Logf("Disk aggregation: Total=%d GB, Used=%d GB, Inodes: %d / %d",
		dStat.Total/(1024*1024*1024), dStat.Used/(1024*1024*1024),
		dStat.InodesUsed, dStat.InodesTotal)

	if dStat.Total <= 0 || dStat.Used <= 0 {
		t.Errorf("Invalid disk capacity: total=%d, used=%d", dStat.Total, dStat.Used)
	}

	// 2. Disk I/O rate differential sampling
	c := New()
	ioCold, err := c.GetDiskIOMetrics()
	if err != nil {
		t.Fatalf("Cold-start disk I/O collection failed: %v", err)
	}
	if ioCold.DiskReadRate != 0 || ioCold.DiskWriteRate != 0 {
		t.Errorf("Cold-start disk I/O rates must be 0, got read=%d, write=%d", ioCold.DiskReadRate, ioCold.DiskWriteRate)
	}

	time.Sleep(100 * time.Millisecond)

	ioWarm, err := c.GetDiskIOMetrics()
	if err != nil {
		t.Fatalf("Warm disk I/O collection failed: %v", err)
	}
	t.Logf("Warm disk I/O throughput: read=%d B/s, write=%d B/s", ioWarm.DiskReadRate, ioWarm.DiskWriteRate)
}

func TestDiskWriteLive(t *testing.T) {
	c := New()

	// Initial baseline snapshot
	beforeMap, _ := disk.IOCounters()
	for k, v := range beforeMap {
		t.Logf("[Before Write] Device %s: read=%d, write=%d, readIO=%d, writeIO=%d",
			k, v.ReadBytes, v.WriteBytes, v.ReadCount, v.WriteCount)
	}
	_, _ = c.GetDiskIOMetrics()

	// Write 50MB to a sandboxed temporary file and force fsync
	tmpFile := filepath.Join(t.TempDir(), "test_write.tmp")

	f, err := os.Create(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	data := make([]byte, 1024*1024) // 1MB block
	for range 50 {
		_, _ = f.Write(data)
	}
	_ = f.Sync()
	_ = f.Close()

	// Subsequent sample to capture the write burst
	time.Sleep(200 * time.Millisecond)
	afterMap, _ := disk.IOCounters()
	for k, v := range afterMap {
		t.Logf("[After Write] Device %s: read=%d, write=%d, readIO=%d, writeIO=%d",
			k, v.ReadBytes, v.WriteBytes, v.ReadCount, v.WriteCount)
	}

	metric, err := c.GetDiskIOMetrics()
	if err != nil {
		t.Fatalf("Failed to collect post-write I/O metrics: %v", err)
	}
	t.Logf("Measured rate after 50MB write: read=%d B/s (%d KB/s), write=%d B/s (%d KB/s)",
		metric.DiskReadRate, metric.DiskReadRate/1024,
		metric.DiskWriteRate, metric.DiskWriteRate/1024)
}

func TestIsPhysicalDiskName(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		// macOS
		{"disk0", true},
		{"disk1", true},
		{"disk0s1", false},
		{"disk0s2", false},
		{"disk1s3", false},
		// Linux NVMe
		{"nvme0n1", true},
		{"nvme0n1p1", false},
		{"nvme1n1p2", false},
		// Linux SCSI/SATA/VirtIO
		{"sda", true},
		{"sda1", false},
		{"vda", true},
		{"vda2", false},
		// Virtual / Loop
		{"loop0", false},
		{"ram0", false},
		{"dm-0", false},
	}

	for _, tc := range tests {
		actual := isPhysicalDiskName(tc.name)
		if actual != tc.expected {
			t.Errorf("isPhysicalDiskName(%q) = %v, expected %v", tc.name, actual, tc.expected)
		}
	}
}

func TestGetDiskDeduplicationKey(t *testing.T) {
	tests := []struct {
		dev      string
		expected string
	}{
		{"/dev/disk3s1s1", "disk3"},
		{"/dev/disk3s5", "disk3"},
		{"/dev/disk3s6", "disk3"},
		{"/dev/disk0s2", "disk0"},
		{"/dev/sda1", "sda1"},
		{"/dev/nvme0n1p1", "nvme0n1p1"},
	}

	for _, tc := range tests {
		actual := getDiskDeduplicationKey(tc.dev)
		if actual != tc.expected {
			t.Errorf("getDiskDeduplicationKey(%q) = %q, expected %q", tc.dev, actual, tc.expected)
		}
	}
}

func TestCPUMetricsAndDelta(t *testing.T) {
	c := New()

	// Initial cold-start sample
	p1, err := c.GetCPUPercent()
	if err != nil {
		t.Fatalf("Cold-start CPU collection failed: %v", err)
	}
	t.Logf("Cold-start CPU baseline: %.2f%%", p1)

	// Wait 150ms before second sample to calculate time slice differential
	time.Sleep(150 * time.Millisecond)

	p2, err := c.GetCPUPercent()
	if err != nil {
		t.Fatalf("Warm CPU collection failed: %v", err)
	}
	t.Logf("Warm sample CPU: %.2f%%", p2)

	if p2 < 0 || p2 > 100 {
		t.Errorf("CPU percentage out of bounds [0, 100]: %.2f", p2)
	}
}

func TestCalcRate(t *testing.T) {
	testCases := []struct {
		name      string
		cur       uint64
		prev      uint64
		timeDelta float64
		expected  uint64
	}{
		{"Normal delta over 1s", 2000, 1000, 1.0, 1000},
		{"Normal delta over 2s", 3000, 1000, 2.0, 1000},
		{"Normal delta over 0.5s", 1500, 1000, 0.5, 1000},
		{"Zero delta", 1000, 1000, 1.0, 0},
		{"Rollover / reset guard", 500, 1000, 1.0, 0},
		{"Zero time delta guard", 2000, 1000, 0.0, 0},
		{"Negative time delta guard", 2000, 1000, -1.0, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := calcRate(tc.cur, tc.prev, tc.timeDelta)
			if actual != tc.expected {
				t.Errorf("calcRate(%d, %d, %f) = %d, expected %d",
					tc.cur, tc.prev, tc.timeDelta, actual, tc.expected)
			}
		})
	}
}

func TestCollector_ConcurrentSafety(t *testing.T) {
	c := New()

	const concurrency = 20
	const iterations = 5

	var wg sync.WaitGroup
	wg.Add(concurrency * 2)

	// Concurrent CollectMeta callers
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				meta, err := c.CollectMeta()
				if err != nil {
					t.Errorf("Concurrent CollectMeta error: %v", err)
				}
				if meta == nil {
					t.Errorf("Concurrent CollectMeta returned nil")
				}
			}
		}()
	}

	// Concurrent CollectMetrics callers
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				report, err := c.CollectMetrics()
				if err != nil {
					t.Errorf("Concurrent CollectMetrics error: %v", err)
				}
				if report == nil {
					t.Errorf("Concurrent CollectMetrics returned nil")
				}
			}
		}()
	}

	wg.Wait()
}

func TestCollector_ClockSkewSafety(t *testing.T) {
	c := New()

	// Initial warm up
	_, _ = c.GetNetMetrics()
	_, _ = c.GetDiskIOMetrics()

	// Simulate clock skew: set baseline timestamps far into the future
	c.mu.Lock()
	c.prevNetTime = time.Now().Add(1 * time.Hour)
	c.prevDiskTime = time.Now().Add(1 * time.Hour)
	c.mu.Unlock()

	// Both methods should handle timeDelta <= 0 defensively by returning zero rates
	netMetric, err := c.GetNetMetrics()
	if err != nil {
		t.Fatalf("GetNetMetrics failed during clock skew: %v", err)
	}
	if netMetric.NetInRate != 0 || netMetric.NetOutRate != 0 {
		t.Errorf("Expected 0 net rate on clock skew, got in=%d, out=%d",
			netMetric.NetInRate, netMetric.NetOutRate)
	}

	diskMetric, err := c.GetDiskIOMetrics()
	if err != nil {
		t.Fatalf("GetDiskIOMetrics failed during clock skew: %v", err)
	}
	if diskMetric.DiskReadRate != 0 || diskMetric.DiskWriteRate != 0 {
		t.Errorf("Expected 0 disk rate on clock skew, got read=%d, write=%d",
			diskMetric.DiskReadRate, diskMetric.DiskWriteRate)
	}
}

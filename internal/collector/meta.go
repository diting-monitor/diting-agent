package collector

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/diting-monitor/diting-agent/internal/version"
	protocol "github.com/diting-monitor/diting-protocol"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// CollectMeta gathers static host hardware and operating system metadata for agent.meta.
// The result is cached after the first discovery to eliminate duplicate system calls on reconnection.
func (c *Collector) CollectMeta() (*protocol.MetaParams, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cachedMeta != nil {
		return c.cachedMeta, nil
	}

	info := &protocol.MetaParams{
		Version: version.Current,
		Arch:    runtime.GOARCH,
	}

	// 1. Host and OS information.
	hInfo, err := host.Info()
	if err == nil {
		info.Hostname = hInfo.Hostname
		info.Kernel = hInfo.KernelVersion
		if hInfo.Platform != "" {
			info.OS = strings.TrimSpace(fmt.Sprintf("%s %s", hInfo.Platform, hInfo.PlatformVersion))
		} else {
			info.OS = hInfo.OS
		}
		if hInfo.KernelArch != "" {
			info.Arch = hInfo.KernelArch
		}
	} else {
		info.Hostname = "unknown-host"
		info.OS = runtime.GOOS
	}

	// 2. CPU cores and model.
	cores, err := cpu.Counts(true) // logical core count (vCPU / hyperthreading)
	if err == nil && cores > 0 {
		info.CPUCores = cores
	} else {
		info.CPUCores = runtime.NumCPU()
	}

	physCores, err := cpu.Counts(false) // physical core count
	if err == nil && physCores > 0 && physCores <= info.CPUCores {
		info.CPUPhysicalCores = physCores
	} else {
		// Fallback to logical cores when topology cannot be resolved (e.g. inside containers).
		info.CPUPhysicalCores = info.CPUCores
	}

	cpuInfos, err := cpu.Info()
	if err == nil && len(cpuInfos) > 0 {
		info.CPUModel = cpuInfos[0].ModelName
	}
	// Fallback for Apple Silicon when ModelName is not reported on macOS.
	if info.CPUModel == "" {
		if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
			info.CPUModel = "Apple Silicon"
		} else {
			info.CPUModel = "Unknown CPU"
		}
	}

	// 3. Memory and swap totals.
	vmem, err := mem.VirtualMemory()
	if err == nil {
		info.MemoryTotal = vmem.Total
	}

	// Honor container cgroup memory limits if lower than host physical memory.
	if cgroupLimit, _, ok := getCgroupMemory(); ok && cgroupLimit > 0 {
		if info.MemoryTotal == 0 || cgroupLimit < info.MemoryTotal {
			info.MemoryTotal = cgroupLimit
		}
	}

	smem, err := mem.SwapMemory()
	if err == nil {
		info.SwapTotal = smem.Total
	}

	// 4. Aggregated physical disk capacity.
	dStat, err := GetDiskUsage()
	if err == nil {
		info.DiskTotal = dStat.Total
	}

	// 5. Host virtualization / container runtime detection.
	info.Virtualization = DetectVirtualization()

	// 6. Absolute system boot timestamp.
	if btime, err := host.BootTime(); err == nil {
		info.BootTime = btime
	}

	c.cachedMeta = info
	return info, nil
}

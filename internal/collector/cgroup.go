package collector

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// cgroupV1UnlimitedThreshold represents the threshold for detecting unbounded memory in cgroup v1.
// When unlimited, the Linux kernel sets limit_in_bytes to PAGE_COUNTER_MAX (~9.22 Exabytes).
const cgroupV1UnlimitedThreshold uint64 = 9_000_000_000_000_000_000

// getCgroupMemory queries the Linux cgroup v1/v2 controller for active container memory limits and usage.
// This prevents containers (e.g. Docker, LXC) from erroneously reporting total host physical memory.
// Returns (0, 0, false) on non-Linux platforms or when running unconstrained.
func getCgroupMemory() (limit uint64, usage uint64, ok bool) {
	if runtime.GOOS != "linux" {
		return 0, 0, false
	}

	// 1. Attempt cgroups v2: /sys/fs/cgroup/memory.max & memory.current
	// If unconstrained, memory.max contains the string "max", which fails uint64 parsing.
	if maxLimit, ok := readCgroupUint64("/sys/fs/cgroup/memory.max"); ok && maxLimit > 0 {
		curUsage, _ := readCgroupUint64("/sys/fs/cgroup/memory.current")
		return maxLimit, curUsage, true
	}

	// 2. Attempt cgroups v1: /sys/fs/cgroup/memory/memory.limit_in_bytes & memory.usage_in_bytes
	if v1Limit, ok := readCgroupUint64("/sys/fs/cgroup/memory/memory.limit_in_bytes"); ok && v1Limit > 0 && v1Limit < cgroupV1UnlimitedThreshold {
		curUsage, _ := readCgroupUint64("/sys/fs/cgroup/memory/memory.usage_in_bytes")
		return v1Limit, curUsage, true
	}

	return 0, 0, false
}

// readCgroupUint64 reads and parses a uint64 value from a specified cgroup pseudo-file.
func readCgroupUint64(path string) (uint64, bool) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	val, err := strconv.ParseUint(strings.TrimSpace(string(bytes)), 10, 64)
	if err != nil {
		return 0, false
	}
	return val, true
}

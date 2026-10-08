package collector

import (
	"os"
	"runtime"
	"strconv"

	"github.com/shirou/gopsutil/v4/process"
)

// GetProcessCount retrieves the current total number of active processes running on the system.
// On Linux, it rapidly scans numeric subdirectories under /proc with zero slice heap allocations.
// On other operating systems (macOS, Windows), it falls back to gopsutil.
func GetProcessCount() int {
	if runtime.GOOS == "linux" {
		if count := countProcDirs("/proc"); count > 0 {
			return count
		}
	}

	// Fallback implementation for non-Linux platforms or restricted sandboxes.
	pids, err := process.Pids()
	if err == nil {
		return len(pids)
	}
	return 0
}

// countProcDirs scans a procfs directory and counts subdirectories with purely numeric names.
func countProcDirs(procPath string) int {
	entries, err := os.ReadDir(procPath)
	if err != nil {
		return 0
	}

	count := 0
	for _, entry := range entries {
		name := entry.Name()
		// Fast-path: Linux PIDs always start with digits '1'-'9'.
		// This skips system files and non-process directories (e.g. cpuinfo, meminfo, sys, net).
		if len(name) == 0 || name[0] < '1' || name[0] > '9' {
			continue
		}
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(name); err == nil {
			count++
		}
	}
	return count
}

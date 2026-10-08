package collector

import (
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

// DiskUsageStat aggregates capacity and inode statistics across physical disks.
type DiskUsageStat struct {
	Total       uint64 // Total physical storage capacity in bytes
	Used        uint64 // Used physical storage capacity in bytes
	InodesTotal uint64 // Total inode count (Linux/Unix; 0 on Windows)
	InodesUsed  uint64 // Used inode count
}

// GetDiskUsage aggregates physical disk storage capacity and inode statistics.
// It filters virtual filesystems, container overlays, and remote network mounts,
// deduplicating physical block devices to prevent inflated metrics from bind mounts or APFS container pools.
func GetDiskUsage() (DiskUsageStat, error) {
	stat := DiskUsageStat{}
	parts, err := disk.Partitions(true)
	if err != nil || len(parts) == 0 {
		// Fallback to root partition on error.
		u, err := disk.Usage("/")
		if err != nil {
			return stat, err
		}
		stat.Total = u.Total
		stat.Used = u.Used
		stat.InodesTotal = u.InodesTotal
		stat.InodesUsed = u.InodesUsed
		return stat, nil
	}

	seenDevices := make(map[string]bool)

	for _, p := range parts {
		if isIgnoredDisk(p) {
			continue
		}

		// Deduplicate physical block devices (e.g. Linux bind mounts or macOS APFS container pools).
		dedupKey := getDiskDeduplicationKey(p.Device)
		if dedupKey != "" && seenDevices[dedupKey] {
			continue
		}

		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}

		if dedupKey != "" {
			seenDevices[dedupKey] = true
		}
		stat.Total += u.Total
		stat.Used += u.Used
		stat.InodesTotal += u.InodesTotal
		stat.InodesUsed += u.InodesUsed
	}

	// Fallback to root partition if filtered results are empty.
	if stat.Total == 0 {
		u, err := disk.Usage("/")
		if err != nil {
			return stat, err
		}
		stat.Total = u.Total
		stat.Used = u.Used
		stat.InodesTotal = u.InodesTotal
		stat.InodesUsed = u.InodesUsed
		return stat, nil
	}

	return stat, nil
}

// getDiskDeduplicationKey extracts a unique identifier for block devices or containers to prevent duplicate counting.
func getDiskDeduplicationKey(dev string) string {
	dev = strings.TrimPrefix(dev, "/dev/")
	lower := strings.ToLower(dev)

	// macOS: APFS volumes share storage pools within a parent container (e.g. disk3s1, disk3s5 share disk3).
	// Normalize to the parent container to avoid counting shared space multiple times.
	if runtime.GOOS == "darwin" && strings.HasPrefix(lower, "disk") && len(lower) > 4 {
		rest := lower[4:]
		if idx := strings.Index(rest, "s"); idx != -1 {
			return lower[:4+idx] // returns "disk3"
		}
	}

	return lower
}

// isIgnoredDisk identifies and filters virtual filesystems, network mounts, and pseudo mountpoints.
func isIgnoredDisk(p disk.PartitionStat) bool {
	// Always retain the root mountpoint.
	if p.Mountpoint == "/" {
		return false
	}

	fstype := strings.ToLower(p.Fstype)

	// 1. Filter virtual, pseudo, and remote network filesystems.
	// Network filesystems (NFS, CIFS, SMB) are excluded to prevent blocking on network dropouts.
	ignoredFstypes := []string{
		"tmpfs", "devtmpfs", "udev", "overlay", "squashfs",
		"autofs", "iso9660", "cgroup", "cgroup2", "proc",
		"sysfs", "devpts", "mqueue", "securityfs", "hugetlbfs",
		"binfmt_misc", "fuse.portal", "nullfs", "rpc_pipefs",
		"tracefs", "debugfs", "configfs", "pstore",
		// Remote / Network filesystems
		"nfs", "nfs4", "cifs", "smbfs", "smb", "fuse.sshfs",
		"glusterfs", "ceph", "afs",
	}
	for _, vf := range ignoredFstypes {
		if fstype == vf || strings.HasPrefix(fstype, vf) {
			return true
		}
	}

	// 2. Filter reserved OS and container prefix paths.
	mp := strings.ToLower(p.Mountpoint)
	ignorePrefixes := []string{
		// Linux system and container runtime paths
		"/proc", "/sys", "/dev", "/run", "/var/run",
		"/tmp", "/var/tmp", "/var/lib/docker", "/var/lib/containerd",
		"/var/lib/containers", "/snap",
		// macOS internal APFS volumes and simulator paths
		"/system/volumes",
		"/volumes/recovery",
		"/private/var/folders",
		"/library/developer/coresimulator",
	}
	for _, prefix := range ignorePrefixes {
		if mp == prefix || strings.HasPrefix(mp, prefix+"/") {
			return true
		}
	}

	// 3. Filter loopback virtual image mounts.
	if strings.HasPrefix(p.Device, "/dev/loop") {
		return true
	}

	return false
}

// DiskIOMetrics represents instantaneous disk I/O throughput rates.
type DiskIOMetrics struct {
	DiskReadRate  uint64 // Instantaneous read throughput in Bytes/s
	DiskWriteRate uint64 // Instantaneous write throughput in Bytes/s
}

// GetDiskIOMetrics measures physical disk read/write throughput rates via differential calculation.
func (c *Collector) GetDiskIOMetrics() (DiskIOMetrics, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := DiskIOMetrics{}

	ioCounters, err := disk.IOCounters()
	if err != nil {
		return result, err
	}

	var curReadBytes uint64
	var curWriteBytes uint64

	for name, stat := range ioCounters {
		if !isPhysicalDiskName(name) {
			continue
		}
		curReadBytes += stat.ReadBytes
		curWriteBytes += stat.WriteBytes
	}

	now := time.Now()

	// Cold-start baseline capture.
	if !c.isDiskIOWarm {
		c.prevDiskReadBytes = curReadBytes
		c.prevDiskWriteBytes = curWriteBytes
		c.prevDiskTime = now
		c.isDiskIOWarm = true
		return result, nil
	}

	timeDelta := now.Sub(c.prevDiskTime).Seconds()
	if timeDelta <= 0 {
		return result, nil
	}

	result.DiskReadRate = calcRate(curReadBytes, c.prevDiskReadBytes, timeDelta)
	result.DiskWriteRate = calcRate(curWriteBytes, c.prevDiskWriteBytes, timeDelta)

	c.prevDiskReadBytes = curReadBytes
	c.prevDiskWriteBytes = curWriteBytes
	c.prevDiskTime = now

	return result, nil
}

// isPhysicalDiskName determines if a block device name represents a top-level physical drive,
// preventing double-counting of partition I/O on top of device-level I/O.
func isPhysicalDiskName(name string) bool {
	lower := strings.ToLower(name)

	// 1. Filter loopback, RAM disks, device-mapper (LVM/crypto), optical, and floppy devices.
	if strings.HasPrefix(lower, "loop") ||
		strings.HasPrefix(lower, "ram") ||
		strings.HasPrefix(lower, "zram") ||
		strings.HasPrefix(lower, "dm-") ||
		strings.HasPrefix(lower, "sr") ||
		strings.HasPrefix(lower, "fd") {
		return false
	}

	// 2. Linux: Check /sys/block/xxx for top-level block devices (e.g. sda exists, but sda1 does not).
	if runtime.GOOS == "linux" {
		if fileExists("/sys/block/" + lower) {
			return true
		}
	}

	// 3. macOS: disk0, disk1 are physical drives; disk0s1, disk0s2 are slice partitions.
	if strings.HasPrefix(lower, "disk") {
		if len(lower) > 4 {
			return !strings.Contains(lower[4:], "s")
		}
		return true
	}

	// 4. NVMe / eMMC: nvme0n1 is a physical drive; nvme0n1p1 is a partition.
	if strings.HasPrefix(lower, "nvme") || strings.HasPrefix(lower, "mmcblk") {
		return !(strings.Contains(lower, "p") && endsWithDigit(lower))
	}

	// 5. Standard sdX, vdX, xvdX, hdX: top-level drives do not end in digits, while partitions do (sda vs sda1).
	if strings.HasPrefix(lower, "sd") || strings.HasPrefix(lower, "vd") ||
		strings.HasPrefix(lower, "xvd") || strings.HasPrefix(lower, "hd") {
		return !endsWithDigit(lower)
	}

	return true
}

func endsWithDigit(s string) bool {
	if len(s) == 0 {
		return false
	}
	last := s[len(s)-1]
	return last >= '0' && last <= '9'
}

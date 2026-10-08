package collector

import (
	"testing"

	"github.com/shirou/gopsutil/v4/disk"
)

func TestIsIgnoredDisk(t *testing.T) {
	testCases := []struct {
		name     string
		part     disk.PartitionStat
		expected bool
	}{
		// 1. Root mountpoint must always be retained regardless of filesystem
		{
			name:     "Root ext4 mountpoint",
			part:     disk.PartitionStat{Mountpoint: "/", Fstype: "ext4", Device: "/dev/sda1"},
			expected: false,
		},
		{
			name:     "Root APFS mountpoint",
			part:     disk.PartitionStat{Mountpoint: "/", Fstype: "apfs", Device: "/dev/disk3s1s1"},
			expected: false,
		},

		// 2. Normal secondary partitions must be retained
		{
			name:     "Secondary ext4 partition",
			part:     disk.PartitionStat{Mountpoint: "/home", Fstype: "ext4", Device: "/dev/sda2"},
			expected: false,
		},
		{
			name:     "Secondary xfs partition",
			part:     disk.PartitionStat{Mountpoint: "/data", Fstype: "xfs", Device: "/dev/nvme0n1p1"},
			expected: false,
		},
		{
			name:     "Secondary btrfs partition",
			part:     disk.PartitionStat{Mountpoint: "/var", Fstype: "btrfs", Device: "/dev/vda1"},
			expected: false,
		},

		// 3. Remote / Network filesystems (must be ignored to prevent D-state hang)
		{
			name:     "NFS mount",
			part:     disk.PartitionStat{Mountpoint: "/mnt/nfs", Fstype: "nfs", Device: "192.168.1.10:/share"},
			expected: true,
		},
		{
			name:     "NFS4 mount",
			part:     disk.PartitionStat{Mountpoint: "/mnt/nfs4", Fstype: "nfs4", Device: "192.168.1.10:/share"},
			expected: true,
		},
		{
			name:     "CIFS mount",
			part:     disk.PartitionStat{Mountpoint: "/mnt/cifs", Fstype: "cifs", Device: "//192.168.1.10/share"},
			expected: true,
		},
		{
			name:     "SMB mount",
			part:     disk.PartitionStat{Mountpoint: "/mnt/smb", Fstype: "smbfs", Device: "//server/share"},
			expected: true,
		},
		{
			name:     "SSHFS mount",
			part:     disk.PartitionStat{Mountpoint: "/mnt/sshfs", Fstype: "fuse.sshfs", Device: "user@host:/"},
			expected: true,
		},
		{
			name:     "Ceph mount",
			part:     disk.PartitionStat{Mountpoint: "/mnt/ceph", Fstype: "ceph", Device: "10.0.0.1:/"},
			expected: true,
		},

		// 4. Virtual, pseudo, and memory filesystems
		{
			name:     "tmpfs in /run",
			part:     disk.PartitionStat{Mountpoint: "/run", Fstype: "tmpfs", Device: "tmpfs"},
			expected: true,
		},
		{
			name:     "devtmpfs in /dev",
			part:     disk.PartitionStat{Mountpoint: "/dev", Fstype: "devtmpfs", Device: "udev"},
			expected: true,
		},
		{
			name:     "cgroup2 filesystem",
			part:     disk.PartitionStat{Mountpoint: "/sys/fs/cgroup", Fstype: "cgroup2", Device: "cgroup2"},
			expected: true,
		},
		{
			name:     "overlay filesystem (Docker layer)",
			part:     disk.PartitionStat{Mountpoint: "/var/lib/docker/overlay2/merged", Fstype: "overlay", Device: "overlay"},
			expected: true,
		},
		{
			name:     "squashfs in /snap",
			part:     disk.PartitionStat{Mountpoint: "/snap/core/123", Fstype: "squashfs", Device: "/dev/loop1"},
			expected: true,
		},

		// 5. System prefix paths
		{
			name:     "procfs path",
			part:     disk.PartitionStat{Mountpoint: "/proc", Fstype: "proc", Device: "proc"},
			expected: true,
		},
		{
			name:     "sysfs path",
			part:     disk.PartitionStat{Mountpoint: "/sys", Fstype: "sysfs", Device: "sysfs"},
			expected: true,
		},
		{
			name:     "macOS System Volumes path",
			part:     disk.PartitionStat{Mountpoint: "/System/Volumes/Data", Fstype: "apfs", Device: "/dev/disk3s5"},
			expected: true,
		},
		{
			name:     "macOS Recovery path",
			part:     disk.PartitionStat{Mountpoint: "/Volumes/Recovery", Fstype: "apfs", Device: "/dev/disk3s3"},
			expected: true,
		},

		// 6. Loopback virtual image mounts
		{
			name:     "Loopback device",
			part:     disk.PartitionStat{Mountpoint: "/mnt/loop", Fstype: "ext4", Device: "/dev/loop0"},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := isIgnoredDisk(tc.part)
			if actual != tc.expected {
				t.Errorf("isIgnoredDisk(%+v) = %v, expected %v", tc.part, actual, tc.expected)
			}
		})
	}
}

func TestEndsWithDigit(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"disk0", true},
		{"disk10", true},
		{"nvme0n1", true},
		{"sda", false},
		{"vda", false},
		{"", false},
		{"a", false},
		{"0", true},
		{"9", true},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			actual := endsWithDigit(tc.input)
			if actual != tc.expected {
				t.Errorf("endsWithDigit(%q) = %v, expected %v", tc.input, actual, tc.expected)
			}
		})
	}
}

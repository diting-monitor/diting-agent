package collector

import (
	"bufio"
	"os"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/host"
)

// dmiSignatures maps vendor or hypervisor keywords to canonical virtualization identifiers.
var dmiSignatures = []struct {
	keyword  string
	virtType string
}{
	{"kvm", "kvm"},
	{"qemu", "kvm"},
	{"bochs", "kvm"},
	{"seabios", "kvm"},
	{"vmware", "vmware"},
	{"virtualbox", "vbox"},
	{"innotek", "vbox"},
	{"hyper-v", "hyper-v"},
	{"microsoft", "hyper-v"},
	{"xen", "xen"},
	{"openstack", "openstack"},
	{"bhyve", "bhyve"},
	// Major cloud hypervisors predominantly built on KVM / Nitro:
	{"amazon", "kvm"},
	{"alibaba", "kvm"},
	{"tencent", "kvm"},
	{"google", "kvm"},
}

// DetectVirtualization inspects the host environment to determine the virtualization or container platform
// (e.g., kvm, vmware, vbox, docker, lxc, openvz, wsl, or bare-metal).
func DetectVirtualization() string {
	if runtime.GOOS != "linux" {
		if runtime.GOOS == "windows" {
			if virt := detectWindowsVirtualization(); virt != "" {
				return virt
			}
		}
		// Attempt cross-platform hypervisor detection (e.g. macOS sysctl).
		if vSys, _, err := host.Virtualization(); err == nil && vSys != "" {
			return strings.ToLower(vSys)
		}
		// Default to bare-metal when no hypervisor is detected.
		return "bare-metal"
	}

	// 1. Fast-path container environment detection via sentinel files.
	if fileExists("/.dockerenv") {
		return "docker"
	}
	if fileExists("/run/.containerenv") {
		return "podman"
	}
	if fileExists("/dev/.lxc-boot-id") {
		return "lxc"
	}
	if fileExists("/proc/vz") && !fileExists("/proc/bc") {
		return "openvz"
	}

	// 2. Check WSL (Windows Subsystem for Linux).
	if fileExists("/proc/sys/fs/binfmt_misc/WSLInterop") {
		return "wsl"
	}
	if vData, err := os.ReadFile("/proc/version"); err == nil {
		if strings.Contains(strings.ToLower(string(vData)), "microsoft") {
			return "wsl"
		}
	}

	// 3. Inspect /proc/self/cgroup for container markers.
	if cgData, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		cgStr := strings.ToLower(string(cgData))
		if strings.Contains(cgStr, "docker") {
			return "docker"
		}
		if strings.Contains(cgStr, "kubepods") {
			return "k8s"
		}
		if strings.Contains(cgStr, "libpod") {
			return "podman"
		}
		if strings.Contains(cgStr, "/lxc/") {
			return "lxc"
		}
	}

	// 4. Inspect DMI BIOS and product vendor strings for hardware virtualization.
	dmiVendors := []string{
		"/sys/class/dmi/id/product_name",
		"/sys/class/dmi/id/sys_vendor",
		"/sys/class/dmi/id/bios_vendor",
	}
	for _, path := range dmiVendors {
		if content, err := readFirstLine(path); err == nil && content != "" {
			if virt := matchDMIVendor(content); virt != "" {
				return virt
			}
		}
	}

	// 5. Inspect /proc/1/environ when DMI table is unavailable (common in minimalist containers).
	if !fileExists("/sys/class/dmi/id/product_name") {
		if envData, err := os.ReadFile("/proc/1/environ"); err == nil {
			if virt := parseContainerEnv(envData); virt != "" {
				return virt
			}
		}
	}

	return "bare-metal"
}

// matchDMIVendor matches raw DMI strings against known virtualization signatures.
func matchDMIVendor(content string) string {
	lower := strings.ToLower(content)
	for _, sig := range dmiSignatures {
		if strings.Contains(lower, sig.keyword) {
			return sig.virtType
		}
	}
	return ""
}

// parseContainerEnv extracts the container runtime identifier from /proc/1/environ data.
// In systemd/Linux containers, environment variables are null-delimited (e.g. "container=lxc\0").
func parseContainerEnv(envData []byte) string {
	for part := range strings.SplitSeq(string(envData), "\x00") {
		if after, ok := strings.CutPrefix(part, "container="); ok {
			val := strings.TrimSpace(after)
			if val != "" {
				return val
			}
			return "container"
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readFirstLine(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), nil
	}
	return "", scanner.Err()
}

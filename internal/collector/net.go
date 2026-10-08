package collector

import (
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/net"
)

// NetMetrics encapsulates instantaneous network interface telemetry.
type NetMetrics struct {
	NetInBytes     uint64 // Cumulative received bytes across physical interfaces since boot
	NetOutBytes    uint64 // Cumulative transmitted bytes across physical interfaces since boot
	NetInRate      uint64 // Instantaneous download throughput (Bytes/s)
	NetOutRate     uint64 // Instantaneous upload throughput (Bytes/s)
	NetDropInRate  uint64 // Inbound packet drop rate (packets/s)
	NetDropOutRate uint64 // Outbound packet drop rate (packets/s)
	NetErrInRate   uint64 // Inbound packet error rate (packets/s)
}

// IsIgnoredInterface filters virtual, container, VPN, bridge, and tunnel interfaces,
// ensuring only physical network interfaces are included in bandwidth telemetry.
func IsIgnoredInterface(name string) bool {
	lower := strings.ToLower(name)

	// 1. Loopback and pseudo-local dummy interfaces.
	if strings.HasPrefix(lower, "lo") || strings.HasPrefix(lower, "dummy") {
		return true
	}

	// 2. Containers and Cloud-Native CNI virtual networks (Docker, Podman, K8s).
	if strings.HasPrefix(lower, "docker") ||
		strings.HasPrefix(lower, "veth") ||
		strings.HasPrefix(lower, "cni") ||
		strings.HasPrefix(lower, "flannel") ||
		strings.HasPrefix(lower, "calico") ||
		strings.HasPrefix(lower, "cilium") {
		return true
	}

	// 3. Host and virtualization bridges (Docker bridges, br0, bridge0, libvirt, VirtualBox, VMware).
	if strings.HasPrefix(lower, "br-") ||
		strings.HasPrefix(lower, "bridge") ||
		(strings.HasPrefix(lower, "br") && len(lower) > 2 && lower[2] >= '0' && lower[2] <= '9') ||
		strings.HasPrefix(lower, "virbr") ||
		strings.HasPrefix(lower, "vboxnet") ||
		strings.HasPrefix(lower, "vmnet") {
		return true
	}

	// 4. VPN and mesh overlays (Tailscale, WireGuard, OpenVPN, ZeroTier, macOS utun).
	if strings.HasPrefix(lower, "tailscale") ||
		strings.HasPrefix(lower, "wg") ||
		strings.HasPrefix(lower, "wireguard") ||
		strings.HasPrefix(lower, "tun") ||
		strings.HasPrefix(lower, "utun") ||
		strings.HasPrefix(lower, "tap") ||
		strings.HasPrefix(lower, "zt") {
		return true
	}

	// 5. Tunnels, encapsulation, and Apple peer-to-peer meshes (AWDL, AirDrop, Sidecar).
	if strings.HasPrefix(lower, "gre") ||
		strings.HasPrefix(lower, "sit") ||
		strings.HasPrefix(lower, "ip6tnl") ||
		strings.HasPrefix(lower, "erspan") ||
		strings.HasPrefix(lower, "awdl") ||
		strings.HasPrefix(lower, "llw") ||
		strings.HasPrefix(lower, "gif") ||
		strings.HasPrefix(lower, "stf") {
		return true
	}

	return false
}

// GetNetMetrics measures instantaneous network throughput and packet rates via differential calculation.
func (c *Collector) GetNetMetrics() (NetMetrics, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := NetMetrics{}

	// 1. Query real-time I/O counters across all network interfaces.
	counters, err := net.IOCounters(true)
	if err != nil {
		return result, err
	}

	var curInBytes uint64
	var curOutBytes uint64
	var curDropIn uint64
	var curDropOut uint64
	var curErrIn uint64

	// 2. Sum metrics across valid physical interfaces.
	for _, io := range counters {
		if IsIgnoredInterface(io.Name) {
			continue
		}
		curInBytes += io.BytesRecv
		curOutBytes += io.BytesSent
		curDropIn += io.Dropin
		curDropOut += io.Dropout
		curErrIn += io.Errin
	}

	result.NetInBytes = curInBytes
	result.NetOutBytes = curOutBytes

	now := time.Now()

	// 3. Handle cold-start baseline capture.
	if !c.isNetWarm {
		c.prevNetInBytes = curInBytes
		c.prevNetOutBytes = curOutBytes
		c.prevNetDropIn = curDropIn
		c.prevNetDropOut = curDropOut
		c.prevNetErrIn = curErrIn
		c.prevNetTime = now
		c.isNetWarm = true
		return result, nil
	}

	// 4. Calculate sampling interval in seconds.
	timeDelta := now.Sub(c.prevNetTime).Seconds()
	if timeDelta <= 0 {
		return result, nil
	}

	// 5. Differential rate calculation: Rate = Delta / TimeDelta.
	result.NetInRate = calcRate(curInBytes, c.prevNetInBytes, timeDelta)
	result.NetOutRate = calcRate(curOutBytes, c.prevNetOutBytes, timeDelta)
	result.NetDropInRate = calcRate(curDropIn, c.prevNetDropIn, timeDelta)
	result.NetDropOutRate = calcRate(curDropOut, c.prevNetDropOut, timeDelta)
	result.NetErrInRate = calcRate(curErrIn, c.prevNetErrIn, timeDelta)

	// 6. Update baseline snapshot.
	c.prevNetInBytes = curInBytes
	c.prevNetOutBytes = curOutBytes
	c.prevNetDropIn = curDropIn
	c.prevNetDropOut = curDropOut
	c.prevNetErrIn = curErrIn
	c.prevNetTime = now

	return result, nil
}

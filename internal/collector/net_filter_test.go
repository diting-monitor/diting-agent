package collector

import "testing"

func TestIsIgnoredInterface(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
	}{
		// Virtual, loopback, container, bridge, and VPN interfaces that should be ignored
		{"lo", true},
		{"lo0", true},
		{"dummy0", true},
		{"docker0", true},
		{"docker_gwbridge", true},
		{"vethabc123", true},
		{"br-0123456789ab", true},
		{"br0", true},
		{"bridge0", true},
		{"cni0", true},
		{"flannel.1", true},
		{"calico123", true},
		{"cilium_vxlan", true},
		{"virbr0", true},
		{"vboxnet0", true},
		{"vmnet8", true},
		{"tailscale0", true},
		{"wg0", true},
		{"wireguard", true},
		{"tun0", true},
		{"utun0", true},
		{"utun3", true},
		{"awdl0", true},
		{"llw0", true},
		{"tap1", true},
		{"zt0", true},
		{"gre0", true},
		{"sit0", true},

		// Physical hardware interfaces that must be retained
		{"eth0", false},
		{"eth1", false},
		{"ens3", false},
		{"ens33", false},
		{"eno1", false},
		{"enp0s3", false},
		{"en0", false},
		{"en1", false},
		{"wlan0", false},
		{"wlp2s0", false},
	}

	for _, tc := range testCases {
		actual := IsIgnoredInterface(tc.name)
		if actual != tc.expected {
			t.Errorf("IsIgnoredInterface(%q) = %v, expected %v", tc.name, actual, tc.expected)
		}
	}
}

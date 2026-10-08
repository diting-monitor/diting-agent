package collector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectVirtualization(t *testing.T) {
	virt := DetectVirtualization()
	if virt == "" {
		t.Fatal("DetectVirtualization should not return an empty string")
	}
	t.Logf("Detected virtualization environment: %s", virt)
}

func TestMatchDMIVendor(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"QEMU Standard PC (Q35 + ICH9, 2009)", "kvm"},
		{"KVM", "kvm"},
		{"Bochs", "kvm"},
		{"SeaBIOS", "kvm"},
		{"VMware, Inc. VMware Virtual Platform", "vmware"},
		{"VirtualBox", "vbox"},
		{"innotek GmbH", "vbox"},
		{"Microsoft Corporation Hyper-V", "hyper-v"},
		{"Xen", "xen"},
		{"OpenStack Nova", "openstack"},
		{"Bhyve", "bhyve"},
		{"Amazon EC2", "kvm"},
		{"Alibaba Cloud ECS", "kvm"},
		{"Tencent Cloud CVM", "kvm"},
		{"Google Compute Engine", "kvm"},
		// Physical hardware vendor names should not match
		{"Dell Inc. PowerEdge R740", ""},
		{"Supermicro SYS-5019", ""},
		{"HP ProLiant DL380 Gen10", ""},
		{"Apple Inc. Mac15,12", ""},
		{"", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			actual := matchDMIVendor(tc.input)
			if actual != tc.expected {
				t.Errorf("matchDMIVendor(%q) = %q, expected %q", tc.input, actual, tc.expected)
			}
		})
	}
}

func TestParseContainerEnv(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Docker container",
			input:    "PATH=/usr/local/bin\x00container=docker\x00HOME=/root\x00",
			expected: "docker",
		},
		{
			name:     "LXC container",
			input:    "container=lxc\x00PATH=/bin\x00",
			expected: "lxc",
		},
		{
			name:     "Podman container",
			input:    "container=podman\x00",
			expected: "podman",
		},
		{
			name:     "Generic container variable with empty value",
			input:    "container=\x00PATH=/bin\x00",
			expected: "container",
		},
		{
			name:     "No container variable",
			input:    "PATH=/usr/bin\x00USER=root\x00",
			expected: "",
		},
		{
			name:     "Empty environment",
			input:    "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := parseContainerEnv([]byte(tc.input))
			if actual != tc.expected {
				t.Errorf("parseContainerEnv() = %q, expected %q", actual, tc.expected)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	tempDir := t.TempDir()
	existingFile := filepath.Join(tempDir, "existing.txt")
	if err := os.WriteFile(existingFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	if !fileExists(existingFile) {
		t.Errorf("fileExists(%q) = false, expected true", existingFile)
	}

	nonExistingFile := filepath.Join(tempDir, "non_existent.txt")
	if fileExists(nonExistingFile) {
		t.Errorf("fileExists(%q) = true, expected false", nonExistingFile)
	}
}

func TestReadFirstLine(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("Multi-line file", func(t *testing.T) {
		p := filepath.Join(tempDir, "multiline.txt")
		if err := os.WriteFile(p, []byte("  First Line  \nSecond Line\nThird Line"), 0644); err != nil {
			t.Fatalf("Failed to write multiline file: %v", err)
		}
		line, err := readFirstLine(p)
		if err != nil {
			t.Fatalf("readFirstLine failed: %v", err)
		}
		if line != "First Line" {
			t.Errorf("readFirstLine = %q, expected %q", line, "First Line")
		}
	})

	t.Run("Empty file", func(t *testing.T) {
		p := filepath.Join(tempDir, "empty.txt")
		if err := os.WriteFile(p, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to write empty file: %v", err)
		}
		line, err := readFirstLine(p)
		if err != nil {
			t.Fatalf("readFirstLine failed: %v", err)
		}
		if line != "" {
			t.Errorf("readFirstLine = %q, expected empty string", line)
		}
	})

	t.Run("Non-existent file", func(t *testing.T) {
		_, err := readFirstLine(filepath.Join(tempDir, "does_not_exist"))
		if err == nil {
			t.Error("Expected error for non-existent file, got nil")
		}
	})
}

func TestDetectWindowsVirtualization_Stub(t *testing.T) {
	// On non-Windows platforms, detectWindowsVirtualization is a stub that returns "".
	// On Windows, it reads the registry and may return a hypervisor or "".
	res := detectWindowsVirtualization()
	t.Logf("detectWindowsVirtualization result: %q", res)
}

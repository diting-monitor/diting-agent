package collector

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetCgroupMemory(t *testing.T) {
	limit, usage, ok := getCgroupMemory()

	if runtime.GOOS != "linux" {
		if ok || limit != 0 || usage != 0 {
			t.Errorf("Expected (0, 0, false) on non-linux OS (%s), got (%d, %d, %v)",
				runtime.GOOS, limit, usage, ok)
		}
	} else {
		// On Linux, if running inside a container with memory limits, ok may be true
		if ok {
			if limit == 0 {
				t.Error("Limit should be greater than 0 when ok is true")
			}
			t.Logf("Detected cgroup memory: limit=%d bytes, usage=%d bytes", limit, usage)
		} else {
			t.Log("No cgroup memory limit active on host")
		}
	}
}

func TestReadCgroupUint64(t *testing.T) {
	tempDir := t.TempDir()

	testCases := []struct {
		name        string
		content     string
		createFile  bool
		expectedVal uint64
		expectedOK  bool
	}{
		{
			name:        "Valid uint64 with newline",
			content:     "1073741824\n",
			createFile:  true,
			expectedVal: 1073741824,
			expectedOK:  true,
		},
		{
			name:        "Valid uint64 with spaces",
			content:     "  536870912  ",
			createFile:  true,
			expectedVal: 536870912,
			expectedOK:  true,
		},
		{
			name:        "Literal max in cgroup v2",
			content:     "max\n",
			createFile:  true,
			expectedVal: 0,
			expectedOK:  false,
		},
		{
			name:        "Empty file",
			content:     "",
			createFile:  true,
			expectedVal: 0,
			expectedOK:  false,
		},
		{
			name:        "Non-numeric string",
			content:     "unlimited",
			createFile:  true,
			expectedVal: 0,
			expectedOK:  false,
		},
		{
			name:        "Non-existent file",
			content:     "",
			createFile:  false,
			expectedVal: 0,
			expectedOK:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tempDir, tc.name)
			if tc.createFile {
				if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			}

			val, ok := readCgroupUint64(path)
			if ok != tc.expectedOK || val != tc.expectedVal {
				t.Errorf("readCgroupUint64(%q) = (%d, %v), expected (%d, %v)",
					path, val, ok, tc.expectedVal, tc.expectedOK)
			}
		})
	}
}

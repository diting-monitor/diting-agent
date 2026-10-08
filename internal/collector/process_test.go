package collector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetProcessCount(t *testing.T) {
	count := GetProcessCount()
	if count <= 0 {
		t.Errorf("Expected positive process count, got %d", count)
	}
	t.Logf("Active processes on host: %d", count)
}

func TestCountProcDirs(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("Non-existent directory", func(t *testing.T) {
		count := countProcDirs(filepath.Join(tempDir, "does_not_exist"))
		if count != 0 {
			t.Errorf("Expected 0 for non-existent directory, got %d", count)
		}
	})

	t.Run("Empty directory", func(t *testing.T) {
		emptyDir := filepath.Join(tempDir, "empty")
		if err := os.Mkdir(emptyDir, 0755); err != nil {
			t.Fatalf("Failed to create empty directory: %v", err)
		}
		count := countProcDirs(emptyDir)
		if count != 0 {
			t.Errorf("Expected 0 for empty directory, got %d", count)
		}
	})

	t.Run("Mock procfs with mixed entries", func(t *testing.T) {
		mockProc := filepath.Join(tempDir, "mock_proc")
		if err := os.Mkdir(mockProc, 0755); err != nil {
			t.Fatalf("Failed to create mock proc directory: %v", err)
		}

		// Valid PID directories (should be counted)
		validPIDs := []string{"1", "42", "65535"}
		for _, pid := range validPIDs {
			if err := os.Mkdir(filepath.Join(mockProc, pid), 0755); err != nil {
				t.Fatalf("Failed to create PID dir %s: %v", pid, err)
			}
		}

		// Numeric regular file (should NOT be counted as process)
		numericFile := filepath.Join(mockProc, "100")
		if err := os.WriteFile(numericFile, []byte("data"), 0644); err != nil {
			t.Fatalf("Failed to create numeric file: %v", err)
		}

		// Non-numeric system directories (should NOT be counted)
		sysDirs := []string{"net", "sys", "bus"}
		for _, d := range sysDirs {
			if err := os.Mkdir(filepath.Join(mockProc, d), 0755); err != nil {
				t.Fatalf("Failed to create sys dir %s: %v", d, err)
			}
		}

		// Non-numeric system files (should NOT be counted)
		sysFiles := []string{"cpuinfo", "meminfo", "version", "stat"}
		for _, f := range sysFiles {
			if err := os.WriteFile(filepath.Join(mockProc, f), []byte("info"), 0644); err != nil {
				t.Fatalf("Failed to create sys file %s: %v", f, err)
			}
		}

		// Directory starting with 0 (should NOT be counted as valid Linux PID)
		if err := os.Mkdir(filepath.Join(mockProc, "0123"), 0755); err != nil {
			t.Fatalf("Failed to create 0-prefixed dir: %v", err)
		}

		count := countProcDirs(mockProc)
		if count != len(validPIDs) {
			t.Errorf("Expected %d process directories, got %d", len(validPIDs), count)
		}
	})
}

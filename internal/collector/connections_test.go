package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetConnectionsCount(t *testing.T) {
	metrics := GetConnectionsCount()

	if metrics.TCPCount < 0 {
		t.Errorf("TCPCount should be non-negative, got: %d", metrics.TCPCount)
	}
	if metrics.UDPCount < 0 {
		t.Errorf("UDPCount should be non-negative, got: %d", metrics.UDPCount)
	}

	t.Logf("Active sockets: TCP=%d, UDP=%d", metrics.TCPCount, metrics.UDPCount)
}

func TestCountProcNetLines(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("Non-existent file", func(t *testing.T) {
		count := countProcNetLines(filepath.Join(tempDir, "does_not_exist"))
		if count != 0 {
			t.Errorf("Expected 0 for non-existent file, got %d", count)
		}
	})

	t.Run("Empty file", func(t *testing.T) {
		emptyFile := filepath.Join(tempDir, "empty")
		if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to write empty file: %v", err)
		}
		count := countProcNetLines(emptyFile)
		if count != 0 {
			t.Errorf("Expected 0 for empty file, got %d", count)
		}
	})

	t.Run("Header only file", func(t *testing.T) {
		headerFile := filepath.Join(tempDir, "header_only")
		content := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"
		if err := os.WriteFile(headerFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write header file: %v", err)
		}
		count := countProcNetLines(headerFile)
		if count != 0 {
			t.Errorf("Expected 0 for header-only file, got %d", count)
		}
	})

	t.Run("Header and three connections", func(t *testing.T) {
		dataFile := filepath.Join(tempDir, "three_conns")
		lines := []string{
			"  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode",
			"   0: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0",
			"   1: 0100007F:0050 0200007F:1F90 01 00000000:00000000 00:00000000 00000000  1000        0 23456 1 0000000000000000 100 0 0 10 0",
			"   2: 0100007F:0050 0300007F:1F91 01 00000000:00000000 00:00000000 00000000  1000        0 34567 1 0000000000000000 100 0 0 10 0",
		}
		content := strings.Join(lines, "\n") + "\n"
		if err := os.WriteFile(dataFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write data file: %v", err)
		}
		count := countProcNetLines(dataFile)
		if count != 3 {
			t.Errorf("Expected 3 for file with 3 connections, got %d", count)
		}
	})
}

//go:build !windows

package collector

// detectWindowsVirtualization is a stub for non-Windows platforms.
func detectWindowsVirtualization() string {
	return ""
}

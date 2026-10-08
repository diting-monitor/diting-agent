//go:build windows

package collector

import (
	"golang.org/x/sys/windows/registry"
)

// detectWindowsVirtualization inspects the Windows Registry for hardware BIOS manufacturer and model strings.
func detectWindowsVirtualization() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()

	// Inspect system product name, manufacturer, and BIOS vendor against known signatures.
	for _, valName := range []string{"SystemProductName", "SystemManufacturer", "BIOSVendor"} {
		if val, _, err := k.GetStringValue(valName); err == nil && val != "" {
			if virt := matchDMIVendor(val); virt != "" {
				return virt
			}
		}
	}
	return ""
}

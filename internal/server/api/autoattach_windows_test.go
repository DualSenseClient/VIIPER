//go:build windows

package api

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestUsbipAttachABISizes(t *testing.T) {
	require.Equal(t, uintptr(1120), unsafe.Sizeof(attachIOCTL{}),
		"usbip-win2 0.9.8.0 plugin_hardware ABI changed")
	require.Equal(t, uintptr(1100), unsafe.Sizeof(attachIOCTL077{}),
		"usbip-win2 0.9.7.7 plugin_hardware ABI changed")
	// Serial must start at 1100 (after the 3-byte C++ base tail padding),
	// wsk_events at 1116.
	require.Equal(t, uintptr(1100), unsafe.Offsetof(attachIOCTL{}.Serial))
	require.Equal(t, uintptr(1116), unsafe.Offsetof(attachIOCTL{}.WskEvents))
}

func TestNativeAutoAttachResultCarriesExactPort(t *testing.T) {
	got := AutoAttachResult{
		USBIPPort: 7,
	}

	require.Equal(t, int32(7), got.USBIPPort)
	require.Empty(t, got.USBIPOwnerSerial)
}

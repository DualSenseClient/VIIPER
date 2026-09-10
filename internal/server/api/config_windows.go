//go:build windows

package api

type PlatformOpts struct {
	AutoAttachWindowsNative bool `default:"true" help:"Use native IOCTL instead of usbip.exe for auto-attach"  env:"VIIPER_API_AUTO_ATTACH_WINDOWS_NATIVE"`
	AutoAttachLowLatency    bool `default:"false" help:"Use low-latency receive mode (wsk_events) for native auto-attach on usbip-win2 0.9.8.0+" env:"VIIPER_API_AUTO_ATTACH_LOW_LATENCY"`
}

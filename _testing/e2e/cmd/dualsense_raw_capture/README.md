# Passive DualSense Raw Input capture

This Windows-only diagnostic registers a message-only `RIDEV_INPUTSINK` for
Generic Desktop/Gamepad Raw Input. It does not open HID device handles and does
not send feature, output, or force-feedback reports.

Build from the VIIPER repository root:

```powershell
go build -trimpath -o ..\_results\dualsense_raw_capture.exe .\_testing\e2e\cmd\dualsense_raw_capture
```

Capture for 90 seconds (or press Ctrl+C early):

```powershell
..\_results\dualsense_raw_capture.exe -duration 90s -capacity 262144 -format csv -out ..\_results\resonance-l2-raw.csv
```

At startup the utility prints every matching Sony VID 054C DualSense
(PID 0CE6) or DualSense Edge (PID 0DF2) Gamepad collection and its exact Raw
Input path. A second run can label paths explicitly without changing which Sony
reports are captured:

```powershell
..\_results\dualsense_raw_capture.exe `
  -duration 90s `
  -out ..\_results\resonance-l2-raw.csv `
  -physical-path '\\?\HID#the_exact_physical_path' `
  -viiper-path '\\?\HID#the_exact_viiper_path'
```

The CSV begins with capture and device/path metadata comments. Each report row
contains its QPC timestamp, complete report length and hex, plus the raw USB
DualSense comparison fields: L2 byte 5, digital L2 at byte 9 bit 2, byte 7
sequence, little-endian packet sequence at bytes 12-15, sensor clock at bytes
28-31, and bytes 41-54. Reports longer than the fixed 128-byte slot are
counted and discarded whole rather than truncated. Ring overwrite counters are
written into the artifact.

Use `-format bin` for a compact variable-record binary stream or `-format both`
to emit both forms. The binary stream is little-endian: an eight-byte
`DSRAWV1\x00` magic, capture counters and a UTF-8 device table, followed by
`ordinal uint64`, `qpc int64`, `deviceID uint16`, `size uint16`, and exactly
`size` report bytes per retained record.

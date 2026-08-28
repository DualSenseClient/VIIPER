# DualSense Controller

VIIPER emulates complete USB-connected DualSense and DualSense Edge devices,
including controls, touch, motion, adaptive triggers, lightbar, speaker,
advanced haptics, and microphone endpoints.

All functions are part of the [libVIIPER C API](../libviiper/overview.md).

## Device variants

| Create function | USB functions |
| --- | --- |
| `CreateDualSenseDevice(...)` | DualSense HID, speaker/haptics OUT, microphone IN |
| `CreateDualSenseEdgeDevice(...)` | DualSense Edge HID, speaker/haptics OUT, microphone IN |
| `CreateDualSenseAudioOnlyDevice(...)` | DualSense speaker/haptics OUT and microphone IN sidecar (no HID gamepad) |
| `CreateDualSenseEdgeAudioOnlyDevice(...)` | DualSense Edge speaker/haptics OUT and microphone IN sidecar (no HID gamepad) |
| `CreateDualSenseGamepadOnlyDevice(...)` | DualSense HID gamepad only (no audio interfaces) |
| `CreateDualSenseEdgeGamepadOnlyDevice(...)` | DualSense Edge HID gamepad only (no audio interfaces) |

All create functions share the signature
`(serverHandle, &handle, busID, autoAttach, vid, pid, meta)`. The audio-only
variants are useful when the gamepad is emulated by another device and Windows
should only see the speaker/microphone endpoints.

## API

| Function | Description |
| --- | --- |
| `CreateDualSenseDevice` / `CreateDualSenseEdgeDevice` | Create a virtual DualSense / Edge (HID + audio) |
| `CreateDualSenseAudioOnlyDevice` / `CreateDualSenseEdgeAudioOnlyDevice` | Create an audio-only sidecar |
| `CreateDualSenseGamepadOnlyDevice` / `CreateDualSenseEdgeGamepadOnlyDevice` | Create a gamepad-only device |
| `SetDualSenseDeviceState(handle, state)` | Push an input state to the device |
| `SetDualSenseMetaState(handle, meta)` | Merge-update identity/battery/connection state at runtime |
| `SetDualSenseOutputCallback(handle, cb)` | Register a callback for rumble, LEDs and player LEDs |
| `SetDualSenseOutputStateCallback(handle, cb)` | Register a callback for the full output state |
| `SetDualSenseRealtimeHapticsCallback(handle, cb)` | Register a callback for low-latency rear haptics |
| `SetDualSenseAtomicAudioHapticsCallback(handle, cb)` | Register a callback pairing each V5 output state with its speaker PCM |
| `SetDualSenseSpeakerResetCallback(handle, cb)` | Register a callback for speaker-stream resets |
| `SetDualSenseAudioOutCallback(handle, cb)` | Register a callback for haptics/speaker PCM from the host |
| `SetDualSenseMicrophonePCM(handle, data, length)` | Queue a microphone PCM frame (1920 bytes) |
| `RemoveDualSenseDevice(handle)` | Remove the device |

Only one output callback may be active at a time; the most recently installed
wins.

## Input state

```c
typedef struct {
    int8_t   LX;
    int8_t   LY;
    int8_t   RX;
    int8_t   RY;
    uint32_t Buttons;
    uint8_t  DPad;
    uint8_t  L2;
    uint8_t  R2;
    uint16_t Touch1X;
    uint16_t Touch1Y;
    uint8_t  Touch1Active;
    uint8_t  Touch1Tracking;
    uint16_t Touch2X;
    uint16_t Touch2Y;
    uint8_t  Touch2Active;
    uint8_t  Touch2Tracking;
    int16_t  GyroX;
    int16_t  GyroY;
    int16_t  GyroZ;
    int16_t  AccelX;
    int16_t  AccelY;
    int16_t  AccelZ;
} DSDeviceState;
```

Button bits:

| Control | Value |
| --- | --- |
| Square | `0x00000010` |
| Cross | `0x00000020` |
| Circle | `0x00000040` |
| Triangle | `0x00000080` |
| L1 / R1 | `0x00000100` / `0x00000200` |
| L2 / R2 | `0x00000400` / `0x00000800` |
| Create / Options | `0x00001000` / `0x00002000` |
| L3 / R3 | `0x00004000` / `0x00008000` |
| PS | `0x00010000` |
| Touchpad click | `0x00020000` |
| Mic mute | `0x00040000` |
| Edge LFn / RFn | `0x00100000` / `0x00200000` |
| Edge L4 / R4 | `0x00400000` / `0x00800000` |

D-pad bits are **Up** `0x01`, **Down** `0x02`, **Left** `0x04`, **Right** `0x08`.

## Meta state

Metadata passed to the create functions and `SetDualSenseMetaState`. Fields left
at their zero value (`NULL`/`0`) keep the current value, so a partial update only
changes what the caller supplies.

```c
typedef struct {
    const char* SerialNumber;       // NULL = use default
    const char* MACAddress;         // NULL = use default
    const char* Board;              // NULL = use default
    uint8_t     BatteryStatus;      // 0 = use default
    double      TemperatureCelsius; // 0 = use default
    double      BatteryVoltage;     // 0 = use default
    const char* ShellColor;         // NULL = use default (2-char code, e.g. "00", "Z1")
    const char* BuildTime;          // NULL = use default (RFC3339 or "YYYY-MM-DD HH:MM:SS")
    uint8_t     ConnectionStatus;   // 0 = use default (DS_CONNECTION_* flags)
} DSMetaState;
```

### Connection status flags

The input-report connection status byte. The virtual device always reports USB
data/power; the headphone/mic/mute flags are application-controlled.

| Constant | Value |
| --- | --- |
| `DS_CONNECTION_HEADPHONE` | `0x01` |
| `DS_CONNECTION_MIC` | `0x02` |
| `DS_CONNECTION_MIC_MUTED` | `0x04` |
| `DS_CONNECTION_USB_DATA` | `0x08` |
| `DS_CONNECTION_USB_POWER` | `0x10` |

### Shell colors

`DS_SHELL_COLOR_*` constants are defined for all hardware variants, e.g.
`DS_SHELL_COLOR_WHITE` (`"00"`), `DS_SHELL_COLOR_BLACK` (`"01"`),
`DS_SHELL_COLOR_GOD_OF_WAR_RAGNAROK` (`"Z1"`), `DS_SHELL_COLOR_ASTRO_BOT` (`"Z3"`),
`DS_SHELL_COLOR_GENSHIN_IMPACT` (`"ZE"`), and more — see `libVIIPER.h`.

## Output callback

The basic callback reports rumble, lightbar color, and player LEDs:

```c
typedef void (*DSOutputCallback)(
    DSDeviceHandle handle,
    uint8_t rumbleSmall,
    uint8_t rumbleLarge,
    uint8_t ledRed,
    uint8_t ledGreen,
    uint8_t ledBlue,
    uint8_t playerLeds
);
```

## Output state callback

`SetDualSenseOutputStateCallback` delivers the full output state, including the
adaptive-trigger blocks for R2 and L2, the native 48-byte USB output report, and
the 398-byte Bluetooth combined carrier:

```c
typedef struct {
    uint8_t RumbleSmall;
    uint8_t RumbleLarge;
    uint8_t LedRed;
    uint8_t LedGreen;
    uint8_t LedBlue;
    uint8_t PlayerLeds;
    uint8_t TriggerR2Mode;
    uint8_t TriggerR2StartResistance;
    uint8_t TriggerR2EffectForce;
    uint8_t TriggerR2RangeForce;
    uint8_t TriggerR2NearReleaseStrength;
    uint8_t TriggerR2NearMiddleStrength;
    uint8_t TriggerR2PressedStrength;
    uint8_t TriggerR2Frequency;
    // ... mirror fields for TriggerL2*
    uint8_t RawOutputReport[48];            // native USB output report 0x02
    uint8_t BluetoothCombinedOutputReport[398];
    uint8_t MicLed;             // 0=off, 1=on, 2=pulse
    uint8_t LightbarSetup;      // 0x01 default, 0x02 custom
    uint8_t LightbarBrightness; // 0=high, 1=medium, 2=low
} DSOutputState;

typedef void (*DSOutputStateCallback)(DSDeviceHandle handle, DSOutputState output);
```

## Realtime haptics callback

`SetDualSenseRealtimeHapticsCallback` is invoked as soon as a rear haptics
interval completes (every 512 source frames ≈ 93.75 Hz), before the next
480-frame speaker boundary. This is the low-latency haptics lane; the delivered
output state carries a fresh `BluetoothCombinedOutputReport`.

The 398-byte combined Bluetooth carrier (report ID `0x36`) bundles the output
state, the haptics sample, and the speaker lane in one HID report:

| Offset | Length | Content |
| --- | --- | --- |
| `0` | 1 | Report ID `0x36` |
| `1` | 1 | Rolling sequence nibble (`(seq & 0x0F) << 4`) |
| `2` | 9 | Packet 0x11 header: `0x91 0x07 0xFE` + 5 queue-length bytes + seq |
| `11` | 1 | `0x90` — packet 0x10 (output state) marker |
| `12` | 1 | `63` — state length |
| `13` | 63 | Output state (mirrors USB output report bytes 1-47 when set) |
| `76` | 1 | `0x92` — packet 0x12 (haptics) marker |
| `77` | 1 | `64` — haptics sample length |
| `78` | 64 | Haptics sample: signed 8-bit stereo, 32 frames @ 3 kHz |
| `142` | 1 | `0x93` — packet 0x13 (speaker) marker |
| `143` | 1 | `0` — speaker length (empty in VIIPER) |
| `394` | 4 | CRC-32 (little-endian) over bytes 0-393 — custom seed |

```c
#include <math.h>
#include <stdint.h>
#include <stdio.h>
#include "libVIIPER.h"

void realtimeHapticsCallback(DSDeviceHandle handle, DSOutputState output) {
    (void)handle;
    const uint8_t* report = output.BluetoothCombinedOutputReport;

    // Guard against a carrier without a fresh haptics sample.
    if (report[0] != 0x36 || report[76] != 0x92 || report[77] != 64)
        return;

    // 64 signed 8-bit samples, stereo-interleaved (L/R): 32 per channel.
    const int8_t* haptics = (const int8_t*)&report[78];
    double l2 = 0.0, r2 = 0.0;
    for (int i = 0; i < 32; i++) {
        l2 += (double)haptics[i * 2] * haptics[i * 2];     // rear-left actuator
        r2 += (double)haptics[i * 2 + 1] * haptics[i * 2 + 1]; // rear-right
    }
    double lRms = sqrt(l2 / 32) / 127.0; // normalize to 0..1
    double rRms = sqrt(r2 / 32) / 127.0;
    printf("<- Realtime haptics: L=%.0f%% R=%.0f%%\n", lRms * 100.0, rRms * 100.0);

    // Rumble, adaptive triggers and lightbar are decoded struct fields — no
    // need to parse bytes 13..75 yourself.
}
```

See the [C# Bindings](../libviiper/csharp.md#dualsense-audio-example) page for the
same decode in C#.

## Audio

### Speaker / haptics PCM out

`SetDualSenseAudioOutCallback` receives the raw bytes the host wrote to the
haptics audio-out endpoint: **four S16LE channels at 48 kHz** (front stereo +
rear haptics). The buffer is only valid during the call.

### Atomic audio+haptics callback

`SetDualSenseAtomicAudioHapticsCallback` is invoked once per 480-frame (10 ms)
speaker generation of the V5 transport. Each invocation pairs the native
feedback output state with exactly that generation's speaker PCM: **two S16LE
channels (front stereo) at 48 kHz, 1920 bytes**. The buffer is only valid
during the call.

While installed it supersedes `SetDualSenseOutputStateCallback` and
`SetDualSenseOutputCallback` for audio generations; the realtime haptics lane
(`SetDualSenseRealtimeHapticsCallback`) continues independently.
`SetDualSenseSpeakerResetCallback` remains the generation barrier for this
stream: flush queued PCM when it fires.

`SetDualSenseSpeakerResetCallback` is invoked when the haptics audio interface
alternate setting changes or the endpoint is reset. Treat it as a stream
generation barrier: flush queued speaker PCM and haptics.

### Microphone in

`SetDualSenseMicrophonePCM` queues a microphone PCM frame captured from the
host-facing mic stream. The frame must be exactly **1920 bytes** (480 frames of
two S16LE channels at 48 kHz).

## V5 device type registry

The registry exposes only production V5 contracts. Input size is negotiated by
the exact device type; there is no per-frame downgrade:

| Device type | Input | Output event `0x85` | USB functions |
| --- | --- | --- | --- |
| `dualsensecombinedaudioduplexv5` | 33 bytes | No | DualSense HID, speaker/haptics OUT, microphone IN |
| `dualsenseaudioonlyduplexv5` | 33 bytes | No | DualSense audio sidecar |
| `dualsensegamepadv5` | 33 bytes | No | DualSense HID only |
| `dualsensecombinedaudioduplexv5events` | 33 bytes | Yes | DualSense HID and audio |
| `dualsenseaudioonlyduplexv5events` | 33 bytes | Yes | DualSense audio sidecar |
| `dualsensecombinedaudioduplexv5rawinputevents` | 53 bytes | Yes | DualSense HID and audio |
| `dualsenseaudioonlyduplexv5rawinputevents` | 53 bytes | Yes | DualSense audio sidecar |
| `dualsensegamepadv5rawinput` | 53 bytes | No | DualSense HID only |
| `dualsenseedgecombinedaudioduplexv5` | 33 bytes | No | DualSense Edge HID and audio |
| `dualsenseedgeaudioonlyduplexv5` | 33 bytes | No | DualSense Edge audio sidecar |
| `dualsenseedgegamepadv5` | 33 bytes | No | DualSense Edge HID only |
| `dualsenseedgecombinedaudioduplexv5events` | 33 bytes | Yes | DualSense Edge HID and audio |
| `dualsenseedgecombinedaudioduplexv5rawinputevents` | 53 bytes | Yes | DualSense Edge HID and audio |
| `dualsenseedgegamepadv5rawinput` | 53 bytes | No | DualSense Edge HID only |

Deprecated pre-V5 raw, extended, V1, V2, V3, and V4 names are intentionally
not registered. Clients must use an exact V5 alias; VIIPER does not silently
negotiate an older header or split audio/state transport.

## V5 stream contract

Every packet uses a 16-byte header followed by its payload:

| Offset | Size | Field |
| --- | --- | --- |
| 0 | 4 | ASCII `VPCM` |
| 4 | 1 | Version `0x05` |
| 5 | 1 | Frame type |
| 6 | 2 | Payload length, little endian |
| 8 | 4 | Monotonic sequence, little endian |
| 12 | 4 | IEEE CRC32, little endian |

The CRC covers header bytes 4 through 11 followed by the payload. Sequence
numbers are shared by every frame type in one direction. A version mismatch,
sequence gap, CRC mismatch, invalid payload length, or unknown frame type
closes the stream instead of changing protocols.

| Direction | Type | Payload |
| --- | --- | --- |
| Client to VIIPER | `0x01` | Exact 33- or 53-byte controller input state selected by device alias |
| Client to VIIPER | `0x02` | 1,920-byte microphone PCM block: stereo S16LE, 48 kHz, 10 ms |
| VIIPER to client | `0x81` | 474-byte current combined controller feedback |
| VIIPER to client | `0x83` | Atomic feedback plus the matching 1,920-byte speaker PCM generation |
| VIIPER to client | `0x85` | Microphone-interface active byte plus 64-bit stream generation, event aliases only |

An atomic `0x83` payload begins with a little-endian 16-bit feedback length,
then the 474-byte feedback object, then exactly 480 stereo S16LE speaker
frames. The four-channel virtual USB source is preserved at 48 kHz: front
left/right become the speaker generation, while rear left/right independently
complete the 512-frame advanced-haptics clock. At each 480-frame presentation
boundary VIIPER consumes one completed rear sample or emits silence for that
lane, matching the proven V5 cadence without replaying stale haptics.

Controller state, adaptive triggers, lightbar, rumble, haptics, and speaker
data are serialized by one V5 writer. Media backpressure is bounded and
newest-wins; interface resets form a hard generation boundary so stale audio
cannot cross a stop/reconnect.

### Raw-input extension

Only exact `...v5rawinput...` aliases require the 53-byte payload. Existing
legacy aliases and `...v5events` aliases remain exactly 33 bytes; the events
suffix negotiates output lifecycle events, not enhanced input.

| Payload bytes | Meaning |
| --- | --- |
| `0:33` | mapped state described above |
| `33` | flags: bit 0 physical metadata valid; bit 1 physical source uses Edge layout |
| `34:38` | physical input report bytes `28:32`, normalized from USB or Bluetooth |
| `38:53` | physical input report bytes `41:56`, normalized from USB or Bluetooth |

Bit 1 is valid only together with bit 0; unknown bits close the framed stream.
Legacy decode explicitly clears the extension fields to prevent stale metadata
after reconnect or alias changes.

Valid metadata supplies the physical sensor timestamp, trigger mechanism and
effect status, host timestamp echo, battery, and common headset/filter status.
VIIPER copies physical report
bytes 49 through 52 only when the physical source and virtual target both use
the same base or Edge layout. On a mismatch it synthesizes the target layout:
base uses its generated device clock, while Edge uses `80 00 00 00`. The
virtual connection byte `54` reports the device's configured connection status
(USB data and power by default) even when the source report arrived over
Bluetooth. Without valid metadata, trigger status is the
confirmed physical off/no-load value `09 09`, effect status is `00`, and the
clock/profile and battery use VIIPER's generated state.

Physical report bytes 56 through 63 are an eight-byte AES-CMAC. They are
deliberately excluded: remapping controls and replacing virtual
counters/connect state invalidates the physical authentication tag, and
VIIPER cannot recompute it without the controller key. The corresponding
virtual report tail stays zero.

Physical metadata is continuous latest-state data and never creates an ordered
queue entry by itself. A real button, D-pad, touch-contact/ID, or trigger
zero-crossing transition carries its complete metadata snapshot. In a trigger
epoch, peak strengthening couples only that trigger's status byte and effect
nibble; equal-analog settling such as L2 `28` to `29` is preserved before
release without turning normal mechanical progression into a stale FIFO.

## Feedback state

The 474-byte V5 feedback object contains:

- Bytes 0 through 5: compatible rumble, lightbar RGB, and player LEDs.
- Bytes 6 through 27: native-spaced R2 and L2 adaptive-trigger blocks.
- Bytes 28 through 75: the native 48-byte USB output report `0x02`.
- Bytes 76 through 473: the current 398-byte combined Bluetooth carrier.

The combined carrier keeps state and media on one presentation clock. The
physical-controller bridge supplies the encoded speaker lane and forwards it
using its V5 transport.
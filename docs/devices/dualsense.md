# DualSense Controller

VIIPER emulates USB-connected DualSense and DualSense Edge gamepads,
including sticks, triggers, buttons, touchpad, motion, lightbar, rumble,
and player LEDs.

This branch has no audio-only, gamepad-only, speaker/haptics PCM, or
microphone variants: only `CreateDualSenseDevice` and
`CreateDualSenseEdgeDevice` exist. Meta is create-time only.

All functions are part of the [libVIIPER C API](../libviiper/overview.md).

## API

| Function | Description |
| --- | --- |
| `CreateDualSenseDevice(serverHandle, &handle, busID, autoAttach, vid, pid, meta)` | Create a virtual DualSense gamepad |
| `CreateDualSenseEdgeDevice(serverHandle, &handle, busID, autoAttach, vid, pid, meta)` | Create a virtual DualSense Edge gamepad |
| `SetDualSenseDeviceState(handle, state)` | Push an input state to the device |
| `SetDualSenseOutputCallback(handle, cb)` | Register a callback for rumble, lightbar and player LEDs |
| `RemoveDualSenseDevice(handle)` | Remove the device |

Only one output callback may be active at a time; pass `NULL` to clear it.

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
    uint16_t Touch2X;
    uint16_t Touch2Y;
    uint8_t  Touch2Active;
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

Optional metadata passed to the create functions. Fields left at
their zero value (`NULL`/`0`) use defaults.

```c
typedef struct {
    const char* SerialNumber;       // NULL = use default
    const char* MACAddress;         // NULL = use default
    const char* Board;              // NULL = use default
    uint8_t     BatteryStatus;      // 0 = use default
    double      TemperatureCelsius; // 0 = use default
    double      BatteryVoltage;     // 0 = use default
    const char* ShellColor;         // NULL = use default (2-char code, e.g. "00", "Z1")
} DSMetaState;
```

### Shell colors

`DS_SHELL_COLOR_*` constants are defined for hardware variants, e.g.
`DS_SHELL_COLOR_WHITE` (`"00"`), `DS_SHELL_COLOR_BLACK` (`"01"`),
`DS_SHELL_COLOR_GOD_OF_WAR_RAGNAROK` (`"Z1"`), `DS_SHELL_COLOR_ASTRO_BOT` (`"Z3"`),
`DS_SHELL_COLOR_GENSHIN_IMPACT` (`"ZE"`), and more — see `libVIIPER.h`.

## Output callback

The callback reports rumble, lightbar color, and player LEDs:

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

Pass `NULL` to `SetDualSenseOutputCallback` to clear a previously registered callback.

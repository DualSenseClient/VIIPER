# DualShock 4 Controller

The DualShock 4 virtual gamepad emulates a complete PlayStation 4 Controller (V1)
connected via USB.
It supports sticks, triggers, D-pad, face/shoulder buttons, PS button,
touchpad click, IMU (gyro + accelerometer), touchpad finger coordinates, and
the native DualShock 4 USB speaker and microphone interfaces.

All functions are part of the [libVIIPER C API](../libviiper/overview.md).

## API

| Function | Description |
| --- | --- |
| `CreateDS4Device(serverHandle, &handle, busID, autoAttach, vid, pid, meta)` | Create a virtual DualShock 4 |
| `SetDS4DeviceState(handle, state)` | Push an input state to the device |
| `SetDS4MetaState(handle, meta)` | Merge-update identity/battery state at runtime |
| `SetDS4OutputCallback(handle, cb)` | Register a callback for rumble and LED output |
| `SetDS4SpeakerCallback(handle, cb)` | Register a callback for speaker PCM from the host |
| `SetDS4SpeakerResetCallback(handle, cb)` | Register a callback for speaker-stream resets |
| `SetDS4MicrophonePCM(handle, data, length)` | Queue a microphone PCM frame (320 bytes) |
| `RemoveDS4Device(handle)` | Remove the device |

## Input state

```c
typedef struct {
    int8_t   LX;
    int8_t   LY;
    int8_t   RX;
    int8_t   RY;
    uint16_t Buttons;
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
} DS4DeviceState;
```

- Sticks: -128 to 127 per axis (-128=min, 0=center, 127=max)
- Triggers: 0-255 (0=not pressed, 255=fully pressed)
- Touch coordinates are clamped to the DS4 range: X **0..1920**, Y **0..942**

### Button constants

| Button | Hex Value |
| --- | --- |
| Square button | 0x0010 |
| Cross (X) button | 0x0020 |
| Circle button | 0x0040 |
| Triangle button | 0x0080 |
| L1 (Left bumper) | 0x0100 |
| R1 (Right bumper) | 0x0200 |
| L2 button | 0x0400 |
| R2 button | 0x0800 |
| Share button | 0x1000 |
| Options button | 0x2000 |
| L3 (Left stick button) | 0x4000 |
| R3 (Right stick button) | 0x8000 |
| PS button | 0x0001 |
| Touchpad click | 0x0002 |

### D-Pad constants

| D-Pad Direction | Hex Value |
| --- | --- |
| Up | 0x01 |
| Down | 0x02 |
| Left | 0x04 |
| Right | 0x08 |

### IMU (Gyro + Accelerometer)

IMU values are fixed-point:

- `GyroCountsPerDps = 16` → resolution `0.0625 deg/s`, max approx `2048 deg/s`
- `AccelCountsPerMS2 = 512` → resolution approx `0.00195 m/s2`, max approx `64 m/s2`

On device creation, the accelerometer is initialized to a controller lying flat
with gravity downwards (`AccelZ = -5023`, i.e. `round(-9.81 * 512)`).

## Meta state

Optional metadata passed to `CreateDS4Device` and `SetDS4MetaState`. Fields left at
their zero value (`NULL`/`0`) keep the current value, so a partial update only
changes what the caller supplies.

```c
typedef struct {
    const char* SerialNumber;       // NULL = use default
    const char* Board;              // NULL = use default
    uint8_t     BatteryStatus;      // 0 = use default
    double      TemperatureCelsius; // 0 = use default
    double      BatteryVoltage;     // 0 = use default
    const char* BuildTime;          // NULL = use default (RFC3339 or "YYYY-MM-DD HH:MM:SS")
} DS4MetaState;
```

## Output callback

Called when the host sends rumble or LED commands to the device. The callback
replays the latest output state when it is first registered.

```c
typedef void (*DS4OutputCallback)(
    DS4DeviceHandle handle,
    uint8_t rumbleSmall,
    uint8_t rumbleLarge,
    uint8_t ledRed,
    uint8_t ledGreen,
    uint8_t ledBlue,
    uint8_t flashOn,
    uint8_t flashOff
);
```

- Rumble: 0-255 intensity values
- LED color: 0-255 per channel
- LED flash: units of 2.5 ms per value

Pass `NULL` to `SetDS4OutputCallback` to clear a previously registered callback.

## Speaker callback

Called with the raw bytes the host wrote to the speaker audio-out endpoint:
two S16LE channels at 32 kHz. The buffer is only valid during the call.

```c
typedef void (*DS4SpeakerCallback)(DS4DeviceHandle handle, const uint8_t* pcm, size_t length);
```

`DS4SpeakerResetCallback` (registered with `SetDS4SpeakerResetCallback`) is invoked
when the speaker interface alternate setting changes or the endpoint is reset.
Treat it as a stream generation barrier: flush any queued speaker PCM.

## Microphone input

`SetDS4MicrophonePCM` queues a microphone PCM frame captured from the host-facing
mic stream. The frame must be exactly **320 bytes** (160 frames of one S16LE
channel at 16 kHz).
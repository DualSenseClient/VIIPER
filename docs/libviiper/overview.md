# libVIIPER Documentation

libVIIPER is a shared library (`libVIIPER.dll` on Windows, `libVIIPER.so` on Linux) that embeds the full VIIPER USB/USBIP stack directly into your application.

- Single shared library (`libVIIPER.dll` / `libVIIPER.so`)
- Pure C API callable from any language with C FFI support
- In-process + threadsafe
  the USBIP server runs in a background thread inside your application
- Optional auto-attach to the local USBIP client on the same machine

!!! warning "License"
    libVIIPER is licensed under **GPL-3.0**.
    Linking against it **requires your application to be GPL-3.0 compatible**.

!!! info "USBIP Required"
    libVIIPER uses USBIP internally. A USBIP client must be installed on the target machine.
    See [Installation › Requirements](../getting-started/installation.md#requirements) for setup instructions.

## C# and client libraries

- Looking for C#? See the complete [C# Bindings](csharp.md) reference (all functions,
  structs, enums and callbacks with P/Invoke declarations).
- Want to drive VIIPER over TCP instead of embedding it? See
  [Client Libraries](client-libraries.md) for the generated C#, C++, Rust,
  TypeScript and Go clients.

## API Overview

The libVIIPER C API is declared in `libVIIPER.h`.

- All functions return `bool` (`true` on success, `false` on failure).
- Handles (`USBServerHandle`, `Xbox360DeviceHandle`, …) are opaque `uintptr_t` values.
- Callbacks are invoked from the USBIP server's background threads. Keep them short and
  thread-safe; never call into libVIIPER from inside a callback.
  PCM buffers passed to audio callbacks are **only valid during the call** — copy the data
  if you need to keep it.
- Pass `NULL` to any callback setter to clear a previously registered callback.
- Struct fields set to their zero value (or `NULL`/`0`) select the device's default, so a
  zeroed struct always creates a device with default identity.

### Server lifecycle

| Function | Description |
| --- | --- |
| `NewUSBServer(config, &handle, logCb)` | Start a USB server in a background thread |
| `CloseUSBServer(handle)` | Stop the server and free all resources |

```c
typedef struct {
    char*    addr;                            // default "0.0.0.0:3241"
    uint64_t connection_timeout_ms;           // default 30000 (30s)
    uint64_t device_handler_connect_timeout_ms; // default 5000 (5s)
    uint32_t write_batch_flush_interval_ms;   // default 1 (1ms)
} USBServerConfig;
```

`NewUSBServer` blocks until the server is ready. On `localhost` the server automatically
attaches the USBIP client, so devices created with `autoAttachLocalhost = true` appear on
your machine without any manual USBIP interaction.

### Bus management

| Function | Description |
| --- | --- |
| `CreateUSBBus(serverHandle, &busID)` | Create a new USB bus (pass `0` to auto-assign ID) |
| `RemoveUSBBus(serverHandle, busID)` | Remove a bus and all its devices |

### Logging

Pass a `VIIPERLogCallback` to `NewUSBServer` to receive log messages from the library.
Pass `NULL` to discard all log output.

```c
typedef enum {
    VIIPER_LOG_DEBUG = -4,
    VIIPER_LOG_INFO  = 0,
    VIIPER_LOG_WARN  = 4,
    VIIPER_LOG_ERROR = 8,
} VIIPERLogLevel;

typedef void (*VIIPERLogCallback)(VIIPERLogLevel level, const char* message);
```

## Devices

Each device family has its own create/state/callback functions documented on its page:

- [Xbox 360 Controller](../devices/xbox360.md)
- [DualShock 4](../devices/dualshock4.md)
- [DualSense (and Edge)](../devices/dualsense.md)
- [Switch 2 Pro Controller](../devices/ns2pro.md)
- [Keyboard](../devices/keyboard.md)
- [Mouse](../devices/mouse.md)

### Common device API

#### Xbox 360

| Function | Description |
| --- | --- |
| `CreateXbox360Device(serverHandle, &handle, busID, autoAttach, vid, pid, subType)` | Create a virtual Xbox 360 controller |
| `SetXbox360DeviceState(handle, state)` | Push an input state to the device |
| `SetXbox360RumbleCallback(handle, cb)` | Register a callback for rumble output |
| `RemoveXbox360Device(handle)` | Remove the device |

#### DualShock 4

| Function | Description |
| --- | --- |
| `CreateDS4Device(serverHandle, &handle, busID, autoAttach, vid, pid, meta)` | Create a virtual DualShock 4 |
| `CreateDS4AudioOnlyDevice(...)` | Create an audio-only DualShock 4 sidecar (no HID gamepad) |
| `SetDS4DeviceState(handle, state)` | Push an input state to the device |
| `SetDS4MetaState(handle, meta)` | Merge-update identity/battery state at runtime |
| `SetDS4OutputCallback(handle, cb)` | Register a callback for rumble and LED output |
| `SetDS4SpeakerCallback(handle, cb)` | Register a callback for speaker PCM from the host |
| `SetDS4SpeakerResetCallback(handle, cb)` | Register a callback for speaker-stream resets |
| `SetDS4MicrophonePCM(handle, data, length)` | Queue a microphone PCM frame (320 bytes) |
| `RemoveDS4Device(handle)` | Remove the device |

#### DualSense (and Edge)

| Function | Description |
| --- | --- |
| `CreateDualSenseDevice(serverHandle, &handle, busID, autoAttach, vid, pid, meta)` | Create a virtual DualSense (HID + audio) |
| `CreateDualSenseEdgeDevice(...)` | Create a virtual DualSense Edge (HID + audio) |
| `CreateDualSenseAudioOnlyDevice(...)` | Create an audio-only DualSense sidecar (no HID gamepad) |
| `CreateDualSenseEdgeAudioOnlyDevice(...)` | Create an audio-only DualSense Edge sidecar |
| `CreateDualSenseGamepadOnlyDevice(...)` | Create a gamepad-only DualSense (no audio) |
| `CreateDualSenseEdgeGamepadOnlyDevice(...)` | Create a gamepad-only DualSense Edge |
| `SetDualSenseDeviceState(handle, state)` | Push an input state to the device |
| `SetDualSenseMetaState(handle, meta)` | Merge-update identity/battery/state at runtime |
| `SetDualSenseOutputCallback(handle, cb)` | Register a callback for rumble, LEDs and player LEDs |
| `SetDualSenseOutputStateCallback(handle, cb)` | Register a callback for the full output state (incl. adaptive triggers) |
| `SetDualSenseRealtimeHapticsCallback(handle, cb)` | Register a callback for low-latency rear haptics |
| `SetDualSenseAtomicAudioHapticsCallback(handle, cb)` | Register a callback pairing each V5 output state with its speaker PCM |
| `SetDualSenseSpeakerResetCallback(handle, cb)` | Register a callback for speaker-stream resets |
| `SetDualSenseAudioOutCallback(handle, cb)` | Register a callback for haptics/speaker PCM from the host |
| `SetDualSenseMicrophonePCM(handle, data, length)` | Queue a microphone PCM frame (1920 bytes) |
| `RemoveDualSenseDevice(handle)` | Remove the device |

#### Switch 2 Pro

| Function | Description |
| --- | --- |
| `CreateNS2ProDevice(...)` | Create a virtual Switch 2 Pro Controller |
| `SetNS2ProDeviceState(handle, state)` | Push input state |
| `SetNS2ProOutputCallback(handle, cb)` | Register output (rumble/LED) callback |
| `RemoveNS2ProDevice(handle)` | Remove the device |

#### Keyboard

| Function | Description |
| --- | --- |
| `CreateKeyboardDevice(serverHandle, &handle, busID, autoAttach, vid, pid)` | Create a virtual HID keyboard |
| `SetKeyboardDeviceState(handle, state)` | Push an input state to the device |
| `SetKeyboardLEDCallback(handle, cb)` | Register a callback for LED state changes |
| `RemoveKeyboardDevice(handle)` | Remove the device |

#### Mouse

| Function | Description |
| --- | --- |
| `CreateMouseDevice(serverHandle, &handle, busID, autoAttach, vid, pid)` | Create a virtual HID mouse |
| `SetMouseDeviceState(handle, state)` | Push an input state to the device |
| `RemoveMouseDevice(handle)` | Remove the device |

## Examples

Full working examples are in [`examples/libVIIPER/`](https://github.com/DualSenseClient/VIIPER/tree/main/examples/libVIIPER).

=== "C"

    ```c
    USBServerConfig conf = { .addr = "localhost:3245" };
    USBServerHandle serverHandle = 0;
    NewUSBServer(&conf, &serverHandle, logCallback);

    uint32_t busID = 0;
    CreateUSBBus(serverHandle, &busID);

    Xbox360DeviceHandle deviceHandle = 0;
    CreateXbox360Device(serverHandle, &deviceHandle, busID, /*autoAttach=*/true, 0, 0, 0);

    SetXbox360RumbleCallback(deviceHandle, rumbleCallback);

    Xbox360DeviceState state = {0};
    while (running) {
        // only required when an actual change occurs
        state.Buttons = XBOX360_BUTTON_A;
        state.LT      = 128;
        state.LX      = 20000;
        SetXbox360DeviceState(deviceHandle, state);
        _sleep(16);
    }

    CloseUSBServer(serverHandle);
    ```

=== "C#"

    ```csharp
    USBServerConfig conf = new() { addr = "localhost:3245" };
    LibVIIPER.NewUSBServer(ref conf, out nuint serverHandle, logCb);

    uint busID = 0;
    LibVIIPER.CreateUSBBus(serverHandle, ref busID);

    LibVIIPER.CreateXbox360Device(serverHandle, out nuint deviceHandle, busID, autoAttachLocalhost: true, 0, 0, 0);

    Xbox360RumbleCallbackDelegate rumbleCb = RumbleCallback;
    LibVIIPER.SetXbox360RumbleCallback(deviceHandle, rumbleCb);

    Xbox360DeviceState state = new();
    while (running) {
        // only required when an actual change occurs
        state.Buttons = Xbox360Buttons.A;
        state.LT      = 128;
        state.LX      = 20000;
        LibVIIPER.SetXbox360DeviceState(deviceHandle, state);
        Thread.Sleep(16);
    }

    LibVIIPER.CloseUSBServer(serverHandle);
    ```

    See [`examples/libVIIPER/csharp/`](https://github.com/DualSenseClient/VIIPER/tree/main/examples/libVIIPER/csharp) for the full project including P/Invoke declarations.

    For a complete C# reference (all functions, structs, enums and callbacks), see
    the [C# Bindings](csharp.md) page. If you prefer a managed TCP client instead of
    P/Invoke, see [Client Libraries](client-libraries.md).

## C# Interop Notes

When calling from C#, keep the delegate instances alive for the lifetime of the
device/server they are registered on — the garbage collector cannot see references
held by native code:

```csharp
Xbox360RumbleCallbackDelegate rumbleCb = RumbleCallback; // keep in a field
VIIPERLogCallbackDelegate logCb = LogCallback;            // keep in a field
```

- `[StructLayout(LayoutKind.Sequential)]` structs must mirror the C layout exactly
  (see the example project).
- `[UnmanagedFunctionPointer(CallingConvention.Cdecl)]` is required on callback delegates.
- `[DllImport("libVIIPER", CallingConvention = CallingConvention.Cdecl)]` on all functions.
- `bool` returns need `[return: MarshalAs(UnmanagedType.I1)]`.
- String members of config/meta structs are marshaled with `[MarshalAs(UnmanagedType.LPStr)]`.
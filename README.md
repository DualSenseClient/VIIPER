<img src="docs/viiper.svg" align="right" width="128" alt="VIIPER logo" />
<br />

[![Build Status](https://github.com/DualSenseClient/VIIPER/actions/workflows/snapshots.yml/badge.svg)](https://github.com/DualSenseClient/VIIPER/actions/workflows/snapshots.yml)
[![License: GPL-3.0](https://img.shields.io/github/license/DualSenseClient/VIIPER)](https://github.com/DualSenseClient/VIIPER/blob/main/LICENSE.txt)
[![Release](https://img.shields.io/github/v/release/DualSenseClient/VIIPER?include_prereleases&sort=semver)](https://github.com/DualSenseClient/VIIPER/releases)
[![Issues](https://img.shields.io/github/issues/DualSenseClient/VIIPER)](https://github.com/DualSenseClient/VIIPER/issues)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/DualSenseClient/VIIPER/pulls)

# VIIPER 🐍

**Virtual** **I**nput over **IP** **E**mulato**R**

A **cross-platform virtual USB input framework** for creating virtual USB input devices (game controllers, keyboards, mice and more)
that are indistinguishable from real hardware to the operating system and applications.

VIIPER lets developers create and programmatically control virtual USB input devices (using USBIP under the hood),
enabling seamless integration for gaming, automation, testing and remote control scenarios.

This repository is a fork of [hbashton/VIIPER](https://github.com/hbashton/VIIPER) and serves as the embedded backend
for the **DualSense Client** project, providing native virtual controller output including the ongoing DualSense audio,
haptics, and microphone work. The DualSenseClient release channel aims for feature parity with the upstream fork.
Unlike [DS4Windows](https://github.com/hbashton/DS4Windows), DualSense Client embeds VIIPER directly as a library
instead of running it as a separate app.

## ✨ Features

- Runs on Linux and Windows.
- Pure C API callable from any language with C FFI support.
- VIIPER abstracts away all USB / USBIP details.
- VIIPER is portable and runs entirely in userspace.
    - Utilizes a generic USBIP kernel mode driver
      (built into Linux; on Windows [usbip-win2](https://github.com/vadimgrn/usbip-win2) provides a signed kernel mode driver)
      New device types never require touching kernel code.
- After installing USBIP once, VIIPER can run without additional dependencies or system-wide installation.

## 🍦 libVIIPER

libVIIPER is a single shared library (`libVIIPER.dll` on Windows, `libVIIPER.so` on Linux) that embeds the full
VIIPER USB/USBIP stack directly into your application.

- Pure C API callable from any language with C FFI support
- In-process and threadsafe: the USBIP server runs in a background thread inside your application
- Optional auto-attach to the local USBIP client on the same machine
- No separate server process or network protocol to implement
- See examples for C and C# in [`examples/libVIIPER`](examples/libVIIPER)
- See the [libVIIPER documentation](docs/libviiper/overview.md) for details
- C# developers: the complete [C# Bindings](docs/libviiper/csharp.md) reference covers
  every function, struct, enum and callback with P/Invoke declarations
- Prefer a managed TCP client over P/Invoke? See
  [Client Libraries](docs/libviiper/client-libraries.md) for the generated C#, C++,
  Rust, TypeScript and Go clients

## 🔀 What the DualSenseClient fork adds

The features that [hbashton/VIIPER](https://github.com/hbashton/VIIPER) added to the standalone app were ported to
libVIIPER, so VIIPER can be embedded in an application as a library instead of requiring a separate app.

## 🎮 Emulatable devices

- Xbox 360 controller emulation; see [Devices › Xbox 360 Controller](docs/devices/xbox360.md)
- HID Keyboard with N-key rollover and LED feedback; see [Devices › Keyboard](docs/devices/keyboard.md)
- HID Mouse with 5 buttons and horizontal/vertical wheel; see [Devices › Mouse](docs/devices/mouse.md)
- PS4 controller emulation; see [Devices › DualShock 4 Controller](docs/devices/dualshock4.md)
- PS5 DualSense controller emulation (including Edge variant); see [Devices › DualSense Controller](docs/devices/dualsense.md)
- Nintendo Switch 2 Pro Controller emulation; see [Devices › Switch 2 Pro Controller](docs/devices/ns2pro.md)

## 🏗️ Architecture

```text
Physical controller
        |
        | HID input, audio, and feedback
        v
Feeder application
        |
        | libVIIPER C API (in-process)
        v
VIIPER userspace USB device
        |
        | USBIP
        v
usbip-win2 virtual host controller
        |
        v
Windows, games, and audio services
```

VIIPER does not emulate a Bluetooth radio and does not make the virtual device appear wirelessly paired. The game sees
a native-style USB controller. A separate app, such as DualSense Client or DS4Windows, is then responsible for
translating and forwarding supported feedback between that virtual USB device and the physical USB or Bluetooth
controller.

## 💻 Installation

Download the latest `libVIIPER` release artifact (containing `libVIIPER.dll`/`libVIIPER.so`, `libVIIPER.h` and the
Windows import definition) from the [latest DualSenseClient release](https://github.com/DualSenseClient/VIIPER/releases/latest).
libVIIPER itself is portable, but virtual devices on Windows still require the
[`usbip-win2`](https://github.com/vadimgrn/usbip-win2) kernel driver.

## 🔌 Requirements

**Linux:**

- **Arch Linux:**
    - Install: `sudo pacman -S usbip`
    - Docs: [Arch Wiki: USBIP](https://wiki.archlinux.org/title/USB/IP)
- **Ubuntu / Debian:**
    - Install: `sudo apt install linux-tools-generic`
    - Docs: [Ubuntu USBIP Manual](https://manpages.ubuntu.com/manpages/noble/man8/usbip.8.html)

**Windows:**

- Windows 10 or Windows 11 x64
- [usbip-win2](https://github.com/vadimgrn/usbip-win2) — by far the most complete implementation of USBIP for Windows
  (comes with a **SIGNED** kernel mode driver)

## 🥫 Feeder application development

Integrate libVIIPER directly into your application — embed the full USBIP stack in-process and drive virtual
devices through the pure C API. See [Examples](examples/libVIIPER) for examples in either C or C#.

### 🔌 API

The libVIIPER C API is declared in `libVIIPER.h` (generated at build time) and covers:

- **Server lifecycle** — `NewUSBServer`, `CloseUSBServer`
- **Bus management** — `CreateUSBBus`, `RemoveUSBBus`
- **Device creation** — one `Create<Device>Device` per emulatable device type, with a `meta` parameter to control
  identity (serial number, battery, colors, …)
- **Input feeding** — `Set<Device>DeviceState` to push input (buttons, sticks, touch, IMU, …)
- **Host feedback** — output callbacks for rumble, LEDs, adaptive triggers, and speaker/haptics PCM, plus microphone
  PCM input for the DualSense and DualShock 4

All functions return `bool` and are callable from any language with C FFI support. VIIPER takes care of all USBIP
protocol details, so you can focus on implementing the device logic only. On `localhost`, libVIIPER also automatically
attaches the USBIP client, so you don't have to worry about USBIP details at all.

See the [libVIIPER documentation](docs/libviiper/overview.md) for the complete API reference, the
[C# Bindings](docs/libviiper/csharp.md) for the full C# P/Invoke reference, and
[Client Libraries](docs/libviiper/client-libraries.md) if you prefer driving a standalone
`viiper server` over TCP.

## 🛠️ VIIPER development

### 🧰 Prerequisites

- [Go](https://go.dev/) 1.26 or newer
- USBIP installed
- (Optional) [just](https://github.com/casey/just)
    - Windows: `winget install --id Casey.Just --exact`
    - Linux: use your package manager (`sudo pacman -S just`, `sudo apt install just`, ...)
- Windows compiler (required for `build-libVIIPER`):
    - `winget install -e --id MartinStorsjo.LLVM-MinGW.UCRT --accept-package-agreements --accept-source-agreements`

### 🔄 Building from source

```bash
git clone https://github.com/DualSenseClient/VIIPER.git
cd VIIPER
just build-libVIIPER
```

The output is written to `dist/libVIIPER/` (`libVIIPER.dll`/`libVIIPER.so` plus the generated `libVIIPER.h` header).
Building libVIIPER requires CGO (`CGO_ENABLED=1`) and a C compiler (GCC / MSVC / Clang) in `PATH`.

For more build options:

```bash
just --list            # Show all available targets
just test              # Run tests
go test ./...          # Run tests directly
```

## 🤝 Contributing

Contributions are welcome!
Please open issues or pull requests on GitHub.
See the [issues page](https://github.com/DualSenseClient/VIIPER/issues) for bugs and feature requests.

## ❓ FAQ

### What is USBIP and why does VIIPER use it?

USBIP is a protocol that allows USB devices to be shared over a network.
VIIPER uses it because it's already built into Linux and available for Windows, making virtual device emulation
possible without writing custom kernel drivers yourself.

### Why should my application be GPL-3.0 compatible?

libVIIPER is licensed under **GPL-3.0**. Linking against it (as a shared library) requires your application to be
GPL-3.0 compatible.

### Can I use VIIPER for gaming?

Yes! VIIPER can create virtual input devices that appear as real hardware to games and applications.
This works with Steam, native Windows games and any other application that supports the emulated device types.

### How is VIIPER different from other controller emulators?

Many controller emulation approaches require writing a custom kernel driver for every device type you want to support.
VIIPER uses USBIP to handle the USB protocol layer, so device emulation code lives entirely in userspace.

USBIP itself does require a kernel driver. On Linux, the USBIP driver is built into the kernel. On Windows,
[usbip-win2](https://github.com/vadimgrn/usbip-win2) provides a signed kernel mode driver. That driver is generic and
does not need to know anything about specific device types — all device-type logic stays in userspace.

This makes VIIPER portable, easier to extend and simpler to bundle with applications. Adding a new device type never
requires touching kernel code.

### Can I add support for other device types?

Yes! VIIPER's architecture is designed to be extensible.
Check the [xbox360 device implementation](./device/xbox360/) as a reference for creating new device types.

### What about input latency?

End-to-end input latency for virtual devices created with VIIPER is typically well below 1 millisecond on a modern
desktop. To not stress the CPU excessively, reports get batched and sent every millisecond, so the best you will achieve
is a 1000 Hz update rate — more than enough, and more than what most real hardware devices provide.
_Note_: Actual device polling rates may be lower depending on the device type and configuration.

## 🔧 Troubleshooting

Report backend issues at [DualSenseClient/VIIPER Issues](https://github.com/DualSenseClient/VIIPER/issues). Report
controller mapping or DS4Windows UI issues at [hbashton/DS4Windows Issues](https://github.com/hbashton/DS4Windows/issues).

## 📄 License

```license
VIIPER - Virtual Input over IP EmulatoR

Copyright (C) 2025-2026 Peter Repukat

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
```

libVIIPER and the VIIPER core are licensed under GPL-3.0-or-later; see [`LICENSE.txt`](LICENSE.txt) for the full text.

## Credits

VIIPER was originally created by Peter Repukat and the [Alia5/VIIPER](https://github.com/Alia5/VIIPER) contributors,
and this fork builds on the fork created by Hunter Ashton and the
[hbashton/VIIPER](https://github.com/hbashton/VIIPER) contributors.

- [ViGEmBus](https://github.com/nefarius/ViGEmBus)
  (retired, but still widely used) — Windows kernel-mode driver emulating well-known USB game controllers.
  Shoutout and thank you to @nefarius for paving the way.
- [Valve Software](https://www.valvesoftware.com/)
  for creating the OG Steam Controller (2015) and Steam Input, which sent this project down the rabbit hole in the first place.
- **USBIP** — without it, VIIPER would not be possible.
    - [USBIP](https://usbip.sourceforge.net/)
    - [usbip-win2](https://github.com/vadimgrn/usbip-win2)
- [SDL](https://www.libsdl.org/)
  for their excellent work on input device handling, reducing reversing efforts to a minimum.

It also depends on controller/audio protocol research shared by SAxense, DualSense reverse-engineering projects, and
the wider open-source community.

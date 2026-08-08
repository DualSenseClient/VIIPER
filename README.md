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
- _(Optional)_ network support built in: control devices over a network with lower overhead than raw USBIP alone.
- VIIPER abstracts away all USB / USBIP details.
- VIIPER is portable and runs entirely in userspace.
    - Utilizes a generic USBIP kernel mode driver
      (built into Linux; on Windows [usbip-win2](https://github.com/vadimgrn/usbip-win2) provides a signed kernel mode driver)
      New device types never require touching kernel code.
- After installing USBIP once, VIIPER can run without additional dependencies or system-wide installation.

## 🍦 Two flavors

VIIPER comes in two distinct flavors:

- **VIIPER server** — a self-contained, dependency-free, statically linked, portable standalone executable
    - exposes a lightweight TCP API
    - control devices from any language or machine on the network
- **libVIIPER** — a single shared library to embed device emulation directly into your application
    - see examples for C and C# in [`examples/libVIIPER`](examples/libVIIPER)
    - see the [libVIIPER documentation](docs/libviiper/overview.md) for details

For help deciding between the two, see the [FAQ](#why-choose-the-standalone-executable-and-interfacing-via-tcp-over-the-shared-object-libviiper-library).

Beyond device emulation, VIIPER can proxy real USB devices for traffic inspection and reverse engineering.

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
        | local framed TCP API
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

You can download the packaged release (a zip containing `viiper.exe`) from the
[latest DualSenseClient release](https://github.com/DualSenseClient/VIIPER/releases/latest).
VIIPER itself is portable, but virtual devices on Windows still require the
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

You have two options for developing feeder applications that control the virtual devices created by VIIPER:

- Use the standalone VIIPER server and interface via the exposed TCP API, preferably using one of the client libraries
  included in this repository (see the [`clients/`](clients/) directory)
- Integrate libVIIPER directly into your application — see [Examples](examples/libVIIPER) for examples in either C or C#

### 🔌 API

VIIPER includes a lightweight TCP based API for device and bus management, as well as streaming device control.
It's designed to be trivial to drive from any language that can open a TCP socket and send null-byte-terminated commands.

> Most of the time you don't need to implement the raw protocol yourself, as client libraries are available.
> See the [API overview](docs/api/overview.md) and the [client library documentation](docs/clients/).

- The TCP API uses a string-based request/response protocol terminated by null bytes (`\0`) for device and bus management.
    - Requests have a _path_ and an optional payload (sometimes JSON),
      e.g. `bus/{id}/add {"type": "keyboard", "idVendor": "0x6969"}\0`
    - Responses are often JSON as well!
    - Errors are reported using JSON objects similar to [RFC 7807 Problem Details](https://datatracker.ietf.org/doc/html/rfc7807)
      (the use of JSON allows for future extensibility without breaking compatibility)
- For controlling, or feeding, a device, a long-lived TCP stream is used, with a wire protocol specific to each device type.
  After an initial _handshake_ (`bus/{busId}/{deviceId}\0`), a device-specific **binary protocol** is used to send input
  reports and receive output reports (e.g., rumble commands).

VIIPER takes care of all USBIP protocol details, so you can focus on implementing the device logic only.
On `localhost`, VIIPER also automatically attaches the USBIP client, so you don't have to worry about USBIP details at all.

See the [API documentation](docs/api/overview.md) for details.

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
just build Release
```

By default the binary is written to `dist/viiper` (`dist/viiper.exe` on Windows); the CI builds are named
`dist/viiper-<goos>-<goarch>` (for example `dist/viiper-windows-amd64.exe`).

For more build options:

```bash
just --list            # Show all available targets
just test              # Run tests
go test ./...          # Run tests directly
```

Client bindings are generated for TypeScript, C#, C++, and Rust. Run `go run ./cmd/viiper codegen` whenever a public
device-state or feedback contract changes.

## 🤝 Contributing

Contributions are welcome!
Please open issues or pull requests on GitHub.
See the [issues page](https://github.com/DualSenseClient/VIIPER/issues) for bugs and feature requests.

## ❓ FAQ

### What is USBIP and why does VIIPER use it?

USBIP is a protocol that allows USB devices to be shared over a network.
VIIPER uses it because it's already built into Linux and available for Windows, making virtual device emulation
possible without writing custom kernel drivers yourself.

### Why choose the standalone executable and interfacing via TCP over the (shared-object) libVIIPER library?

- **Flexibility**
    - allows one to use VIIPER as a service on the same host as the USBIP client and use the feeder on a different, remote machine
    - allows software written using VIIPER to **not** be licensed under the terms of the GPLv3
    - allows users to independently update VIIPER to receive updates and bugfixes without affecting other components
      or having to recompile applications themselves — this also removes maintenance burdens for feeder-application developers (likely you)

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

### Does VIIPER proxy USB devices?

Yes — VIIPER has a proxy mode that sits between a USBIP client and a USBIP server (like a Linux machine sharing real
USB devices). It intercepts and logs all URBs passing through, without handling the devices directly. Useful for
reverse engineering USB protocols and understanding how devices communicate.

### What about TCP overhead or input latency?

End-to-end input latency for virtual devices created with VIIPER is typically well below 1 millisecond on a modern
desktop. Detailed methodology and sample runs can be found in the [E2E Latency Benchmarks](docs/testing/e2e_latency.md).
However, to not stress the CPU excessively, reports get batched and sent every millisecond, so the best you will achieve
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

The VIIPER server and core are licensed under GPL-3.0-or-later; see [`LICENSE.txt`](LICENSE.txt) for the full text.
Generated client libraries retain their documented MIT licensing; see
[`internal/codegen/common/license.go`](internal/codegen/common/license.go).

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

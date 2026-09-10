# Installation

libVIIPER is a shared library (`libVIIPER.dll` on Windows, `libVIIPER.so` on Linux) that you link
against directly from your application, eliminating the need for a separate server process.

libVIIPER relies on USBIP, so you must have a USBIP-Client implementation available on your
system to use the virtual devices.

!!! warning "License"
    Linking against libVIIPER requires your application to be licensed under the **GPL-3.0**
    (or a compatible license).

## Requirements

### USBIP

=== "Windows"

    libVIIPER requires the signed
    [usbip-win2 0.9.8.0 or 0.9.7.7 x64 release](https://github.com/vadimgrn/usbip-win2/releases/tag/v.0.9.8.0).
    Install one of those releases before starting VIIPER or an application that
    embeds it.

    !!! danger "Do not substitute another USBIP build"
        VIIPER supports **0.9.8.0** (serial + low-latency receive mode) with
        fallback to **0.9.7.7**. Version 0.9.7.8 reproduced
        kernel pool corruption during controller attachment and is explicitly
        rejected. The installer verifies the version and live ABI before
        allowing VIIPER to start.

    !!! warning "USBIP-Win2 signing certificate"
        The upstream installer may add the publicly available USBIP
        test-signing CA to **Trusted Root Certification Authorities**. After
        installation, you may remove the certificate named **USBIP** with
        `certlm.msc` (run as administrator). Keep the driver and userspace
        files from the same release: VIIPER validates this userspace/driver ABI.

=== "Linux"

    #### Ubuntu/Debian

    ```bash
    sudo apt install linux-tools-generic
    ```

    [Ubuntu USBIP Manual](https://manpages.ubuntu.com/manpages/noble/man8/usbip.8.html)

    #### Arch Linux

    ```bash
    sudo pacman -S usbip
    ```

    [Arch Wiki: USBIP](https://wiki.archlinux.org/title/USB/IP)

    ### Linux Kernel Module Setup

    !!! info "USBIP Client Requirement"
        USBIP requires the `vhci-hcd` (Virtual Host Controller Interface) kernel module on Linux for client operations. This includes libVIIPER's auto-attach feature and manual device attachment.

    Most Linux distributions include this module but do not load it automatically.

    See the [USBIP Setup guide](usbip.md) for detailed instructions.

## Getting libVIIPER

### Pre-built Binaries

Download the latest `libVIIPER` release artifact from the [DualSenseClient/VIIPER Releases](https://github.com/DualSenseClient/VIIPER/releases) page.
The archive contains:

- `libVIIPER.dll` / `libVIIPER.so`: the shared library
- `libVIIPER.h`: the C header
- `libVIIPER.def`: Windows import definition (for generating `.lib`/`.dll.a` import libraries)

### Building from Source

```bash
git clone https://github.com/DualSenseClient/VIIPER.git
cd VIIPER
just build-libVIIPER
```

The output will be in `dist/libVIIPER/`.

!!! info "CGO Required"
    Building libVIIPER requires CGO (`CGO_ENABLED=1`) and a C compiler (GCC / MSVC / Clang) in `PATH`.
    On Windows, [mingw-w64](https://www.mingw-w64.org/) or MSVC is required.

See [libVIIPER documentation](../libviiper/overview.md) for integration guides and examples.
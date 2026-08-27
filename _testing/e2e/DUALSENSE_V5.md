# DualSense V5 consumer-observed benchmark

`Benchmark_DualSenseV5_ConsumerObservedLatency` complements the existing Xbox
benchmark. It creates the current
`dualsensecombinedaudioduplexv5rawinputevents` device, sends the negotiated
53-byte input payload, and timestamps its observation through SDL3's Windows
virtual PS5 gamepad. Every measured epoch publishes
neutral, R2=1, R2=80 with new stick/motion state, R2=255, and R2=0 before
waiting. A press is delivered only when SDL observes the 255-level axis peak
before release.

The benchmark reports delivered, missed, reordered, and false-repress counts,
plus median, p95, p99, p99.9 (at 1,000+ delivered samples), maximum, and sample
count. It also reports the maximum observed trigger axis, validated output
frame/sequence/CRC errors, process allocation/GC counts, and (for runs longer
than the five-second aggregation interval) USB/IP queue age/high-water,
endpoint lateness, response-queue wait, send-lock wait, and socket-write
metrics. HID interrupt queue age/lateness is reported separately from aggregate
ISO endpoint maxima. After timing stops, one diagnostic device snapshot reports
input transition high-water/overflow and VIIPER receive-to-selection and
receive-to-presentation distributions. Those process-local percentiles are the
conservative upper bounds of fixed histogram buckets; maximum remains the exact
observed duration and receive timestamps are never rebased. Timed-loop latency
storage and V5 writer queues are allocated before warmup. The trigger observer
consumes a fixed C-side SDL3 axis-event ring
without Go callbacks or timed-loop allocation, and one reusable watchdog timer
is reset for transition and stale-repress windows. The default 100 ms watchdog
records long scheduler tails as latency rather than misclassifying a late but
valid SDL event as lost; it adds no delay when transitions arrive normally.

## Production and smoke runs

Run a Release-equivalent Go build with a fixed iteration count. Production
comparisons require at least 10,000 individual transitions per mode:

```powershell
$env:CGO_ENABLED = '1'
$env:PATH = '<repo>\_testing\e2e\deps\SDL\build\Debug;<mingw-bin>;' + $env:PATH
go test ./_testing/e2e -run '^$' -bench '^Benchmark_DualSenseV5_ConsumerObservedLatency$' -benchtime=10000x -count=5 -benchmem
```

A practical end-to-end smoke run uses a lower fixed count and shorter warmup:

```powershell
$env:VIIPER_E2E_WARMUP = '10'
go test ./_testing/e2e -run '^$' -bench '^Benchmark_DualSenseV5_ConsumerObservedLatency$' -benchtime=20x -count=1 -benchmem -v
```

Both commands run `Idle` and `Loaded`. Loaded mode continuously multiplexes V5
microphone PCM with input, drives SDL rumble, trigger rumble, LED, and a native
adaptive-trigger effect, and attempts to open the newly enumerated virtual
four-channel 48 kHz playback and stereo recording endpoints. Playback feeds
nonzero front speaker and rear haptics channels; recording is continuously
drained. If Windows or SDL3 does not expose or permit opening either endpoint,
the benchmark explicitly logs that the corresponding consumer-side ISO stream
was not activated; V5 microphone transport and controller output load still
run.

## Patched and baseline binaries

By default the benchmark starts the server from the source tree under test on
ports 3344/3345. To exercise an already-running Release binary, including an
unmodified baseline, set:

```powershell
$env:VIIPER_E2E_EXTERNAL = '1'
$env:VIIPER_E2E_API_ADDR = '127.0.0.1:3245'
```

The same patched harness then creates and removes only its own bus and device
through that binary. For a baseline predating the event-capability alias, also
set:

```powershell
$env:VIIPER_DUALSENSE_DEVICE_TYPE = 'dualsensecombinedaudioduplexv5'
```

The default remains the exact current raw-input/events alias.
`VIIPER_E2E_PASSWORD`,
`VIIPER_E2E_API_LISTEN`, `VIIPER_E2E_USB_LISTEN`, and `VIIPER_E2E_WARMUP` are
available for controlled test environments. A baseline that loses most epochs
can use `VIIPER_E2E_TRANSITION_TIMEOUT_MS=5` to keep comparison runs practical;
the minimum accepted timeout is two advertised endpoint intervals.

## Physical L2 waveform comparison

`TestDualSenseV5L2PhysicalWaveformSDL` is an opt-in correctness diagnostic for
comparing a physical USB DualSense trace with VIIPER's actual Windows virtual
PS5 consumer output. It starts an in-process server on an ephemeral USB/IP port
and a separately reserved loopback API port; it never connects to an installed
VIIPER service. The trace rises through the captured analog values at roughly
four-millisecond intervals, holds 255 for approximately 2.45 seconds, falls to
zero, and retransmits each value at the one-millisecond physical report
cadence. SDL3's fixed C-side event ring captures the LEFT trigger axis.
The same run then presents 255, holds raw L2=40 for 250 milliseconds, and
releases, distinguishing a lower-but-active trigger value from a literal
consumer-observed zero.

```powershell
$env:VIIPER_E2E_L2_WAVEFORM = '1'
go test ./_testing/e2e -run '^TestDualSenseV5L2PhysicalWaveformSDL$' -count=1 -v
```

The diagnostic reports the complete changed-value sequences sent over V5 and
observed by SDL. It fails on a missing or unexpected analog value, a
non-monotonic edge, a neutral event before the final V5 zero, a nonzero event
after consumer-observed release, or an SDL event-ring drop. It also builds three
reports through the production interrupt encoder and prints the L2 fields,
both committed counters, controller clocks, touch/trigger feedback region,
battery, and connection status beside the physical-report behavioral
reference. The captured trigger-mechanism contract is asserted: R2 remains
`0x09`, L2 settles at `0x29` while held and returns to `0x09` on release, the
L2 effect nibble remains `0x20`, physical sensor/device clocks are preserved,
the physical headset/filter status byte `55` is preserved, and the virtual
connection byte is wired USB `0x08`. The physical AES-CMAC
tail is intentionally excluded because virtual remapping and counters make it
impossible to copy validly without the controller key.

For allocation reporting around one complete consumer-observed waveform, use
the one-shot benchmark form:

```powershell
go test ./_testing/e2e -run '^$' -bench '^Benchmark_DualSenseV5_L2PhysicalWaveformSDL$' -benchtime=1x -count=1 -benchmem -v
```

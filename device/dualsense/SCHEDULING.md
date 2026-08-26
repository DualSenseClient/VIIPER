# DualSense scheduling and ownership

## Input

`DualSense.input` is the only virtual-input state and encoder owner. Its fixed
64-entry ring contains complete transition-bearing `InputState` values. A
single replaceable slot contains continuous motion/state, and a dedicated
front retry slot preserves a failed ordered claim even if the transition ring
refills while a response waits for socket ownership.

The semantic lanes are:

| Work | Semantics | Owner |
| --- | --- | --- |
| Button, D-pad, touch contact/ID and trigger zero-edge state | bounded ordered complete-state ring | input scheduler |
| Stick, motion, contact coordinates and in-epoch trigger movement | one latest snapshot | input scheduler |
| Claimed interrupt report | one immutable report/token | input scheduler until `CompleteInputReport` |
| HID sequence, virtual timestamp, last presented report | commit-on-success state | input scheduler |

The endpoint worker calls `ClaimInputReport` once at an interrupt service
opportunity. Claiming selects and encodes but does not make the state visible to
`GET_REPORT`, advance the committed HID sequence, or mark a trigger peak as
presented. `CompleteInputReport(token, true)` performs those commits only after
the complete USB/IP response is written successfully. Cancellation, reset, or
write failure completes false: ordered work moves to the front retry slot;
continuous work is restored only if no newer or contradictory state exists.

Receive-to-selection latency is recorded once when a received logical state is
first claimed at an endpoint opportunity. Its sampled marker follows the state
through failed-send recovery, so a retry does not double-count or replace the
first-opportunity measurement. Receive-to-presentation is retained separately
and ends only after the complete USB/IP response write succeeds. Both use
fixed buckets and exclude repeated HID idle reports.

A V5 TCP attach advances only the receive-generation token used to reject a
displaced reader. It does not clear accepted transition/latest/retry work, an
in-flight HID claim, the committed encoder sequence, or the previous final
state used for edge classification. Those belong to the virtual USB device
lifecycle, not to a recoverable transport reconnect.

Trigger epochs retain the complete state that actually accompanied each new
analog peak. An unclaimed press may be strengthened only in its own trigger
field. Once claimed, it is immutable; an unrepresented later peak is promoted
as its truthful complete snapshot before a release or unrelated transition.

## Output and media

The V5 framed writer is the sole socket and frame-sequence owner. Its lanes are:

| Work | Semantics |
| --- | --- |
| Adaptive trigger/rumble/light/LED output state | one replaceable latest validity-aware state |
| Microphone-interface lifecycle event | bounded ordered control queue |
| Realtime rear haptics and atomic speaker/haptics | generation-tagged time-indexed queues |

The writer round-robins ready lanes, so sustained realtime haptics cannot
starve lifecycle, latest output, or speaker work. Every payload is marshaled
into a writer-owned fixed slot before enqueue. The completed frame is built in
one reusable buffer and written under the writer's sole socket ownership. The
latest-state latch has storage independent of the ordered-control pool. When a
realtime lane is saturated, its oldest unstarted generation is replaced so a
stale media backlog is never replayed after transport backpressure.

Speaker assembly uses fixed per-device accumulators, a bounded rear-generation
ring, and four fixed callback slots that each own their 1,920-byte speaker
payload. No callback aliases the mutable assembly buffer. Endpoint reset and
alternate-setting changes advance `speakerMediaGeneration` while holding
`mediaMu`; admitted ISO work carries that token. The device rejects stale work
under the same lock, and the writer invalidates/drains both realtime and atomic
media lanes before its lifecycle barrier returns.

Active speaker gain uses an explicit pooled scratch-buffer lease. The media
consumer releases that lease after synchronous assembly; no per-block closure
or heap object is created, and no media/generation lock remains held while the
consumer callback executes.

V5 microphone-interface events are opt-in. Only these device aliases emit
frame `0x85`:

- `dualsensecombinedaudioduplexv5events`
- `dualsenseaudioonlyduplexv5events`
- `dualsenseedgecombinedaudioduplexv5events`

The payload is an active byte followed by a little-endian 64-bit stream
generation. Legacy aliases never receive this frame. A patched client that
cannot create an events alias can query the narrow
`bus/{busId}/{devId}/microphone-interface` endpoint.

## Lock partition and ordering

There is no required nested lock order; subsystem locks must be acquired one at
a time and released before acquiring another:

| Lock | State |
| --- | --- |
| `input.mu` | input queues, epochs, claim/retry, encoder and input telemetry |
| `metaMu` | immutable-style metadata pointer |
| `outputMu` | output/effect merge state and feature subcommand |
| `mediaMu` | speaker feature state, media accumulators/ring and media generation |
| `microphoneMu` | microphone feature state and bounded jitter buffer |
| `callbackMu` | callback registrations and stream-generation ownership |

Callbacks are copied under `callbackMu`, then invoked after it and every
subsystem lock have been released. Diagnostics snapshot each subsystem under
its own short lock and only then construct maps or JSON. No device lock is held
during USB/IP response I/O, V5 socket I/O, waits, logging, or callbacks.

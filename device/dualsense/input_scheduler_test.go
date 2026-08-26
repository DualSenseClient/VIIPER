package dualsense

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestDualSenseInputSchedulerPreservesRapidTriggerPeakBeforeRelease(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	neutral := neutralInputState()
	dev.UpdateInputState(&neutral)
	state := neutral
	state.R2 = 1
	dev.UpdateInputState(&state)
	state.R2 = 80
	state.GyroX = 1200
	dev.UpdateInputState(&state)
	state.R2 = 255
	state.GyroX = 2400
	dev.UpdateInputState(&state)
	state.R2 = 0
	state.Buttons &^= ButtonR2
	state.GyroX = 3600
	dev.UpdateInputState(&state)

	var report [InputReportSize]byte
	if got := dev.BuildInputReportInto(report[:]); got != InputReportSize {
		t.Fatalf("peak report length=%d", got)
	}
	if report[6] != 255 || report[9]&byte(ButtonR2>>8) == 0 {
		t.Fatalf("first service did not present coherent R2 peak: % x", report[:11])
	}
	if got := dev.BuildInputReportInto(report[:]); got != InputReportSize {
		t.Fatalf("release report length=%d", got)
	}
	if report[6] != 0 || report[9]&byte(ButtonR2>>8) != 0 {
		t.Fatalf("release did not follow peak: % x", report[:11])
	}
	dev.BuildInputReportInto(report[:])
	if report[6] != 0 || report[9]&byte(ButtonR2>>8) != 0 {
		t.Fatalf("stale pressed state followed release: % x", report[:11])
	}

	snapshot := dev.InputSchedulerState()
	if snapshot.TransitionHighWater != 2 || snapshot.Overflows != 0 {
		t.Fatalf("unexpected queue telemetry: %+v", snapshot)
	}
}

func TestDualSenseInputSchedulerOrdersTruthfulPeakBeforeButtonTransitions(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	neutral := neutralInputState()
	dev.UpdateInputState(&neutral)

	state := neutral
	state.R2 = 1
	dev.UpdateInputState(&state)
	var report [InputReportSize]byte
	dev.BuildInputReportInto(report[:]) // The initial press is already presented.
	if report[6] != 1 {
		t.Fatalf("initial press=%d", report[6])
	}

	state.R2 = 255 // Continuous peak; no unrelated control changed yet.
	state.GyroY = 111
	dev.UpdateInputState(&state)
	state.R2 = 80
	state.Buttons |= ButtonCross // Ordered transition after the truthful peak.
	state.GyroY = 222
	dev.UpdateInputState(&state)
	state.Buttons &^= ButtonCross
	state.GyroY = 333
	dev.UpdateInputState(&state)
	state.R2 = 0
	state.Buttons &^= ButtonR2
	state.GyroY = 444
	dev.UpdateInputState(&state)

	dev.BuildInputReportInto(report[:])
	if report[6] != 255 || report[8]&byte(ButtonCross) != 0 ||
		int16(binary.LittleEndian.Uint16(report[18:20])) != 111 {
		t.Fatalf("truthful peak snapshot was not ordered first: % x", report[:22])
	}
	dev.BuildInputReportInto(report[:])
	if report[6] != 80 || report[8]&byte(ButtonCross) == 0 {
		t.Fatalf("button down did not follow peak: % x", report[:11])
	}
	dev.BuildInputReportInto(report[:])
	if report[6] != 80 || report[8]&byte(ButtonCross) != 0 {
		t.Fatalf("button release was reordered: % x", report[:11])
	}
	dev.BuildInputReportInto(report[:])
	if report[6] != 0 || report[8]&byte(ButtonCross) != 0 {
		t.Fatalf("trigger release did not remain last: % x", report[:11])
	}
}

func TestDualSenseInputSchedulerEqualClockPreservesIndependentSimultaneousPeaksTruthfully(t *testing.T) {
	tests := []struct {
		name               string
		firstPeak          [2]uint8
		secondPeak         [2]uint8
		firstPresentation  [2]uint8
		secondPresentation [2]uint8
	}{
		{
			name:      "left peak first",
			firstPeak: [2]uint8{240, 90}, secondPeak: [2]uint8{80, 250},
			firstPresentation:  [2]uint8{240, 2},
			secondPresentation: [2]uint8{80, 250},
		},
		{
			name:      "right peak first",
			firstPeak: [2]uint8{90, 250}, secondPeak: [2]uint8{240, 80},
			firstPresentation:  [2]uint8{1, 250},
			secondPresentation: [2]uint8{240, 80},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dev, err := New(nil)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			// Force every receive observation onto the same clock reading. Peak
			// chronology must come from the scheduler receive ordinal, not a
			// timestamp tie-break that can reverse R2-first reports.
			receivedAt := time.Unix(123, 456)
			state := neutralInputState()
			dev.input.updateAt(&state, 0, receivedAt)
			state.L2, state.R2 = 1, 2
			dev.input.updateAt(&state, 0, receivedAt)
			state.L2, state.R2 = tc.firstPeak[0], tc.firstPeak[1]
			dev.input.updateAt(&state, 0, receivedAt)
			state.L2, state.R2 = tc.secondPeak[0], tc.secondPeak[1]
			dev.input.updateAt(&state, 0, receivedAt)
			state.L2, state.R2 = 0, 0
			state.Buttons &^= ButtonL2 | ButtonR2
			dev.input.updateAt(&state, 0, receivedAt)

			var report [InputReportSize]byte
			dev.BuildInputReportInto(report[:])
			if report[5] != tc.firstPresentation[0] ||
				report[6] != tc.firstPresentation[1] {
				t.Fatalf("first presentation invented or reordered trigger state: got=(%d,%d) want=(%d,%d)",
					report[5], report[6], tc.firstPresentation[0],
					tc.firstPresentation[1])
			}
			dev.BuildInputReportInto(report[:])
			if report[5] != tc.secondPresentation[0] ||
				report[6] != tc.secondPresentation[1] {
				t.Fatalf("second truthful peak missing: got=(%d,%d) want=(%d,%d)",
					report[5], report[6], tc.secondPresentation[0],
					tc.secondPresentation[1])
			}
			dev.BuildInputReportInto(report[:])
			if report[5] != 0 || report[6] != 0 ||
				report[9]&byte((ButtonL2|ButtonR2)>>8) != 0 {
				t.Fatalf("simultaneous release did not follow peaks: % x",
					report[:11])
			}
			dev.BuildInputReportInto(report[:])
			if report[5] != 0 || report[6] != 0 {
				t.Fatalf("stale simultaneous re-press followed release: % x",
					report[:11])
			}
		})
	}
}

func TestDualSenseInputSchedulerAllowsCooccurringCombinedTriggerPeak(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	state := neutralInputState()
	dev.UpdateInputState(&state)
	state.L2, state.R2 = 1, 2
	dev.UpdateInputState(&state)
	state.L2, state.R2 = 240, 250
	dev.UpdateInputState(&state)
	state.L2, state.R2 = 0, 0
	state.Buttons &^= ButtonL2 | ButtonR2
	dev.UpdateInputState(&state)

	var report [InputReportSize]byte
	dev.BuildInputReportInto(report[:])
	if report[5] != 240 || report[6] != 250 {
		t.Fatalf("co-occurring combined peak was not preserved: % x", report[:11])
	}
	dev.BuildInputReportInto(report[:])
	if report[5] != 0 || report[6] != 0 {
		t.Fatalf("release did not follow combined peak: % x", report[:11])
	}
}

func TestDualSenseInputSchedulerClaimedInitialKeepsTruthfulPeakSnapshots(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	state := neutralInputState()
	dev.UpdateInputState(&state)
	state.L2, state.R2 = 1, 2
	dev.UpdateInputState(&state)

	var report [InputReportSize]byte
	_, initialToken := dev.ClaimInputReport(report[:])
	if initialToken == 0 || report[5] != 1 || report[6] != 2 {
		t.Fatalf("initial simultaneous press was not claimed: % x", report[:11])
	}
	state.L2, state.R2 = 240, 90
	dev.UpdateInputState(&state)
	time.Sleep(time.Microsecond)
	state.L2, state.R2 = 80, 250
	dev.UpdateInputState(&state)
	state.L2, state.R2 = 0, 0
	state.Buttons &^= ButtonL2 | ButtonR2
	dev.UpdateInputState(&state)
	dev.CompleteInputReport(initialToken, true)

	dev.BuildInputReportInto(report[:])
	if report[5] != 240 || report[6] != 90 {
		t.Fatalf("claimed press was followed by invented first peak: % x",
			report[:11])
	}
	dev.BuildInputReportInto(report[:])
	if report[5] != 80 || report[6] != 250 {
		t.Fatalf("claimed press lost second truthful peak: % x", report[:11])
	}
	dev.BuildInputReportInto(report[:])
	if report[5] != 0 || report[6] != 0 {
		t.Fatalf("release did not follow claimed peak snapshots: % x", report[:11])
	}
}

func TestDualSenseInputSchedulerRetryInitialCannotMergeIndependentPeaks(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	state := neutralInputState()
	dev.UpdateInputState(&state)
	state.L2, state.R2 = 1, 2
	dev.UpdateInputState(&state)

	var report [InputReportSize]byte
	_, failedToken := dev.ClaimInputReport(report[:])
	dev.CompleteInputReport(failedToken, false)
	state.L2, state.R2 = 240, 90
	dev.UpdateInputState(&state)
	time.Sleep(time.Microsecond)
	state.L2, state.R2 = 80, 250
	dev.UpdateInputState(&state)
	state.L2, state.R2 = 0, 0
	state.Buttons &^= ButtonL2 | ButtonR2
	dev.UpdateInputState(&state)

	dev.BuildInputReportInto(report[:])
	if report[5] != 240 || report[6] != 2 {
		t.Fatalf("retry merged independently timed peaks: % x", report[:11])
	}
	dev.BuildInputReportInto(report[:])
	if report[5] != 80 || report[6] != 250 {
		t.Fatalf("retry did not preserve second truthful peak: % x", report[:11])
	}
	dev.BuildInputReportInto(report[:])
	if report[5] != 0 || report[6] != 0 {
		t.Fatalf("release did not follow retry peaks: % x", report[:11])
	}
}

func TestDualSenseGetReportSnapshotsWithoutSelectingOrAdvancing(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	state := neutralInputState()
	dev.UpdateInputState(&state)
	state.Buttons = ButtonCross
	dev.UpdateInputState(&state)

	var streaming [InputReportSize]byte
	dev.BuildInputReportInto(streaming[:])
	if streaming[7] != 1 {
		t.Fatalf("first streaming sequence=%d", streaming[7])
	}
	state.Buttons = ButtonCircle
	dev.UpdateInputState(&state)

	for attempt := 0; attempt < 2; attempt++ {
		control, handled := dev.HandleControl(hidClassIN, hidGetReport,
			uint16(reportTypeInput)<<8|uint16(ReportIDInput), 0,
			InputReportSize, nil)
		if !handled || len(control) != InputReportSize {
			t.Fatalf("GET_REPORT attempt %d: handled=%t length=%d",
				attempt, handled, len(control))
		}
		if control[7] != 1 || control[8]&byte(ButtonCross) == 0 ||
			control[8]&byte(ButtonCircle) != 0 {
			t.Fatalf("GET_REPORT advanced or selected pending input: % x", control[:11])
		}
	}

	dev.BuildInputReportInto(streaming[:])
	if streaming[7] != 2 || streaming[8]&byte(ButtonCircle) == 0 {
		t.Fatalf("streaming owner did not retain sequence/state ownership: % x",
			streaming[:11])
	}
}

func TestDualSenseVersionedInputSnapshotAdvancesOnlyOnPresentation(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var initial, claimed, snapshot [InputReportSize]byte
	n, initialVersion := dev.SnapshotInputReportInto(initial[:])
	if n != InputReportSize || initialVersion == 0 ||
		!dev.InputReportSnapshotCurrent(initialVersion) {
		t.Fatalf("initial snapshot n=%d version=%d current=%t",
			n, initialVersion, dev.InputReportSnapshotCurrent(initialVersion))
	}

	state := neutralInputState()
	dev.UpdateInputState(&state)
	state.Buttons = ButtonCross
	dev.UpdateInputState(&state)
	n, token := dev.ClaimInputReport(claimed[:])
	if n != InputReportSize || token == 0 {
		t.Fatalf("claim n=%d token=%d", n, token)
	}
	dev.CompleteInputReport(token, false)
	if !dev.InputReportSnapshotCurrent(initialVersion) {
		t.Fatal("failed presentation advanced the GET_REPORT version")
	}

	n, token = dev.ClaimInputReport(claimed[:])
	if n != InputReportSize || token == 0 {
		t.Fatalf("retry claim n=%d token=%d", n, token)
	}
	dev.CompleteInputReport(token, true)
	if dev.InputReportSnapshotCurrent(initialVersion) {
		t.Fatal("successful presentation left the old snapshot current")
	}
	n, currentVersion := dev.SnapshotInputReportInto(snapshot[:])
	if n != InputReportSize || currentVersion == initialVersion ||
		!dev.InputReportSnapshotCurrent(currentVersion) {
		t.Fatalf("current snapshot n=%d old=%d current=%d",
			n, initialVersion, currentVersion)
	}
	if snapshot != claimed {
		t.Fatal("versioned snapshot did not copy the successfully presented report")
	}

	allocations := testing.AllocsPerRun(1000, func() {
		_, version := dev.SnapshotInputReportInto(snapshot[:])
		if !dev.InputReportSnapshotCurrent(version) {
			panic("fresh snapshot is stale")
		}
	})
	if allocations != 0 {
		t.Fatalf("versioned snapshot allocated %.2f objects", allocations)
	}
}

func TestDualSenseCancelledClaimRetriesTransitionBeforeRelease(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	state := neutralInputState()
	dev.UpdateInputState(&state)
	state.R2 = 1
	dev.UpdateInputState(&state)

	var claimed [InputReportSize]byte
	n, token := dev.ClaimInputReport(claimed[:])
	if n != InputReportSize || token == 0 || claimed[6] == 0 {
		t.Fatalf("failed to claim trigger down: n=%d token=%d report=% x",
			n, token, claimed[:11])
	}
	state.R2 = 0
	state.Buttons &^= ButtonR2
	dev.UpdateInputState(&state)

	control, handled := dev.HandleControl(hidClassIN, hidGetReport,
		uint16(reportTypeInput)<<8|uint16(ReportIDInput), 0,
		InputReportSize, nil)
	if !handled || control[6] != 0 || control[7] != 0 ||
		control[9]&byte(ButtonR2>>8) != 0 {
		t.Fatalf("GET_REPORT exposed an unpresented claim: % x", control[:11])
	}

	dev.CompleteInputReport(token, false)
	n, retryToken := dev.ClaimInputReport(claimed[:])
	if n != InputReportSize || retryToken == 0 || claimed[6] == 0 ||
		claimed[9]&byte(ButtonR2>>8) == 0 || claimed[7] != 1 {
		t.Fatalf("failed down was not retried first: n=%d token=%d report=% x",
			n, retryToken, claimed[:11])
	}
	dev.CompleteInputReport(retryToken, true)

	n, releaseToken := dev.ClaimInputReport(claimed[:])
	if n != InputReportSize || releaseToken == 0 || claimed[6] != 0 ||
		claimed[9]&byte(ButtonR2>>8) != 0 || claimed[7] != 2 {
		t.Fatalf("release did not follow successful retry: n=%d token=%d report=% x",
			n, releaseToken, claimed[:11])
	}
	dev.CompleteInputReport(releaseToken, true)
}

func TestDualSenseCancelledContinuousPeakClaimRetriesBeforeRelease(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	state := neutralInputState()
	dev.UpdateInputState(&state)
	var report [InputReportSize]byte
	dev.BuildInputReportInto(report[:])

	state.R2 = 1
	state.Buttons |= ButtonR2
	dev.UpdateInputState(&state)
	dev.BuildInputReportInto(report[:])
	if report[6] != 1 {
		t.Fatalf("initial press was not presented: % x", report[:11])
	}

	state.R2 = 255
	state.LX = 77
	dev.UpdateInputState(&state)
	n, peakToken := dev.ClaimInputReport(report[:])
	if n != InputReportSize || peakToken == 0 || report[6] != 255 {
		t.Fatalf("failed to claim continuous peak: n=%d token=%d report=% x",
			n, peakToken, report[:11])
	}

	state.R2 = 0
	state.Buttons &^= ButtonR2
	dev.UpdateInputState(&state)
	dev.CompleteInputReport(peakToken, false)

	n, retryToken := dev.ClaimInputReport(report[:])
	if n != InputReportSize || retryToken == 0 || report[6] != 255 ||
		report[9]&byte(ButtonR2>>8) == 0 {
		t.Fatalf("failed continuous peak was not recovered first: n=%d token=%d report=% x",
			n, retryToken, report[:11])
	}
	dev.CompleteInputReport(retryToken, true)

	n, releaseToken := dev.ClaimInputReport(report[:])
	if n != InputReportSize || releaseToken == 0 || report[6] != 0 ||
		report[9]&byte(ButtonR2>>8) != 0 {
		t.Fatalf("release did not follow recovered peak: n=%d token=%d report=% x",
			n, releaseToken, report[:11])
	}
	dev.CompleteInputReport(releaseToken, true)

	// Idle HID service repeats the committed release, never the failed peak.
	n, idleToken := dev.ClaimInputReport(report[:])
	if n != InputReportSize || idleToken == 0 || report[6] != 0 ||
		report[9]&byte(ButtonR2>>8) != 0 {
		t.Fatalf("stale peak replayed after release: n=%d token=%d report=% x",
			n, idleToken, report[:11])
	}
	dev.CompleteInputReport(idleToken, true)
}

func TestDualSenseFinalTriggerAnalogDigitalCoherence(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	state := neutralInputState()
	dev.UpdateInputState(&state)
	state.Buttons = ButtonL2 | ButtonR2
	// Explicit mapped digital presses with zero analog are normalized to the
	// smallest non-zero actuation before both classification and encoding.
	dev.UpdateInputState(&state)

	var report [InputReportSize]byte
	dev.BuildInputReportInto(report[:])
	if report[5] != 1 || report[6] != 1 ||
		report[9]&byte((ButtonL2|ButtonR2)>>8) != byte((ButtonL2|ButtonR2)>>8) {
		t.Fatalf("analog/digital trigger wire state disagreed: % x", report[:11])
	}
}

func TestDualSenseInputTelemetryExposesFixedBucketDistributions(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dev.SetInputTelemetryEnabled(true)
	dev.inputTransportTelemetry.frameReadToDecode.record(40 * time.Microsecond)
	dev.inputTransportTelemetry.frameReadToDecode.record(2 * time.Millisecond)
	dev.inputTransportTelemetry.decodeToPublish.record(15 * time.Microsecond)

	state := neutralInputState()
	state.Buttons = ButtonCross
	dev.UpdateInputState(&state)
	var report [InputReportSize]byte
	dev.BuildInputReportInto(report[:])
	dev.BuildInputReportInto(report[:]) // Repeated HID idle is not a receive sample.

	snapshot := dev.InputTelemetryState()
	if !snapshot.Enabled || snapshot.FrameReadToDecode.Count != 2 ||
		snapshot.FrameReadToDecode.P50 != 50*time.Microsecond ||
		snapshot.FrameReadToDecode.P99 != 2*time.Millisecond ||
		snapshot.DecodeToPublication.Count != 1 ||
		snapshot.ReceiveToSelected.Count != 1 ||
		snapshot.ReceiveToPresented.Count != 1 {
		t.Fatalf("unexpected telemetry snapshot: %+v", snapshot)
	}
}

func TestDualSenseReceiveToSelectedSamplesOnceAcrossRetry(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	receivedAt := time.Unix(123, 456)
	selectedAt := receivedAt.Add(750 * time.Microsecond)
	state := neutralInputState()
	state.R2 = 220
	state.Buttons |= ButtonR2
	if !dev.input.updateAt(&state, 0, receivedAt) {
		t.Fatal("input update was rejected")
	}

	var report [InputReportSize]byte
	dev.input.mu.Lock()
	n, token := dev.input.beginClaim(selectedAt, BatteryFullyCharged, report[:])
	dev.input.mu.Unlock()
	if n != InputReportSize || token == 0 || report[6] != state.R2 {
		t.Fatalf("first selection failed: n=%d token=%d report=% x",
			n, token, report[:11])
	}
	dev.input.mu.Lock()
	dev.input.completeClaimAt(token, false, selectedAt.Add(time.Millisecond))
	dev.input.mu.Unlock()

	// The failed ordered state is selected again much later. Its sampled bit is
	// part of the retry entry, so only the original endpoint opportunity counts.
	retryAt := receivedAt.Add(3 * time.Millisecond)
	dev.input.mu.Lock()
	n, retryToken := dev.input.beginClaim(
		retryAt, BatteryFullyCharged, report[:],
	)
	dev.input.mu.Unlock()
	if n != InputReportSize || retryToken == 0 || report[6] != state.R2 {
		t.Fatalf("retry selection failed: n=%d token=%d report=% x",
			n, retryToken, report[:11])
	}
	presentedAt := receivedAt.Add(4 * time.Millisecond)
	dev.input.mu.Lock()
	dev.input.completeClaimAt(retryToken, true, presentedAt)
	dev.input.mu.Unlock()

	snapshot := dev.InputTelemetryState()
	if snapshot.ReceiveToSelected.Count != 1 ||
		snapshot.ReceiveToSelected.Maximum != 750*time.Microsecond ||
		snapshot.ReceiveToSelected.P99 != time.Millisecond {
		t.Fatalf("selection distribution double-counted retry: %+v",
			snapshot.ReceiveToSelected)
	}
	if snapshot.ReceiveToPresented.Count != 1 ||
		snapshot.ReceiveToPresented.Maximum != 4*time.Millisecond ||
		snapshot.ReceiveToPresented.P99 != 4*time.Millisecond {
		t.Fatalf("presentation distribution was not independent: %+v",
			snapshot.ReceiveToPresented)
	}

	args := dev.GetDeviceSpecificArgs()
	distributions, ok := args["inputLatencyDistributions"].(InputTelemetrySnapshot)
	if !ok || distributions.ReceiveToSelected.Count != 1 {
		t.Fatalf("DeviceSpecificArgs omitted selection distribution: %#v",
			args["inputLatencyDistributions"])
	}
	if got, ok := args["inputMaximumSelectionAgeUS"].(int64); !ok || got != 750 {
		t.Fatalf("DeviceSpecificArgs selection maximum = %#v",
			args["inputMaximumSelectionAgeUS"])
	}
}

func TestDualSenseInputGenerationRejectsStalePublisher(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	old := dev.input.currentGeneration()
	current := dev.beginInputStreamGeneration()
	if current == old || current == 0 {
		t.Fatalf("generation did not advance: old=%d current=%d", old, current)
	}
	state := neutralInputState()
	state.Buttons = ButtonTriangle
	if dev.updateInputStateForGeneration(old, &state) {
		t.Fatal("stale stream generation published input")
	}
	var report [InputReportSize]byte
	dev.BuildInputReportInto(report[:])
	if report[8]&byte(ButtonTriangle) != 0 {
		t.Fatalf("stale input reached HID report: % x", report[:11])
	}
}

func TestDualSenseInputReconnectPreservesAcceptedTransitions(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	generationA := dev.input.currentGeneration()
	state := neutralInputState()
	if !dev.updateInputStateForGeneration(generationA, &state) {
		t.Fatal("generation A baseline was rejected")
	}
	state.R2 = 200
	state.Buttons |= ButtonR2
	if !dev.updateInputStateForGeneration(generationA, &state) {
		t.Fatal("generation A press was rejected")
	}

	generationB := dev.beginInputStreamGeneration()
	if generationB == generationA || generationB == 0 {
		t.Fatalf("receive generation did not advance: A=%d B=%d",
			generationA, generationB)
	}
	stale := state
	stale.Buttons |= ButtonTriangle
	if dev.updateInputStateForGeneration(generationA, &stale) {
		t.Fatal("displaced generation A reader published after reconnect")
	}
	state.R2 = 0
	state.Buttons &^= ButtonR2
	if !dev.updateInputStateForGeneration(generationB, &state) {
		t.Fatal("generation B release was rejected")
	}

	var report [InputReportSize]byte
	n, pressToken := dev.ClaimInputReport(report[:])
	if n != InputReportSize || pressToken == 0 || report[6] != 200 ||
		report[9]&byte(ButtonR2>>8) == 0 ||
		report[8]&byte(ButtonTriangle) != 0 {
		t.Fatalf("accepted generation A press was not selected first: n=%d token=%d report=% x",
			n, pressToken, report[:11])
	}
	dev.CompleteInputReport(pressToken, true)
	n, releaseToken := dev.ClaimInputReport(report[:])
	if n != InputReportSize || releaseToken == 0 || report[6] != 0 ||
		report[9]&byte(ButtonR2>>8) != 0 {
		t.Fatalf("generation B release did not follow press: n=%d token=%d report=% x",
			n, releaseToken, report[:11])
	}
	dev.CompleteInputReport(releaseToken, true)
}

func TestDualSenseInputReconnectPreservesInflightClaimRecovery(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	generationA := dev.input.currentGeneration()
	state := neutralInputState()
	dev.updateInputStateForGeneration(generationA, &state)
	state.R2 = 180
	state.Buttons |= ButtonR2
	dev.updateInputStateForGeneration(generationA, &state)

	var report [InputReportSize]byte
	_, claimToken := dev.ClaimInputReport(report[:])
	if claimToken == 0 || report[6] != 180 {
		t.Fatalf("generation A press was not claimed: token=%d report=% x",
			claimToken, report[:11])
	}
	generationB := dev.beginInputStreamGeneration()
	state.R2 = 0
	state.Buttons &^= ButtonR2
	dev.updateInputStateForGeneration(generationB, &state)
	dev.CompleteInputReport(claimToken, false)

	_, retryToken := dev.ClaimInputReport(report[:])
	if retryToken == 0 || report[6] != 180 ||
		report[9]&byte(ButtonR2>>8) == 0 {
		t.Fatalf("reconnect erased failed in-flight press: token=%d report=% x",
			retryToken, report[:11])
	}
	dev.CompleteInputReport(retryToken, true)
	_, releaseToken := dev.ClaimInputReport(report[:])
	if releaseToken == 0 || report[6] != 0 ||
		report[9]&byte(ButtonR2>>8) != 0 {
		t.Fatalf("release did not follow recovered claim: token=%d report=% x",
			releaseToken, report[:11])
	}
	dev.CompleteInputReport(releaseToken, true)
}

func TestDualSenseInputHotPathAllocations(t *testing.T) {
	if raceEnabled {
		t.Skip("race instrumentation allocates; allocation contract is tested without -race")
	}
	scheduler := newDualSenseInputScheduler(BatteryFullyCharged)
	state := neutralInputState()
	scheduler.update(&state, 0)
	allocations := testing.AllocsPerRun(1000, func() {
		state.GyroX++
		scheduler.update(&state, 0)
	})
	if allocations != 0 {
		t.Fatalf("scheduler update allocated %.2f objects/run", allocations)
	}

	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dev.UpdateInputState(&state)
	var report [InputReportSize]byte
	allocations = testing.AllocsPerRun(1000, func() {
		n, token := dev.ClaimInputReport(report[:])
		if n != 0 {
			dev.CompleteInputReport(token, true)
		}
	})
	if allocations != 0 {
		t.Fatalf("select/encode allocated %.2f objects/run", allocations)
	}

	// Exercise both receive-to-selection and receive-to-presentation histogram
	// updates with a newly received state on every measured iteration.
	telemetryDev, err := New(nil)
	if err != nil {
		t.Fatalf("New telemetry device: %v", err)
	}
	telemetryState := neutralInputState()
	allocations = testing.AllocsPerRun(1000, func() {
		telemetryState.GyroX++
		telemetryDev.UpdateInputState(&telemetryState)
		n, token := telemetryDev.ClaimInputReport(report[:])
		if n != InputReportSize || token == 0 {
			panic("received state was not claimed")
		}
		telemetryDev.CompleteInputReport(token, true)
	})
	if allocations != 0 {
		t.Fatalf("receive/select/present telemetry allocated %.2f objects/run",
			allocations)
	}

	var payload [InputStateSize]byte
	if err := state.MarshalInto(payload[:]); err != nil {
		t.Fatalf("MarshalInto: %v", err)
	}
	var decoded InputState
	header := [...]byte{StreamFrameVersionV5, StreamFrameInputState, InputStateSize, 0}
	allocations = testing.AllocsPerRun(1000, func() {
		_ = framedStreamCRC(header[:], payload[:])
		_ = decoded.UnmarshalBinary(payload[:])
	})
	if allocations != 0 {
		t.Fatalf("V5 CRC/decode allocated %.2f objects/run", allocations)
	}
}

func BenchmarkDualSenseInputSchedulerUpdate(b *testing.B) {
	scheduler := newDualSenseInputScheduler(BatteryFullyCharged)
	state := neutralInputState()
	scheduler.update(&state, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		state.GyroX++
		scheduler.update(&state, 0)
	}
}

func BenchmarkDualSenseInputSelectEncode(b *testing.B) {
	dev, err := New(nil)
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	state := neutralInputState()
	dev.UpdateInputState(&state)
	var report [InputReportSize]byte
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, token := dev.ClaimInputReport(report[:])
		dev.CompleteInputReport(token, true)
	}
}

func BenchmarkDualSenseV5CRCDecode(b *testing.B) {
	state := neutralInputState()
	var payload [InputStateSize]byte
	if err := state.MarshalInto(payload[:]); err != nil {
		b.Fatalf("MarshalInto: %v", err)
	}
	header := [...]byte{StreamFrameVersionV5, StreamFrameInputState, InputStateSize, 0}
	var decoded InputState
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = framedStreamCRC(header[:], payload[:])
		_ = decoded.UnmarshalBinary(payload[:])
	}
}

package usb

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"slices"
	"time"

	"github.com/DualSenseClient/VIIPER/usb"
	"github.com/DualSenseClient/VIIPER/usbip"
	"github.com/DualSenseClient/VIIPER/virtualbus"
)

const maximumVersionedInputReportSize = 1024

func (s *Server) handleUrbStream(conn net.Conn, dev usb.Device) error {
	_ = conn.SetDeadline(time.Time{})
	defer s.resetInterfaceAlts(dev)

	var wireWriter = conn
	var batcher *batchingWriter
	if s.config.WriteBatchFlushInterval > 0 {
		batcher = newBatchingWriter(
			conn,
			writeBatcherBufferSize,
			s.config.WriteBatchFlushInterval,
			writeBatcherFlushAtBytes,
		)
		wireWriter = nil
		defer func() { _ = batcher.Close() }()
	}
	var responseDestination interface{ Write([]byte) (int, error) } = wireWriter
	if batcher != nil {
		responseDestination = batcher
	}
	responses := newResponseWriter(responseDestination, batcher)

	var owningBus *virtualbus.VirtualBus
	s.busesMu.Lock()
	buses := make([]*virtualbus.VirtualBus, 0, len(s.busses))
	for _, bus := range s.busses {
		buses = append(buses, bus)
	}
	s.busesMu.Unlock()
	for _, bus := range buses {
		if slices.Contains(bus.Devices(), dev) {
			owningBus = bus
			break
		}
	}
	if owningBus == nil {
		return fmt.Errorf("device does not belong to any bus")
	}

	deviceContext := owningBus.GetDeviceContext(dev)
	if deviceContext == nil {
		return fmt.Errorf("no device context available from bus")
	}

	schedulers := newEndpointSchedulers(deviceContext, dev, responses, conn)
	s.registerEndpointSchedulers(schedulers)
	defer func() {
		schedulers.close()
		s.unregisterEndpointSchedulers(schedulers)
	}()
	go s.emitEndpointDiagnostics(schedulers.ctx, schedulers)
	go func() {
		<-schedulers.ctx.Done()
		_ = conn.Close()
	}()

	var outPayloadScratch []byte
	isoPacketScratch := make([]usbip.IsoPacketDescriptor, maxIsoPackets)
	isoPacketWireScratch := make([]byte, maxIsoPackets*usbip.IsoPacketDescriptorSize)
	var responseScratch []byte
	var unlinkScratch []byte
	var inputReportScratch [maximumVersionedInputReportSize]byte
	var header [urbHdrSize]byte

	for {
		if err := usbip.ReadExactly(conn, header[:]); err != nil {
			if failure := schedulers.failure(); failure != nil {
				return failure
			}
			if deviceContext.Err() != nil {
				s.scheduleEmptyBusCleanup(owningBus)
				return nil
			}
			return fmt.Errorf("read URB header: %w", err)
		}

		command := binary.BigEndian.Uint32(
			header[urbHdrOffsetCommand : urbHdrOffsetCommand+4],
		)
		seq := binary.BigEndian.Uint32(header[urbHdrOffsetSeqnum : urbHdrOffsetSeqnum+4])
		dir := binary.BigEndian.Uint32(header[urbHdrOffsetDir : urbHdrOffsetDir+4])
		ep := binary.BigEndian.Uint32(header[urbHdrOffsetEp : urbHdrOffsetEp+4])
		if dir != usbip.DirOut && dir != usbip.DirIn {
			return fmt.Errorf("invalid URB direction %d (seq=%d)", dir, seq)
		}
		if ep > 15 {
			return fmt.Errorf("invalid URB endpoint %d (seq=%d)", ep, seq)
		}

		if command == usbip.CmdUnlinkCode {
			unlinkSeq := binary.BigEndian.Uint32(
				header[urbHdrOffsetUnlink : urbHdrOffsetUnlink+4],
			)
			removed := schedulers.unlink(unlinkSeq)
			status := int32(0)
			if removed {
				status = errConnReset
			}
			unlinkScratch = buildRetUnlinkPacket(unlinkScratch, seq, status)
			if err := responses.write(unlinkScratch, true, time.Now()); err != nil {
				return err
			}
			continue
		}
		if command != usbip.CmdSubmitCode {
			devid := binary.BigEndian.Uint32(
				header[urbHdrOffsetDevid : urbHdrOffsetDevid+4],
			)
			return fmt.Errorf("unsupported cmd %d (seq=%d, devid=%d)", command, seq, devid)
		}

		xferLen := binary.BigEndian.Uint32(header[urbHdrOffsetLength : urbHdrOffsetLength+4])
		if xferLen > maximumTransferSize {
			return fmt.Errorf("transfer length %d exceeds limit %d", xferLen, maximumTransferSize)
		}
		packetCountWire := int32(binary.BigEndian.Uint32(
			header[urbHdrOffsetPackets : urbHdrOffsetPackets+4],
		))
		if packetCountWire < -1 || packetCountWire > maxIsoPackets {
			return fmt.Errorf("invalid ISO packet count %d", packetCountWire)
		}

		descriptorIso := endpointIsIsochronous(dev.GetDescriptor(), ep, dir)
		// Some usbip-win clients encode ordinary control/bulk submissions with
		// NumberOfPackets == 0 instead of the documented -1 marker. A positive
		// count still asserts ISO, while the endpoint descriptor remains
		// authoritative for zero-descriptor ISO requests.
		isIso := packetCountWire > 0 || descriptorIso
		setup := header[urbHdrOffsetSetup:urbHdrSize]

		var outPayload []byte
		if dir == usbip.DirOut && xferLen > 0 {
			outPayloadScratch = resizeBytes(outPayloadScratch, int(xferLen))
			outPayload = outPayloadScratch
			if err := usbip.ReadExactly(conn, outPayload); err != nil {
				return fmt.Errorf("read OUT payload: %w", err)
			}
		}

		var isoPackets []usbip.IsoPacketDescriptor
		if isIso && packetCountWire > 0 {
			isoPackets = isoPacketScratch[:packetCountWire]
			if err := readIsoPacketDescriptors(
				conn, isoPacketWireScratch, isoPackets,
			); err != nil {
				return fmt.Errorf("read/decode %d ISO packet descriptors: %w",
					packetCountWire, err)
			}
		}

		if isIso {
			if err := validateIsoSubmission(
				dev.GetDescriptor(), ep, dir, xferLen, isoPackets,
			); err != nil {
				return fmt.Errorf("invalid ISO submission seq %d: %w", seq, err)
			}
		}

		if dir == usbip.DirIn && ep != 0 {
			if isIso {
				// An ISO request without packet descriptors has no service slots.
				// Complete it empty without touching the microphone source.
				if len(isoPackets) == 0 {
					responseScratch = buildRetSubmitPacket(
						responseScratch, seq, 0, 0, nil, nil, true,
					)
					if err := responses.write(responseScratch, true, time.Now()); err != nil {
						return err
					}
					continue
				}
				if schedulers.enqueueIsoIn(seq, ep, xferLen, isoPackets) {
					continue
				}
			} else if endpointIsInterrupt(dev.GetDescriptor(), ep, dir) {
				if err := validateInterruptSubmission(
					dev.GetDescriptor(), ep, dir, xferLen,
				); err != nil {
					return fmt.Errorf("invalid interrupt submission seq %d: %w", seq, err)
				}
				if schedulers.enqueueInterruptIn(seq, ep, xferLen) {
					continue
				}
			} else if schedulers.enqueueGenericIn(seq, ep, xferLen) {
				continue
			}

			responseScratch = buildRetSubmitPacket(
				responseScratch, seq, errNoSpace, 0, nil, isoPackets, isIso,
			)
			if err := responses.write(responseScratch, true, time.Now()); err != nil {
				return err
			}
			continue
		}

		if dir == usbip.DirOut && isIso {
			if schedulers.enqueueIsoOut(seq, ep, xferLen, outPayload, isoPackets) {
				continue
			}
			for index := range isoPackets {
				isoPackets[index].ActualLength = 0
				isoPackets[index].Status = errNoSpace
			}
			responseScratch = buildRetSubmitPacket(
				responseScratch, seq, errNoSpace, 0, nil, isoPackets, true,
			)
			if err := responses.write(responseScratch, true, time.Now()); err != nil {
				return err
			}
			continue
		}

		if snapshotter, reportLength, ok := versionedInputReportRequest(
			dev, ep, dir, setup, xferLen,
		); ok {
			var err error
			responseScratch, err = writeVersionedInputReportResponse(
				responses, snapshotter, responseScratch, inputReportScratch[:],
				seq, reportLength,
			)
			if err != nil {
				return err
			}
			continue
		}

		// Publish the endpoint generation boundary before the device performs its
		// reset. A due old-generation media callback can then either finish before
		// the device reset (and be drained by it) or observe the new device token;
		// it cannot be admitted after the reset under an old scheduler generation.
		lifecycleResetFromSetup(schedulers, setup)
		responseData := s.processSubmit(deviceContext, dev, ep, dir, setup, outPayload)
		actualLength := uint32(len(responseData))
		if dir == usbip.DirOut {
			actualLength = uint32(len(outPayload))
		}
		flush := ep == 0 || endpointIsInterrupt(dev.GetDescriptor(), ep, dir)
		responseScratch = buildRetSubmitPacket(
			responseScratch, seq, 0, actualLength, responseData, nil, false,
		)
		if err := responses.write(responseScratch, flush, time.Now()); err != nil {
			return err
		}
	}
}

func readIsoPacketDescriptors(
	reader io.Reader,
	wireScratch []byte,
	packets []usbip.IsoPacketDescriptor,
) error {
	if len(packets) > len(wireScratch)/usbip.IsoPacketDescriptorSize {
		return fmt.Errorf("descriptor scratch length %d cannot hold packet count %d",
			len(wireScratch), len(packets))
	}
	wire := wireScratch[:len(packets)*usbip.IsoPacketDescriptorSize]
	if _, err := io.ReadFull(reader, wire); err != nil {
		return err
	}
	return decodeIsoPacketDescriptors(wire, packets)
}

func decodeIsoPacketDescriptors(
	wire []byte,
	packets []usbip.IsoPacketDescriptor,
) error {
	if len(packets) > len(wire)/usbip.IsoPacketDescriptorSize ||
		len(wire) != len(packets)*usbip.IsoPacketDescriptorSize {
		return fmt.Errorf("descriptor wire length %d does not match packet count %d",
			len(wire), len(packets))
	}
	for index := range packets {
		offset := index * usbip.IsoPacketDescriptorSize
		if err := packets[index].Decode(
			wire[offset : offset+usbip.IsoPacketDescriptorSize],
		); err != nil {
			return err
		}
	}
	return nil
}

func versionedInputReportRequest(
	dev usb.Device,
	ep, dir uint32,
	setup []byte,
	xferLen uint32,
) (inputReportSnapshotter, int, bool) {
	if ep != 0 || dir != usbip.DirIn || len(setup) != 8 ||
		setup[0] != hidReqTypeIn || setup[1] != hidReqGetReport {
		return nil, 0, false
	}
	wValue := binary.LittleEndian.Uint16(setup[2:4])
	const (
		hidInputReportType     = 0x01
		dualSenseInputReportID = 0x01
	)
	if uint8(wValue>>8) != hidInputReportType ||
		uint8(wValue) != dualSenseInputReportID {
		return nil, 0, false
	}
	snapshotter, ok := dev.(inputReportSnapshotter)
	if !ok {
		return nil, 0, false
	}
	wLength := int(binary.LittleEndian.Uint16(setup[6:8]))
	reportLength := min(int(xferLen), wLength, maximumVersionedInputReportSize)
	if reportLength < 0 {
		reportLength = 0
	}
	return snapshotter, reportLength, true
}

// writeVersionedInputReportResponse builds the complete EP0 response before
// acquiring send ownership, then validates that no newer interrupt report was
// presented in the meantime. A stale snapshot is discarded and rebuilt. The
// input lock is held only by the two device methods; it is never held during
// response I/O.
func writeVersionedInputReportResponse(
	responses *responseWriter,
	snapshotter inputReportSnapshotter,
	responseBuffer []byte,
	reportBuffer []byte,
	seq uint32,
	reportLength int,
) ([]byte, error) {
	if reportLength < 0 {
		reportLength = 0
	}
	reportLength = min(reportLength, len(reportBuffer))
	for {
		n, version := snapshotter.SnapshotInputReportInto(
			reportBuffer[:reportLength],
		)
		if n < 0 {
			n = 0
		}
		n = min(n, reportLength)
		responseBuffer = buildRetSubmitPacket(
			responseBuffer, seq, 0, uint32(n), reportBuffer[:n], nil, false,
		)
		written, err := responses.writeIf(
			responseBuffer, true, time.Now(),
			func() bool { return snapshotter.InputReportSnapshotCurrent(version) },
		)
		if err != nil {
			return responseBuffer, err
		}
		if written {
			return responseBuffer, nil
		}
	}
}

func endpointIsInterrupt(desc *usb.Descriptor, ep, dir uint32) bool {
	endpoint, found := findEndpointDescriptor(desc, ep, dir)
	return found && endpoint.BMAttributes&0x03 == 0x03
}

func (s *Server) scheduleEmptyBusCleanup(bus *virtualbus.VirtualBus) {
	if bus == nil {
		return
	}
	busID := bus.BusID()
	emptyContext := bus.GetBusEmptyContext()
	if emptyContext == nil {
		if current := s.GetBus(busID); current != nil && len(current.Devices()) == 0 {
			if err := s.RemoveBus(busID); err != nil {
				s.logger.Error("failed to remove empty bus", "busID", busID, "error", err)
			}
		}
		return
	}

	go func() {
		timer := time.NewTimer(s.config.BusCleanupTimeout)
		defer timer.Stop()
		select {
		case <-emptyContext.Done():
			return
		case <-timer.C:
			if current := s.GetBus(busID); current != nil && len(current.Devices()) == 0 {
				if err := s.RemoveBus(busID); err != nil {
					s.logger.Error("timeout: failed to remove empty bus", "busID", busID, "error", err)
				} else {
					s.logger.Info("timeout: removed empty bus", "busID", busID)
				}
			}
		}
	}()
}

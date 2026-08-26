package usb

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Alia5/VIIPER/internal/log"
	"github.com/Alia5/VIIPER/usb"
	"github.com/Alia5/VIIPER/usbip"
	"github.com/Alia5/VIIPER/virtualbus"
)

type batchingWriter struct {
	mu           sync.Mutex
	w            *bufio.Writer
	flushEvery   time.Duration
	flushAtBytes int
	stopCh       chan struct{}
	closeOnce    sync.Once
	err          error
}

const (
	retSubmitHeaderSize = 0x30

	// avoid windows socket overhead while keeping latency very low.
	writeBatcherBufferSize   = 256 * 1024
	writeBatcherFlushAtBytes = 64 * 1024
)

func newBatchingWriter(dst io.Writer, bufSize int, flushEvery time.Duration, flushAtBytes int) *batchingWriter {
	if bufSize <= 0 {
		bufSize = writeBatcherBufferSize
	}
	if flushAtBytes < 0 {
		flushAtBytes = 0
	}
	if flushAtBytes > bufSize {
		flushAtBytes = bufSize
	}
	bw := &batchingWriter{
		w:            bufio.NewWriterSize(dst, bufSize),
		flushEvery:   flushEvery,
		flushAtBytes: flushAtBytes,
		stopCh:       make(chan struct{}),
	}
	if flushEvery > 0 {
		go bw.flushLoop()
	}
	return bw
}

func (b *batchingWriter) flushLoop() {
	t := time.NewTicker(b.flushEvery)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			_ = b.Flush()
		case <-b.stopCh:
			return
		}
	}
}

func (b *batchingWriter) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return 0, b.err
	}

	n, err := b.w.Write(p)
	if err != nil {
		b.err = err
		return n, err
	}
	if b.flushAtBytes > 0 && b.w.Buffered() >= b.flushAtBytes {
		if err := b.w.Flush(); err != nil {
			b.err = err
			return n, err
		}
	}
	return n, nil
}

func (b *batchingWriter) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	if err := b.w.Flush(); err != nil {
		b.err = err
		return err
	}
	return nil
}

func (b *batchingWriter) Close() error {
	b.closeOnce.Do(func() {
		close(b.stopCh)
	})
	return b.Flush()
}

const (
	// USB standard request codes
	usbReqGetStatus        = 0x00
	usbReqClearFeature     = 0x01
	usbReqSetFeature       = 0x03
	usbReqSetAddress       = 0x05
	usbReqGetDescriptor    = 0x06
	usbReqSetDescriptor    = 0x07
	usbReqGetConfiguration = 0x08
	usbReqSetConfiguration = 0x09
	usbReqGetInterface     = 0x0A
	usbReqSetInterface     = 0x0B

	// USB descriptor types
	usbDescTypeDevice        = 0x01
	usbDescTypeConfiguration = 0x02
	usbDescTypeString        = 0x03
	usbDescTypeHID           = 0x21
	usbDescTypeHIDReport     = 0x22

	// USB request types (bmRequestType)
	usbReqTypeStandardToDevice      = 0x00
	usbReqTypeStandardFromInterface = 0x01
	usbReqTypeStandardToEndpoint    = 0x02
	usbReqTypeStandardToInterface   = 0x81
	usbReqTypeStandardFromDevice    = 0x80
	usbReqTypeMask                  = 0x60
	usbReqTypeClass                 = 0x20

	// USB interface classes
	usbInterfaceClassHID = 0x03

	// HID class requests (bRequest)
	hidReqGetReport   = 0x01
	hidReqGetIdle     = 0x02
	hidReqGetProtocol = 0x03
	hidReqSetReport   = 0x09
	hidReqSetIdle     = 0x0A
	hidReqSetProtocol = 0x0B

	// HID class request types (bmRequestType)
	hidReqTypeIn  = 0xA1
	hidReqTypeOut = 0x21

	// wIndex low-byte interface selector mask.
	usbIfaceIndexMask = 0x00FF

	// USB configuration values
	usbConfigValueDefault   = 1
	usbConfigAttrBusPowered = 0x80
	usbConfigMaxPower100mA  = 50 // In units of 2mA

	// URB header field offsets
	urbHdrSize          = 0x30
	urbHdrOffsetCommand = 0x00
	urbHdrOffsetSeqnum  = 0x04
	urbHdrOffsetDevid   = 0x08
	urbHdrOffsetDir     = 0x0c
	urbHdrOffsetEp      = 0x10
	urbHdrOffsetUnlink  = 0x14
	urbHdrOffsetFlags   = 0x14
	urbHdrOffsetLength  = 0x18
	urbHdrOffsetPackets = 0x20
	urbHdrOffsetSetup   = 0x28
	maxIsoPackets       = 1024

	// Standard header peek size
	headerPeekSize = 8

	// BUSID buffer size for import
	busIDSize = 32

	// Error codes
	errConnReset = -104 // -ECONNRESET
)

type Server struct {
	config                *ServerConfig
	logger                *slog.Logger
	rawLogger             log.RawLogger
	busses                map[uint32]*virtualbus.VirtualBus
	busesMu               sync.Mutex
	alts                  map[usb.Device]map[uint8]uint8
	altsMu                sync.Mutex
	ready                 chan struct{}
	readyOnce             sync.Once
	ln                    net.Listener
	diagnosticsMu         sync.Mutex
	diagnosticConnections map[*endpointSchedulers]struct{}
}

func New(config ServerConfig, logger *slog.Logger, rawLogger log.RawLogger) *Server {
	return &Server{
		config:                &config,
		logger:                logger,
		rawLogger:             rawLogger,
		busses:                make(map[uint32]*virtualbus.VirtualBus),
		alts:                  make(map[usb.Device]map[uint8]uint8),
		ready:                 make(chan struct{}),
		diagnosticConnections: make(map[*endpointSchedulers]struct{}),
	}
}

// AddBus registers a bus with the server. If the bus number is already present,
// an error is returned.
func (s *Server) AddBus(bus *virtualbus.VirtualBus) error {
	s.busesMu.Lock()
	defer s.busesMu.Unlock()
	if bus == nil {
		return fmt.Errorf("bus is nil")
	}
	if _, ok := s.busses[bus.BusID()]; ok {
		return fmt.Errorf("bus %d already registered", bus.BusID())
	}
	s.busses[bus.BusID()] = bus
	return nil
}

// RemoveBus unregisters a bus from the server.
func (s *Server) RemoveBus(busID uint32) error {
	s.busesMu.Lock()
	bus, ok := s.busses[busID]
	if !ok {
		s.busesMu.Unlock()
		return fmt.Errorf("bus %d not found", busID)
	}

	devices := bus.Devices()
	s.busesMu.Unlock()

	if len(devices) > 0 {
		s.logger.Warn(fmt.Sprintf("Removing non-empty bus %d with %d device(s) attached; removing devices", busID, len(devices)))
		for _, dev := range devices {
			_ = bus.Remove(dev)
		}
	}

	s.busesMu.Lock()
	delete(s.busses, busID)
	s.busesMu.Unlock()

	return bus.Close()
}

// RemoveDeviceByID removes a device by busId and cancels its connections.
func (s *Server) RemoveDeviceByID(busID uint32, deviceID string) error {
	s.busesMu.Lock()
	bus, ok := s.busses[busID]
	s.busesMu.Unlock()

	if !ok {
		return fmt.Errorf("bus %d not found", busID)
	}
	err := bus.RemoveDeviceByID(deviceID)
	if err != nil {
		return err
	}

	if emptyCtx := bus.GetBusEmptyContext(); emptyCtx != nil {
		go func() {
			slog.Debug("Started bus cleanup goroutine (RemoveDeviceByID)")
			select {
			case <-emptyCtx.Done():
				// Cancelled - a new device was added
				return
			case <-time.After(s.config.BusCleanupTimeout):
				if b := s.GetBus(busID); b != nil && len(b.Devices()) == 0 {
					if err := s.RemoveBus(busID); err != nil {
						s.logger.Error("timeout: failed to remove empty bus", "busID", busID, "error", err)
					} else {
						s.logger.Info("timeout: removed empty bus", "busID", busID)
					}
				}
			}
		}()
	} else {
		s.logger.Debug("No bus empty context; Cleaning bus immediately")
		if b := s.GetBus(busID); b != nil && len(b.Devices()) == 0 {
			if err := s.RemoveBus(busID); err != nil {
				s.logger.Error("timeout: failed to remove empty bus", "busID", busID, "error", err)
			} else {
				s.logger.Info("timeout: removed empty bus", "busID", busID)
			}
		}
	}

	return nil
}

// ListBuses returns a snapshot of active bus numbers.
func (s *Server) ListBuses() []uint32 {
	s.busesMu.Lock()
	defer s.busesMu.Unlock()
	out := make([]uint32, 0, len(s.busses))
	for k := range s.busses {
		out = append(out, k)
	}
	return out
}

// GetBus returns a bus by ID or nil if not present.
func (s *Server) GetBus(busID uint32) *virtualbus.VirtualBus {
	s.busesMu.Lock()
	defer s.busesMu.Unlock()
	return s.busses[busID]
}

func (s *Server) NextFreeBusID() uint32 {
	s.busesMu.Lock()
	defer s.busesMu.Unlock()
	var id uint32 = 1
	for {
		if _, exists := s.busses[id]; !exists {
			return id
		}
		id++
	}
}

func (s *Server) Addr() string {
	if s.ln != nil {
		return s.ln.Addr().String()
	}
	if s.config != nil {
		return s.config.Addr
	}
	return ""
}

// ListenAndServe starts the USB-IP server and handles incoming connections.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return err
	}
	s.ln = ln
	s.config.Addr = ln.Addr().String()
	s.readyOnce.Do(func() { close(s.ready) })
	s.logger.Info("USBIP server listening", "addr", s.config.Addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || strings.Contains(strings.ToLower(err.Error()), "use of closed network connection") {
				s.logger.Info("USBIP server stopped")
				return nil
			}
			s.logger.Error("Accept error", "error", err)
			continue
		}
		if tcpConn, ok := c.(*net.TCPConn); ok {
			if err := tcpConn.SetNoDelay(true); err != nil {
				s.logger.Warn("failed to set TCP_NODELAY", "error", err)
			}
		}
		s.logger.Info("Client connected", "remote", c.RemoteAddr())
		go func() {
			if err := s.handleConn(c); err != nil {
				if isClientDisconnect(err) {
					s.logger.Info("Client disconnected", "error", err)
				} else {
					s.logger.Error("Connection handler error", "error", err)
				}
			}
		}()
	}
}

// Ready returns a channel that is closed once the server has successfully bound
// to its listen address and is ready to accept connections.
func (s *Server) Ready() <-chan struct{} { return s.ready }

// Close stops the USB server by closing its listener.
func (s *Server) Close() error {
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

// GetListenPort extracts and returns the port number from the server's listen address.
func (s *Server) GetListenPort() uint16 {
	addr := s.Addr()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return 0
	}
	return uint16(port)
}

// --

func (s *Server) handleConn(conn net.Conn) error {
	defer conn.Close() //nolint:errcheck
	conn = &logConn{Conn: conn, s: s}
	if err := conn.SetDeadline(time.Now().Add(s.config.ConnectionTimeout)); err != nil {
		s.logger.Warn("Failed to set deadline", "error", err)
	}

	// Peek first 8 bytes to determine management op or URB stream.
	var hdrBuf [headerPeekSize]byte
	if err := usbip.ReadExactly(conn, hdrBuf[:]); err != nil {
		return fmt.Errorf("read header: %w", err)
	}

	ver := binary.BigEndian.Uint16(hdrBuf[0:2])
	code := binary.BigEndian.Uint16(hdrBuf[2:4])

	if ver == usbip.Version && (code == usbip.OpReqDevlist || code == usbip.OpReqImport) {
		switch code {
		case usbip.OpReqDevlist:
			s.logger.Info("OP_REQ_DEVLIST")
			return s.handleDevList(conn)
		case usbip.OpReqImport:
			s.logger.Info("OP_REQ_IMPORT")
			dev, err := s.handleImport(conn)
			if err != nil {
				return fmt.Errorf("handle import: %w", err)
			}
			return s.handleUrbStream(conn, dev)
		}
	}

	return fmt.Errorf("protocol violation: client sent URB data without OP_REQ_IMPORT")
}

func (s *Server) handleDevList(conn net.Conn) error {
	_ = conn.SetDeadline(time.Time{})
	var buf bytes.Buffer
	rep := usbip.MgmtHeader{Version: usbip.Version, Command: usbip.OpRepDevlist, Status: 0}
	_ = rep.Write(&buf)
	metas := s.getAllDeviceMetas()
	n := uint32(len(metas))
	dlh := usbip.DevListReplyHeader{NDevices: n}
	_ = dlh.Write(&buf)
	for _, m := range metas {
		desc := m.Dev.GetDescriptor()
		meta := m.Meta

		exp := usbip.ExportedDevice{
			ExportMeta:          meta,
			Speed:               desc.Device.Speed,
			IDVendor:            desc.Device.IDVendor,
			IDProduct:           desc.Device.IDProduct,
			BcdDevice:           desc.Device.BcdDevice,
			BDeviceClass:        desc.Device.BDeviceClass,
			BDeviceSubClass:     desc.Device.BDeviceSubClass,
			BDeviceProtocol:     desc.Device.BDeviceProtocol,
			BConfigurationValue: usbConfigValueDefault,
			BNumConfigurations:  desc.Device.BNumConfigurations,
			BNumInterfaces:      desc.NumInterfaces(),
		}

		for _, iface := range descriptorListInterfaces(desc) {
			exp.Interfaces = append(exp.Interfaces, usbip.InterfaceDesc{
				Class:    iface.Descriptor.BInterfaceClass,
				SubClass: iface.Descriptor.BInterfaceSubClass,
				Protocol: iface.Descriptor.BInterfaceProtocol,
			})
		}
		_ = exp.WriteDevlist(&buf)
	}
	if _, err := conn.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("write devlist: %w", err)
	}
	return nil
}

func (s *Server) handleImport(conn net.Conn) (usb.Device, error) {
	var rest [busIDSize]byte
	if err := usbip.ReadExactly(conn, rest[:]); err != nil {
		return nil, fmt.Errorf("read import busid: %w", err)
	}
	reqBus := string(rest[:bytes.IndexByte(rest[:], 0)])
	s.logger.Info("Import request", "busid", reqBus)
	var chosen usb.Device
	var chosenMeta *usbip.ExportMeta
	var chosenDesc *usb.Descriptor
	for _, m := range s.getAllDeviceMetas() {
		meta := m.Meta
		end := bytes.IndexByte(meta.USBBusID[:], 0)
		bid := string(meta.USBBusID[:end])
		if bid == reqBus {
			chosen = m.Dev
			chosenMeta = &meta
			chosenDesc = m.Dev.GetDescriptor()
			break
		}
	}
	if chosen == nil || chosenMeta == nil || chosenDesc == nil {
		return nil, fmt.Errorf("no device matches busid %s", reqBus)
	}
	var buf bytes.Buffer
	rep := usbip.MgmtHeader{Version: usbip.Version, Command: usbip.OpRepImport, Status: 0}
	_ = rep.Write(&buf)
	exp := usbip.ExportedDevice{
		ExportMeta:          *chosenMeta,
		Speed:               chosenDesc.Device.Speed,
		IDVendor:            chosenDesc.Device.IDVendor,
		IDProduct:           chosenDesc.Device.IDProduct,
		BcdDevice:           chosenDesc.Device.BcdDevice,
		BDeviceClass:        chosenDesc.Device.BDeviceClass,
		BDeviceSubClass:     chosenDesc.Device.BDeviceSubClass,
		BDeviceProtocol:     chosenDesc.Device.BDeviceProtocol,
		BConfigurationValue: usbConfigValueDefault,
		BNumConfigurations:  chosenDesc.Device.BNumConfigurations,
		BNumInterfaces:      chosenDesc.NumInterfaces(),
	}
	for _, iface := range descriptorListInterfaces(chosenDesc) {
		exp.Interfaces = append(exp.Interfaces, usbip.InterfaceDesc{
			Class:    iface.Descriptor.BInterfaceClass,
			SubClass: iface.Descriptor.BInterfaceSubClass,
			Protocol: iface.Descriptor.BInterfaceProtocol,
		})
	}
	_ = exp.WriteImport(&buf)
	if _, err := conn.Write(buf.Bytes()); err != nil {
		return nil, fmt.Errorf("write import reply failed: %w", err)
	}
	return chosen, nil
}

// getAllDeviceMetas aggregates device metas from all registered busses.
func (s *Server) getAllDeviceMetas() []virtualbus.DeviceMeta {
	s.busesMu.Lock()
	defer s.busesMu.Unlock()
	out := []virtualbus.DeviceMeta{}
	for _, b := range s.busses {
		out = append(out, b.GetAllDeviceMetas()...)
	}
	return out
}

type logConn struct {
	net.Conn
	s *Server
}

func (lc *logConn) Read(p []byte) (int, error) {
	n, err := lc.Conn.Read(p)
	if n > 0 && lc.s.rawLogger != nil {
		lc.s.rawLogger.Log(true, p[:n])
	}
	return n, err
}

func (lc *logConn) Write(p []byte) (int, error) {
	n, err := lc.Conn.Write(p)
	if n > 0 && lc.s.rawLogger != nil {
		lc.s.rawLogger.Log(false, p[:n])
	}
	return n, err
}

func endpointIsIsochronous(desc *usb.Descriptor, ep, dir uint32) bool {
	if desc == nil || ep == 0 {
		return false
	}

	epAddr := uint8(ep) & 0x0f
	if dir == usbip.DirIn {
		epAddr |= 0x80
	}
	for i := range desc.Interfaces {
		for _, endpoint := range desc.Interfaces[i].Endpoints {
			if endpoint.BEndpointAddress == epAddr && endpoint.BMAttributes&0x03 == 0x01 {
				return true
			}
		}
	}
	return false
}

func usbServiceInterval(speed uint32, bInterval uint8) time.Duration {
	if bInterval == 0 {
		return 0
	}
	if speed >= 3 {
		return time.Duration(1<<(bInterval-1)) * 125 * time.Microsecond
	}
	return time.Duration(bInterval) * time.Millisecond
}

func isClientDisconnect(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		switch t := opErr.Err.(type) {
		case syscall.Errno:
			if t == syscall.ECONNRESET || t == syscall.EPIPE {
				return true
			}
		}
	}
	e := strings.ToLower(err.Error())
	if strings.Contains(e, "connection reset by peer") || strings.Contains(e, "forcibly closed") || strings.Contains(e, "an existing connection was forcibly closed") || strings.Contains(e, "aborted") {
		return true
	}
	return false
}

func (s *Server) processSubmit(ctx context.Context, dev usb.Device, ep uint32, dir uint32, setup []byte, out []byte) []byte {
	if ep != 0 {
		return dev.HandleTransfer(ctx, ep, dir, out)
	}
	if len(setup) != 8 {
		s.logger.Debug("EP0 submit with invalid setup size", "setupLen", len(setup), "setup", setup)
		return nil
	}
	bm := setup[0]
	breq := setup[1]
	wValue := binary.LittleEndian.Uint16(setup[2:4])
	wIndex := binary.LittleEndian.Uint16(setup[4:6])
	wLength := binary.LittleEndian.Uint16(setup[6:8])
	desc := dev.GetDescriptor()

	if breq == usbReqGetStatus {
		return []byte{0x00, 0x00}
	}
	if breq == usbReqSetAddress && bm == usbReqTypeStandardToDevice {
		return nil
	}
	if breq == usbReqSetConfiguration && bm == usbReqTypeStandardToDevice {
		s.resetInterfaceAlts(dev)
		return nil
	}
	if breq == usbReqGetConfiguration && bm == usbReqTypeStandardFromDevice {
		return []byte{0x01}
	}
	if breq == usbReqClearFeature && bm == usbReqTypeStandardToEndpoint &&
		wValue == 0 && wLength == 0 && uint8(wIndex>>8) == 0 &&
		descriptorHasEndpoint(desc, uint8(wIndex)) {
		if resetter, ok := dev.(usb.EndpointResetDevice); ok {
			resetter.ResetEndpoint(uint8(wIndex))
		}
		return nil
	}
	if breq == usbReqGetInterface && bm == usbReqTypeStandardToInterface {
		return []byte{s.getInterfaceAlt(dev, uint8(wIndex&usbIfaceIndexMask))}
	}
	if breq == usbReqSetInterface && bm == usbReqTypeStandardFromInterface {
		iface := uint8(wIndex & usbIfaceIndexMask)
		alt := uint8(wValue & 0xff)
		if descriptorHasInterfaceAlt(desc, iface, alt) {
			s.setInterfaceAlt(dev, iface, alt)
			s.notifyInterfaceAlt(dev, iface, alt)
		}
		return nil
	}

	if breq == usbReqGetDescriptor && bm == usbReqTypeStandardFromDevice {
		dtype := uint8(wValue >> 8)
		dindex := uint8(wValue & 0xff)
		var data []byte
		switch dtype {
		case usbDescTypeDevice:
			data = desc.Bytes()
		case usbDescTypeConfiguration:
			data = s.buildConfigDescriptor(desc)
		case usbDescTypeString:
			if dindex == 0xEE && desc.MicrosoftOS10 != nil {
				data = desc.MicrosoftOS10.StringDescriptor()
			} else if s, ok := desc.Strings[dindex]; ok {
				data = usb.EncodeStringDescriptor(s)
			}
		}
		if len(data) == 0 {
			return nil
		}
		if int(wLength) < len(data) {
			return data[:wLength]
		}
		return data
	}

	if desc.MicrosoftOS10 != nil &&
		(bm == 0xC0 || bm == 0xC1) &&
		(breq == desc.MicrosoftOS10.EffectiveVendorCode() ||
			wIndex == 0x0004 || wIndex == 0x0005) {
		if data, ok := desc.MicrosoftOS10.ControlResponse(wValue, wIndex); ok {
			if int(wLength) < len(data) {
				return data[:wLength]
			}
			return data
		}
	}

	if breq == usbReqGetDescriptor && bm == usbReqTypeStandardToInterface {
		dtype := uint8(wValue >> 8)
		iface := uint8(wIndex & 0xff)
		var data []byte
		if ifaceConf, ok := desc.Interface(iface); ok {
			if ifaceConf.HID != nil {
				switch dtype {
				case usbDescTypeHID:
					d, err := ifaceConf.HID.DescriptorBytes()
					if err != nil {
						s.logger.Error("failed to build HID descriptor", "iface", iface, "error", err)
						return nil
					}
					data = []byte(d)
				case usbDescTypeHIDReport:
					d, err := ifaceConf.HID.ReportBytes()
					if err != nil {
						s.logger.Error("failed to build HID report descriptor", "iface", iface, "error", err)
						return nil
					}
					data = []byte(d)
				}
			}
			if len(data) == 0 {
				for _, cd := range ifaceConf.ClassDescriptors {
					if cd.DescriptorType == dtype {
						data = []byte(cd.Bytes())
						break
					}
				}
			}
		}
		if len(data) == 0 {
			return nil
		}
		if int(wLength) < len(data) {
			return data[:wLength]
		}
		return data
	}

	// Handle common HID state requests before controller-specific dispatch. The
	// descriptor slice contains alternate-setting entries, so interface numbers
	// must be resolved logically rather than used as slice indexes.
	if ifaceConfig, ok := desc.Interface(uint8(wIndex & usbIfaceIndexMask)); ok &&
		ifaceConfig.Descriptor.BInterfaceClass == usbInterfaceClassHID {
		switch {
		case bm == hidReqTypeIn && breq == hidReqGetIdle:
			return []byte{0x00}
		case bm == hidReqTypeOut && breq == hidReqSetIdle:
			return nil
		case bm == hidReqTypeIn && breq == hidReqGetProtocol:
			return []byte{0x01}
		case bm == hidReqTypeOut && breq == hidReqSetProtocol:
			return nil
		}
	}

	if cd, ok := dev.(usb.ControlDevice); ok {
		if resp, handled := cd.HandleControl(bm, breq, wValue, wIndex, wLength, out); handled {
			if resp == nil {
				return nil
			}
			if int(wLength) < len(resp) {
				return resp[:wLength]
			}
			return resp
		}
	}

	if ifaceConfig, ok := desc.Interface(uint8(wIndex & usbIfaceIndexMask)); ok {
		if ifaceConfig.Descriptor.BInterfaceClass == usbInterfaceClassHID {
			switch {
			case (bm == hidReqTypeIn || bm == hidReqTypeOut) && (breq == hidReqGetReport || breq == hidReqSetReport):
				return nil
			}
		}
	}

	if (bm & usbReqTypeMask) != usbReqTypeClass {
		s.logger.Debug("EP0 control unhandled", "bmRequestType", bm, "bRequest", breq, "wValue", wValue, "wIndex", wIndex, "wLength", wLength)
	}

	return nil
}

func (s *Server) buildConfigDescriptor(desc *usb.Descriptor) []byte {
	var b bytes.Buffer
	configValue := desc.Configuration.BConfigurationValue
	if configValue == 0 {
		configValue = usbConfigValueDefault
	}
	attrs := desc.Configuration.BMAttributes
	if attrs == 0 {
		attrs = usbConfigAttrBusPowered
	}
	maxPower := desc.Configuration.BMaxPower
	if maxPower == 0 {
		maxPower = usbConfigMaxPower100mA
	}
	h := usb.ConfigHeader{
		WTotalLength:        0, // to be patched
		BNumInterfaces:      desc.NumInterfaces(),
		BConfigurationValue: configValue,
		IConfiguration:      desc.Configuration.IConfiguration,
		BMAttributes:        attrs,
		BMaxPower:           maxPower,
	}
	h.Write(&b)
	for _, iface := range desc.Interfaces {
		for _, iad := range desc.Associations {
			if iad.BFirstInterface == iface.Descriptor.BInterfaceNumber && iface.Descriptor.BAlternateSetting == 0 {
				iad.Write(&b)
			}
		}
		iface.Descriptor.Write(&b)
		if iface.HID != nil {
			hd, err := iface.HID.DescriptorBytes()
			if err != nil {
				s.logger.Error("failed to build HID descriptor", "iface", iface.Descriptor.BInterfaceNumber, "error", err)
				// Stall/return minimal config descriptor.
				return nil
			}
			b.Write([]byte(hd))
		}
		for _, cd := range iface.ClassDescriptors {
			b.Write([]byte(cd.Bytes()))
		}
		for _, ep := range iface.Endpoints {
			ep.Write(&b)
			for _, cd := range ep.ClassDescriptors {
				b.Write([]byte(cd.Bytes()))
			}
		}
	}

	data := b.Bytes()
	binary.LittleEndian.PutUint16(data[2:4], uint16(len(data)))
	return data
}

func descriptorListInterfaces(desc *usb.Descriptor) []usb.InterfaceConfig {
	out := make([]usb.InterfaceConfig, 0, desc.NumInterfaces())
	seen := map[uint8]struct{}{}
	for _, iface := range desc.Interfaces {
		n := iface.Descriptor.BInterfaceNumber
		if _, ok := seen[n]; ok || iface.Descriptor.BAlternateSetting != 0 {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, iface)
	}
	for _, iface := range desc.Interfaces {
		n := iface.Descriptor.BInterfaceNumber
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, iface)
	}
	return out
}

func descriptorHasInterfaceAlt(desc *usb.Descriptor, ifaceNumber, altSetting uint8) bool {
	for _, iface := range desc.Interfaces {
		if iface.Descriptor.BInterfaceNumber == ifaceNumber &&
			iface.Descriptor.BAlternateSetting == altSetting {
			return true
		}
	}
	return false
}

func descriptorHasEndpoint(desc *usb.Descriptor, endpointAddress uint8) bool {
	if desc == nil || endpointAddress&0x0f == 0 {
		return false
	}
	for _, iface := range desc.Interfaces {
		for _, endpoint := range iface.Endpoints {
			if endpoint.BEndpointAddress == endpointAddress {
				return true
			}
		}
	}
	return false
}

func descriptorInterfaceNumbers(desc *usb.Descriptor) []uint8 {
	out := make([]uint8, 0, desc.NumInterfaces())
	seen := map[uint8]struct{}{}
	for _, iface := range desc.Interfaces {
		n := iface.Descriptor.BInterfaceNumber
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

func (s *Server) notifyInterfaceAlt(dev usb.Device, iface, alt uint8) {
	if notifier, ok := dev.(usb.InterfaceAltSettingDevice); ok {
		notifier.SetInterfaceAltSetting(iface, alt)
	}
}

func (s *Server) notifyInterfaceAltsCleared(dev usb.Device) {
	notifier, ok := dev.(usb.InterfaceAltSettingDevice)
	if !ok {
		return
	}

	for _, iface := range descriptorInterfaceNumbers(dev.GetDescriptor()) {
		notifier.SetInterfaceAltSetting(iface, 0)
	}
}

func (s *Server) getInterfaceAlt(dev usb.Device, iface uint8) uint8 {
	s.altsMu.Lock()
	defer s.altsMu.Unlock()
	if s.alts == nil {
		return 0
	}
	if devAlts, ok := s.alts[dev]; ok {
		return devAlts[iface]
	}
	return 0
}

func (s *Server) setInterfaceAlt(dev usb.Device, iface, alt uint8) {
	s.altsMu.Lock()
	defer s.altsMu.Unlock()
	if s.alts == nil {
		s.alts = make(map[usb.Device]map[uint8]uint8)
	}
	devAlts := s.alts[dev]
	if devAlts == nil {
		devAlts = make(map[uint8]uint8)
		s.alts[dev] = devAlts
	}
	devAlts[iface] = alt
}

func (s *Server) clearInterfaceAlt(dev usb.Device) {
	s.altsMu.Lock()
	defer s.altsMu.Unlock()
	delete(s.alts, dev)
}

func (s *Server) resetInterfaceAlts(dev usb.Device) {
	s.clearInterfaceAlt(dev)
	s.notifyInterfaceAltsCleared(dev)
}

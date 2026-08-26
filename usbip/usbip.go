package usbip

import (
	"encoding/binary"
	"io"
)

// Wire constants (network byte order / big-endian)
const (
	Version = 0x0111

	// Management commands
	OpReqDevlist = 0x8005
	OpRepDevlist = 0x0005
	OpReqImport  = 0x8003
	OpRepImport  = 0x0003

	// URB transfer commands
	CmdSubmitCode = 0x00000001
	CmdUnlinkCode = 0x00000002
	RetSubmitCode = 0x00000003
	RetUnlinkCode = 0x00000004

	// Directions used in usbip_header_basic.direction
	DirOut = 0x00000000
	DirIn  = 0x00000001
)

// MgmtHeader is the 8-byte header for management ops (devlist/import).
type MgmtHeader struct {
	Version uint16
	Command uint16
	Status  uint32
}

func (h *MgmtHeader) Write(w io.Writer) error {
	var buf [8]byte
	binary.BigEndian.PutUint16(buf[0:2], h.Version)
	binary.BigEndian.PutUint16(buf[2:4], h.Command)
	binary.BigEndian.PutUint32(buf[4:8], h.Status)
	_, err := w.Write(buf[:])
	return err
}

// DevListReplyHeader is the header after MgmtHeader for OP_REP_DEVLIST.
type DevListReplyHeader struct {
	NDevices uint32
}

func (d *DevListReplyHeader) Write(w io.Writer) error {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[0:4], d.NDevices)
	_, err := w.Write(buf[:])
	return err
}

// ExportMeta carries USB-IP bus identity for an emulated device.
// Uses fixed-size arrays matching the wire protocol format.
type ExportMeta struct {
	Path     [256]byte
	USBBusID [32]byte
	BusID    uint32
	DevID    uint32
}

// ExportedDevice describes one exported device in devlist/import replies.
// Layout matches kernel doc, strings are fixed-size, remaining numbers are BE.
type ExportedDevice struct {
	ExportMeta
	Speed uint32

	IDVendor            uint16
	IDProduct           uint16
	BcdDevice           uint16
	BDeviceClass        uint8
	BDeviceSubClass     uint8
	BDeviceProtocol     uint8
	BConfigurationValue uint8
	BNumConfigurations  uint8
	BNumInterfaces      uint8

	// Interfaces: for each interface: class, subclass, protocol, pad
	Interfaces []InterfaceDesc
}

type InterfaceDesc struct {
	Class    uint8
	SubClass uint8
	Protocol uint8
}

// WriteDevlist writes the device entry for OP_REP_DEVLIST (includes interface triplets).
func (d *ExportedDevice) WriteDevlist(w io.Writer) error {
	if _, err := w.Write(d.Path[:]); err != nil {
		return err
	}
	if _, err := w.Write(d.USBBusID[:]); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.BusID); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.DevID); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.Speed); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.IDVendor); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.IDProduct); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.BcdDevice); err != nil {
		return err
	}
	if _, err := w.Write([]byte{
		d.BDeviceClass,
		d.BDeviceSubClass,
		d.BDeviceProtocol,
		d.BConfigurationValue,
		d.BNumConfigurations,
		d.BNumInterfaces,
	}); err != nil {
		return err
	}

	for _, iface := range d.Interfaces {
		if _, err := w.Write([]byte{iface.Class, iface.SubClass, iface.Protocol, 0}); err != nil {
			return err
		}
	}
	return nil
}

// WriteImport writes the device entry for OP_REP_IMPORT (ends at bNumInterfaces).
func (d *ExportedDevice) WriteImport(w io.Writer) error {
	if _, err := w.Write(d.Path[:]); err != nil {
		return err
	}
	if _, err := w.Write(d.USBBusID[:]); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.BusID); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.DevID); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.Speed); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.IDVendor); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.IDProduct); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.BcdDevice); err != nil {
		return err
	}
	if _, err := w.Write([]byte{
		d.BDeviceClass,
		d.BDeviceSubClass,
		d.BDeviceProtocol,
		d.BConfigurationValue,
		d.BNumConfigurations,
		d.BNumInterfaces,
	}); err != nil {
		return err
	}
	return nil
}

// HeaderBasic is common to all URB cmds and replies.
type HeaderBasic struct {
	Command uint32
	Seqnum  uint32
	Devid   uint32
	Dir     uint32
	Ep      uint32
}

// CmdSubmit header (before payload) length is 0x30.
type CmdSubmit struct {
	Basic             HeaderBasic
	TransferFlags     uint32
	TransferBufferLen uint32
	StartFrame        uint32
	NumberOfPackets   int32
	Interval          uint32
	Setup             [8]byte
}

func (c *CmdSubmit) Write(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, c.Basic.Command); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.Basic.Seqnum); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.Basic.Devid); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.Basic.Dir); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.Basic.Ep); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.TransferFlags); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.TransferBufferLen); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.StartFrame); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.NumberOfPackets); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.Interval); err != nil {
		return err
	}
	_, err := w.Write(c.Setup[:])
	return err
}

// RetSubmit header (before payload) length is 0x30.
type RetSubmit struct {
	Basic           HeaderBasic
	Status          int32
	ActualLength    uint32
	StartFrame      uint32
	NumberOfPackets int32
	ErrorCount      uint32
	Padding         [8]byte
}

// IsoPacketDescriptor is the 16-byte USB/IP representation of one packet in
// an isochronous URB. USB/IP appends one descriptor per packet after the data
// payload for both submit and return messages.
type IsoPacketDescriptor struct {
	Offset       uint32
	Length       uint32
	ActualLength uint32
	Status       int32
}

// IsoPacketDescriptorSize is the fixed USB/IP wire size of one ISO packet
// descriptor.
const IsoPacketDescriptorSize = 16

// Decode reads one fixed-layout descriptor from an already-owned wire buffer.
// Hot USB/IP readers should read all descriptors into connection-owned scratch
// first, then decode from that scratch so the io.Reader interface cannot make a
// temporary descriptor buffer escape once per packet.
func (d *IsoPacketDescriptor) Decode(wire []byte) error {
	if len(wire) < IsoPacketDescriptorSize {
		return io.ErrUnexpectedEOF
	}
	d.Offset = binary.BigEndian.Uint32(wire[0:4])
	d.Length = binary.BigEndian.Uint32(wire[4:8])
	d.ActualLength = binary.BigEndian.Uint32(wire[8:12])
	d.Status = int32(binary.BigEndian.Uint32(wire[12:16]))
	return nil
}

func (d *IsoPacketDescriptor) Read(r io.Reader) error {
	var wire [IsoPacketDescriptorSize]byte
	if _, err := io.ReadFull(r, wire[:]); err != nil {
		return err
	}
	return d.Decode(wire[:])
}

func (d IsoPacketDescriptor) Write(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, d.Offset); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.Length); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, d.ActualLength); err != nil {
		return err
	}
	return binary.Write(w, binary.BigEndian, d.Status)
}

func (r *RetSubmit) Write(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, r.Basic.Command); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.Basic.Seqnum); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.Basic.Devid); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.Basic.Dir); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.Basic.Ep); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.Status); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.ActualLength); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.StartFrame); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.NumberOfPackets); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.ErrorCount); err != nil {
		return err
	}
	_, err := w.Write(r.Padding[:])
	return err
}

// CmdUnlink and RetUnlink
type CmdUnlink struct {
	Basic        HeaderBasic
	UnlinkSeqnum uint32
	Padding      [24]byte
}

type RetUnlink struct {
	Basic   HeaderBasic
	Status  int32
	Padding [24]byte
}

func (c *CmdUnlink) Write(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, c.Basic.Command); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.Basic.Seqnum); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.Basic.Devid); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.Basic.Dir); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.Basic.Ep); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, c.UnlinkSeqnum); err != nil {
		return err
	}
	_, err := w.Write(c.Padding[:])
	return err
}

func (r *RetUnlink) Write(w io.Writer) error {
	if err := binary.Write(w, binary.BigEndian, r.Basic.Command); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.Basic.Seqnum); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.Basic.Devid); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.Basic.Dir); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.Basic.Ep); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, r.Status); err != nil {
		return err
	}
	_, err := w.Write(r.Padding[:])
	return err
}

func ReadExactly(r io.Reader, buf []byte) error {
	n := 0
	for n < len(buf) {
		m, err := r.Read(buf[n:])
		if err != nil {
			return err
		}
		n += m
	}
	return nil
}

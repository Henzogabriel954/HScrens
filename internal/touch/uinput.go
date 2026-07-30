package touch

import (
	"encoding/binary"
	"io"
	"math"
	"net"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	UINPUT_MAX_NAME_SIZE = 80
	UI_DEV_CREATE        = 0x5501
	UI_DEV_DESTROY       = 0x5502
	UI_SET_EVBIT         = 0x40045564
	UI_SET_KEYBIT        = 0x40045565
	UI_SET_ABSBIT        = 0x40045567
	UI_DEV_SETUP         = 0x405c5503
	UI_ABS_SETUP         = 0x401c5504

	EV_SYN = 0x00
	EV_KEY = 0x01
	EV_ABS = 0x03

	SYN_REPORT = 0

	BTN_TOUCH = 0x14a

	ABS_MT_SLOT        = 0x2f
	ABS_MT_POSITION_X  = 0x35
	ABS_MT_POSITION_Y  = 0x36
	ABS_MT_TRACKING_ID = 0x39

	ActionDown   uint8 = 0
	ActionMove   uint8 = 1
	ActionUp     uint8 = 2
	ActionCancel uint8 = 3
)

type inputId struct {
	Bustype uint16
	Vendor  uint16
	Product uint16
	Version uint16
}

type uinputSetup struct {
	id           inputId
	name         [UINPUT_MAX_NAME_SIZE]byte
	ffEffectsMax uint32
}

type absInfo struct {
	Value      int32
	Minimum    int32
	Maximum    int32
	Fuzz       int32
	Flat       int32
	Resolution int32
}

type uinputAbsSetup struct {
	code    uint16
	absinfo absInfo
}

type inputEvent struct {
	Time  unix.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

type TouchDevice struct {
	fd *os.File
}

func ioctl(fd uintptr, req uint, arg uintptr) error {
	_, _, err := unix.Syscall(unix.SYS_IOCTL, fd, uintptr(req), arg)
	if err != 0 {
		return err
	}
	return nil
}

func NewTouchDevice(maxX, maxY int32) (*TouchDevice, error) {
	fd, err := os.OpenFile("/dev/uinput", os.O_WRONLY|unix.O_NONBLOCK, 0660)
	if err != nil {
		return nil, err
	}

	ioctl(fd.Fd(), UI_SET_EVBIT, EV_KEY)
	ioctl(fd.Fd(), UI_SET_KEYBIT, BTN_TOUCH)
	ioctl(fd.Fd(), UI_SET_EVBIT, EV_ABS)

	absBits := []uint16{ABS_MT_SLOT, ABS_MT_POSITION_X, ABS_MT_POSITION_Y, ABS_MT_TRACKING_ID}
	for _, bit := range absBits {
		ioctl(fd.Fd(), UI_SET_ABSBIT, uintptr(bit))
	}

	absSetupX := uinputAbsSetup{
		code: ABS_MT_POSITION_X,
		absinfo: absInfo{
			Maximum: maxX,
		},
	}
	ioctl(fd.Fd(), UI_ABS_SETUP, uintptr(unsafe.Pointer(&absSetupX)))

	absSetupY := uinputAbsSetup{
		code: ABS_MT_POSITION_Y,
		absinfo: absInfo{
			Maximum: maxY,
		},
	}
	ioctl(fd.Fd(), UI_ABS_SETUP, uintptr(unsafe.Pointer(&absSetupY)))

	absSetupSlot := uinputAbsSetup{
		code: ABS_MT_SLOT,
		absinfo: absInfo{
			Maximum: 9, // 10 slots
		},
	}
	ioctl(fd.Fd(), UI_ABS_SETUP, uintptr(unsafe.Pointer(&absSetupSlot)))

	absSetupTrack := uinputAbsSetup{
		code: ABS_MT_TRACKING_ID,
		absinfo: absInfo{
			Maximum: 65535,
		},
	}
	ioctl(fd.Fd(), UI_ABS_SETUP, uintptr(unsafe.Pointer(&absSetupTrack)))

	var setup uinputSetup
	copy(setup.name[:], "SecondScreen Multi-Touch")
	setup.id.Bustype = 0x03 // BUS_USB
	setup.id.Vendor = 0x1234
	setup.id.Product = 0x5678
	setup.id.Version = 1

	if err := ioctl(fd.Fd(), UI_DEV_SETUP, uintptr(unsafe.Pointer(&setup))); err != nil {
		fd.Close()
		return nil, err
	}

	if err := ioctl(fd.Fd(), UI_DEV_CREATE, 0); err != nil {
		fd.Close()
		return nil, err
	}

	return &TouchDevice{fd: fd}, nil
}

func (t *TouchDevice) Close() error {
	ioctl(t.fd.Fd(), UI_DEV_DESTROY, 0)
	return t.fd.Close()
}

func (t *TouchDevice) emit(typ, code uint16, value int32) error {
	ev := inputEvent{
		Type:  typ,
		Code:  code,
		Value: value,
	}
	b := (*[unsafe.Sizeof(ev)]byte)(unsafe.Pointer(&ev))[:]
	_, err := t.fd.Write(b)
	return err
}

func (t *TouchDevice) ServeTCP(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}
		go t.handleConn(conn)
	}
}

func (t *TouchDevice) handleConn(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 10)
	for {
		_, err := io.ReadFull(conn, buf)
		if err != nil {
			return
		}

		action := buf[0]
		pointerID := buf[1]
		xBits := binary.LittleEndian.Uint32(buf[2:6])
		yBits := binary.LittleEndian.Uint32(buf[6:10])

		x := math.Float32frombits(xBits)
		y := math.Float32frombits(yBits)

		t.emit(EV_ABS, ABS_MT_SLOT, int32(pointerID))

		switch action {
		case ActionDown:
			// Type B: ao abrir slot, emite TRACKING_ID + posição inicial
			t.emit(EV_ABS, ABS_MT_TRACKING_ID, int32(pointerID))
			t.emit(EV_ABS, ABS_MT_POSITION_X, int32(x))
			t.emit(EV_ABS, ABS_MT_POSITION_Y, int32(y))
			t.emit(EV_KEY, BTN_TOUCH, 1)
		case ActionMove:
			// Type B: no MOVE, só atualiza posição — não reemite TRACKING_ID
			t.emit(EV_ABS, ABS_MT_POSITION_X, int32(x))
			t.emit(EV_ABS, ABS_MT_POSITION_Y, int32(y))
		case ActionUp, ActionCancel:
			// Libera o slot definindo TRACKING_ID = -1
			t.emit(EV_ABS, ABS_MT_TRACKING_ID, -1)
			t.emit(EV_KEY, BTN_TOUCH, 0)
		}
		t.emit(EV_SYN, SYN_REPORT, 0)
	}
}

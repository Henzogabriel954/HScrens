package adb

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type USBEvent struct {
	Type   string // "plugged", "unplugged", "state_changed"
	Serial string
	State  string // "device", "offline", "unauthorized"
}

func WatchUSB(ctx context.Context, events chan<- USBEvent) error {
	var conn net.Conn
	var err error

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		conn, err = net.Dial("tcp", "127.0.0.1:5037")
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	defer conn.Close()

	req := "0012host:track-devices"
	if _, err := conn.Write([]byte(req)); err != nil {
		return err
	}

	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return err
	}
	if string(buf) != "OKAY" {
		return fmt.Errorf("adb server returned %s", string(buf))
	}

	knownDevices := make(map[string]string)
	missingCounts := make(map[string]int)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if _, err := io.ReadFull(conn, buf); err != nil {
			return err
		}
		length, err := strconv.ParseInt(string(buf), 16, 64)
		if err != nil {
			return err
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return err
		}

		currentDevices := make(map[string]string)
		lines := strings.Split(string(payload), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) == 2 {
				currentDevices[parts[0]] = parts[1]
			}
		}

		// Dispositivo detectado ou alterado
		for serial, state := range currentDevices {
			missingCounts[serial] = 0
			prevState, exists := knownDevices[serial]
			if !exists {
				if state == "device" {
					knownDevices[serial] = state
					events <- USBEvent{Type: "plugged", Serial: serial, State: state}
				}
			} else if prevState != state {
				knownDevices[serial] = state
				events <- USBEvent{Type: "state_changed", Serial: serial, State: state}
			}
		}

		// Debounce de desconexão (deve sumir por 3 updates consecutivos para considerar unplugged)
		for serial, state := range knownDevices {
			if _, exists := currentDevices[serial]; !exists {
				missingCounts[serial]++
				if missingCounts[serial] >= 3 {
					delete(knownDevices, serial)
					delete(missingCounts, serial)
					events <- USBEvent{Type: "unplugged", Serial: serial, State: state}
				}
			}
		}
	}
}

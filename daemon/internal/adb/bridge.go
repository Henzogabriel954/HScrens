package adb

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

func runCommandWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Run()
}

// WakeUp wakes up the screen of the given device
func WakeUp(ctx context.Context, serial string) error {
	return runCommandWithTimeout(ctx, 5*time.Second, "adb", "-s", serial, "shell", "input", "keyevent", "KEYCODE_WAKEUP")
}

// ReversePorts sets up adb reverse so the Android device can reach host ports
func ReversePorts(ctx context.Context, serial string, videoPort, touchPort, controlPort int) error {
	err := runCommandWithTimeout(ctx, 5*time.Second, "adb", "-s", serial, "reverse", "tcp:5000", fmt.Sprintf("tcp:%d", videoPort))
	if err != nil {
		return err
	}
	err = runCommandWithTimeout(ctx, 5*time.Second, "adb", "-s", serial, "reverse", "tcp:5001", fmt.Sprintf("tcp:%d", touchPort))
	if err != nil {
		return err
	}
	return runCommandWithTimeout(ctx, 5*time.Second, "adb", "-s", serial, "reverse", "tcp:5100", fmt.Sprintf("tcp:%d", controlPort))
}

// StartApp starts the main activity of the second screen app
func StartApp(ctx context.Context, serial string) error {
	return runCommandWithTimeout(ctx, 5*time.Second, "adb", "-s", serial, "shell", "am", "start", "-n", "online.hcraft.hscrens/.MainActivity")
}

// RemoveAllForwards cleans up reverse forwards for the given device
func RemoveAllForwards(ctx context.Context, serial string) error {
	return runCommandWithTimeout(ctx, 5*time.Second, "adb", "-s", serial, "reverse", "--remove-all")
}

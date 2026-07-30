package encode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// StartTestEncode mounts and starts a GStreamer pipeline using filesrc for testing.
func StartTestEncode(ctx context.Context, videoPort int, testVideoPath string, bitrate int) *exec.Cmd {
	pipeline := fmt.Sprintf(
		"gst-launch-1.0 filesrc location=\"%s\" ! "+
			"decodebin ! "+
			"videoscale ! video/x-raw,width=1080,height=2340 ! "+
			"videoconvert ! video/x-raw,format=I420 ! "+
			"x264enc tune=zerolatency speed-preset=ultrafast key-int-max=30 bitrate=%d sliced-threads=false ! "+
			"h264parse config-interval=1 ! "+
			"video/x-h264,stream-format=avc,alignment=au ! "+
			"tcpserversink host=127.0.0.1 port=%d sync=true async=false",
		testVideoPath, bitrate, videoPort,
	)

	cmd := exec.CommandContext(ctx, "sh", "-c", pipeline)
	return cmd
}

// StartPipeWireEncode mounts and starts a GStreamer pipeline using pipewiresrc.
func StartPipeWireEncode(ctx context.Context, videoPort int, pipewireFD int, nodeID uint32, bitrate int) *exec.Cmd {
	pipeline := fmt.Sprintf(
		"gst-launch-1.0 pipewiresrc fd=3 path=%d ! "+
			"videoconvert ! video/x-raw,format=I420 ! "+
			"x264enc tune=zerolatency speed-preset=ultrafast key-int-max=30 bitrate=%d sliced-threads=false ! "+
			"h264parse config-interval=1 ! "+
			"video/x-h264,stream-format=avc,alignment=au ! "+
			"tcpserversink host=127.0.0.1 port=%d sync=false async=false",
		nodeID, bitrate, videoPort,
	)

	cmd := exec.CommandContext(ctx, "sh", "-c", pipeline)
	cmd.ExtraFiles = []*os.File{os.NewFile(uintptr(pipewireFD), "pipewire")}
	return cmd
}

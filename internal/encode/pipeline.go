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
			"queue max-size-buffers=1 leaky=downstream ! "+
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
			"queue max-size-buffers=1 leaky=downstream ! "+
			"videorate drop-only=true ! video/x-raw,max-framerate=30/1 ! "+
			"videoconvert ! video/x-raw,format=I420 ! "+
			"x264enc tune=zerolatency speed-preset=superfast key-int-max=30 bitrate=%d sliced-threads=false ! "+
			"h264parse config-interval=1 ! "+
			"video/x-h264,stream-format=avc,alignment=au ! "+
			"tcpserversink host=127.0.0.1 port=%d sync=false async=false",
		nodeID, bitrate, videoPort,
	)

	cmd := exec.CommandContext(ctx, "sh", "-c", pipeline)
	cmd.ExtraFiles = []*os.File{os.NewFile(uintptr(pipewireFD), "pipewire")}
	return cmd
}

// StartDiagPipeWireEncode runs the normal pipeline with identity probes for timestamp analysis.
// probe_src: right after pipewiresrc (before queue) — shows if frames arrive regularly from PipeWire.
// probe_enc: right after x264enc (before h264parse) — shows encoder output timing.
// Compare the two to see where delay is introduced.
func StartDiagPipeWireEncode(ctx context.Context, videoPort int, pipewireFD int, nodeID uint32, bitrate int) *exec.Cmd {
	pipeline := fmt.Sprintf(
		"gst-launch-1.0 -v pipewiresrc fd=3 path=%d ! "+
			"identity name=probe_src silent=false ! "+
			"queue max-size-buffers=1 leaky=downstream ! "+
			"videorate drop-only=true ! video/x-raw,max-framerate=30/1 ! "+
			"videoconvert ! video/x-raw,format=I420 ! "+
			"x264enc tune=zerolatency speed-preset=superfast key-int-max=30 bitrate=%d sliced-threads=false ! "+
			"identity name=probe_enc silent=false ! "+
			"h264parse config-interval=1 ! "+
			"video/x-h264,stream-format=avc,alignment=au ! "+
			"tcpserversink host=127.0.0.1 port=%d sync=false async=false",
		nodeID, bitrate, videoPort,
	)

	cmd := exec.CommandContext(ctx, "sh", "-c", pipeline)
	cmd.ExtraFiles = []*os.File{os.NewFile(uintptr(pipewireFD), "pipewire")}
	return cmd
}

// StartVisualPipeWireEncode runs the full encode pipeline but displays locally via fpsdisplaysink
// instead of sending over TCP. This isolates the encoder from the network/ADB path.
// If THIS stutters → encoder is the bottleneck.
// If THIS is smooth → problem is in the network/ADB/Android path.
func StartVisualPipeWireEncode(ctx context.Context, pipewireFD int, nodeID uint32, bitrate int) *exec.Cmd {
	pipeline := fmt.Sprintf(
		"gst-launch-1.0 -v pipewiresrc fd=3 path=%d ! "+
			"queue max-size-buffers=1 leaky=downstream ! "+
			"videorate drop-only=true ! video/x-raw,max-framerate=30/1 ! "+
			"videoconvert ! video/x-raw,format=I420 ! "+
			"x264enc tune=zerolatency speed-preset=superfast key-int-max=30 bitrate=%d sliced-threads=false ! "+
			"h264parse config-interval=1 ! "+
			"avdec_h264 ! videoconvert ! autovideosink sync=false",
		nodeID, bitrate,
	)

	cmd := exec.CommandContext(ctx, "sh", "-c", pipeline)
	cmd.ExtraFiles = []*os.File{os.NewFile(uintptr(pipewireFD), "pipewire")}
	return cmd
}

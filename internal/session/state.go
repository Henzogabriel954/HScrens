package session

import (
	"context"
	"os/exec"
)

type SessionState int

const (
	StateIdle SessionState = iota
	StateDetected          // visto no track-devices, ainda sem forward
	StateAwaitingSelection // múltiplos dispositivos, aguardando escolha do usuário via Qt
	StateForwarding        // adb forward feito, app sendo lançado
	StateHandshaking       // aguardando handshake no canal de controle
	StateStreaming         // vídeo e toque ativos
	StateError
)

func (s SessionState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateDetected:
		return "detected"
	case StateAwaitingSelection:
		return "awaiting_selection"
	case StateForwarding:
		return "forwarding"
	case StateHandshaking:
		return "handshaking"
	case StateStreaming:
		return "streaming"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

type DeviceProfile struct {
	FriendlyName string `json:"friendly_name"`
	Orientation  string `json:"orientation"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	BitrateKbps  int    `json:"bitrate_kbps"`
	Position     string `json:"position"`
	Encoder      string `json:"encoder"`
	LastSeen     string `json:"last_seen"`
}

type DeviceSession struct {
	Serial        string // serial ADB, ex: "R58M912ABCD"
	DeviceID      string // ANDROID_ID vindo do handshake
	FriendlyName  string // "Samsung SM-S911B" — vem do handshake ou do profile salvo
	State         SessionState
	VideoPort     int // porta TCP local para vídeo
	TouchPort     int // porta TCP local para toque
	ControlPort   int // porta TCP local para canal de controle
	VirtualOutput string // nome do output KDE, ex: "VIRTUAL-1"
	Profile       DeviceProfile
	EncoderProc   *exec.Cmd // subprocesso gstreamer/ffmpeg ativo
	UinputFD      int       // file descriptor do device uinput criado
	CancelFunc    context.CancelFunc
}

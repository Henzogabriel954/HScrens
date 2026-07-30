package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"secondscreen-daemon/internal/capture"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; cancel() }()

	configDir := os.ExpandEnv("$HOME/.config/secondscreen")

	log.Println("🔌 Fazendo handshake com o portal D-Bus (xdg-desktop-portal-kde)...")
	log.Println("   → Uma janela de permissão vai aparecer — selecione a tela que quer capturar")
	log.Println("   → Na 1ª vez isso é esperado. Das próximas, vai ser automático (restore token)")

	stream, err := capture.RequestPortalStream(ctx, configDir)
	if err != nil {
		log.Fatalf("❌ Portal falhou: %v", err)
	}

	log.Printf("✅ Portal OK! fd=%d  node_id=%d", stream.PipewireFD, stream.NodeID)
	log.Println("🎥 Iniciando pipeline GStreamer com preview na tela...")

	// Monta o pipeline com os parâmetros reais do portal
	// Usa waylandsink que funciona nativamente em Wayland/KDE
	pipeline := fmt.Sprintf(
		"gst-launch-1.0 pipewiresrc fd=%d path=%d ! videoconvert ! waylandsink",
		stream.PipewireFD, stream.NodeID,
	)

	log.Printf("Pipeline: %s", pipeline)

	cmd := exec.CommandContext(ctx, "sh", "-c", pipeline)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Tenta com glimagesink se waylandsink não estiver disponível
		log.Printf("⚠️  waylandsink falhou (%v), tentando glimagesink...", err)
		pipeline2 := fmt.Sprintf(
			"gst-launch-1.0 pipewiresrc fd=%d path=%d ! videoconvert ! glimagesink",
			stream.PipewireFD, stream.NodeID,
		)
		cmd2 := exec.CommandContext(ctx, "sh", "-c", pipeline2)
		cmd2.Stdout = os.Stdout
		cmd2.Stderr = os.Stderr
		if err2 := cmd2.Run(); err2 != nil {
			log.Fatalf("❌ GStreamer falhou: %v", err2)
		}
	}
}

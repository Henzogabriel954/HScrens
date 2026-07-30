package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"secondscreen-daemon/internal/adb"
	"secondscreen-daemon/internal/capture"
	"secondscreen-daemon/internal/control"
	"secondscreen-daemon/internal/encode"
	"secondscreen-daemon/internal/ipc"
	"secondscreen-daemon/internal/kscreen"
	"secondscreen-daemon/internal/profile"
	"secondscreen-daemon/internal/session"
	"secondscreen-daemon/internal/touch"
)

const testMode = false
const testVideoPath = "/home/henzogabriel954/Videos/0726-01.mp4"

var activePortalStream *capture.PortalStream

func main() {
	log.Println("Starting SecondScreen Daemon...")

	_, err := profile.NewStore()
	if err != nil {
		log.Printf("Failed to init profile store: %v", err)
	}

	sessManager := session.NewManager(6000)

	ipcHandler := func(cmd map[string]interface{}, conn net.Conn) {
		action, _ := cmd["action"].(string)
		log.Printf("IPC Command received: %s", action)

		// Basic handling
		if action == "ENABLE_VIRTUAL" {
			kscreen.EnableVirtualOutput(1080, 2400, "1920,0")
			conn.Write([]byte("{\"type\":\"response\",\"ok\":true}\n"))
		} else if action == "DISABLE_VIRTUAL" {
			kscreen.DisableVirtualOutput()
			conn.Write([]byte("{\"type\":\"response\",\"ok\":true}\n"))
		} else {
			conn.Write([]byte("{\"type\":\"response\",\"ok\":false,\"error\":\"unknown action\"}\n"))
		}
	}

	ipcServer := ipc.NewServer(ipcHandler)
	if err := ipcServer.Start(); err != nil {
		log.Fatalf("Failed to start IPC server: %v", err)
	}
	defer ipcServer.Stop()
	log.Println("IPC Server started")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan adb.USBEvent, 10)
	go func() {
		if err := adb.WatchUSB(ctx, events); err != nil {
			log.Printf("USB Watcher stopped: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case evt := <-events:
			log.Printf("USB Event: %+v", evt)
			switch evt.Type {
			case "plugged":
				log.Printf("🚀 Dispositivo detectado: %s", evt.Serial)

				adb.WakeUp(ctx, evt.Serial)

				if out, err := kscreen.EnableVirtualOutput(1080, 2400, "1920,0"); err != nil {
					log.Printf("⚠️ kscreen error: %v | %s", err, out)
				} else {
					log.Println("✅ Monitor virtual VIRTUAL-1 habilitado")
				}

				videoPort, touchPort, controlPort := sessManager.AllocatePorts()

				t, touchErr := touch.NewTouchDevice(1080, 2400)
				if touchErr == nil {
					go t.ServeTCP(fmt.Sprintf("127.0.0.1:%d", touchPort))
					log.Printf("✅ Touch listener pronto na :%d", touchPort)
				} else {
					log.Printf("⚠️ Touch error (rode newgrp input?): %v", touchErr)
				}

				controlReady := make(chan struct{})
				go func() {
					l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", controlPort))
					if err != nil {
						log.Printf("❌ Não conseguiu escutar na %d: %v", controlPort, err)
						close(controlReady)
						return
					}
					log.Printf("✅ Control listener pronto na :%d", controlPort)
					close(controlReady)
					defer l.Close()
					for {
						conn, err := l.Accept()
						if err != nil {
							log.Printf("❌ Control accept err: %v", err)
							break
						}
						log.Printf("🔗 Control connection accepted from %v", conn.RemoteAddr())
						
						go func(c net.Conn) {
							ch := control.NewChannel(c)
							defer ch.Close()
							
							msg := control.Message{
								Type:     "handshake_ack",
								Accepted: true,
							}
							if err := ch.Send(msg); err != nil {
								log.Printf("❌ Failed to send handshake_ack: %v", err)
							} else {
								log.Println("📤 Handshake ACK sent to Android")
							}
							
							for {
								inMsg, err := ch.Receive()
								if err != nil {
									log.Printf("📥 Control connection closed by client: %v", err)
									break
								}
								log.Printf("📥 Control msg received: %+v", inMsg)
							}
						}(conn)
					}
				}()

				<-controlReady

				if testMode {
					cmd := encode.StartTestEncode(ctx, videoPort, testVideoPath, 8000)
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					if err := cmd.Start(); err != nil {
						log.Printf("❌ GStreamer error: %v", err)
					} else {
						log.Printf("🎥 GStreamer TEST MODE iniciado na porta %d", videoPort)
					}
				} else {
					if activePortalStream == nil {
						log.Println("🔌 Iniciando handshake com o portal D-Bus...")
						log.Println("   → Uma janela do KDE vai aparecer — selecione o monitor VIRTUAL-1")
						configDir := filepath.Join(os.Getenv("HOME"), ".config", "secondscreen")
	
						portalStream, portalErr := capture.RequestPortalStream(ctx, configDir)
						if portalErr != nil {
							log.Printf("❌ Portal D-Bus error: %v", portalErr)
						} else {
							activePortalStream = portalStream
							cmd := encode.StartPipeWireEncode(ctx, videoPort, activePortalStream.PipewireFD, activePortalStream.NodeID, 8000)
							cmd.Stdout = os.Stdout
							cmd.Stderr = os.Stderr
							if err := cmd.Start(); err != nil {
								log.Printf("❌ GStreamer error: %v", err)
							} else {
								log.Printf("🎥 GStreamer iniciado! fd=%d node_id=%d", activePortalStream.PipewireFD, activePortalStream.NodeID)
							}
						}
					}
				}

				if err := adb.ReversePorts(ctx, evt.Serial, videoPort, touchPort, controlPort); err != nil {
					log.Printf("❌ ADB Reverse error: %v", err)
				} else {
					log.Printf("✅ ADB Reverse feito! (v:%d t:%d c:%d)", videoPort, touchPort, controlPort)
				}

				if err := adb.StartApp(ctx, evt.Serial); err != nil {
					log.Printf("❌ am start error: %v", err)
				} else {
					log.Println("✅ App Android iniciado!")
				}

			case "unplugged":
				if activePortalStream != nil {
					activePortalStream.Close()
					activePortalStream = nil
				}
				adb.RemoveAllForwards(ctx, evt.Serial)
				kscreen.DisableVirtualOutput()
				log.Printf("🛑 Disconnected %s", evt.Serial)
			}
		case <-sigCh:
			log.Println("Shutting down...")
			if activePortalStream != nil {
				activePortalStream.Close()
				activePortalStream = nil
			}
			kscreen.DisableVirtualOutput()
			return
		}
	}
}

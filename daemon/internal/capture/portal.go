// Package capture implementa o handshake com o portal xdg-desktop-portal-kde
// para obter um fd PipeWire e node_id válidos para capturar o monitor virtual.
package capture

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/godbus/dbus/v5"
)

const (
	portalDest      = "org.freedesktop.portal.Desktop"
	portalPath      = "/org/freedesktop/portal/desktop"
	screenCastIface = "org.freedesktop.portal.ScreenCast"
	requestIface    = "org.freedesktop.portal.Request"
	tokenFile       = "portal_restore_token.txt"
)

// PortalStream é o resultado de um handshake bem-sucedido com o portal.
type PortalStream struct {
	PipewireFD int        // fd Unix passado para pipewiresrc fd=
	NodeID     uint32     // node_id passado para pipewiresrc path=
	DBusConn   *dbus.Conn // Conexão D-Bus mantida viva para evitar cancelamento da sessão pelo portal (timeout de 30s)
}

// Close encerra a conexão D-Bus associada à sessão ScreenCast.
func (ps *PortalStream) Close() {
	if ps.DBusConn != nil {
		ps.DBusConn.Close()
	}
}

// RequestPortalStream faz o handshake completo com o portal xdg-desktop-portal-kde
// e retorna fd e node_id para o GStreamer.
func RequestPortalStream(ctx context.Context, configDir string) (*PortalStream, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("dbus session bus: %w", err)
	}

	obj := conn.Object(portalDest, portalPath)

	// Registra todos os sinais do portal para não perder nenhum Response
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface(requestIface),
		dbus.WithMatchMember("Response"),
	); err != nil {
		conn.Close()
		return nil, fmt.Errorf("add match signal: %w", err)
	}
	sigCh := make(chan *dbus.Signal, 8)
	conn.Signal(sigCh)

	senderToken := safeName(conn.Names()[0])
	restoreToken := readRestoreToken(configDir)

	// ─── 1. CreateSession ─────────────────────────────────────────────────────
	tok1 := randToken()
	var createReqPath dbus.ObjectPath
	err = obj.CallWithContext(ctx, screenCastIface+".CreateSession", 0, map[string]dbus.Variant{
		"handle_token":         dbus.MakeVariant(tok1),
		"session_handle_token": dbus.MakeVariant(randToken()),
	}).Store(&createReqPath)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("CreateSession call: %w", err)
	}

	expectedReqPath := buildRequestPath(senderToken, tok1)
	res1, err := waitResponse(ctx, sigCh, expectedReqPath, createReqPath)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("CreateSession response: %w", err)
	}

	sessionHandleVar, ok := res1["session_handle"]
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("CreateSession: session_handle ausente na resposta")
	}
	sessionHandle := dbus.ObjectPath(sessionHandleVar.Value().(string))
	log.Printf("[portal] session_handle: %s", sessionHandle)

	// ─── 2. SelectSources ─────────────────────────────────────────────────────
	tok2 := randToken()
	selectOpts := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(tok2),
		"types":        dbus.MakeVariant(uint32(1)), // 1 = MONITOR
		"multiple":     dbus.MakeVariant(false),
		"persist_mode": dbus.MakeVariant(uint32(0)), // 0 = nao persistir
	}
	if restoreToken != "" {
		// Ignorando token salvo a pedido do usuario para forçar dialogo
		log.Printf("[portal] restore_token ignorado (persist_mode=0)")
	}

	var selectReqPath dbus.ObjectPath
	if err := obj.CallWithContext(ctx, screenCastIface+".SelectSources", 0,
		sessionHandle, selectOpts).Store(&selectReqPath); err != nil {
		conn.Close()
		return nil, fmt.Errorf("SelectSources call: %w", err)
	}
	if _, err := waitResponse(ctx, sigCh, buildRequestPath(senderToken, tok2), selectReqPath); err != nil {
		conn.Close()
		return nil, fmt.Errorf("SelectSources response: %w", err)
	}

	// ─── 3. Start ─────────────────────────────────────────────────────────────
	tok3 := randToken()
	var startReqPath dbus.ObjectPath
	if err := obj.CallWithContext(ctx, screenCastIface+".Start", 0,
		sessionHandle, "", map[string]dbus.Variant{
			"handle_token": dbus.MakeVariant(tok3),
		}).Store(&startReqPath); err != nil {
		conn.Close()
		return nil, fmt.Errorf("Start call: %w", err)
	}

	log.Println("[portal] aguardando confirmação do usuário (diálogo do KDE)...")
	res3, err := waitResponse(ctx, sigCh, buildRequestPath(senderToken, tok3), startReqPath)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("Start response: %w", err)
	}

	nodeID, err := extractNodeID(res3)
	if err != nil {
		conn.Close()
		return nil, err
	}
	log.Printf("[portal] node_id=%d", nodeID)

	if tv, ok := res3["restore_token"]; ok {
		if t, ok := tv.Value().(string); ok && t != "" {
			saveRestoreToken(configDir, t)
			log.Println("[portal] restore_token salvo")
		}
	}

	// ─── 4. OpenPipeWireRemote ─────────────────────────────────────────────────
	var fd dbus.UnixFD
	if err := obj.CallWithContext(ctx, screenCastIface+".OpenPipeWireRemote", 0,
		sessionHandle, map[string]dbus.Variant{}).Store(&fd); err != nil {
		conn.Close()
		return nil, fmt.Errorf("OpenPipeWireRemote: %w", err)
	}

	log.Printf("[portal] ✅ fd=%d node_id=%d (conexão DBus mantida aberta)", int(fd), nodeID)
	return &PortalStream{PipewireFD: int(fd), NodeID: nodeID, DBusConn: conn}, nil
}

func waitResponse(ctx context.Context, ch <-chan *dbus.Signal, expected, actual dbus.ObjectPath) (map[string]dbus.Variant, error) {
	for {
		select {
		case sig := <-ch:
			if sig.Name != requestIface+".Response" {
				continue
			}
			if sig.Path != expected && sig.Path != actual {
				continue
			}
			if len(sig.Body) < 2 {
				return nil, fmt.Errorf("Response com body inválido: %v", sig.Body)
			}
			code, _ := sig.Body[0].(uint32)
			if code != 0 {
				return nil, fmt.Errorf("portal recusou (code=%d): 1=cancelado, 2=outro", code)
			}
			results, _ := sig.Body[1].(map[string]dbus.Variant)
			return results, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func extractNodeID(results map[string]dbus.Variant) (uint32, error) {
	sv, ok := results["streams"]
	if !ok {
		return 0, fmt.Errorf("campo 'streams' ausente na resposta do Start")
	}

	switch v := sv.Value().(type) {
	case [][]interface{}:
		if len(v) > 0 {
			if id, ok := v[0][0].(uint32); ok {
				return id, nil
			}
		}
	case []struct {
		NodeID uint32
		Opts   map[string]dbus.Variant
	}:
		if len(v) > 0 {
			return v[0].NodeID, nil
		}
	}

	log.Printf("[portal] streams tipo=%T valor=%v", sv.Value(), sv.Value())
	return 0, fmt.Errorf("não foi possível extrair node_id de streams (tipo=%T)", sv.Value())
}

func buildRequestPath(sender, token string) dbus.ObjectPath {
	return dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", sender, token))
}

func safeName(busName string) string {
	out := ""
	for _, c := range busName {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			out += string(c)
		default:
			out += "_"
		}
	}
	return out
}

func randToken() string {
	return fmt.Sprintf("ss_%d", rand.Uint32())
}

func readRestoreToken(configDir string) string {
	data, err := os.ReadFile(filepath.Join(configDir, tokenFile))
	if err != nil {
		return ""
	}
	return string(data)
}

func saveRestoreToken(configDir, token string) {
	os.MkdirAll(configDir, 0700)
	os.WriteFile(filepath.Join(configDir, tokenFile), []byte(token), 0600)
}

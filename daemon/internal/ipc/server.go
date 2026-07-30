package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
)

type Server struct {
	mu         sync.Mutex
	listener   net.Listener
	clients    map[net.Conn]bool
	socketPath string
	handler    func(cmd map[string]interface{}, conn net.Conn)
}

func NewServer(handler func(cmd map[string]interface{}, conn net.Conn)) *Server {
	return &Server{
		clients: make(map[net.Conn]bool),
		handler: handler,
	}
}

func (s *Server) Start() error {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/tmp/secondscreen-%d", os.Getuid())
	} else {
		runtimeDir = filepath.Join(runtimeDir, "secondscreen")
	}
	
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		return err
	}

	s.socketPath = filepath.Join(runtimeDir, "control.sock")
	os.Remove(s.socketPath) // Clean up old socket

	l, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}
	
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		l.Close()
		return err
	}

	s.listener = l

	go s.acceptLoop()
	return nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		
		s.mu.Lock()
		s.clients[conn] = true
		s.mu.Unlock()

		go s.handleClient(conn)
	}
}

func (s *Server) handleClient(conn net.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		var cmd map[string]interface{}
		if err := json.Unmarshal(line, &cmd); err != nil {
			continue
		}
		if s.handler != nil {
			s.handler(cmd, conn)
		}
	}
}

func (s *Server) SendResponse(conn net.Conn, id string, ok bool, extra map[string]interface{}) {
	msg := map[string]interface{}{
		"type": "response",
		"id":   id,
		"ok":   ok,
	}
	for k, v := range extra {
		msg[k] = v
	}
	b, _ := json.Marshal(msg)
	b = append(b, '\n')
	conn.Write(b)
}

func (s *Server) BroadcastEvent(eventName string, payload map[string]interface{}) {
	msg := map[string]interface{}{
		"type":  "event",
		"event": eventName,
	}
	for k, v := range payload {
		msg[k] = v
	}
	b, _ := json.Marshal(msg)
	b = append(b, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	for conn := range s.clients {
		conn.Write(b)
	}
}

func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
}

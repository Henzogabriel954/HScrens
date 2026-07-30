package control

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
)

// Message representa qualquer mensagem trocada no canal de controle (NDJSON).
type Message struct {
	Type         string `json:"type"`
	DeviceID     string `json:"device_id,omitempty"`
	Model        string `json:"model,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	NativeWidth  int    `json:"native_width,omitempty"`
	NativeHeight int    `json:"native_height,omitempty"`
	DensityDPI   int    `json:"density_dpi,omitempty"`
	AppVersion   int    `json:"app_version,omitempty"`
	Accepted     bool   `json:"accepted,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Value        string `json:"value,omitempty"`
	Kbps         int    `json:"kbps,omitempty"`
	Timestamp    int64  `json:"ts,omitempty"`
}

// Channel representa um canal de controle TCP bidirecional com o Android.
type Channel struct {
	mu     sync.Mutex
	conn   net.Conn
	writer *json.Encoder
}

// NewChannel cria um Channel a partir de uma conexão TCP já estabelecida.
func NewChannel(conn net.Conn) *Channel {
	return &Channel{
		conn:   conn,
		writer: json.NewEncoder(conn),
	}
}

// Send envia uma mensagem JSON para o Android.
func (c *Channel) Send(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writer.Encode(msg)
}

// Receive lê a próxima mensagem JSON do Android (bloqueia).
func (c *Channel) Receive() (*Message, error) {
	var msg Message
	scanner := bufio.NewScanner(c.conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, net.ErrClosed
	}
	if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// Close fecha a conexão.
func (c *Channel) Close() error {
	return c.conn.Close()
}

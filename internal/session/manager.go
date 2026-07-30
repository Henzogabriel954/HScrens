package session

import (
	"sync"
)

type Manager struct {
	mu            sync.RWMutex
	sessions      map[string]*DeviceSession // keyed by serial
	basePort      int
	nextPortIndex int
}

func NewManager(basePort int) *Manager {
	return &Manager{
		sessions: make(map[string]*DeviceSession),
		basePort: basePort,
	}
}

func (m *Manager) AllocatePorts() (videoPort, touchPort, controlPort int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	videoPort = m.basePort + m.nextPortIndex*3
	touchPort = m.basePort + m.nextPortIndex*3 + 1
	controlPort = m.basePort + m.nextPortIndex*3 + 2
	
	// simple allocation, should reuse ports if needed later
	m.nextPortIndex++
	return
}

func (m *Manager) GetSession(serial string) (*DeviceSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[serial]
	return session, ok
}

func (m *Manager) AddSession(session *DeviceSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.Serial] = session
}

func (m *Manager) RemoveSession(serial string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, serial)
}

func (m *Manager) GetAllSessions() []*DeviceSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sessions := make([]*DeviceSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

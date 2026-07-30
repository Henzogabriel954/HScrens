package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	
	"secondscreen-daemon/internal/session"
)

type Store struct {
	mu       sync.RWMutex
	filePath string
	profiles map[string]session.DeviceProfile
}

func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	
	configDir := filepath.Join(home, ".config", "secondscreen")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}
	
	s := &Store{
		filePath: filepath.Join(configDir, "devices.json"),
		profiles: make(map[string]session.DeviceProfile),
	}
	
	s.load()
	return s, nil
}

func (s *Store) load() {
	b, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	json.Unmarshal(b, &s.profiles)
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.profiles, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, b, 0600)
}

func (s *Store) GetProfile(deviceID string) (session.DeviceProfile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.profiles[deviceID]
	return p, ok
}

func (s *Store) SaveProfile(deviceID string, profile session.DeviceProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles[deviceID] = profile
	return s.save()
}

package credentialvault

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// MemoryBackend is an in-memory VaultBackend for testing.
type MemoryBackend struct {
	mu      sync.RWMutex
	secrets map[string]memorySecret
}

type memorySecret struct {
	value   []byte
	version int
}

// NewMemoryBackend returns a new in-memory vault backend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		secrets: make(map[string]memorySecret),
	}
}

func (m *MemoryBackend) Store(_ context.Context, path string, value []byte) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.secrets[path]; exists {
		return "", "", fmt.Errorf("secret already exists: %s", path)
	}
	m.secrets[path] = memorySecret{value: copyBytes(value), version: 1}
	return path, "v1", nil
}

func (m *MemoryBackend) Get(_ context.Context, path string) ([]byte, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.secrets[path]
	if !ok {
		return nil, "", fmt.Errorf("secret not found: %s", path)
	}
	return copyBytes(s.value), fmt.Sprintf("v%d", s.version), nil
}

func (m *MemoryBackend) Rotate(_ context.Context, path string, newValue []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.secrets[path]
	if !ok {
		return "", fmt.Errorf("secret not found: %s", path)
	}
	s.value = copyBytes(newValue)
	s.version++
	m.secrets[path] = s
	return fmt.Sprintf("v%d", s.version), nil
}

func (m *MemoryBackend) Delete(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.secrets[path]; !ok {
		return fmt.Errorf("secret not found: %s", path)
	}
	delete(m.secrets, path)
	return nil
}

func (m *MemoryBackend) List(_ context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var paths []string
	for k := range m.secrets {
		if strings.HasPrefix(k, prefix) {
			paths = append(paths, k)
		}
	}
	return paths, nil
}

func copyBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

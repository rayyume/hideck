package volte

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var ErrSnapshotNotFound = errors.New("volte provision: snapshot not found")

type SnapshotStore interface {
	Save(snap Snapshot) (path string, err error)
	Load(imei string) (Snapshot, string, error)
	PathFor(imei string) (string, error)
}

type FileStore struct {
	Dir string
	mu  sync.Mutex
}

func (s *FileStore) Save(snap Snapshot) (string, error) {
	if s == nil {
		return "", errors.New("volte provision: snapshot store is not configured")
	}
	if strings.TrimSpace(snap.IMEI) == "" {
		return "", errors.New("volte provision: snapshot IMEI is required")
	}
	if snap.Schema == "" {
		snap.Schema = SnapshotSchema
	}
	path, err := s.PathFor(snap.IMEI)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}

func (s *FileStore) Load(imei string) (Snapshot, string, error) {
	if s == nil {
		return Snapshot{}, "", errors.New("volte provision: snapshot store is not configured")
	}
	path, err := s.PathFor(imei)
	if err != nil {
		return Snapshot{}, "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{}, path, fmt.Errorf("%w", ErrSnapshotNotFound)
		}
		return Snapshot{}, path, err
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return Snapshot{}, path, err
	}
	if snap.Schema != "" && snap.Schema != SnapshotSchema {
		return Snapshot{}, path, fmt.Errorf("volte provision: unsupported snapshot schema %q", snap.Schema)
	}
	return snap, path, nil
}

func (s *FileStore) PathFor(imei string) (string, error) {
	if s == nil {
		return "", errors.New("volte provision: snapshot store is not configured")
	}
	dir := strings.TrimSpace(s.Dir)
	if dir == "" {
		dir = DefaultBackupDir
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(imei)))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json"), nil
}

type MemoryStore struct {
	mu    sync.Mutex
	items map[string]Snapshot
}

func (m *MemoryStore) Save(snap Snapshot) (string, error) {
	if m.items == nil {
		m.items = map[string]Snapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[strings.TrimSpace(snap.IMEI)] = snap
	return "memory:" + IMEITail(snap.IMEI), nil
}

func (m *MemoryStore) Load(imei string) (Snapshot, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap, ok := m.items[strings.TrimSpace(imei)]
	path := "memory:" + IMEITail(imei)
	if !ok {
		return Snapshot{}, path, ErrSnapshotNotFound
	}
	return snap, path, nil
}

func (m *MemoryStore) PathFor(imei string) (string, error) {
	return "memory:" + IMEITail(imei), nil
}

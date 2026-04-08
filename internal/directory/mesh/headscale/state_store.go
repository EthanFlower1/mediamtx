package headscale

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bluenviron/mediamtx/internal/shared/cryptostore"
)

// stateStore abstracts the on-disk coordinator state so the stub
// backend can be constructed with a nil store in TestMode and with a
// cryptostore-backed store in real mode.
type stateStore interface {
	// load reads and decrypts the state file. The second return is
	// false when the file does not yet exist — a first-boot bootstrap
	// should follow by calling save with the initial state.
	load() (persistedState, bool, error)
	// save encrypts and atomically replaces the state file.
	save(ps persistedState) error
}

// fileStateStore persists the stub backend's state as a single
// encrypted blob under StateDir. Atomicity is achieved via
// write-to-temp + rename. Permissions are locked to 0600.
type fileStateStore struct {
	path  string
	store cryptostore.Cryptostore
}

// cryptostoreInfoCoordinator is the HKDF info string that domains
// the coordinator's subkey. It is a package-local constant (not added
// to cryptostore's Info* list) so the cryptostore package stays
// unaware of mesh internals.
const cryptostoreInfoCoordinator = "directory-mesh-headscale"

// stateFileName is the filename relative to Config.StateDir.
const stateFileName = "coordinator.state.enc"

// openEncryptedStateStore validates cfg.StateDir, derives a subkey
// from the master key, and returns a cryptostore-backed stateStore.
// It does not create or read the state file itself — that happens on
// the first call to load/save.
func openEncryptedStateStore(cfg Config) (stateStore, error) {
	if len(cfg.MasterKey) == 0 {
		return nil, ErrMissingMasterKey
	}
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", cfg.StateDir, err)
	}
	cs, err := cryptostore.NewFromMaster(cfg.MasterKey, nil, cryptostoreInfoCoordinator)
	if err != nil {
		return nil, fmt.Errorf("derive coordinator subkey: %w", err)
	}
	return &fileStateStore{
		path:  filepath.Join(cfg.StateDir, stateFileName),
		store: cs,
	}, nil
}

func (f *fileStateStore) load() (persistedState, bool, error) {
	raw, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return persistedState{}, false, nil
	}
	if err != nil {
		return persistedState{}, false, fmt.Errorf("read state: %w", err)
	}
	pt, err := f.store.Decrypt(raw)
	if err != nil {
		return persistedState{}, false, fmt.Errorf("decrypt state: %w", err)
	}
	ps, err := decodeState(pt)
	if err != nil {
		return persistedState{}, false, fmt.Errorf("decode state: %w", err)
	}
	return ps, true, nil
}

func (f *fileStateStore) save(ps persistedState) error {
	pt, err := encodeState(ps)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	ct, err := f.store.Encrypt(pt)
	if err != nil {
		return fmt.Errorf("encrypt state: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".coordinator.state.*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup if anything below fails.
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if _, err := tmp.Write(ct); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, f.path); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}

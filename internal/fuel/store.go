package fuel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type fileStore struct {
	path     string
	lock     *os.File
	once     sync.Once
	closeErr error
}

func Open(cfg Config, source, path string, reset bool) (*Computer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	store, err := openFileStore(path)
	if err != nil {
		return nil, err
	}
	c, err := newComputer(cfg, source, store, time.Now(), reset)
	if err != nil {
		return nil, errors.Join(err, store.Close())
	}
	return c, nil
}
func openFileStore(path string) (*fileStore, error) {
	if path == "" {
		return nil, fmt.Errorf("fuel state path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("acquire fuel state lock: %w; stop the other dashboard or verify an old process has exited before removing its lock", err)
	}
	s := &fileStore{path: path, lock: f}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	return s, nil
}
func (s *fileStore) Load() (*State, error) {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	err = errors.Join(err, f.Close())
	if err != nil {
		return nil, err
	}
	if len(data) > 65536 {
		return nil, fmt.Errorf("fuel state exceeds 64KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state State
	if err := decoder.Decode(&state); err != nil {
		return nil, fmt.Errorf("decode fuel state: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("fuel state must contain exactly one JSON object")
	}
	if err := state.validate(); err != nil {
		return nil, err
	}
	return &state, nil
}
func (s *fileStore) Save(state State) (err error) {
	if err := state.validate(); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".fuel-*")
	if err != nil {
		return err
	}
	defer func() {
		if cleanup := os.Remove(f.Name()); !errors.Is(cleanup, os.ErrNotExist) {
			err = errors.Join(err, cleanup)
		}
	}()
	if err := json.NewEncoder(f).Encode(state); err != nil {
		return errors.Join(err, f.Close())
	}
	err = errors.Join(f.Sync(), f.Close())
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}
func (s *fileStore) Close() error {
	s.once.Do(func() {
		own, err := s.lock.Stat()
		s.closeErr = errors.Join(err, s.lock.Close())
		current, err := os.Stat(s.path + ".lock")
		if err != nil {
			s.closeErr = errors.Join(s.closeErr, err)
			return
		}
		if own == nil || !os.SameFile(own, current) {
			s.closeErr = errors.Join(s.closeErr, fmt.Errorf("fuel lock was replaced; refusing to remove it"))
			return
		}
		s.closeErr = errors.Join(s.closeErr, os.Remove(s.path+".lock"))
	})
	return s.closeErr
}

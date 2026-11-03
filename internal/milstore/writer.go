package milstore

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/privacy"
)

type Recorder struct {
	mu       sync.Mutex
	disk     *diskWriter
	lock     *os.File
	queue    chan Entry
	done     chan struct{}
	status   Status
	closed   bool
	err      error
	closeErr error
	once     sync.Once
	sequence uint64
}

func Open(cfg Config) (*Recorder, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := privateDir(cfg.Dir); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(cfg.Dir, lockName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("acquire MIL writer lock: %w", err)
	}
	r := &Recorder{lock: lock, queue: make(chan Entry, queueSize), done: make(chan struct{})}
	r.disk = &diskWriter{cfg: cfg, started: time.Now()}
	if err := r.open(); err != nil {
		return nil, errors.Join(err, r.disk.finishPart(false), r.unlock())
	}
	go r.run()
	return r, nil
}

func (r *Recorder) open() error {
	m, err := readManifest(r.disk.cfg.Dir)
	if errors.Is(err, os.ErrNotExist) {
		if _, statErr := os.Lstat(filepath.Join(r.disk.cfg.Dir, manifestName)); !errors.Is(statErr, os.ErrNotExist) {
			return err
		}
		m = manifest{Version: Version, Source: r.disk.cfg.Source}
	} else if err != nil {
		return err
	}
	if m.Source != r.disk.cfg.Source {
		return fmt.Errorf("mil source mismatch; use separate simulator and live directories")
	}
	if err := rejectOrphans(r.disk.cfg.Dir, m); err != nil {
		return err
	}
	for i := range m.Parts {
		info, err := regular(filepath.Join(r.disk.cfg.Dir, m.Parts[i].Name))
		if err != nil {
			return err
		}
		if info.Size() > r.disk.cfg.MaxBytes {
			return fmt.Errorf("existing MIL part exceeds configured byte limit")
		}
		if !m.Parts[i].Closed {
			m.Parts[i].Aborted = true
		}
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	r.disk.manifest = m
	r.disk.session = fmt.Sprintf("%x", id)
	r.status = Status{State: "recording", Session: r.disk.session, RetentionTruncated: m.Truncated}
	if err := r.disk.newPart(); err != nil {
		return err
	}
	return r.write(Entry{Kind: "session_start", At: time.Now()})
}

func (r *Recorder) Submit(e Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	if r.closed {
		return fmt.Errorf("mil recorder closed")
	}
	if e.Kind == "" || len(e.Kind) > 128 || len(e.Incident) > 512 || len(e.Data) > MaxLine || e.Kind == "session_start" || e.Kind == "session_end" {
		return r.failLocked(fmt.Errorf("invalid or oversized MIL entry"))
	}
	e.Data = append(json.RawMessage(nil), e.Data...)
	select {
	case r.queue <- e:
		return nil
	default:
		return r.failLocked(fmt.Errorf("mil queue overflow; recording stopped"))
	}
}

func (r *Recorder) failLocked(err error) error {
	if r.err == nil {
		r.err = fmt.Errorf("%s", privacy.VIN(err.Error()))
	}
	r.status.State = "failed"
	r.status.Error = r.err.Error()
	return r.err
}

func (r *Recorder) fail(err error) { r.mu.Lock(); defer r.mu.Unlock(); r.failLocked(err) }
func (r *Recorder) Status() Status { r.mu.Lock(); defer r.mu.Unlock(); return r.status }

func (r *Recorder) write(e Entry) error {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	e.At = e.At.UTC()
	r.sequence++
	row := diskEntry{Version: Version, Session: r.disk.session, Source: r.disk.cfg.Source, Sequence: r.sequence, ElapsedNS: time.Since(r.disk.started).Nanoseconds(), Entry: e}
	err := r.disk.write(row)
	r.mu.Lock()
	if err == nil {
		r.status.Rows++
	}
	r.status.RetentionTruncated = r.disk.manifest.Truncated
	r.mu.Unlock()
	return err
}

func (r *Recorder) run() {
	defer close(r.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	failed := false
	for {
		select {
		case e, ok := <-r.queue:
			if !ok {
				r.finish(failed)
				return
			}
			if !failed {
				if err := r.write(e); err != nil {
					r.fail(err)
					failed = true
				}
			}
		case <-ticker.C:
			if !failed {
				if err := r.disk.file.Sync(); err != nil {
					r.fail(err)
					failed = true
				}
			}
		}
	}
}

func (r *Recorder) finish(failed bool) {
	r.mu.Lock()
	cause := r.err
	r.mu.Unlock()
	if !failed {
		footer := map[string]string{"reason": "closed"}
		if cause != nil {
			footer["reason"] = "recording_failed"
			footer["error"] = cause.Error()
		}
		data, err := json.Marshal(footer)
		if err != nil {
			r.fail(err)
			failed = true
		} else if err := r.write(Entry{Kind: "session_end", At: time.Now(), Data: data}); err != nil {
			r.fail(err)
			failed = true
		}
	}
	if err := r.disk.finishPart(!failed); err != nil {
		r.fail(err)
	}
}

func (r *Recorder) Close() error {
	r.once.Do(func() {
		r.mu.Lock()
		r.closed = true
		close(r.queue)
		r.mu.Unlock()
		<-r.done
		unlockErr := r.unlock()
		r.mu.Lock()
		defer r.mu.Unlock()
		r.closeErr = errors.Join(r.err, unlockErr)
		if r.closeErr == nil {
			r.status.State = "closed"
		} else {
			r.failLocked(r.closeErr)
		}
	})
	return r.closeErr
}

func (r *Recorder) unlock() error {
	own, err := r.lock.Stat()
	err = errors.Join(err, r.lock.Close())
	path := filepath.Join(r.disk.cfg.Dir, lockName)
	current, statErr := os.Lstat(path)
	if statErr != nil {
		return errors.Join(err, statErr)
	}
	if own == nil || !os.SameFile(own, current) {
		return errors.Join(err, fmt.Errorf("mil writer lock was replaced"))
	}
	return errors.Join(err, os.Remove(path), syncDir(r.disk.cfg.Dir))
}

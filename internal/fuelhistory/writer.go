package fuelhistory

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
)

type Recorder struct {
	mu          sync.Mutex
	disk        *diskWriter
	lock        *os.File
	queue       chan Entry
	done        chan struct{}
	status      Status
	sequence    uint64
	lastElapsed int64
	closed      bool
	err         error
	closeErr    error
	once        sync.Once
}

func Open(cfg Config) (*Recorder, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.Dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(cfg.Dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("history directory must be a real directory")
	}
	lock, err := os.OpenFile(filepath.Join(cfg.Dir, ".writer.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("acquire history writer lock: %w", err)
	}
	r := &Recorder{lock: lock, queue: make(chan Entry, 256), done: make(chan struct{})}
	r.disk = &diskWriter{cfg: cfg, started: time.Now()}
	if err := r.open(); err != nil {
		return nil, errors.Join(err, r.disk.close(), r.unlock())
	}
	go r.run()
	return r, nil
}
func (r *Recorder) open() error {
	m, err := loadManifest(r.disk.cfg.Dir)
	if err != nil {
		if _, statErr := os.Lstat(filepath.Join(r.disk.cfg.Dir, "manifest.json")); !errors.Is(statErr, os.ErrNotExist) {
			return err
		}
		m = manifest{Version: Version, Source: r.disk.cfg.Source}
	}
	if m.Source != r.disk.cfg.Source {
		return fmt.Errorf("history source mismatch; use a separate directory")
	}
	for i := range m.Parts {
		m.Parts[i].Closed = true
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	r.disk.manifest = m
	r.disk.session = fmt.Sprintf("%x", id)
	r.status = Status{State: "recording", Session: r.disk.session}
	r.sequence = 1
	if err := r.disk.newPart(); err != nil {
		return err
	}
	return r.disk.write(r.entry("session_start", 1, 0, time.Now(), "", nil))
}
func (r *Recorder) entry(kind string, sequence uint64, elapsed int64, at time.Time, reason string, d *fuel.Diagnostic) Entry {
	return Entry{Version: Version, Session: r.disk.session, Source: r.disk.cfg.Source, Sequence: sequence, ElapsedNS: elapsed, At: at.UTC(), Kind: kind, Reason: reason, Sample: d}
}
func (r *Recorder) Submit(d fuel.Diagnostic) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	if r.closed {
		return fmt.Errorf("history recorder closed")
	}
	if d.Snapshot.Source != "" && d.Snapshot.Source != r.disk.cfg.Source {
		return r.failLocked(fmt.Errorf("diagnostic source mismatch"))
	}
	e := r.entry(d.Kind, r.sequence+1, d.ElapsedNS, d.At, d.Reason, &d)
	select {
	case r.queue <- e:
		r.sequence++
		r.lastElapsed = max(r.lastElapsed, d.ElapsedNS)
		return nil
	default:
		return r.failLocked(fmt.Errorf("history queue overflow; recording stopped"))
	}
}
func (r *Recorder) failLocked(err error) error {
	r.err = errors.Join(r.err, err)
	r.status.State = "failed"
	r.status.Error = r.err.Error()
	return r.err
}
func (r *Recorder) fail(err error) { r.mu.Lock(); defer r.mu.Unlock(); r.failLocked(err) }
func (r *Recorder) Status() Status { r.mu.Lock(); defer r.mu.Unlock(); return r.status }
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
			if failed {
				continue
			}
			if err := r.disk.write(e); err != nil {
				r.fail(err)
				failed = true
				continue
			}
			r.mu.Lock()
			r.status.Rows++
			r.mu.Unlock()
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
	seq, cause, elapsed := r.sequence, r.err, r.lastElapsed
	r.mu.Unlock()
	if !failed {
		reason := "closed"
		if cause != nil {
			reason = "recording_failed"
		}
		e := r.entry("session_end", seq+1, max(elapsed, time.Since(r.disk.started).Nanoseconds()), time.Now(), reason, nil)
		if cause != nil {
			e.Error = fmt.Sprintf("%.512s", cause.Error())
		}
		if err := r.disk.write(e); err != nil {
			r.fail(err)
		}
	}
	if err := r.disk.close(); err != nil {
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
		}
	})
	return r.closeErr
}
func (r *Recorder) unlock() error {
	own, err := r.lock.Stat()
	err = errors.Join(err, r.lock.Close())
	path := filepath.Join(r.disk.cfg.Dir, ".writer.lock")
	current, statErr := os.Lstat(path)
	if statErr != nil {
		return errors.Join(err, statErr)
	}
	if own == nil || !os.SameFile(own, current) {
		return errors.Join(err, fmt.Errorf("history lock was replaced"))
	}
	return errors.Join(err, os.Remove(path))
}

package fuelhistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type diskWriter struct {
	cfg      Config
	manifest manifest
	session  string
	started  time.Time
	file     *os.File
	size     int64
}

func (d *diskWriter) newPart() error {
	if d.file != nil {
		if err := errors.Join(d.file.Sync(), d.file.Close()); err != nil {
			return err
		}
		d.file = nil
		d.manifest.Parts[len(d.manifest.Parts)-1].Closed = true
	}
	// Publish retirement before deleting; interrupted retirement may leave an
	// orphan, never a manifest that points outside the managed directory.
	for len(d.manifest.Parts) >= d.cfg.MaxFiles {
		old := d.manifest.Parts[0]
		if !old.Closed {
			return fmt.Errorf("cannot retire an active history part")
		}
		d.manifest.Parts = d.manifest.Parts[1:]
		d.manifest.Truncated = true
		if err := saveManifest(d.cfg.Dir, d.manifest); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(d.cfg.Dir, old.Name)); err != nil {
			return err
		}
	}
	d.manifest.Next++
	name := fmt.Sprintf("part-%016x.jsonl", d.manifest.Next)
	f, err := os.OpenFile(filepath.Join(d.cfg.Dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	d.file, d.size = f, 0
	d.manifest.Parts = append(d.manifest.Parts, part{Name: name, Session: d.session, StartedAt: d.started.UTC()})
	return saveManifest(d.cfg.Dir, d.manifest)
}
func (d *diskWriter) write(e Entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if len(data)+1 > MaxLine {
		return fmt.Errorf("history row exceeds 64KiB")
	}
	if d.size+int64(len(data)+1) > d.cfg.MaxBytes {
		if err := d.newPart(); err != nil {
			return err
		}
	}
	n, err := d.file.Write(append(data, '\n'))
	d.size += int64(n)
	return err
}
func (d *diskWriter) close() error {
	if d.file == nil {
		return nil
	}
	err := errors.Join(d.file.Sync(), d.file.Close())
	d.file = nil
	if err != nil {
		return err
	}
	d.manifest.Parts[len(d.manifest.Parts)-1].Closed = true
	return saveManifest(d.cfg.Dir, d.manifest)
}

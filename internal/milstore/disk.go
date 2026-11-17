package milstore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/privacy"
)

type diskWriter struct {
	cfg      Config
	manifest manifest
	session  string
	started  time.Time
	file     *os.File
	size     int64
}

func writeAll(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if n != len(data) {
		err = errors.Join(err, io.ErrShortWrite)
	}
	return err
}

func (d *diskWriter) retire() error {
	for len(d.manifest.Parts) >= d.cfg.MaxFiles {
		old := d.manifest.Parts[0]
		if !old.Closed && !old.Aborted {
			return fmt.Errorf("cannot retire active MIL part")
		}
		if _, err := regular(filepath.Join(d.cfg.Dir, old.Name)); err != nil {
			return err
		}
		d.manifest.Parts = d.manifest.Parts[1:]
		d.manifest.Truncated = true
		// Publish ownership retirement before removal: a crash leaves a detectable orphan.
		if err := saveManifest(d.cfg.Dir, d.manifest); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(d.cfg.Dir, old.Name)); err != nil {
			return err
		}
		if err := syncDir(d.cfg.Dir); err != nil {
			return err
		}
	}
	return nil
}

func (d *diskWriter) newPart() error {
	if d.file != nil {
		if err := d.finishPart(true); err != nil {
			return err
		}
	}
	if err := d.retire(); err != nil {
		return err
	}
	if d.manifest.Next == ^uint64(0)-1 {
		return fmt.Errorf("mil part sequence exhausted")
	}
	d.manifest.Next++
	name := fmt.Sprintf("mil-part-%016x.jsonl", d.manifest.Next)
	f, err := os.OpenFile(filepath.Join(d.cfg.Dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	d.file, d.size = f, 0
	d.manifest.Parts = append(d.manifest.Parts, part{Name: name, Session: d.session, StartedAt: d.started.UTC()})
	return saveManifest(d.cfg.Dir, d.manifest)
}

func (d *diskWriter) write(e diskEntry) error {
	data, err := privacy.JSON(e, true)
	if err != nil {
		return err
	}
	if len(data) > MaxLine {
		return fmt.Errorf("mil row exceeds 1MiB")
	}
	if d.size+int64(len(data)) > d.cfg.MaxBytes {
		if err := d.newPart(); err != nil {
			return err
		}
	}
	if err := writeAll(d.file, data); err != nil {
		return err
	}
	d.size += int64(len(data))
	return nil
}

func (d *diskWriter) finishPart(clean bool) error {
	if d.file == nil {
		return nil
	}
	err := errors.Join(d.file.Sync(), d.file.Close())
	d.file = nil
	p := &d.manifest.Parts[len(d.manifest.Parts)-1]
	p.Closed = clean && err == nil
	p.Aborted = !p.Closed
	return errors.Join(err, saveManifest(d.cfg.Dir, d.manifest))
}

package record

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/privacy"
)

const MaxFileBytes int64 = 256 * 1024 * 1024
const finalReserve int64 = 64 * 1024

var ErrSizeLimit = errors.New("recording file size limit reached")

type syncFile interface {
	io.WriteCloser
	Sync() error
}

type Writer struct {
	file           syncFile
	path           string
	redact         bool
	sequence       uint64
	written, limit int64
	err            error
	closed         bool
}

func Open(path string, redact bool) (*Writer, error) {
	file, err := createFile(path)
	if err != nil {
		return nil, fmt.Errorf("create trip recording: %w", err)
	}
	return newWriter(file, file.Name(), redact), nil
}

func createFile(path string) (*os.File, error) {
	if path != "" {
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	}
	if err := os.MkdirAll("diagnostics", 0700); err != nil {
		return nil, err
	}
	return os.CreateTemp("diagnostics", "trip-"+time.Now().UTC().Format("20060102T150405Z")+"-*.jsonl")
}

func newWriter(file syncFile, path string, redact bool) *Writer {
	return &Writer{file: file, path: path, redact: redact, limit: MaxFileBytes}
}

func (w *Writer) Path() string { return w.path }

// Write never buffers trip history. ErrSizeLimit leaves room for recording_end.
// Other I/O failures are sticky, so no later event can mask or continue them.
func (w *Writer) Write(e Event) error {
	if w.closed {
		return os.ErrClosed
	}
	if w.err != nil {
		return w.err
	}
	if e.Type == "" || e.Timestamp.IsZero() || e.ElapsedNS < 0 {
		w.err = fmt.Errorf("invalid recording event metadata")
		return w.err
	}
	e.Version, e.Sequence = 1, w.sequence+1
	e.Timestamp = e.Timestamp.UTC()
	data, err := privacy.JSON(e, w.redact)
	if err != nil {
		w.err = err
		return err
	}
	limit := w.limit - finalReserve
	if e.Type == "recording_end" {
		limit = w.limit
	}
	if w.written+int64(len(data)) > limit {
		return ErrSizeLimit
	}
	n, err := w.file.Write(data)
	if n != len(data) && err == nil {
		err = io.ErrShortWrite
	}
	w.written += int64(n)
	w.err = err
	if err != nil {
		return err
	}
	w.sequence++
	// Sync before the next potentially long adapter operation. This also bounds
	// unsynced data during protocol search, recovery and idle gaps without a
	// background goroutine racing file closure or failure handling.
	w.err = w.file.Sync()
	return w.err
}

func (w *Writer) Close() error {
	if w.closed {
		return w.err
	}
	w.closed = true
	w.err = errors.Join(w.err, w.file.Sync(), w.file.Close())
	return w.err
}

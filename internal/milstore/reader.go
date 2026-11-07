package milstore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxReadPart = 64 << 20
const maxReadIssues = 128

type ReadOptions struct {
	Session string
}

// ReadEntry retains the recorded order and the original observation timestamp.
// Boundary marks the first accepted row of a session or after an integrity gap;
// ordinary part rotation does not itself break continuity.
type ReadEntry struct {
	Version   int    `json:"version"`
	Session   string `json:"session_id"`
	Source    string `json:"source"`
	Sequence  uint64 `json:"sequence"`
	ElapsedNS int64  `json:"elapsed_ns"`
	Entry
	File     string `json:"-"`
	Line     int    `json:"-"`
	Boundary bool   `json:"-"`
}

type ReadIssue struct {
	Code   string `json:"code"`
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Detail string `json:"detail"`
}

type readSession struct {
	seen      bool
	sequence  uint64
	elapsed   int64
	ended     bool
	partEnded bool
	boundary  bool
}

type archiveReader struct {
	ctx      context.Context
	sink     func(ReadEntry) error
	issues   []ReadIssue
	sessions map[string]*readSession
}

// Read streams validated rows without changing the archive or acquiring its
// writer lock. Manifest/open failures and sink errors stop reading. Damaged
// parts and rows yield bounded issues and break evidence continuity.
func Read(ctx context.Context, dir string, options ReadOptions, sink func(ReadEntry) error) ([]ReadIssue, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if sink == nil {
		return nil, fmt.Errorf("MIL reader requires a sink")
	}
	if err := readDirectory(dir); err != nil {
		return nil, err
	}
	m, err := readManifest(dir)
	if err != nil {
		return nil, err
	}
	found := options.Session == ""
	last := make(map[string]int)
	for i, p := range m.Parts {
		last[p.Session] = i
		found = found || p.Session == options.Session
	}
	if !found {
		return nil, fmt.Errorf("unknown MIL session %q", options.Session)
	}
	r := archiveReader{ctx: ctx, sink: sink, sessions: make(map[string]*readSession)}
	if m.Truncated {
		r.issue(nil, "retention_truncated", "", 0, "archive retention removed earlier parts")
	}
	for i, p := range m.Parts {
		if err := ctx.Err(); err != nil {
			return r.issues, err
		}
		if options.Session != "" && options.Session != p.Session {
			continue
		}
		s := r.sessions[p.Session]
		if s == nil {
			s = &readSession{boundary: true}
			r.sessions[p.Session] = s
		}
		if !p.Closed {
			r.issue(s, "incomplete", p.Name, 0, "part is active or aborted")
		}
		s.partEnded = false
		if err := r.readPart(dir, p, m.Source, s); err != nil {
			return r.issues, err
		}
		if last[p.Session] == i && !s.partEnded {
			r.issue(s, "incomplete", p.Name, 0, "session_end is missing from final session part")
		}
	}
	return r.issues, ctx.Err()
}

// Unlike the writer's privateDir, this check never creates directories.
func readDirectory(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	current := string(filepath.Separator)
	for _, name := range strings.Split(strings.TrimPrefix(abs, current), string(filepath.Separator)) {
		current = filepath.Join(current, name)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("MIL directory must have real directory components")
		}
	}
	return nil
}

func (r *archiveReader) issue(s *readSession, code, file string, line int, detail string) {
	if s != nil {
		s.boundary = true
	}
	issue := ReadIssue{Code: code, File: file, Line: line, Detail: detail}
	if len(r.issues) < maxReadIssues {
		r.issues = append(r.issues, issue)
		return
	}
	r.issues[maxReadIssues-1] = ReadIssue{Code: "issues_truncated", Detail: "further archive issues omitted after the 128 issue limit"}
}

func (r *archiveReader) readPart(dir string, p part, source string, s *readSession) (err error) {
	path := filepath.Join(dir, p.Name)
	info, err := regular(path)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	actual, err := f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, actual) || !actual.Mode().IsRegular() || actual.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("MIL part changed while opening: %s", p.Name)
	}
	if actual.Size() > maxReadPart {
		r.issue(s, "part_too_large", p.Name, 0, "part exceeds 64MiB; not read")
		return nil
	}
	reader := bufio.NewReaderSize(io.LimitReader(f, maxReadPart+1), 4096)
	var total int64
	for line := 1; ; line++ {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		data, e := boundedReadLine(reader)
		total += int64(len(data))
		if total > maxReadPart {
			r.issue(s, "part_too_large", p.Name, line, "part grew beyond 64MiB")
			return nil
		}
		if errors.Is(e, errReadLineLimit) {
			r.issue(s, "line_too_large", p.Name, line, "row exceeds 1MiB; remainder of part skipped")
			return nil
		}
		if errors.Is(e, io.EOF) {
			if len(data) != 0 {
				r.issue(s, "incomplete", p.Name, line, "partial final row is not evidence")
			}
			return nil
		}
		if e != nil {
			return e
		}
		if err := r.row(bytes.TrimSuffix(data, []byte{'\n'}), p, source, line, s); err != nil {
			return err
		}
	}
}

var errReadLineLimit = errors.New("MIL row exceeds limit")

func boundedReadLine(reader *bufio.Reader) ([]byte, error) {
	var data []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(data)+len(fragment) > MaxLine {
			return data, errReadLineLimit
		}
		data = append(data, fragment...)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return data, err
		}
	}
}

func (r *archiveReader) row(data []byte, p part, source string, line int, s *readSession) error {
	e, err := decodeReadRow(data, p.Session, source)
	if err != nil {
		r.issue(s, "invalid_row", p.Name, line, err.Error())
		return nil
	}
	if s.ended {
		r.issue(s, "invalid_row", p.Name, line, "row follows session_end")
		return nil
	}
	if !s.seen {
		if e.Kind != "session_start" || e.Sequence != 1 {
			r.issue(s, "incomplete", p.Name, line, "session_start or earlier session rows missing")
		}
	} else {
		if e.Sequence != s.sequence+1 {
			r.issue(s, "sequence_gap", p.Name, line, "sequence is not the next recorded row")
			if e.Sequence <= s.sequence {
				return nil
			}
		}
		if e.ElapsedNS < s.elapsed {
			r.issue(s, "elapsed_regression", p.Name, line, "elapsed recording time moved backwards")
			return nil
		}
		if e.Kind == "session_start" {
			r.issue(s, "invalid_row", p.Name, line, "duplicate session_start")
			return nil
		}
	}
	s.seen, s.sequence, s.elapsed, s.ended = true, e.Sequence, e.ElapsedNS, e.Kind == "session_end"
	s.partEnded = s.ended
	entry := ReadEntry{Version: e.Version, Session: e.Session, Source: e.Source, Sequence: e.Sequence, ElapsedNS: e.ElapsedNS, Entry: e.Entry, File: p.Name, Line: line, Boundary: s.boundary}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if err := r.sink(entry); err != nil {
		return err
	}
	s.boundary = false
	return nil
}

func decodeReadRow(data []byte, session, source string) (diskEntry, error) {
	var e diskEntry
	if err := uniqueJSON(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
		return e, fmt.Errorf("invalid row JSON")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&e); err != nil {
		return e, fmt.Errorf("invalid row envelope")
	}
	if !errors.Is(d.Decode(new(any)), io.EOF) {
		return e, fmt.Errorf("extra row data")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return e, fmt.Errorf("invalid row envelope")
	}
	for _, key := range []string{"version", "session_id", "source", "sequence", "elapsed_ns", "kind", "at"} {
		value := bytes.TrimSpace(fields[key])
		if len(value) == 0 || bytes.Equal(value, []byte("null")) {
			return e, fmt.Errorf("missing or null row metadata: %s", key)
		}
	}
	if e.Version != Version || e.Session != session || e.Source != source || e.Sequence == 0 || e.ElapsedNS < 0 || e.At.IsZero() || len(e.Incident) > 512 {
		return e, fmt.Errorf("invalid row metadata")
	}
	if err := validateReadPayload(e.Entry); err != nil {
		return e, err
	}
	return e, nil
}

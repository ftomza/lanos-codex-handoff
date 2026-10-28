package fuelhistory

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
)

func Read(ctx context.Context, dir string, o Options, sink func(Entry) error) ([]Issue, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if sink == nil {
		return nil, fmt.Errorf("history reader requires a sink")
	}
	m, err := loadManifest(dir)
	if err != nil {
		return nil, err
	}
	issues := make([]Issue, 0, 8)
	boundary := false
	for _, p := range m.Parts {
		if err := ctx.Err(); err != nil {
			return issues, err
		}
		if o.Session != "" && p.Session != o.Session {
			boundary = true
			continue
		}
		found, err := readPart(ctx, dir, p, m.Source, o, &boundary, sink)
		for _, issue := range found {
			if len(issues) < 128 {
				issues = append(issues, issue)
			}
		}
		if err != nil {
			return issues, err
		}
	}
	return issues, nil
}
func readPart(ctx context.Context, dir string, p part, source string, o Options, boundary *bool, sink func(Entry) error) (issues []Issue, err error) {
	f, err := os.Open(filepath.Join(dir, p.Name))
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > 64<<20 {
		return nil, fmt.Errorf("history part exceeds 64MiB")
	}
	reader := bufio.NewReaderSize(io.LimitReader(f, 64<<20), MaxLine+1)
	add := func(code, detail string, line int) {
		if len(issues) < 128 {
			issues = append(issues, Issue{Code: code, File: p.Name, Line: line, Detail: detail})
		}
		*boundary = true
	}
	for line := 1; ; line++ {
		if err := ctx.Err(); err != nil {
			return issues, err
		}
		raw, readErr := reader.ReadSlice('\n')
		if errors.Is(readErr, io.EOF) {
			if len(raw) > 0 {
				add("partial_tail", "unterminated final row", line)
			}
			return issues, nil
		}
		if readErr != nil {
			add("oversized_or_unreadable_row", readErr.Error(), line)
			return issues, nil
		}
		if len(raw) > MaxLine {
			add("oversized_row", "row exceeds 64KiB", line)
			continue
		}
		var e Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			add("invalid_json", err.Error(), line)
			continue
		}
		if e.Version != Version {
			add("unknown_version", "unsupported history version", line)
			continue
		}
		if !validSession(e.Session) || e.Session != p.Session || e.Source != source || e.Sequence == 0 || e.ElapsedNS < 0 || e.At.IsZero() || len(e.Error) > 2048 || !slices.Contains([]string{"session_start", "sample", "event", "session_end"}, e.Kind) {
			add("invalid_envelope", "invalid metadata", line)
			continue
		}
		if (e.Kind == "sample" || e.Kind == "event") && e.Sample == nil {
			add("missing_sample", "no diagnostic payload", line)
			continue
		}
		if (e.Kind == "sample" || e.Kind == "event") && (!o.From.IsZero() && e.At.Before(o.From) || !o.To.IsZero() && e.At.After(o.To)) {
			*boundary = true
			continue
		}
		e.Boundary = *boundary
		*boundary = false
		if err := sink(e); err != nil {
			return issues, err
		}
	}
}

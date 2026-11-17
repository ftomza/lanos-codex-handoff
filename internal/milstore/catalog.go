package milstore

import (
	"context"
	"time"
)

type Catalog struct {
	Version            int              `json:"version"`
	Source             string           `json:"source"`
	RetentionTruncated bool             `json:"retention_truncated"`
	Sessions           []CatalogSession `json:"sessions"`
}

type CatalogSession struct {
	ID        string    `json:"id"`
	StartedAt time.Time `json:"started_at"`
	Parts     int       `json:"parts"`
	Closed    bool      `json:"closed"`
	Aborted   bool      `json:"aborted"`
}

// List validates the manifest and retained files without scanning JSONL or
// acquiring the writer lock. Closed describes files, not diagnostic health.
func List(ctx context.Context, dir string) (Catalog, error) {
	if err := ctx.Err(); err != nil {
		return Catalog{}, err
	}
	if err := readDirectory(dir); err != nil {
		return Catalog{}, err
	}
	m, err := readManifest(dir)
	if err != nil {
		return Catalog{}, err
	}
	c := Catalog{Version: Version, Source: m.Source, RetentionTruncated: m.Truncated, Sessions: []CatalogSession{}}
	indexes := make(map[string]int)
	for _, p := range m.Parts {
		if err := ctx.Err(); err != nil {
			return Catalog{}, err
		}
		i, ok := indexes[p.Session]
		if !ok {
			i = len(c.Sessions)
			indexes[p.Session] = i
			c.Sessions = append(c.Sessions, CatalogSession{ID: p.Session, StartedAt: p.StartedAt, Closed: true})
		}
		s := &c.Sessions[i]
		s.Parts++
		s.Closed = s.Closed && p.Closed
		s.Aborted = s.Aborted || p.Aborted
	}
	return c, ctx.Err()
}

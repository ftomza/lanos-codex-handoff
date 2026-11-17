package fuelhistory

import "slices"

// Each reduction keeps a bucket's extrema. A bucket containing unavailable data
// keeps its extrema and a break marker; no line may cross the bucket unmarked.
type series struct {
	points  []Point
	limit   int
	reduced bool
}

func newSeries(limit int) *series { return &series{limit: limit, points: make([]Point, 0, limit*2)} }
func (s *series) add(p Point) {
	s.points = append(s.points, p)
	if len(s.points) > s.limit*2 {
		s.reduce()
	}
}
func (s *series) reduce() {
	if s.limit <= 2 {
		s.reduceTiny()
		return
	}
	out := make([]Point, 0, len(s.points)*3/4+3)
	for start := 0; start < len(s.points); start += 4 {
		end := min(start+4, len(s.points))
		low, high, gap := -1, -1, -1
		for i := start; i < end; i++ {
			p := s.points[i]
			if p.Break || p.Value == nil {
				gap = i
			}
			if p.Value == nil {
				continue
			}
			if low < 0 || *p.Value < *s.points[low].Value {
				low = i
			}
			if high < 0 || *p.Value > *s.points[high].Value {
				high = i
			}
		}
		indexes := []int{low, high, gap}
		slices.Sort(indexes)
		indexes = slices.Compact(indexes)
		for _, i := range indexes {
			if i >= 0 {
				p := s.points[i]
				if gap >= 0 {
					p.Break = true
				}
				out = append(out, p)
			}
		}
	}
	s.points = out
	s.reduced = true
}

// A single point cannot represent both extrema; retain the newest observation.
// Two points retain extrema, but must not imply continuity of discarded data.
func (s *series) reduceTiny() {
	if len(s.points) <= s.limit {
		return
	}
	if s.limit == 1 {
		s.points = []Point{s.points[len(s.points)-1]}
	} else {
		low, high := 0, 0
		for i, p := range s.points {
			if p.Value == nil {
				continue
			}
			if s.points[low].Value == nil || *p.Value < *s.points[low].Value {
				low = i
			}
			if s.points[high].Value == nil || *p.Value > *s.points[high].Value {
				high = i
			}
		}
		first, last := min(low, high), max(low, high)
		s.points = []Point{s.points[first], s.points[last]}
	}
	for i := range s.points {
		s.points[i].Break = true
	}
	s.reduced = true
}
func (s *series) finish() []Point {
	for len(s.points) > s.limit {
		s.reduce()
	}
	return s.points
}

// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// Lightweight, dependency-free metrics and tracing abstraction for the NPS SDK.
//
// The .NET reference (NwpTelemetry.cs / NopTelemetry.cs) uses
// System.Diagnostics.Metrics + ActivitySource so hosts can wire OpenTelemetry
// exporters via the instrument/meter names. Go has no OTel package in the
// module cache, so this package provides an equivalent minimal abstraction:
// counters, histograms, and spans backed by an in-memory reader that tests use
// to assert recorded values. Instrument and meter names match the .NET
// reference exactly so exported metrics are wire-compatible.
package telemetry

import (
	"sort"
	"sync"
	"time"
)

// Attr is a single key/value span or measurement attribute.
type Attr struct {
	Key   string
	Value any
}

// A returns an Attr — convenience constructor.
func A(key string, value any) Attr { return Attr{Key: key, Value: value} }

// attrKey builds a stable, order-independent key from an attribute set so
// counter/histogram cells aggregate identically regardless of argument order.
func attrKey(attrs []Attr) string {
	if len(attrs) == 0 {
		return ""
	}
	pairs := make([]string, len(attrs))
	for i, a := range attrs {
		pairs[i] = a.Key + "=" + valueString(a.Value)
	}
	sort.Strings(pairs)
	out := ""
	for i, p := range pairs {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

// Meter creates instruments under a single meter name. Instruments record into
// a shared in-memory store readable via Snapshot for tests and simple readers.
type Meter struct {
	Name    string
	Version string
	mu      sync.Mutex
	store   *store
}

// NewMeter creates a meter with the given name and version.
func NewMeter(name, version string) *Meter {
	return &Meter{Name: name, Version: version, store: newStore()}
}

// Counter is a monotonic sum instrument.
type Counter struct {
	meter *Meter
	name  string
}

// Histogram records a distribution of values.
type Histogram struct {
	meter *Meter
	name  string
}

// Counter registers (or returns) a counter instrument.
func (m *Meter) Counter(name string) *Counter {
	return &Counter{meter: m, name: name}
}

// Histogram registers (or returns) a histogram instrument.
func (m *Meter) Histogram(name string) *Histogram {
	return &Histogram{meter: m, name: name}
}

// Add increments the counter by delta with the given attributes.
func (c *Counter) Add(delta float64, attrs ...Attr) {
	c.meter.mu.Lock()
	defer c.meter.mu.Unlock()
	c.meter.store.addCounter(c.name, attrKey(attrs), delta)
}

// Inc increments the counter by 1.
func (c *Counter) Inc(attrs ...Attr) { c.Add(1, attrs...) }

// Record adds a value to the histogram with the given attributes.
func (h *Histogram) Record(value float64, attrs ...Attr) {
	h.meter.mu.Lock()
	defer h.meter.mu.Unlock()
	h.meter.store.recordHistogram(h.name, attrKey(attrs), value)
}

// CounterSnapshot is the accumulated value of one counter cell.
type CounterSnapshot struct {
	Name  string
	Attrs string
	Value float64
}

// HistogramSnapshot is the accumulated stats of one histogram cell.
type HistogramSnapshot struct {
	Name  string
	Attrs string
	Count int64
	Sum   float64
	Min   float64
	Max   float64
}

// Snapshot returns the current accumulated counters and histograms. Tests read
// this to assert recorded values.
func (m *Meter) Snapshot() ([]CounterSnapshot, []HistogramSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.snapshot()
}

// CounterValue returns the summed value of a counter across all attribute sets.
func (m *Meter) CounterValue(name string) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.counterTotal(name)
}

// HistogramCount returns the number of recorded values for a histogram across
// all attribute sets.
func (m *Meter) HistogramCount(name string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.histogramCount(name)
}

// ── in-memory store ───────────────────────────────────────────────────────────

type store struct {
	counters   map[string]map[string]float64
	histograms map[string]map[string]*histCell
}

type histCell struct {
	count    int64
	sum      float64
	min, max float64
}

func newStore() *store {
	return &store{
		counters:   map[string]map[string]float64{},
		histograms: map[string]map[string]*histCell{},
	}
}

func (s *store) addCounter(name, key string, delta float64) {
	cells := s.counters[name]
	if cells == nil {
		cells = map[string]float64{}
		s.counters[name] = cells
	}
	cells[key] += delta
}

func (s *store) recordHistogram(name, key string, value float64) {
	cells := s.histograms[name]
	if cells == nil {
		cells = map[string]*histCell{}
		s.histograms[name] = cells
	}
	c := cells[key]
	if c == nil {
		c = &histCell{min: value, max: value}
		cells[key] = c
	}
	c.count++
	c.sum += value
	if value < c.min {
		c.min = value
	}
	if value > c.max {
		c.max = value
	}
}

func (s *store) counterTotal(name string) float64 {
	total := 0.0
	for _, v := range s.counters[name] {
		total += v
	}
	return total
}

func (s *store) histogramCount(name string) int64 {
	var total int64
	for _, c := range s.histograms[name] {
		total += c.count
	}
	return total
}

func (s *store) snapshot() ([]CounterSnapshot, []HistogramSnapshot) {
	var cs []CounterSnapshot
	for name, cells := range s.counters {
		for key, v := range cells {
			cs = append(cs, CounterSnapshot{Name: name, Attrs: key, Value: v})
		}
	}
	var hs []HistogramSnapshot
	for name, cells := range s.histograms {
		for key, c := range cells {
			hs = append(hs, HistogramSnapshot{
				Name: name, Attrs: key,
				Count: c.count, Sum: c.sum, Min: c.min, Max: c.max,
			})
		}
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].Name != cs[j].Name {
			return cs[i].Name < cs[j].Name
		}
		return cs[i].Attrs < cs[j].Attrs
	})
	sort.Slice(hs, func(i, j int) bool {
		if hs[i].Name != hs[j].Name {
			return hs[i].Name < hs[j].Name
		}
		return hs[i].Attrs < hs[j].Attrs
	})
	return cs, hs
}

// ── tracing ───────────────────────────────────────────────────────────────────

// Tracer creates spans under a single source name. Spans are recorded into an
// in-memory buffer readable via Finished for tests.
type Tracer struct {
	Name    string
	Version string
	mu      sync.Mutex
	spans   []*Span
}

// NewTracer creates a tracer with the given source name and version.
func NewTracer(name, version string) *Tracer {
	return &Tracer{Name: name, Version: version}
}

// Span is a single unit of work with a name, timing, attributes, and status.
type Span struct {
	Name       string
	Start      time.Time
	EndTime    time.Time
	Attributes []Attr
	StatusOK   bool
	Error      string

	tracer *Tracer
	ended  bool
	mu     sync.Mutex
}

// Start begins a new span with optional initial attributes.
func (t *Tracer) Start(name string, attrs ...Attr) *Span {
	return &Span{
		Name:       name,
		Start:      time.Now(),
		Attributes: append([]Attr(nil), attrs...),
		StatusOK:   true,
		tracer:     t,
	}
}

// SetAttr adds or replaces an attribute on the span.
func (s *Span) SetAttr(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Attributes {
		if s.Attributes[i].Key == key {
			s.Attributes[i].Value = value
			return
		}
	}
	s.Attributes = append(s.Attributes, Attr{Key: key, Value: value})
}

// SetError marks the span as failed and records the error message.
func (s *Span) SetError(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.StatusOK = false
	s.Error = msg
}

// End finishes the span and records it in the tracer's buffer.
func (s *Span) End() {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	s.EndTime = time.Now()
	tracer := s.tracer
	s.mu.Unlock()

	if tracer != nil {
		tracer.mu.Lock()
		tracer.spans = append(tracer.spans, s)
		tracer.mu.Unlock()
	}
}

// Duration returns the span's elapsed time (0 if not yet ended).
func (s *Span) Duration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return s.EndTime.Sub(s.Start)
	}
	return 0
}

// Finished returns a copy of all ended spans recorded so far.
func (t *Tracer) Finished() []*Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*Span(nil), t.spans...)
}

// Reset clears recorded spans (test convenience).
func (t *Tracer) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.spans = nil
}

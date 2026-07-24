// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// Prometheus-compatible metrics registry and /metrics endpoint. Ported from the
// .NET reference NPS.Daemon.Observability/Metrics/MetricsRegistry.cs +
// MetricsEndpoint.cs. Output matches the Prometheus text exposition format and
// the .NET content type exactly.

package observability

import (
	"crypto/subtle"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// MetricsContentType matches the .NET MetricsEndpoint.ContentType exactly.
const MetricsContentType = "text/plain; version=0.0.4; charset=utf-8"

// cellSeparator matches the .NET Counter.CellSeparator (ASCII Unit Separator).
const cellSeparator = "\x1f"

// MetricsRegistry is a lightweight Prometheus counter/gauge registry backing the
// /metrics endpoint.
type MetricsRegistry struct {
	mu      sync.Mutex
	entries []metricEntry
}

// NewMetricsRegistry builds an empty registry.
func NewMetricsRegistry() *MetricsRegistry { return &MetricsRegistry{} }

type metricEntry interface {
	writeTo(sb *strings.Builder)
}

// RegisterCounter registers a monotonic counter with optional label names.
func (r *MetricsRegistry) RegisterCounter(name, help string, labelNames ...string) *Counter {
	c := &Counter{name: name, help: help, labels: labelNames, cells: map[string]float64{}}
	if len(labelNames) == 0 {
		c.cells[""] = 0
	}
	r.mu.Lock()
	r.entries = append(r.entries, c)
	r.mu.Unlock()
	return c
}

// RegisterGauge registers a single-valued gauge.
func (r *MetricsRegistry) RegisterGauge(name, help string) *Gauge {
	g := &Gauge{name: name, help: help}
	r.mu.Lock()
	r.entries = append(r.entries, g)
	r.mu.Unlock()
	return g
}

// WriteTo writes the registry in Prometheus exposition format.
func (r *MetricsRegistry) WriteTo(sb *strings.Builder) {
	r.mu.Lock()
	snap := append([]metricEntry(nil), r.entries...)
	r.mu.Unlock()
	for _, e := range snap {
		e.writeTo(sb)
	}
}

// Render returns the full exposition text.
func (r *MetricsRegistry) Render() string {
	var sb strings.Builder
	r.WriteTo(&sb)
	return sb.String()
}

// Counter is a monotonic counter with one cell per label-value tuple.
type Counter struct {
	name   string
	help   string
	labels []string
	mu     sync.Mutex
	cells  map[string]float64
}

// Inc increments the (unlabeled or labeled) cell by 1.
func (c *Counter) Inc(labelValues ...string) { c.Add(1, labelValues...) }

// Add increments a labeled cell by delta. labelValues must follow the registered
// label order; missing values count as the empty string.
func (c *Counter) Add(delta float64, labelValues ...string) {
	key := c.cellKey(labelValues)
	c.mu.Lock()
	c.cells[key] += delta
	c.mu.Unlock()
}

func (c *Counter) cellKey(labelValues []string) string {
	if len(c.labels) == 0 {
		return ""
	}
	parts := make([]string, len(c.labels))
	for i := range c.labels {
		if i < len(labelValues) {
			parts[i] = labelValues[i]
		}
	}
	return strings.Join(parts, cellSeparator)
}

func (c *Counter) writeTo(sb *strings.Builder) {
	sb.WriteString("# HELP ")
	sb.WriteString(c.name)
	sb.WriteByte(' ')
	sb.WriteString(c.help)
	sb.WriteByte('\n')
	sb.WriteString("# TYPE ")
	sb.WriteString(c.name)
	sb.WriteString(" counter\n")

	c.mu.Lock()
	keys := make([]string, 0, len(c.cells))
	for k := range c.cells {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		val := c.cells[key]
		sb.WriteString(c.name)
		if len(c.labels) > 0 {
			sb.WriteByte('{')
			parts := strings.Split(key, cellSeparator)
			for i := range c.labels {
				if i > 0 {
					sb.WriteByte(',')
				}
				v := ""
				if i < len(parts) {
					v = parts[i]
				}
				sb.WriteString(c.labels[i])
				sb.WriteString("=\"")
				sb.WriteString(escapeLabel(v))
				sb.WriteByte('"')
			}
			sb.WriteByte('}')
		}
		sb.WriteByte(' ')
		sb.WriteString(formatFloat(val))
		sb.WriteByte('\n')
	}
	c.mu.Unlock()
}

// Gauge is a single-valued, thread-safe gauge.
type Gauge struct {
	name string
	help string
	mu   sync.Mutex
	val  float64
}

// Set sets the gauge value.
func (g *Gauge) Set(v float64) { g.mu.Lock(); g.val = v; g.mu.Unlock() }

// Inc increments the gauge by 1.
func (g *Gauge) Inc() { g.Add(1) }

// Dec decrements the gauge by 1.
func (g *Gauge) Dec() { g.Add(-1) }

// Add adds delta to the gauge.
func (g *Gauge) Add(delta float64) { g.mu.Lock(); g.val += delta; g.mu.Unlock() }

// Value returns the current gauge value.
func (g *Gauge) Value() float64 { g.mu.Lock(); defer g.mu.Unlock(); return g.val }

func (g *Gauge) writeTo(sb *strings.Builder) {
	sb.WriteString("# HELP ")
	sb.WriteString(g.name)
	sb.WriteByte(' ')
	sb.WriteString(g.help)
	sb.WriteByte('\n')
	sb.WriteString("# TYPE ")
	sb.WriteString(g.name)
	sb.WriteString(" gauge\n")
	sb.WriteString(g.name)
	sb.WriteByte(' ')
	sb.WriteString(formatFloat(g.Value()))
	sb.WriteByte('\n')
}

func escapeLabel(v string) string {
	if !strings.ContainsAny(v, "\\\"\n") {
		return v
	}
	var sb strings.Builder
	for _, c := range v {
		switch c {
		case '\\':
			sb.WriteString("\\\\")
		case '"':
			sb.WriteString("\\\"")
		case '\n':
			sb.WriteString("\\n")
		default:
			sb.WriteRune(c)
		}
	}
	return sb.String()
}

// formatFloat renders a metric value using the shortest exact decimal (matches
// the .NET "0.################" format for integral and fractional values).
func formatFloat(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// MetricsHandler returns an http.Handler serving GET /metrics from the registry.
// If bearerToken is non-empty, requests must present a matching Bearer token.
// If requireBearerToken is true and no token is configured, the endpoint 404s.
func MetricsHandler(reg *MetricsRegistry, bearerToken string, requireBearerToken bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !authorizeMetrics(r, bearerToken, requireBearerToken, w) {
			return
		}
		w.Header().Set("Content-Type", MetricsContentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(reg.Render()))
	})
}

func authorizeMetrics(r *http.Request, bearerToken string, requireBearerToken bool, w http.ResponseWriter) bool {
	if bearerToken == "" {
		if !requireBearerToken {
			return true
		}
		w.WriteHeader(http.StatusNotFound)
		return false
	}
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	supplied := header[len(prefix):]
	if subtle.ConstantTimeCompare([]byte(supplied), []byte(bearerToken)) == 1 {
		return true
	}
	w.WriteHeader(http.StatusUnauthorized)
	return false
}

// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package telemetry

import "testing"

func TestCounter_AddAndTotal(t *testing.T) {
	m := NewMeter("test", "1.0.0")
	c := m.Counter("hits")
	c.Inc()
	c.Add(4)
	if got := m.CounterValue("hits"); got != 5 {
		t.Fatalf("total=%v", got)
	}
}

func TestCounter_Attributes_AggregatePerCell(t *testing.T) {
	m := NewMeter("test", "1.0.0")
	c := m.Counter("frames")
	c.Inc(A("kind", "query"))
	c.Inc(A("kind", "query"))
	c.Inc(A("kind", "stream"))
	cs, _ := m.Snapshot()
	if len(cs) != 2 {
		t.Fatalf("cells=%d", len(cs))
	}
	// Order-independent attribute key.
	c.Inc(A("a", "1"), A("b", "2"))
	c.Inc(A("b", "2"), A("a", "1"))
	if m.CounterValue("frames") != 5 {
		t.Fatalf("total=%v", m.CounterValue("frames"))
	}
}

func TestHistogram_RecordStats(t *testing.T) {
	m := NewMeter("test", "1.0.0")
	h := m.Histogram("dur")
	h.Record(10)
	h.Record(20)
	h.Record(30)
	if m.HistogramCount("dur") != 3 {
		t.Fatalf("count=%d", m.HistogramCount("dur"))
	}
	_, hs := m.Snapshot()
	if len(hs) != 1 || hs[0].Sum != 60 || hs[0].Min != 10 || hs[0].Max != 30 {
		t.Fatalf("hs=%+v", hs)
	}
}

func TestSpan_Lifecycle(t *testing.T) {
	tr := NewTracer("nps.test", "1.0.0")
	sp := tr.Start("op", A("frame", "0x04"))
	sp.SetAttr("count", 3)
	sp.End()
	spans := tr.Finished()
	if len(spans) != 1 {
		t.Fatalf("spans=%d", len(spans))
	}
	if spans[0].Name != "op" || !spans[0].StatusOK {
		t.Fatalf("span=%+v", spans[0])
	}
}

func TestSpan_Error(t *testing.T) {
	tr := NewTracer("nps.test", "1.0.0")
	sp := tr.Start("op")
	sp.SetError("boom")
	sp.End()
	spans := tr.Finished()
	if spans[0].StatusOK || spans[0].Error != "boom" {
		t.Fatalf("span=%+v", spans[0])
	}
}

func TestSpan_EndIdempotent(t *testing.T) {
	tr := NewTracer("nps.test", "1.0.0")
	sp := tr.Start("op")
	sp.End()
	sp.End()
	if len(tr.Finished()) != 1 {
		t.Fatalf("double-recorded")
	}
}

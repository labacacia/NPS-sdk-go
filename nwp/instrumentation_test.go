// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import "testing"

func TestNwpTelemetry_MetricNames(t *testing.T) {
	tel := NewNwpTelemetry()
	tel.FramesProcessed.Inc()
	tel.FrameErrors.Add(2)
	tel.CgnConsumed.Add(100)
	tel.FrameDurationMs.Record(12.5)

	if tel.Meter.CounterValue("nps.frames.processed") != 1 {
		t.Fatal("frames.processed")
	}
	if tel.Meter.CounterValue("nps.frames.errors") != 2 {
		t.Fatal("frames.errors")
	}
	if tel.Meter.CounterValue("nps.cgn.consumed") != 100 {
		t.Fatal("cgn.consumed")
	}
	if tel.Meter.HistogramCount("nps.frames.processing_ms") != 1 {
		t.Fatal("frames.processing_ms")
	}
	if NwpMeterName != "nps.nwp" || NwpActivitySourceName != "nps.nwp" {
		t.Fatal("names")
	}
}

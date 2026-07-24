// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// NWP frame-processing instrumentation. Ported from the .NET reference
// (NwpInstrumentation.cs / NwpTelemetry.cs). Instrument, meter, and source
// names match the .NET reference exactly for wire-compatible export.

package nwp

import "github.com/labacacia/NPS-sdk-go/telemetry"

const (
	// NwpActivitySourceName is the tracer/source name for NWP frame spans.
	NwpActivitySourceName = "nps.nwp"
	// NwpMeterName is the meter name for NWP frame metrics.
	NwpMeterName = "nps.nwp"
	// NwpTelemetryVersion is the instrumentation version.
	NwpTelemetryVersion = "1.0.0"
)

// NwpTelemetry holds the NWP meter, tracer, and instruments. Metric names match
// the .NET NwpTelemetry static instruments exactly.
type NwpTelemetry struct {
	Meter  *telemetry.Meter
	Tracer *telemetry.Tracer

	FramesProcessed *telemetry.Counter   // nps.frames.processed
	FrameDurationMs *telemetry.Histogram // nps.frames.processing_ms
	CgnConsumed     *telemetry.Counter   // nps.cgn.consumed
	FrameErrors     *telemetry.Counter   // nps.frames.errors
}

// NewNwpTelemetry builds the NWP telemetry instruments over fresh in-memory
// backing stores. Hosts that want a shared registry can supply one meter/tracer
// pair via NewNwpTelemetryWith.
func NewNwpTelemetry() *NwpTelemetry {
	return NewNwpTelemetryWith(
		telemetry.NewMeter(NwpMeterName, NwpTelemetryVersion),
		telemetry.NewTracer(NwpActivitySourceName, NwpTelemetryVersion),
	)
}

// NewNwpTelemetryWith builds the instruments over the supplied meter and tracer.
func NewNwpTelemetryWith(meter *telemetry.Meter, tracer *telemetry.Tracer) *NwpTelemetry {
	return &NwpTelemetry{
		Meter:           meter,
		Tracer:          tracer,
		FramesProcessed: meter.Counter("nps.frames.processed"),
		FrameDurationMs: meter.Histogram("nps.frames.processing_ms"),
		CgnConsumed:     meter.Counter("nps.cgn.consumed"),
		FrameErrors:     meter.Counter("nps.frames.errors"),
	}
}

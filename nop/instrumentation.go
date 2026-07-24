// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// NOP orchestration instrumentation. Ported from the .NET reference
// (NopInstrumentation.cs / NopTelemetry.cs). Instrument, meter, and source
// names match the .NET reference exactly for wire-compatible export.

package nop

import "github.com/labacacia/NPS-sdk-go/telemetry"

const (
	// NopActivitySourceName is the tracer/source name for NOP orchestration spans.
	NopActivitySourceName = "nps.nop"
	// NopMeterName is the meter name for NOP orchestration metrics.
	NopMeterName = "nps.nop"
	// NopTelemetryVersion is the instrumentation version.
	NopTelemetryVersion = "1.0.0"
)

// NopTelemetry holds the NOP meter, tracer, and instruments. Metric names match
// the .NET NopTelemetry static instruments exactly.
type NopTelemetry struct {
	Meter  *telemetry.Meter
	Tracer *telemetry.Tracer

	TaskDurationMs *telemetry.Histogram // nps.nop.task.duration_ms
	NodeDurationMs *telemetry.Histogram // nps.nop.node.duration_ms
	NodeRetries    *telemetry.Counter   // nps.nop.node.retries
	TasksCompleted *telemetry.Counter   // nps.nop.tasks.completed
	TasksFailed    *telemetry.Counter   // nps.nop.tasks.failed
}

// NewNopTelemetry builds the NOP telemetry instruments over fresh in-memory
// backing stores.
func NewNopTelemetry() *NopTelemetry {
	return NewNopTelemetryWith(
		telemetry.NewMeter(NopMeterName, NopTelemetryVersion),
		telemetry.NewTracer(NopActivitySourceName, NopTelemetryVersion),
	)
}

// NewNopTelemetryWith builds the instruments over the supplied meter and tracer.
func NewNopTelemetryWith(meter *telemetry.Meter, tracer *telemetry.Tracer) *NopTelemetry {
	return &NopTelemetry{
		Meter:          meter,
		Tracer:         tracer,
		TaskDurationMs: meter.Histogram("nps.nop.task.duration_ms"),
		NodeDurationMs: meter.Histogram("nps.nop.node.duration_ms"),
		NodeRetries:    meter.Counter("nps.nop.node.retries"),
		TasksCompleted: meter.Counter("nps.nop.tasks.completed"),
		TasksFailed:    meter.Counter("nps.nop.tasks.failed"),
	}
}

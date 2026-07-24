// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nop

import "testing"

func TestNopTelemetry_MetricNames(t *testing.T) {
	tel := NewNopTelemetry()
	tel.TasksCompleted.Inc()
	tel.TasksFailed.Add(1)
	tel.NodeRetries.Add(3)
	tel.TaskDurationMs.Record(50)
	tel.NodeDurationMs.Record(5)

	if tel.Meter.CounterValue("nps.nop.tasks.completed") != 1 {
		t.Fatal("tasks.completed")
	}
	if tel.Meter.CounterValue("nps.nop.tasks.failed") != 1 {
		t.Fatal("tasks.failed")
	}
	if tel.Meter.CounterValue("nps.nop.node.retries") != 3 {
		t.Fatal("node.retries")
	}
	if tel.Meter.HistogramCount("nps.nop.task.duration_ms") != 1 {
		t.Fatal("task.duration_ms")
	}
	if tel.Meter.HistogramCount("nps.nop.node.duration_ms") != 1 {
		t.Fatal("node.duration_ms")
	}
	if NopMeterName != "nps.nop" || NopActivitySourceName != "nps.nop" {
		t.Fatal("names")
	}
}

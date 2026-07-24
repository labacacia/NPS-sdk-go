// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// Single-line JSON structured logging for NPS daemons. Ported from the .NET
// reference NPS.Daemon.Observability/Logging/JsonStructuredLogging.cs. Each
// record carries timestamp, level, msg, logger, and (when present) trace_id.
// The minimum level is driven by the NPS_LOG_LEVEL environment variable, using
// the same names as .NET (trace/debug/info/warn/error/critical/none).

package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// LogLevelEnvVar overrides the default minimum log level.
const LogLevelEnvVar = "NPS_LOG_LEVEL"

// levelCritical is emitted for slog levels at or above Error+4, mapping to the
// .NET "critical" name.
const levelCritical = slog.Level(12)

// ResolveLogLevel resolves the configured level from NPS_LOG_LEVEL, falling back
// to fallback when unset or unparseable. Accepts the .NET level names.
func ResolveLogLevel(fallback slog.Level) slog.Level {
	raw := strings.TrimSpace(os.Getenv(LogLevelEnvVar))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "trace":
		return slog.LevelDebug - 4
	case "debug":
		return slog.LevelDebug
	case "information", "info":
		return slog.LevelInfo
	case "warning", "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "critical":
		return levelCritical
	case "none":
		// Above any emitted level → suppresses all records.
		return slog.Level(1000)
	default:
		return fallback
	}
}

// NewJSONLogger builds an slog.Logger writing single-line NPS JSON records to w
// (typically os.Stdout), honouring NPS_LOG_LEVEL over defaultLevel.
func NewJSONLogger(w io.Writer, defaultLevel slog.Level) *slog.Logger {
	level := ResolveLogLevel(defaultLevel)
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: replaceAttr,
	})
	return slog.New(handler)
}

// NewDefaultJSONLogger builds a JSON logger to stdout at Info level (default).
func NewDefaultJSONLogger() *slog.Logger {
	return NewJSONLogger(os.Stdout, slog.LevelInfo)
}

// replaceAttr rewrites slog's default keys to the NPS field names and level
// names the operator runbook expects.
func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.TimeKey:
		if t, ok := a.Value.Any().(time.Time); ok {
			a.Value = slog.StringValue(t.UTC().Format(time.RFC3339Nano))
		}
		a.Key = "timestamp"
	case slog.MessageKey:
		a.Key = "msg"
	case slog.LevelKey:
		if lvl, ok := a.Value.Any().(slog.Level); ok {
			a.Value = slog.StringValue(levelName(lvl))
		}
		a.Key = "level"
	}
	return a
}

// levelName maps an slog.Level to the .NET-compatible level name.
func levelName(l slog.Level) string {
	switch {
	case l >= levelCritical:
		return "critical"
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	case l >= slog.LevelInfo:
		return "info"
	case l >= slog.LevelDebug:
		return "debug"
	default:
		return "trace"
	}
}

// LogCritical emits a record at the "critical" level (no direct slog helper).
func LogCritical(ctx context.Context, logger *slog.Logger, msg string, args ...any) {
	logger.Log(ctx, levelCritical, msg, args...)
}

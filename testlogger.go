package nst

import (
	"context"
	"log/slog"
	"testing"
)

// TestHandler implements slog.Handler and outputs to testing.TB
type TestHandler struct {
	t testing.TB
}

// NewTestLogger creates a new slog.Logger that outputs to testing.TB
func NewTestLogger(t testing.TB) *slog.Logger {
	handler := &TestHandler{t: t}
	return slog.New(handler)
}

// Enabled implements slog.Handler
func (h *TestHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

// Handle implements slog.Handler
func (h *TestHandler) Handle(ctx context.Context, record slog.Record) error {
	msg := record.Message
	if record.NumAttrs() > 0 {
		attrs := make([]string, 0, record.NumAttrs())
		record.Attrs(func(a slog.Attr) bool {
			attrs = append(attrs, a.String())
			return true
		})
		if len(attrs) > 0 {
			msg += " " + joinAttrs(attrs)
		}
	}
	h.t.Logf("%s: %s", record.Level, msg)
	return nil
}

func joinAttrs(attrs []string) string {
	result := ""
	for i, attr := range attrs {
		if i > 0 {
			result += " "
		}
		result += attr
	}
	return result
}

// WithAttrs implements slog.Handler
func (h *TestHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TestHandler{t: h.t}
}

// WithGroup implements slog.Handler
func (h *TestHandler) WithGroup(name string) slog.Handler {
	return &TestHandler{t: h.t}
}

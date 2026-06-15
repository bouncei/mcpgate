package audit

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"
)

type Event struct {
	Label    string
	Method   string
	Tool     string
	Decision string
	Status   int
	Latency  time.Duration
}

type Logger struct {
	l *slog.Logger
}

// New builds a Logger writing to stdout or to the given file path.
func New(output string) (*Logger, error) {
	if output == "" || output == "stdout" {
		return NewWithWriter(os.Stdout), nil
	}
	f, err := os.OpenFile(output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	return NewWithWriter(f), nil
}

// NewWithWriter builds a Logger writing JSON lines to w (used in tests).
func NewWithWriter(w io.Writer) *Logger {
	return &Logger{l: slog.New(slog.NewJSONHandler(w, nil))}
}

func (a *Logger) Decision(e Event) {
	a.l.Info("request",
		"label", e.Label,
		"method", e.Method,
		"tool", e.Tool,
		"decision", e.Decision,
		"status", e.Status,
		"latency_ms", e.Latency.Milliseconds(),
	)
}

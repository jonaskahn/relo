// Log lines leave Relo as logrus JSON, one object per line. Call sites keep
// slog; this handler is the bridge.
package platform

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type startupTee struct {
	mu     sync.Mutex
	main   io.Writer
	extra  io.WriteCloser
	mirror io.Writer
}

// Write copies one boot line to the daemon log and the startup transcript,
// so early failures are visible in both places.
func (t *startupTee) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n, err := t.main.Write(p)
	if t.extra != nil {
		if _, extraErr := t.extra.Write(p); extraErr != nil && err == nil {
			err = extraErr
		}
	}
	if t.mirror != nil {
		if _, mirrorErr := t.mirror.Write(p); mirrorErr != nil && err == nil {
			err = mirrorErr
		}
	}
	return n, err
}

func (t *startupTee) dropExtra() {
	t.mu.Lock()
	extra := t.extra
	t.extra = nil
	t.mu.Unlock()
	if extra != nil {
		_ = extra.Close()
	}
}

var startupCloser struct {
	mu    sync.Mutex
	close func()
}

func registerStartupClose(close func()) {
	startupCloser.mu.Lock()
	startupCloser.close = close
	startupCloser.mu.Unlock()
}

// FinishStartupLog stops copying boot lines into the startup transcript.
// Later lines stay in the daemon log.
func FinishStartupLog() {
	startupCloser.mu.Lock()
	close := startupCloser.close
	startupCloser.close = nil
	startupCloser.mu.Unlock()
	if close != nil {
		close()
	}
}

type logrusHandler struct {
	logger *logrus.Logger
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

func newLogrusHandler(output io.Writer, level slog.Level) *logrusHandler {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat:   time.RFC3339Nano,
		DisableHTMLEscape: true,
	})
	logger.SetOutput(output)
	logger.SetLevel(logrus.TraceLevel)
	return &logrusHandler{logger: logger, level: level}
}

// Enabled reports whether this level passes the configured floor.
func (h *logrusHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle renders one record as logrus JSON on the daemon log.
func (h *logrusHandler) Handle(_ context.Context, record slog.Record) error {
	fields := logrus.Fields{}
	for _, attr := range h.attrs {
		fields[attr.Key] = slogValue(attr.Value)
	}
	record.Attrs(func(attr slog.Attr) bool {
		fields[prefixKey(h.groups, attr.Key)] = slogValue(attr.Value)
		return true
	})
	entry := h.logger.WithFields(fields)
	switch {
	case record.Level >= slog.LevelError:
		entry.Log(logrus.ErrorLevel, record.Message)
	case record.Level >= slog.LevelWarn:
		entry.Log(logrus.WarnLevel, record.Message)
	case record.Level >= slog.LevelInfo:
		entry.Log(logrus.InfoLevel, record.Message)
	default:
		entry.Log(logrus.DebugLevel, record.Message)
	}
	return nil
}

// WithAttrs returns a handler carrying these attributes on every record.
func (h *logrusHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := h.clone()
	for _, attr := range attrs {
		next.attrs = append(next.attrs, slog.Attr{
			Key: prefixKey(h.groups, attr.Key), Value: attr.Value,
		})
	}
	return next
}

// WithGroup returns a handler nesting records under this group name.
func (h *logrusHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := h.clone()
	next.groups = append(append([]string{}, h.groups...), name)
	return next
}

func (h *logrusHandler) clone() *logrusHandler {
	return &logrusHandler{
		logger: h.logger, level: h.level,
		attrs:  append([]slog.Attr{}, h.attrs...),
		groups: append([]string{}, h.groups...),
	}
}

func prefixKey(groups []string, key string) string {
	if len(groups) == 0 {
		return key
	}
	return strings.Join(groups, ".") + "." + key
}

func slogValue(value slog.Value) any {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindFloat64:
		return value.Float64()
	case slog.KindBool:
		return value.Bool()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			return err.Error()
		}
		return value.Any()
	default:
		return value.String()
	}
}

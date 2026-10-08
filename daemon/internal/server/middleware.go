// Request logging middleware shared by every listener.
package server

import (
	"log/slog"
	"net/http"
	"time"
)

// LoggingMiddleware logs one line per request. It never logs a body, so
// prompts and secrets stay out of the logs.
func LoggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logRequest(logger, r, recorder.status, time.Since(started))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader remembers the status the handler chose.
func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Flush keeps streaming responses flushable through the middleware.
func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func logRequest(logger *slog.Logger, r *http.Request, status int, duration time.Duration) {
	// The path is logged, and never the query string. A callback redirect
	// carries its authorization grant and the state that guards it in the
	// query, and a line written on the way past is the wrong place for
	// either, so no path of this server is ever logged with one.
	logger.Log(r.Context(), levelFor(status), "http request",
		"method", r.Method,
		"path", r.URL.Path,
		"status", status,
		"duration_ms", duration.Milliseconds(),
	)
}

func levelFor(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

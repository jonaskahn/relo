// Dashboard event streams: registration, framing, and channel vocabulary.
package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	sseRetry = 3 * time.Second
	// sseHeartbeat keeps a connection alive through a proxy that would
	// otherwise time an idle stream out.
	sseHeartbeat = 20 * time.Second
)

func (s *Server) registerEvents(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiPrefix+"events/{channel}", s.handleEventStream)
}

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	channels := streamChannels(r.PathValue("channel"))
	if len(channels) == 0 {
		s.fail(w, r, refusal{Status: http.StatusNotFound, Code: "not_found", Message: "api.events.unknown"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, r, refusal{
			Status: http.StatusInternalServerError, Code: "internal_error", Message: "api.events.cannot_stream",
		})
		return
	}
	subscription, ok := s.events.Subscribe(channels...)
	if !ok {
		s.fail(w, r, refusal{
			Status: http.StatusServiceUnavailable, Code: "service_unavailable", Message: "api.events.too_many",
		})
		return
	}
	defer subscription.Close()
	prepareStream(w)
	flusher.Flush()
	s.streamEvents(w, r, subscription, flusher)
}

func prepareStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "retry: %d\n\n", sseRetry.Milliseconds())
}

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request, subscription *Subscription, flusher http.Flusher) {
	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-subscription.Events:
			if !open {
				return
			}
			writeEvent(w, event)
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, event Event) {
	_, _ = fmt.Fprintf(w, "event: %s\n", event.Channel)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", event.Payload)
}

func streamChannels(value string) []string {
	channels := strings.Split(value, ",")
	for _, channel := range channels {
		if !knownChannel(channel) {
			return nil
		}
	}
	return channels
}

func knownChannel(channel string) bool {
	switch channel {
	case ChannelLogs, ChannelQuota, ChannelStatus:
		return true
	default:
		return false
	}
}

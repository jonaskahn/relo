// Response writers: JSON bodies and localized error envelopes.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/i18n"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}

type refusal struct {
	Status  int
	Code    string
	Message string
	Data    map[string]any
	Detail  error
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, refused refusal) {
	writeError(w, refused.Status, refused.Code, refused.render(s.translator(r)))
}

func (refused refusal) render(translator *i18n.Translator) string {
	if refused.Detail != nil {
		return refused.Detail.Error()
	}
	return translator.Text(refused.Message, refused.Data)
}

func (s *Server) translator(r *http.Request) *i18n.Translator {
	if tag := requestedLanguage(r); tag != "" {
		return s.opts.Catalogs.Translate(tag)
	}
	return s.opts.Catalogs.Translate(s.operatorLanguage(r.Context()))
}

func (s *Server) operatorLanguage(ctx context.Context) string {
	if s.opts.Settings != nil {
		if tag, err := s.opts.Settings.Language(); err == nil && tag != i18n.Auto {
			return tag
		}
	}
	return s.opts.Language
}

func requestedLanguage(r *http.Request) string {
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(part, ";")
		if matched, ok := i18n.Match(strings.TrimSpace(tag)); ok {
			return matched
		}
	}
	return ""
}

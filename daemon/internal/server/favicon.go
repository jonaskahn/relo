// The mark every Relo address shows a browser that opens it.
package server

import (
	_ "embed"
	"net/http"
)

// favicon is the icon this binary serves on every listener, generated from
// assets/logo.svg by make icons.
//
//go:embed assets/favicon.ico
var favicon []byte

func serveFavicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(favicon)
}

// Daemon log export streams the current log as a browser-downloadable ZIP.
package server

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"

	appstatus "github.com/jonaskahn/relo/internal/application/status"
)

func (s *Server) handleDaemonLogExport(w http.ResponseWriter, r *http.Request) {
	reads, ok := s.statusOrUnavailable(w, r)
	if !ok {
		return
	}
	log, err := reads.OpenDaemonLog()
	if err != nil {
		if errors.Is(err, appstatus.ErrDaemonLogNotFound) {
			s.fail(w, r, refusal{Status: http.StatusNotFound, Code: "not_found", Detail: err})
			return
		}
		writeServiceError(w, err)
		return
	}
	defer func() { _ = log.Close() }()

	stamp := s.opts.Clock.Now().UTC().Format("20060102T150405Z")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="relo-daemon-log-%s.zip"`, stamp))
	w.Header().Set("Cache-Control", "no-store")
	archive := zip.NewWriter(w)
	if err := writeLogArchive(archive, log); err != nil && s.opts.Logger != nil {
		s.opts.Logger.Debug("stream the daemon log export", "error", err)
	}
}

func writeLogArchive(archive *zip.Writer, log io.Reader) error {
	entry, err := archive.CreateHeader(&zip.FileHeader{Name: "daemon.log", Method: zip.Deflate})
	if err != nil {
		return err
	}
	if _, err := io.Copy(entry, log); err != nil {
		return err
	}
	return archive.Close()
}

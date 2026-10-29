package catalog

import (
	"context"
	"errors"
	"github.com/jonaskahn/relo/internal/catalog"
	"testing"
	"time"
)

func TestFreePoolProbeAndStaleCommitAreRefused(t *testing.T) {
	ctx := context.Background()
	for _, id := range []string{catalog.OpenCodeFreeTemplate, catalog.KiloFreeTemplate} {
		t.Run(id, func(t *testing.T) {
			svc := New(Options{FreeConnections: func() bool { return false }})
			_, err := svc.ProbeProvider(ctx, ProbeRequest{TemplateID: id})
			if !errors.Is(err, ErrFreeConnectionsDisabled) {
				t.Fatalf("probe error = %v", err)
			}
			svc.probes.Store("stale", &probeSession{ID: "stale", ExpiresAt: time.Now().Add(time.Minute), Provider: catalog.ConnectionRecord{TemplateID: id}})
			_, err = svc.CommitProbe(ctx, "stale", CommitProbeRequest{})
			if !errors.Is(err, ErrFreeConnectionsDisabled) {
				t.Fatalf("commit error = %v", err)
			}
		})
	}
}

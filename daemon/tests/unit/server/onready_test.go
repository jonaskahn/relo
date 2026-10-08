package server_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/server"
)

func TestOnReady(t *testing.T) {
	t.Run("ready reports the bound address", func(t *testing.T) {
		harness := newHarness(t)
		ready := make(chan string, 1)
		harness.server = harness.newServerWithConfig(freeConfig(t), defaultEntry())
		harness.server.OnReady(func(addrs server.Addrs) { ready <- addrs.Management })

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		serving := make(chan error, 1)
		go func() { serving <- harness.server.Start(ctx) }()

		var addr string
		select {
		case addr = <-ready:
		case <-time.After(5 * time.Second):
			t.Fatal("OnReady was not called")
		}
		if addr != harness.server.Addr() {
			t.Fatalf("OnReady address = %q, want %q", addr, harness.server.Addr())
		}
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			t.Fatalf("the reported address does not accept connections: %v", err)
		}
		_ = conn.Close()

		cancel()
		if err := <-serving; err != nil {
			t.Fatalf("Start() error = %v", err)
		}
	})
}

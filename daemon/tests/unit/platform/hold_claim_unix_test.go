//go:build unix

package platform_test

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"

	"github.com/jonaskahn/relo/internal/platform"
)

func holdClaim() {
	home := os.Getenv("RELO_TEST_CLAIM_HOME")
	port := os.Getenv("RELO_TEST_CLAIM_PORT")
	file, err := os.OpenFile(home+"/daemon.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		os.Exit(2)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		os.Exit(2)
	}
	fmt.Println("held")
	_ = os.Stdout.Sync()
	address := net.JoinHostPort("127.0.0.1", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		os.Exit(3)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go func() { _ = server.Serve(listener) }()
	body := fmt.Sprintf(`{"address":%q,"instance_id":"waited","pid":%d}`, address, os.Getpid())
	if err := os.WriteFile(platform.RuntimePath(home), []byte(body), 0o600); err != nil {
		os.Exit(4)
	}
	select {}
}

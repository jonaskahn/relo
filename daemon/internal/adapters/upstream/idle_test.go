package upstream

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// TestIdleBodyResetsOnRead covers a body that keeps sending inside the call
// wait: every read resets the silence timer, so the stream runs past the wait
// total and ends cleanly.
func TestIdleBodyResetsOnRead(t *testing.T) {
	reader, writer := io.Pipe()
	body := newIdleBody(context.Background(), reader, 60*time.Millisecond, nil)

	go func() {
		for round := 0; round < 5; round++ {
			time.Sleep(20 * time.Millisecond)
			if _, err := writer.Write([]byte("x")); err != nil {
				return
			}
		}
		_ = writer.Close()
	}()

	timeout := time.After(2 * time.Second)
	buffer := make([]byte, 1)
	for {
		select {
		case <-timeout:
			t.Fatal("the stream never ended")
		default:
		}
		n, err := body.Read(buffer)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("Read() = %d, %v, want the stream to keep sending", n, err)
		}
	}
}

// TestIdleBodyStalls covers a body that goes silent: the read fails with the
// stall sentinel, which the relay maps to a retryable gateway timeout.
func TestIdleBodyStalls(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	body := newIdleBody(context.Background(), reader, 30*time.Millisecond, nil)

	started := time.Now()
	_, err := body.Read(make([]byte, 1))
	if !errors.Is(err, ErrUpstreamStall) {
		t.Fatalf("Read() error = %v, want ErrUpstreamStall", err)
	}
	if elapsed := time.Since(started); elapsed < 30*time.Millisecond {
		t.Fatalf("Read() stalled after %v, want at least the call wait", elapsed)
	}
}

// TestIdleBodyParentCancelWins covers a client that disconnects while the
// body is silent: the parent's own error is reported rather than a stall, so
// a cancelled request never looks like a provider timeout.
func TestIdleBodyParentCancelWins(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	body := newIdleBody(ctx, reader, time.Minute, nil)

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := body.Read(make([]byte, 1))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Read() error = %v, want the parent cancellation", err)
	}
}

// TestHeaderClockDefusesAndFires covers the wait for headers: a clock that is
// stopped once the answer arrives never cancels the body, and one whose wait
// runs out does.
func TestHeaderClockDefusesAndFires(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calm, calmCtx := startHeaderClock(ctx, time.Minute)
	if calm.defused() {
		t.Fatal("a stopped clock reported that it fired")
	}
	if calmCtx.Err() != nil {
		t.Fatal("a defused clock cancelled the request context")
	}
	calm.release()
	if calmCtx.Err() == nil {
		t.Fatal("release() left the request context alive")
	}

	expiring, expiringCtx := startHeaderClock(ctx, 20*time.Millisecond)
	select {
	case <-expiringCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the clock never cancelled its context")
	}
	if !expiring.defused() {
		t.Fatal("an expired clock reported that it was stopped in time")
	}
}

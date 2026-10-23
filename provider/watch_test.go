package provider

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// a body that never answers: Read blocks until Close, like a socket held open in silence
func silent() io.ReadCloser {
	r, _ := io.Pipe()
	return r
}

func readAll(t *testing.T, rc io.ReadCloser) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(rc); done <- err }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("read still blocked: the watch never fired")
		return nil
	}
}

func TestWatchStallsASilentStream(t *testing.T) {
	rc := Watch(context.Background(), silent(), 20*time.Millisecond)
	defer rc.Close()
	if err := readAll(t, rc); !errors.Is(err, ErrStalled) {
		t.Fatalf("err = %v, want ErrStalled", err)
	}
}

func TestWatchCancelUnblocksWithTheContextsWord(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rc := Watch(ctx, silent(), 0)
	defer rc.Close()
	time.AfterFunc(20*time.Millisecond, cancel)
	if err := readAll(t, rc); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestWatchBytesKeepAStreamAlive(t *testing.T) {
	r, w := io.Pipe()
	rc := Watch(context.Background(), r, 60*time.Millisecond)
	defer rc.Close()
	go func() {
		for i := 0; i < 6; i++ { // 6 × 25ms = 150ms of talk: well past one stall window
			time.Sleep(25 * time.Millisecond)
			_, _ = w.Write([]byte("data: ping\n"))
		}
		w.Close()
	}()
	if err := readAll(t, rc); err != nil {
		t.Fatalf("a talking stream was cut: %v", err)
	}
}

func TestWatchCloseReleasesTheWatchers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rc := Watch(ctx, silent(), time.Hour).(*watched)
	_ = rc.Close()
	if rc.unhook() {
		t.Fatal("the context hook outlived Close")
	}
	if rc.timer.Stop() {
		t.Fatal("the stall timer outlived Close")
	}
}

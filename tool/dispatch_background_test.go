package tool

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A panicking dispatch runner must never take the process down: the background
// goroutine recovers and wakes the dispatcher with a @S FAIL.
func TestDispatchBackgroundRecoversPanic(t *testing.T) {
	woke := make(chan string, 1)
	opts := DispatchOptions{
		Enabled:    true,
		Background: true,
		Wake: func(agent, report string, failed bool) {
			if failed {
				woke <- report
			}
		},
	}
	tool := DispatchTool(opts, func(ctx context.Context, env Env, c Commission) (DispatchReport, error) {
		panic("boom in the runner")
	})
	res := tool.Run(context.Background(), Env{Root: t.TempDir()}, json.RawMessage(`{"agent":"zone-a","ask":"findings","background":true}`))
	if res.Err {
		t.Fatalf("dispatch start: %s", res.Output)
	}
	select {
	case report := <-woke:
		if !strings.Contains(report, "@S FAIL") || !strings.Contains(report, "boom in the runner") {
			t.Fatalf("wake report = %q", report)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no wake within 5s")
	}
}

// A background dispatch is bounded: when the runner sleeps forever, the run is
// cancelled at BackgroundTimeout and the dispatcher is woken with the context's error.
func TestDispatchBackgroundTimeout(t *testing.T) {
	woke := make(chan string, 1)
	started := make(chan struct{})
	var sawDeadline atomic.Bool
	opts := DispatchOptions{
		Enabled:           true,
		Background:        true,
		BackgroundTimeout: 120 * time.Millisecond,
		Wake: func(agent, report string, failed bool) {
			if failed {
				woke <- report
			}
		},
	}
	tool := DispatchTool(opts, func(ctx context.Context, env Env, c Commission) (DispatchReport, error) {
		defer close(started)
		if _, ok := ctx.Deadline(); !ok {
			close(started)
			t.Error("background run without a deadline")
		} else {
			sawDeadline.Store(true)
		}
		<-ctx.Done()
		return DispatchReport{}, ctx.Err()
	})
	res := tool.Run(context.Background(), Env{Root: t.TempDir()}, json.RawMessage(`{"agent":"zone-a","ask":"findings","background":true}`))
	if res.Err {
		t.Fatalf("dispatch start: %s", res.Output)
	}
	<-started
	if !sawDeadline.Load() {
		t.Fatal("the run's context carried no deadline")
	}
	select {
	case report := <-woke:
		if !strings.Contains(report, "context deadline exceeded") {
			t.Fatalf("wake report = %q", report)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no wake within 5s")
	}
}

// spawnWatch is the phantom-dispatch guard (N5): a background dispatch that never registers its
// child journal must not pass unnoticed — the dispatcher is woken with a spawn-failed FAIL.
func TestSpawnWatchFailsOnNoJournal(t *testing.T) {
	root := t.TempDir()
	woke := make(chan string, 1)
	wake := func(agent, report string, failed bool) {
		if failed {
			woke <- report
		}
	}
	spawnWatch(Env{Root: root, WorkspaceDir: ".agent-one"}, "zone-ghost", time.Now().UnixNano(), wake)
	select {
	case rep := <-woke:
		if !strings.Contains(rep, "@S FAIL") || !strings.Contains(rep, "spawn failed") {
			t.Fatalf("%q", rep)
		}
	case <-time.After(spawnGrace + 3*time.Second):
		t.Fatal("spawn never verified, never reported")
	}
}

// when the child journal lands with content within the grace window, spawnWatch stays silent.
func TestSpawnWatchSilentWhenJournalLands(t *testing.T) {
	root := t.TempDir()
	dir := root + "/.agent-one/instruments/loop"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	silent := make(chan string, 1)
	wake := func(agent, report string, failed bool) { silent <- report }
	dispatched := time.Now().UnixNano()
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(dir+"/zone-worker-20261002190000-c3a1.jsonl", []byte(`{"t":"run"}`+"\n"), 0o644)
	}()
	spawnWatch(Env{Root: root, WorkspaceDir: ".agent-one"}, "zone-worker", dispatched, wake)
	select {
	case rep := <-silent:
		t.Fatalf("watched a journal and still reported: %q", rep)
	case <-time.After(spawnGrace + 1*time.Second):
	}
}

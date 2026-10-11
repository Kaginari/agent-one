package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/memory"
)

// F1 (audit 2026-10-05): a missing or stale long-memory index heals once at boot —
// recall never rebuilds in-workspace (rebuildOnStale:false keeps that beat free), so the
// boot gate is the only place staleness dies before agents start recalling.
func TestBootHealsLongIndex(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", nil)
	w.write(".agent-one/log.md", "# chronicle\n\n### [2026-01-01] first\n\none note in the chronicle\n")
	w.write(".agent-one/config.yaml", `
models: {default: mock/m}
providers:
  mock: {enabled: true, script: .agent-one/tmp/session.json}
ui: {board: {autostart: false}}
`)
	w.write(".agent-one/tmp/session.json", "[]")
	open := func() *App {
		a, err := New(Options{Dist: "agent-one", Root: w.root, Cwd: w.root, Home: w.home, Env: func(k string) string {
			if k == "HOME" {
				return w.home
			}
			return ""
		}, In: strings.NewReader(""), Out: &strings.Builder{}, Err: &strings.Builder{}, Quiet: false, IsTTY: func() bool { return false }, NoBoard: true})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	stale := func() string {
		mw, err := memory.OpenIn(w.root, ".agent-one")
		if err != nil {
			t.Fatal(err)
		}
		idx, why := mw.LoadIndex()
		if why != "" {
			return why
		}
		if h := mw.IndexHoles(idx, ""); len(h) > 0 {
			return strings.Join(h, "; ")
		}
		return ""
	}

	// boot 1: no index at all — boot builds it and says so as a hole note
	a := open()
	indexPath := filepath.Join(w.root, ".agent-one", "memory", "long", "index.json")
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("boot did not build the long index: %v", err)
	}
	healed := false
	for _, h := range a.Holes() {
		if strings.Contains(h, "boot re-indexed") {
			healed = true
		}
	}
	if !healed {
		t.Fatalf("no boot-reindex hole note; holes: %v", a.Holes())
	}
	a.Close()
	if why := stale(); why != "" {
		t.Fatalf("index stale right after boot heal: %s", why)
	}

	// rot it: a source newer than its stored stamp
	w.write(".agent-one/log.md", "# chronicle\n\n### [2026-01-01] first\n\none note\n\n### [2026-01-02] second\n\na newer note\n")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(w.root, ".agent-one", "log.md"), future, future); err != nil {
		t.Fatal(err)
	}
	if why := stale(); why == "" {
		t.Fatal("fixture failed: index not stale after source rewrite")
	}

	// boot 2: staleness heals at boot, recall would ride zero nag holes
	a2 := open()
	defer a2.Close()
	if why := stale(); why != "" {
		t.Fatalf("stale index survived boot: %s", why)
	}
}

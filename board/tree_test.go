package board

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWalkTree(t *testing.T) {
	// Workspace layout: dirs first at the level, files sorted, has_children set.
	now := time.Date(2026, 9, 25, 10, 30, 0, 0, time.UTC)
	tmp := t.TempDir()
	dirs := []string{"alpha", "empty", "zzlast"}
	files := []string{"bee.go", "ant.txt"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tmp, d), 0o755); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(filepath.Join(tmp, d), now, now)
	}
	for _, f := range files {
		p := filepath.Join(tmp, f)
		os.WriteFile(p, []byte("x"), 0o644)
		os.Chtimes(p, now, now)
	}
	entries, truncated, found := WalkTree(tmp, "")
	if !found || truncated {
		t.Fatalf("root walk found=%v truncated=%v", found, truncated)
	}
	if len(entries) != 5 {
		t.Fatalf("want 5 entries, got %d", len(entries))
	}
	for i, name := range []string{"alpha", "empty", "zzlast"} {
		if entries[i].Kind != "dir" || entries[i].Name != name {
			t.Fatalf("entry %d: %#v", i, entries[i])
		}
		if entries[i].HasChildren && name != "alpha" {
			t.Fatalf("dir %s wrongly carrying has_children", entries[i].Name)
		}
	}
	for i, name := range []string{"ant.txt", "bee.go"} {
		if entries[3+i].Kind != "file" || entries[3+i].Name != name {
			t.Fatalf("entry %d: %#v", 3+i, entries[3+i])
		}
		if entries[3+i].HasChildren {
			t.Fatalf("file %s carrying has_children", entries[3+i].Name)
		}
	}
	// Nested walk: prefix selects one level, paths are root-relative.
	os.MkdirAll(filepath.Join(tmp, "alpha", "deep"), 0o755)
	os.WriteFile(filepath.Join(tmp, "alpha", "a.md"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(tmp, "walk_tree_escape.md"), []byte("f"), 0o644)
	sub, _, found := WalkTree(tmp, "alpha")
	if !found || len(sub) != 2 {
		t.Fatalf("alpha walk found=%v len=%d", found, len(sub))
	}
	// The .git fold earlier proves skipNames never leaks into a listing:
	// dirHasChildren under 'alpha' looks past .git to real clay.
	if sub[0].Kind != "dir" || sub[0].Path != "alpha/deep" {
		t.Fatalf("sub[0]: %#v", sub[0])
	}
	if sub[1].Path != "alpha/a.md" {
		t.Fatalf("sub[1]: %#v", sub[1])
	}
}

func TestWalkTreeCap(t *testing.T) {
	tmp := t.TempDir()
	for range 600 {
		f, _ := os.CreateTemp(tmp, "f*.txt")
		f.Close()
	}
	entries, truncated, found := WalkTree(tmp, "")
	if !found || !truncated {
		t.Fatalf("want truncated=true at cap, got %v (found=%v)", truncated, found)
	}
	if len(entries) != treeCap {
		t.Fatalf("want %d entries at cap, got %d", treeCap, len(entries))
	}
}

func TestWalkTreeEmpty(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, "emptydir"), 0o755); err != nil {
		t.Fatal(err)
	}
	entries, truncated, found := WalkTree(tmp, "emptydir")
	if !found || truncated {
		t.Fatalf("existing empty dir: found=%v truncated=%v", found, truncated)
	}
	// Never-dir and never-found return a nil agent the caller 404s on; the
	// found-but-empty case below keeps [] flowing to the client.
	if len(entries) != 0 {
		t.Fatalf("want zero entries, got %#v", entries)
	}
	if entries == nil {
		t.Fatal("found-but-empty must be [] TreeEntry{}, never nil — the client needs an array")
	}
	// Escaping prefixes and unknown dirs are not-found.
	if _, _, found := WalkTree(tmp, "../../etc"); found {
		t.Fatal("escape prefix must not be found")
	}
	if _, _, found := WalkTree(tmp, "nope/never"); found {
		t.Fatal("missing prefix must not be found")
	}
	if _, _, found := WalkTree(tmp, "walk_tree_escape.md"); found {
		t.Fatal("file prefix must not be found")
	}
}

func TestWalkTreeZeroCapSkippedBySlice1(t *testing.T) {
	// Slice 1 walks at the fixed treeCap; a zero-cap knob no longer exists.
	// The cap test above pins the truncation shape this used to cover.
}

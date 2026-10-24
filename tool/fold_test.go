package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fold restores a kept payload byte for byte by digest or kept-at path, reads nothing off the
// shelf, and refuses bytes that no longer hash to their name.
func TestFoldTool(t *testing.T) {
	env, root := workspace(t)
	env.RunID = "zone-a-20261024-000000-0001"
	payload := "exact\n  bytes, no trailing newline"
	digest, kept, err := FoldKeep(FoldRunDir(root, "", env.RunID), payload)
	if err != nil {
		t.Fatal(err)
	}
	if d2, k2, _ := FoldKeep(FoldRunDir(root, "", env.RunID), payload); d2 != digest || k2 != kept {
		t.Fatal("the same payload keeps under the same name")
	}
	fold := FoldTool()
	run := func(in string) Result { return fold.Run(context.Background(), env, json.RawMessage(in)) }
	rel, _ := filepath.Rel(root, kept)
	for _, in := range []string{`{"digest":"` + digest + `"}`, `{"digest":"` + digest + `.txt"}`, `{"path":"` + rel + `"}`} {
		if r := run(in); r.Err || r.Output != payload {
			t.Fatalf("%s: %+v", in, r)
		}
	}
	os.WriteFile(filepath.Join(root, "x.txt"), []byte(payload), 0o644)
	if r := run(`{"path":"x.txt"}`); !r.Err || !strings.Contains(r.Output, "not on this run's fold shelf") {
		t.Fatalf("a path off the shelf: %+v", r)
	}
	if r := run(`{"digest":"../../x"}`); !r.Err {
		t.Fatalf("a digest never walks off the shelf: %+v", r)
	}
	os.WriteFile(kept, []byte("tampered"), 0o644)
	if r := run(`{"digest":"` + digest + `"}`); !r.Err || !strings.Contains(r.Output, "no longer hashes") {
		t.Fatalf("tampered bytes are not restored: %+v", r)
	}
	if fold.Class != Read {
		t.Fatal("fold is a read")
	}
}

// The shelf is per run: an agent restores only what its own run folded, never another agent's raw
// output — not by digest, not by the kept-at path, not with no run acting.
func TestFoldRestoresOnlyTheActingRunsOwn(t *testing.T) {
	env, root := workspace(t)
	payload := "another agent's raw output"
	digest, kept, err := FoldKeep(FoldRunDir(root, "", "zone-a-20261024-000000-0001"), payload)
	if err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(root, kept)
	for _, run := range []string{"zone-b-20261024-000000-0001", "", "../zone-a-20261024-000000-0001"} {
		env.RunID = run
		for _, in := range []string{`{"digest":"` + digest + `"}`, `{"path":"` + rel + `"}`} {
			if r := FoldTool().Run(context.Background(), env, json.RawMessage(in)); !r.Err {
				t.Fatalf("%s as run %q: restored a payload it never folded: %+v", in, run, r)
			}
		}
	}
	env.RunID = "zone-a-20261024-000000-0001"
	if r := FoldTool().Run(context.Background(), env, json.RawMessage(`{"path":"`+rel+`"}`)); r.Err || r.Output != payload {
		t.Fatalf("the folding run's own restore: %+v", r)
	}
}

// The shelf does not keep raw outputs forever: a drawer (or a loose file of the old workspace-wide
// shelf) untouched past the keep window goes; a fresh one stays; keep 0 sweeps nothing.
func TestFoldSweepDropsStaleRuns(t *testing.T) {
	root := t.TempDir()
	shelf := FoldDir(root, "")
	FoldKeep(FoldRunDir(root, "", "old-run"), "old")
	FoldKeep(FoldRunDir(root, "", "new-run"), "new")
	os.WriteFile(filepath.Join(shelf, "loose.txt"), []byte("loose"), 0o644)
	stale := time.Now().Add(-10 * 24 * time.Hour)
	os.Chtimes(filepath.Join(shelf, "old-run"), stale, stale)
	os.Chtimes(filepath.Join(shelf, "loose.txt"), stale, stale)
	if n := FoldSweep(shelf, 0, time.Now()); n != 0 {
		t.Fatalf("keep 0 swept %d", n)
	}
	if n := FoldSweep(shelf, 7*24*time.Hour, time.Now()); n != 2 {
		t.Fatalf("swept %d, want the stale drawer and the loose file", n)
	}
	if _, err := os.Stat(FoldRunDir(root, "", "new-run")); err != nil {
		t.Fatal("a fresh drawer was swept")
	}
}

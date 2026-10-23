package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fold restores a kept payload byte for byte by digest or kept-at path, reads nothing off the
// shelf, and refuses bytes that no longer hash to their name.
func TestFoldTool(t *testing.T) {
	env, root := workspace(t)
	payload := "exact\n  bytes, no trailing newline"
	digest, kept, err := FoldKeep(FoldDir(root, ""), payload)
	if err != nil {
		t.Fatal(err)
	}
	if d2, k2, _ := FoldKeep(FoldDir(root, ""), payload); d2 != digest || k2 != kept {
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
	if r := run(`{"path":"x.txt"}`); !r.Err || !strings.Contains(r.Output, "not on the fold shelf") {
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

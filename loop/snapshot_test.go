package loop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/provider/mock"
)

func TestSnapshotSkipsHeavyDirs(t *testing.T) {
	e := engine(t, mock.New(mock.Text("x")))
	plant := func(rel string) {
		t.Helper()
		p := filepath.Join(e.Root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// dependency / build forests an agent never authors
	plant("app/node_modules/pkg/index.js")
	plant("svc/.venv/lib/python/os.py")
	plant("crate/target/debug/build.bin")
	plant("web/dist/bundle.js")
	// the authored surface that must stay watched
	plant("app/index.js")
	plant("svc/main.py")

	snap, why := e.stampTree()
	if why != "" {
		t.Fatal(why)
	}
	for rel := range snap {
		if strings.Contains(rel, "node_modules") || strings.Contains(rel, ".venv") || strings.Contains(rel, "target") || strings.Contains(rel, "dist") {
			t.Fatalf("heavy dir walked: %s", rel)
		}
	}
	for _, want := range []string{"app/index.js", "svc/main.py", "README.md"} {
		if _, ok := snap[want]; !ok {
			t.Fatalf("authored file missing from stamp: %s", want)
		}
	}
}

func TestUnwatchedNamesHeavyDirsAtAnyDepth(t *testing.T) {
	e := engine(t, mock.New(mock.Text("x")))
	for _, rel := range []string{"node_modules", "a/b/node_modules", ".venv", "x/__pycache__", "target", "y/dist", ".agent-one/memory"} {
		if !e.unwatched(rel) {
			t.Fatalf("%s must be unwatched", rel)
		}
	}
	for _, rel := range []string{"vendor", "app/index.js", "targets", "distributor", "src/memory"} {
		if e.unwatched(rel) {
			t.Fatalf("%s must stay watched", rel)
		}
	}
}

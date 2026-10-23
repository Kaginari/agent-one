package onto

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCachedMovesOnlyWithItsInputs(t *testing.T) {
	root := t.TempDir()
	doc := filepath.Join(root, ".agent-one", "zone", "auth", "README.md")
	if err := os.MkdirAll(filepath.Dir(doc), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc, []byte("# zone-auth\n\n- **Rank:** Zone worker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := DefaultLayout()
	a, err := Cached(root, l)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := Cached(root, l); b != a {
		t.Fatal("an unmoved workspace was loaded again")
	}
	before := Stamp(root, l)
	later := time.Now().Add(time.Second)
	_ = os.Chtimes(doc, later, later)
	if Stamp(root, l) == before {
		t.Fatal("an edited doc left the stamp alone")
	}
	c, _ := Cached(root, l)
	if c == a {
		t.Fatal("an edited doc served the old workspace")
	}
	if err := os.MkdirAll(filepath.Join(root, ".agent-one", "zone", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, ".agent-one", "zone", "api", "README.md"), []byte("# zone-api\n"), 0o644)
	d, _ := Cached(root, l)
	if d == c || len(d.Graph.Instances(Ao("ZoneWorker"))) != 2 {
		t.Fatalf("a new member is not on the cached workspace: %d zone workers", len(d.Graph.Instances(Ao("ZoneWorker"))))
	}
}

package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The file kit wave (Operator 2026-10-03): read/write/edit/delete shaped for model work —
// windows and tails and did-you-mean for reads, drift receipts and zone anchors for writes,
// pointer errors for edits, and a delete that archives instead of destroying.

func zoneEnv(t *testing.T) (Env, string) {
	t.Helper()
	env, root := workspace(t)
	env.ZoneOwner = func(rel string) string {
		if strings.HasPrefix(rel, "memory/") {
			return "zone-memory"
		}
		return ""
	}
	return env, root
}

func TestWriteDriftAndZone(t *testing.T) {
	env, root := zoneEnv(t)
	os.MkdirAll(filepath.Join(root, "memory"), 0o755)
	r := run(t, WriteTool(), env, `{"path":"memory/first.md","content":"hello"}`)
	if r.Err || !strings.Contains(r.Output, "5 bytes") || strings.Contains(r.Output, "drift") {
		t.Fatalf("create carries no drift receipt: %q", r.Output)
	}
	if !strings.Contains(r.Output, "zone of zone-memory") {
		t.Fatalf("create names the zone's persistent agent: %q", r.Output)
	}
	r = run(t, WriteTool(), env, `{"path":"memory/first.md","content":"hello"}`)
	if r.Err || !strings.Contains(r.Output, "no drift") {
		t.Fatalf("same-bytes overwrite says so: %q", r.Output)
	}
	r = run(t, WriteTool(), env, `{"path":"memory/first.md","content":"hello world"}`)
	if r.Err || !strings.Contains(r.Output, "over 5 bytes now 11 (drift +6)") {
		t.Fatalf("overwrite names its drift: %q", r.Output)
	}
}

func TestEditHints(t *testing.T) {
	env, root := zoneEnv(t)
	os.MkdirAll(filepath.Join(root, "memory"), 0o755)
	os.WriteFile(filepath.Join(root, "memory/f.md"), []byte("alphaGate beta\ngammaGate delta\nepsilonGate zeta\n"), 0o644)
	r := run(t, EditTool(), env, `{"path":"memory/f.md","old":"gammaGate zulu","new":"x"}`)
	if !r.Err || !strings.Contains(r.Output, "not found") || !strings.Contains(r.Output, "closest: line 2") {
		t.Fatalf("not-found carries a pointer: %q", r.Output)
	}
	r = run(t, EditTool(), env, `{"path":"memory/f.md","old":"Gate","new":"x"}`)
	if !r.Err || !strings.Contains(r.Output, "3 times") || !strings.Contains(r.Output, "lines 1,2,3") {
		t.Fatalf("multi-match names the lines: %q", r.Output)
	}
	r = run(t, EditTool(), env, `{"path":"memory/f.md","old":"alphaGate beta","new":"a0"}`)
	if r.Err || !strings.Contains(r.Output, "zone of zone-memory") {
		t.Fatalf("a landed edit names its zone: %q", r.Output)
	}
}

func TestReadKitUpgrades(t *testing.T) {
	env, root := zoneEnv(t)
	os.MkdirAll(filepath.Join(root, "memory"), 0o755)
	os.WriteFile(filepath.Join(root, "memory/lines.md"), []byte("l1\nl2\nl3\nl500fat "+strings.Repeat("x", 2500)+"\nl5\n"), 0o644)

	r := run(t, ReadTool(), env, `{"path":"memory/lines.md","offset":1,"limit":-2}`)
	if r.Err || !strings.Contains(r.Output, "l5") || strings.Contains(r.Output, "l1\t") {
		t.Fatalf("negative limit tails: %q", r.Output)
	}
	r = run(t, ReadTool(), env, `{"path":"memory/lines.md","offset":4,"limit":1}`)
	if r.Err || !strings.Contains(r.Output, "[truncated]") {
		t.Fatalf("long lines clamp: %q", r.Output)
	}
	r = run(t, ReadTool(), env, `{"path":"memory/lins.md"}`)
	if !r.Err || !strings.Contains(r.Output, "did you mean") || !strings.Contains(r.Output, "lines.md") {
		t.Fatalf("a miss names its siblings: %q", r.Output)
	}
	r = run(t, ReadTool(), env, `{"paths":["memory/lines.md:5:5","memory/lines.md:1"]}`)
	if r.Err || !strings.Contains(r.Output, "l5") || !strings.Contains(r.Output, "l1") {
		t.Fatalf("batch honours per-path windows: %q", r.Output)
	}
}

// The archaeologist's delete: nothing destroyed, everything restorable; records refuse.
func TestDeleteArchives(t *testing.T) {
	env, root := zoneEnv(t)
	os.MkdirAll(filepath.Join(root, "memory"), 0o755)
	os.WriteFile(filepath.Join(root, "memory/old.md"), []byte("the past\n"), 0o644)

	r := run(t, DeleteTool(), env, `{"path":"memory/old.md"}`)
	if r.Err {
		t.Fatalf("delete: %v", r.Output)
	}
	if _, err := os.Stat(filepath.Join(root, "memory/old.md")); !os.IsNotExist(err) {
		t.Fatal("file left the tree")
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-one", "attic")); err != nil {
		t.Fatalf("no attic: %v", err)
	}
	found, ledgerLine := false, ""
	filepath.Walk(filepath.Join(root, ".agent-one", "attic"), func(p string, fi os.FileInfo, err error) error {
		if strings.HasSuffix(p, "memory/old.md") {
			found = true
		}
		if strings.HasSuffix(p, "attic.jsonl") {
			if b, e := os.ReadFile(p); e == nil {
				ledgerLine = string(b)
			}
		}
		return nil
	})
	if !found || !strings.Contains(ledgerLine, "memory/old.md") {
		t.Fatalf("attic + ledger: found=%v ledger=%q", found, ledgerLine)
	}
	if !strings.Contains(r.Output, "archived") || !strings.Contains(r.Output, "zone of zone-memory") {
		t.Fatalf("the archive names its route and soil: %q", r.Output)
	}

	if r = run(t, DeleteTool(), env, `{"path":".agent-one/log.md"}`); !r.Err || !strings.Contains(r.Output, "Policy 4") {
		t.Fatalf("records refuse: %q", r.Output)
	}
	if c, _ := DeleteTool().Settle(env, []byte(`{"path":"/etc/passwd"}`)); c.Class != Outward {
		t.Fatalf("outside is outward: %+v", c)
	}
}

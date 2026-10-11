package onto

// Housekeeping seals the mechanical classes of the chain policy: homes for members
// without one, chain edges rewired to the policy parent — and never a silent rewrite
// of overlaps or prose bonds.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func minWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	ws := filepath.Join(root, ".agent-one")
	for _, mk := range []string{"", "ontology"} {
		if err := os.MkdirAll(filepath.Join(ws, mk), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// coord home (valid) and a domain whose home is missing.
	if err := os.MkdirAll(filepath.Join(ws, "coord", "core"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "coord", "core", "README.md"), []byte(
		"# coord-core\n\n- **Rank:** Coordinator\n- **Reports to:** orchestrator\n- **Purpose:** the heart.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "domain", "eng"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestHousekeepRegistersMissingHome(t *testing.T) {
	root := minWorkspace(t)
	rep, _, err := Housekeep(root, true, false)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, ".agent-one", "domain", "eng", "README.md")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("no home registered: %v (repairs %v)", err, rep.Repairs)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "Reports to:** coord-core") {
		t.Fatal("registered home must wire to the coord parent:\n" + string(b))
	}
	for _, n := range rep.Notes {
		if strings.Contains(n, "domain-eng has no doc") {
			t.Fatal("the note must be sealed in the after-pass: " + n)
		}
	}
}

func TestHousekeepCheckOnlyDoesNotWrite(t *testing.T) {
	root := minWorkspace(t)
	if _, _, err := Housekeep(root, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-one", "domain", "eng", "README.md")); !os.IsNotExist(err) {
		t.Fatal("check-only must not register homes")
	}
}

// A bond to a name with no live seat is dead: the heal rewrites the bond line
// to the policy parent of the seat's pattern class (domain owner → orchestrator) and seals it in
// an attributed repair; the audit after shows zero dead bonds.
func TestHousekeepHealsDeadBond(t *testing.T) {
	root := minWorkspace(t)
	p := filepath.Join(root, ".agent-one", "domain", "eng", "README.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	dead := `# domain-eng

- **Rank:** Domain owner
- **Reports to:** void-retired
- **Purpose:** backed by silence.
`
	if err := os.WriteFile(p, []byte(dead), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, _, err := Housekeep(root, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.DeadBonds) == 0 {
		t.Fatalf("the dead bond must flag as a finding (report %+v)", rep)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Reports to:** coord-core") {
		t.Fatalf("healed bond must name the policy parent (domain owner→coordinator tier), got:\n%s", b)
	}
	if strings.Contains(string(b), "void-retired") {
		t.Fatal("the dead name must be gone from the doc")
	}
	var saw bool
	for _, r := range rep.Repairs {
		if strings.Contains(r.What, "eng") && strings.Contains(r.What, "dead bond") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("no attributed repair line for the dead-bond heal: %+v", rep.Repairs)
	}
	rep2, _, err := Housekeep(root, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep2.DeadBonds) != 0 {
		t.Fatalf("after the heal the audit must show zero dead bonds, got %v", rep2.DeadBonds)
	}
}

// A bond to a living seat that is not the policy parent stays the human's edit:
// the heal act must never touch it.
func TestHousekeepLiveBondUntouched(t *testing.T) {
	root := minWorkspace(t)
	p := filepath.Join(root, ".agent-one", "domain", "eng", "README.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	live := `# domain-eng

- **Rank:** Domain owner
- **Reports to:** coord-core
- **Purpose:** watches the heart.
`
	if err := os.WriteFile(p, []byte(live), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	rep, _, err := Housekeep(root, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.DeadBonds) != 0 {
		t.Fatalf("a live bond is not a dead-bond finding: %v", rep.DeadBonds)
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("live bond doc must be byte-identical after a heal pass:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	rep2, _, err := Housekeep(root, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep2.DeadBonds) != 0 {
		t.Fatalf("second pass still no dead-bond findings: %v", rep2.DeadBonds)
	}
}

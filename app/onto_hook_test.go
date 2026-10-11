package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/workspace"
)

func intoResult() tool.Result { return tool.Result{} }

// the onto hooks: ledger rows always on write-class acts, one unowned-soil advisory per run;
// claimed soil never nags.
func TestOntoNote(t *testing.T) {
	root := t.TempDir()
	write := func(p, s string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(root+"/.agent-one/AGENT-ONE.md", "# policy\n")
	write(root+"/.agent-one/domain/engine/README.md",
		"# domain-engine\n\n- **Rank:** Domain owner\n- **Owns:** `onto/`\n- **Reports to:** coord-agent-one\n")
	a := testTmpApp(t, root)
	a.Cfg.Policy.Ownership.Enabled = true
	w, err := workspace.Open(root, workspace.Default(), workspace.Options{Ontology: true})
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	a.Workspace = w
	note := a.ontoNote()
	if note == nil {
		t.Fatal("ownership policy on + a workspace → the onto hooks ride")
	}
	jpath := filepath.Join(t.TempDir(), "run.jsonl")
	newSess := func() *loop.Session {
		return &loop.Session{RunID: "r-1", Engine: &loop.Engine{As: "coord-agent-one"}, Journal: &loop.Journal{Path: jpath}}
	}
	call := func(s *loop.Session, tool_, path string) string {
		in, _ := json.Marshal(map[string]string{"path": path})
		st := &loop.StepRecord{ID: "s1", Tool: tool_, Input: in}
		st.Result = intoResult() // success — status stamp timing should not matter
		return note(context.Background(), s, st)
	}
	s := newSess()
	// an owned path: ledger row, no advisory
	if out := call(s, "edit", "onto/workspace.go"); out != "" {
		t.Fatalf("claimed soil never nags: %q", out)
	}
	// unowned: advisory fires once, then silence (one per run by policy)
	out := call(s, "write", "docs/x.md")
	if !strings.Contains(out, "unowned soil") || !strings.Contains(out, "docs/x.md") {
		t.Fatalf("advisory must name the unowned path: %q", out)
	}
	if out := call(s, "write", "docs/y.md"); out != "" {
		t.Fatalf("one nudge per run: %q", out)
	}
	// reads don't count
	if out := call(s, "read", "onto/workspace.go"); out != "" {
		t.Fatalf("only write-class acts: %q", out)
	}
	// the journal grew two onto rows + the mapped agents
	data, _ := os.ReadFile(jpath)
	rows := strings.Count(string(data), `"t":"onto"`)
	if rows != 3 {
		t.Fatalf("ledger rows must be logged per write-class act (3 writes, the read-class absent): %d rows in\n%s", rows, string(data))
	}
	if !strings.Contains(string(data), `"owner":"domain-engine"`) || !strings.Contains(string(data), `"owner":"none"`) {
		t.Errorf("ledger must carry owner names: %s", string(data))
	}
	// ownership policy off → hook away (subagent-class safety)
	a.Cfg.Policy.Ownership.Enabled = false
	if a.ontoNote() != nil {
		t.Error("policy.ownership.enabled=false → no onto hooks")
	}
}

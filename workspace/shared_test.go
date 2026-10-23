package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/provider/mock"
	"github.com/Kaginari/agent-one/queue"
)

// TestQueueOutranks: the workspace's queue moves a holder's work for the holder and for anyone up
// its Reports-to chain (orchestrator tops it); a sibling or a child is refused.
func TestQueueOutranks(t *testing.T) {
	w := open(t, fixture(t))
	for _, c := range []struct {
		by, owner string
		want      bool
	}{
		{"domain-security", "zone-auth", true}, {"coord-core", "zone-auth", true}, {Orchestrator, "zone-auth", true},
		{Orchestrator, "", true}, {"zone-api", "zone-auth", false}, {"zone-auth", "domain-security", false}, {"coord-core", "", false},
	} {
		if got := w.Outranks(c.by, c.owner); got != c.want {
			t.Errorf("Outranks(%s, %s) = %v", c.by, c.owner, got)
		}
	}
	q := w.Queue()
	if q.Path() != filepath.Join(w.Dir(), "shared", "queue.jsonl") {
		t.Fatalf("the queue lives on the shared shelf: %s", q.Path())
	}
	q.Add(queue.Item{For: "zone-auth", Ask: "fix"}, "domain-security")
	q.Claim("q1", "zone-auth")
	if _, err := q.Requeue("q1", "zone-api", ""); !queue.IsRefusal(err) {
		t.Fatal("a sibling requeued work it does not hold")
	}
	if _, err := q.Requeue("q1", "domain-security", "reroute"); err != nil {
		t.Fatalf("the domain owner over the holder: %v", err)
	}
}

// TestOwnershipSharedShelfIsHarnessSoil: the shared shelf is the harness's — an agent's write
// there is refused like engine soil, naming the shelf.
func TestOwnershipSharedShelfIsHarnessSoil(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	m := mock.New(
		writeCall("1", ".agent-one/shared/queue.jsonl", "{}\n"),
		mock.Text("@S DONE\n@U team c\n@E 0"),
	)
	e := w.Engine("zone-auth", build(m))
	r, err := e.Run(context.Background(), "edit the queue by hand")
	if err != nil || r.Status != loop.Done {
		t.Fatalf("%v %+v", err, r)
	}
	if len(r.Steps) != 1 || r.Steps[0].Status != "refused" {
		t.Fatalf("the write must refuse: %+v", r.Steps)
	}
	if res := m.Requests[1].Messages[2].ToolResults[0]; !strings.Contains(res.Content, "shared shelf") {
		t.Fatalf("the refusal names the shelf: %q", res.Content)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agent-one", "shared", "queue.jsonl")); err == nil {
		t.Fatal("the write landed on the shelf")
	}
}

// TestSharedShelfMigrates: an old workspace's ledgers and value ledger move onto the shared shelf
// at Open — renamed whole, every line kept — and read back as before; new rows land there.
func TestSharedShelfMigrates(t *testing.T) {
	dir := fixture(t)
	inst := filepath.Join(dir, ".agent-one", "instruments")
	os.MkdirAll(inst, 0o755)
	ledger := `{"kind":"stress","agent":"zone-auth","value":2,"at":"2026-10-09T09:00:00Z"}` + "\n"
	val := `{"kind":"seed","at":"2026-10-01T00:00:00Z","from":"instruments/promotion.jsonl"}` + "\n" +
		`{"kind":"run","at":"2026-10-09T09:00:00Z","agent":"zone-auth","loop":"DONE","word":"DONE","status":"DONE"}` + "\n"
	os.WriteFile(filepath.Join(inst, "ledgers.jsonl"), []byte(ledger), 0o644)
	os.WriteFile(filepath.Join(inst, "value.jsonl"), []byte(val), 0o644)
	// a promotion tally too: a moved ledger already carries its seed, it is never seeded twice
	os.WriteFile(filepath.Join(inst, "promotion.jsonl"), []byte(`{"kind":"report","agent":"zone-api","status":"DONE","at":"2026-09-01T00:00:00Z"}`+"\n"), 0o644)
	w := open(t, dir)
	for name, want := range map[string]string{"ledgers.jsonl": ledger, "value.jsonl": val} {
		b, err := os.ReadFile(filepath.Join(w.SharedDir(), name))
		if err != nil || string(b) != want {
			t.Fatalf("%s moved whole: %v %q", name, err, b)
		}
		if _, err := os.Stat(filepath.Join(inst, name)); err == nil {
			t.Fatalf("%s left behind", name)
		}
	}
	if w.stress["zone-auth"] != 2 || len(w.val.rows) != 2 {
		t.Fatalf("the moved shelf read back: stress %v, %d value rows", w.stress, len(w.val.rows))
	}
	w.ledgerNote(ledgerRow{Kind: "consults", Agent: "zone-auth", Value: 1})
	if b, _ := os.ReadFile(filepath.Join(w.SharedDir(), "ledgers.jsonl")); strings.Count(string(b), "\n") != 2 {
		t.Fatalf("a new row lands on the shared shelf: %q", b)
	}
}

// TestSharedShelfReadsBothHomes: when an older binary wrote the old file again after the move,
// Open reads the old file's lines first, then the shared shelf's — nothing is lost, nothing
// moved over a file that exists.
func TestSharedShelfReadsBothHomes(t *testing.T) {
	dir := fixture(t)
	for rel, row := range map[string]string{
		"instruments/ledgers.jsonl": `{"kind":"stress","agent":"zone-auth","value":1,"at":"2026-10-09T09:00:00Z"}`,
		"shared/ledgers.jsonl":      `{"kind":"stress","agent":"zone-auth","value":3,"at":"2026-10-10T09:00:00Z"}` + "\n" + `{"kind":"stress","agent":"zone-api","value":2,"at":"2026-10-10T09:00:00Z"}`,
	} {
		p := filepath.Join(dir, ".agent-one", filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(row+"\n"), 0o644)
	}
	w := open(t, dir)
	if w.stress["zone-auth"] != 3 || w.stress["zone-api"] != 2 {
		t.Fatalf("both homes, old first: %v", w.stress)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agent-one", "instruments", "ledgers.jsonl")); err != nil {
		t.Fatal("a move over an existing shared file")
	}
}

// TestSharedShelfRefusesTheShell: an agent's shell write onto the shared shelf is refused like a
// write tool's — a heredoc appended to the queue cannot forge a claim on another agent's work.
func TestSharedShelfRefusesTheShell(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	q := w.Queue()
	if _, err := q.Add(queue.Item{For: "zone-api", Ask: "fix the api"}, "domain-security"); err != nil {
		t.Fatal(err)
	}
	forged := `{"id":"q1","rev":2,"op":"claim","state":"claimed","for":"zone-api","ask":"fix the api","owner":"zone-auth","by":"zone-auth"}`
	for i, cmd := range []string{
		"cat >> .agent-one/shared/queue.jsonl <<'EOF'\n" + forged + "\nEOF",
		"echo '" + forged + "' | tee -a ./.agent-one/shared/queue.jsonl",
		"cd .agent-one/shared && printf '%s\\n' '" + forged + "' >>queue.jsonl",
	} {
		m := mock.New(
			mock.Call("1", "bash", map[string]string{"command": cmd}),
			mock.Text("@S DONE\n@U team c\n@E 0"),
		)
		e := w.Engine("zone-auth", build(m))
		r, err := e.Run(context.Background(), "take the api's work")
		w.Close(e)
		if err != nil || len(r.Steps) != 1 || r.Steps[0].Status != "refused" {
			t.Fatalf("%d: the shell write must refuse: %v %+v %+v", i, err, r.Steps, r)
		}
		if res := m.Requests[1].Messages[2].ToolResults[0]; !strings.Contains(res.Content, "shared shelf") || !strings.Contains(res.Content, "queue tool") {
			t.Fatalf("%d: the refusal names the shelf and the agent's door to it: %q", i, res.Content)
		}
		if it, _, _ := q.Get("q1"); it.Owner != "" || it.State != queue.Held {
			t.Fatalf("%d: the forged line landed: %+v", i, it)
		}
	}
}

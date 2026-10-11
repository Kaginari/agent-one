package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/board"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/value"
	"github.com/Kaginari/agent-one/wire"
)

// wireResult is a Subagent's closed run: the loop's status, the agent's @S, its own @? lines and the
// holes the harness raised.
func wireResult(loopStatus, word string, self int, holes ...string) *loop.Result {
	r := &loop.Result{Status: loopStatus, IsWire: true, Report: wire.Report{Status: word}, Holes: holes}
	for i := 0; i < self; i++ {
		r.Report.Holes = append(r.Report.Holes, "an honest unknown")
	}
	return r
}

// The value ledger is always kept: a workspace whose table promotes nobody still measures its agents,
// and the record replays at Open.
func TestValueLedgerKeptWithoutPromotion(t *testing.T) {
	root := promoteWorkspace(t)
	w := openWorkspaceRanks(t, root, Ranks(DefaultRanks(Default())))
	if w.promote != nil {
		t.Fatal("the workspace's own table promotes nobody")
	}
	w.FoldMetrics("zone-login", "DONE", 0)
	w.FoldMetrics("zone-login", "FAIL", 0)
	if _, err := os.Stat(filepath.Join(root, ".agent-one", filepath.FromSlash(value.Shelf))); err != nil {
		t.Fatalf("the ledger is written with no promotion: %v", err)
	}
	w2 := openWorkspaceRanks(t, root, Ranks(DefaultRanks(Default())))
	if m := w2.Metrics("zone-login"); m["reports"] != 2 || m["passes"] != 1 {
		t.Fatalf("the ledger replays at Open: %v", m)
	}
}

// One definition of success: the ASL fold, the council, the board's activity and the score read
// the same word the same way — an agent's own @S FAIL on a clean loop fails everywhere; the human's
// DENIED is neutral everywhere.
func TestFourDefinitionsOfSuccessAgree(t *testing.T) {
	root := promoteWorkspace(t)
	w := openWorkspaceRanks(t, root, Ranks(DefaultRanks(Default())))
	subagent := SubagentOptions{StressEscalateAt: 5}

	failed := wireResult(loop.Done, "FAIL — the tests are red", 0)
	w.councilLedger("zone-login", verdictOf(failed), failed, subagent)
	w.foldResult("zone-login", "domain-auth", failed, w.stress["zone-login"], false, false)
	denied := wireResult(loop.Denied, "", 0)
	w.councilLedger("zone-login", verdictOf(denied), denied, subagent)
	w.foldResult("zone-login", "domain-auth", denied, w.stress["zone-login"], false, false)

	m := w.Metrics("zone-login")
	if m["fails"] != 1 || m["reports"] != 1 || m["neutral"] != 1 {
		t.Fatalf("the fold: one fail, the DENIED neutral: %v", m)
	}
	if w.stress["zone-login"] != 1 {
		t.Fatalf("the council: the FAIL strains once, the DENIED never: %d", w.stress["zone-login"])
	}
	act := board.BuildActivityDef([]board.Run{
		// the journal's end line: the loop said DONE, the run's word is the agent's FAIL
		{As: "zone-login", Status: failed.Status, Word: value.RunWord(failed.Status, failed.Report.Status), At: time.Now().UTC().Format(time.RFC3339Nano)},
		{As: "zone-login", Status: denied.Status, Word: value.RunWord(denied.Status, ""), At: time.Now().UTC().Format(time.RFC3339Nano)},
	}, nil, time.Now(), 28, w.ValueOpts().Def)
	if a := act.Agents["zone-login"]; a == nil || a.Total != 2 || a.Failed != 1 {
		t.Fatalf("the board's activity: one failed of two: %+v", a)
	}
	if v := w.Value("zone-login"); v.Score != 0 || v.Confidence != "low" {
		t.Fatalf("the score reads the same record: %+v", v)
	}
	// a clean landing clears the council and lands in the fold
	ok := wireResult(loop.Done, "DONE", 0)
	w.councilLedger("zone-login", verdictOf(ok), ok, subagent)
	w.foldResult("zone-login", "domain-auth", ok, w.stress["zone-login"], false, false)
	if w.stress["zone-login"] != 0 || w.Metrics("zone-login")["passes"] != 1 {
		t.Fatal("a pass clears the strain and counts as a pass")
	}
}

// Holes are split by whose they are: the workspace's faults and the agent's honest @? never cost it.
func TestInfraHolesDoNotCost(t *testing.T) {
	root := promoteWorkspace(t)
	w := openWorkspaceRanks(t, root, Ranks(DefaultRanks(Default())))
	r := wireResult(loop.Done, "DONE", 2, "the provider refused the request", "recall: memory: index stale", "policy.gate is off — the turn's writes were not checked", "answer off the wire — sent back (1 of 1)")
	w.foldResult("zone-login", "domain-auth", r, 0, false, false)
	rows := w.Ledger()
	got := rows[len(rows)-1].Holes
	if got.Infra != 3 || got.Harness != 1 || got.Self != 2 {
		t.Fatalf("holes by kind: %+v", got)
	}
	if m := w.Metrics("zone-login"); m["holes"] != 1 || m["holeRate"] != 100 {
		t.Fatalf("only the harness hole costs: %v", m)
	}
}

// A gate verdict credits the agent that held the gate; a pass a later fail on the same paths undoes
// is an overturn against the holder.
func TestGateVerdictFoldsAndOverturns(t *testing.T) {
	root := promoteWorkspace(t)
	w := openWorkspaceRanks(t, root, Ranks(DefaultRanks(Default())))
	w.noteGate("zone-login", Verdict{Word: "pass", VerifyRun: 2}, []string{"auth/login/login.go"})
	w.foldResult("zone-login", "domain-auth", wireResult(loop.Done, "DONE", 0), 0, false, false)
	w.noteGate("zone-login", Verdict{Word: "pass", VerifyRun: 1}, []string{"auth/login/other.go"})
	w.foldResult("zone-login", "domain-auth", wireResult(loop.Done, "DONE", 0), 0, false, false)
	w.noteGate("zone-login", Verdict{Word: "fail", VerifyRun: 2, VerifyRed: 1}, []string{"auth/login/login.go"})
	w.foldResult("zone-login", "domain-auth", wireResult(loop.Fail, "", 0), 0, false, false)
	o := w.Metrics("domain-auth")
	if o["verdicts"] != 3 || o["overturned"] != 1 || int(o["gateRight"]) != 66 {
		t.Fatalf("the holder's record: %v", o)
	}
	s := w.Metrics("zone-login")
	if int(s["gateRate"]) != 66 || int(s["verifyGreenRate"]) != 80 {
		t.Fatalf("the author's gate record: gateRate %v verifyGreenRate %v", s["gateRate"], s["verifyGreenRate"])
	}
	// the overturn replays: a reopened workspace knows the standing passes and the undone one
	w2 := openWorkspaceRanks(t, root, Ranks(DefaultRanks(Default())))
	if w2.Metrics("domain-auth")["overturned"] != 1 {
		t.Fatal("the overturn is on the ledger")
	}
	w2.noteGate("zone-login", Verdict{Word: "fail"}, []string{"auth/login/other.go"})
	w2.foldResult("zone-login", "domain-auth", wireResult(loop.Fail, "", 0), 0, false, false)
	if w2.Metrics("domain-auth")["overturned"] != 2 {
		t.Fatal("a pass standing from before the reopen is overturned too")
	}
}

// The orchestrator's turns are the root's record: the Ended hook folds every human turn.
func TestOrchestratorTurnsFold(t *testing.T) {
	root := promoteWorkspace(t)
	w := openWorkspaceRanks(t, root, Ranks(DefaultRanks(Default())))
	end := w.turnEnded(Orchestrator)
	end(nil, &loop.Result{Status: loop.Done, Wrote: []string{"a.go"}})
	end(nil, &loop.Result{Status: loop.Done, Interrupted: true})
	m := w.Metrics(Orchestrator)
	if m["runs"] != 2 || m["reports"] != 1 || m["neutral"] != 1 || m["askDoneRate"] != 100 {
		t.Fatalf("the root's record (the interrupted turn is neutral): %v", m)
	}
	if v := w.Value(Orchestrator); v.Family != "root" || v.Meter.Signal != "askDoneRate" {
		t.Fatalf("the orchestrator is measured as the root: %+v", v)
	}
}

// A custom rank in config gets its family's value policy and meter from its flags; one that names
// its own score keeps it.
func TestCustomRankGetsItsFamilyValue(t *testing.T) {
	rs := append(Ranks(DefaultRanks(Default())),
		Rank{Name: "night-watch", ReportsTo: Orchestrator, Tools: []string{"read", "grep"}, Dir: "night-watch", Prefix: "night-watch-"},
		Rank{Name: "scribe", ReportsTo: "domain", Authors: true, Dir: "scribe", Prefix: "scribe-", Score: "passRate", Meter: value.MeterDef{Signal: "passRate", Label: "Pass", Bands: value.Bands{OK: 90, Warn: 60}}})
	if err := rs.Validate(); err != nil {
		t.Fatal(err)
	}
	if rs.FamilyOf("night-watch") != "auditor" || rs.FamilyOf("night_watch") != "auditor" || rs.FamilyOf("scribe") != "author" || rs.FamilyOf("domain") != "gate" || rs.FamilyOf("coord") != "planner" || rs.FamilyOf("service") != "steward" || rs.FamilyOf("auditor") != "auditor" {
		t.Fatal("families from flags")
	}
	root := promoteWorkspace(t)
	w := openWorkspaceRanks(t, root, rs)
	if sp := w.Spec("night-watch"); sp.Score != value.FamilyDefault("auditor").Score || sp.Meter.Signal != "upheldRate" {
		t.Fatalf("the auditor family's policy: %+v", sp)
	}
	if sp := w.Spec("scribe"); sp.Score != "passRate" || sp.Meter.Bands.OK != 90 {
		t.Fatalf("a rank's own policy: %+v", sp)
	}
}

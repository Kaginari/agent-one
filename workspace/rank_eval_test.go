package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseASLShape(t *testing.T) {
	good := []string{
		"promote domain when reports >= 25 && passRate >= 90%",
		"promote domain when reports>=3",
		"promote zone when (reports > 10 && failRate <= 5%) || holes == 0",
		"promote domain when !(fails > 0)",
	}
	for _, g := range good {
		a, err := ParseASL(g)
		if err != nil {
			t.Fatalf("%q: %v", g, err)
		}
		if a.Target == "" || a.Cond == nil {
			t.Fatalf("%q: target %q cond nil", g, a.Target)
		}
		// round-trip: the parsed form re-renders as spec words
		if !strings.HasPrefix(a.String(), "promote "+a.Target+" when ") {
			t.Fatalf("%q rendered %q", g, a.String())
		}
	}
	bad := []string{
		"",                                  // empty
		"domain owner when reports >= 1",    // no promote
		"promote domain owner reports >= 1", // no when
		"promote domain when",               // no condition
		"promote domain when foo >= 1",      // unknown metric
		"promote domain when reports ~ 1",   // bad op
	}
	for _, b := range bad {
		if _, err := ParseASL(b); err == nil {
			t.Fatalf("%q must not parse", b)
		}
	}
}

func TestASLEval(t *testing.T) {
	as, err := ParseASL("promote domain when reports >= 4 && passRate >= 75%")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		m    Metrics
		want bool
	}{
		{Metrics{Reports: 4, Passes: 4}, true},  // 4 reports, all pass → 100%
		{Metrics{Reports: 4, Passes: 3}, true},  // 75% exactly
		{Metrics{Reports: 4, Passes: 2}, false}, // 50%
		{Metrics{Reports: 3, Passes: 3}, false}, // too few reports
		{Metrics{Reports: 0}, false},            // no record → rates read zero
	}
	for i, c := range cases {
		if got := as.Cond.Eval(c.m); got != c.want {
			t.Fatalf("case %d: %v → %v, want %v", i, c.m, got, c.want)
		}
	}
}

func TestASLBooleanPrecedence(t *testing.T) {
	// && binds tighter than ||; parens group; ! negates
	as, err := ParseASL("promote domain when reports >= 10 && failRate <= 5% || reports >= 50")
	if err != nil {
		t.Fatal(err)
	}
	// reports 50 with bad failRate crosses on the high-report lane
	if !as.Cond.Eval(Metrics{Reports: 50, Passes: 20, Fails: 30}) {
		t.Fatal("the || lane must fire on a high report count")
	}
	// reports 10 clean crosses the && lane
	if !as.Cond.Eval(Metrics{Reports: 10, Passes: 10}) {
		t.Fatal("the && lane must fire on a clean record")
	}
	// reports 10 with a failure does not
	if as.Cond.Eval(Metrics{Reports: 10, Passes: 9, Fails: 1}) {
		t.Fatal("failRate <= 5% with 1/10 a failure must not cross")
	}
}

func TestMetricsStoreFoldsStatus(t *testing.T) {
	s := NewMetricsStore()
	s.Fold("domain-a", "PASS", 0)
	s.Fold("domain-a", "DONE", 1)
	s.Fold("domain-a", "FAIL", 2)
	s.Fold("zone-x", "PASS", 0)
	m := s.Metrics("domain-a")
	if m.Reports != 3 || m.Passes != 2 || m.Fails != 1 || m.Holes != 3 {
		t.Fatalf("tally: %+v", m)
	}
	if got := s.Metrics("nobody"); got.Reports != 0 {
		t.Fatalf("an unknown agent reads zero: %+v", got)
	}
	if len(s.Agents()) != 2 {
		t.Fatalf("agents: %v", s.Agents())
	}
}

func TestRankValidatePromotesWhen(t *testing.T) {
	lex := Default()
	mk := func(rs []Rank) Ranks { return rs }
	good := mk([]Rank{
		{Name: "coord", ReportsTo: Orchestrator},
		{Name: "domain", ReportsTo: "coord", HoldsGate: true},
		{Name: "principal_domain_owner", ReportsTo: "coord", Base: "domain", PromotesWhen: "promote domain when reports >= 5"},
	})
	if err := good.Validate(); err != nil {
		t.Fatalf("a lawful promotesWhen: %v", err)
	}
	badBase := mk([]Rank{
		{Name: "coord", ReportsTo: Orchestrator},
		{Name: "domain", ReportsTo: "coord", HoldsGate: true},
		{Name: "principal_domain_owner", ReportsTo: "coord", Base: "domain", PromotesWhen: "promote zone when reports >= 5"},
	})
	if err := badBase.Validate(); err == nil {
		t.Fatal("promotesWhen of a different rank must be refused")
	}
	badGrammar := mk([]Rank{
		{Name: "coord", ReportsTo: Orchestrator},
		{Name: "domain", ReportsTo: "coord", HoldsGate: true},
		{Name: "principal_domain_owner", ReportsTo: "coord", Base: "domain", PromotesWhen: "quickly now"},
	})
	if err := badGrammar.Validate(); err == nil {
		t.Fatal("a non-ASL promotesWhen must be refused")
	}
	noBase := mk([]Rank{
		{Name: "coord", ReportsTo: Orchestrator},
		{Name: "coord", ReportsTo: Orchestrator, PromotesWhen: "promote coord when reports >= 5"},
	})
	noBase[1].Name = "watcher"
	if err := noBase.Validate(); err == nil {
		t.Fatal("promotesWhen on a non-promoted rank must be refused")
	}
	_ = lex // lex read by implication; the table is the test's own
}

func TestPromotionRegistersAAgent(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, ".agent-one/AGENT-ONE.md", policyText)
	writeDoc(t, root, ".agent-one/domain/auth/README.md",
		"# domain-auth\n\n- **Rank:** Domain owner\n- **Owns:** `auth/`\n- **Reports to:** orchestrator\n")
	writeDoc(t, root, "auth/login.go", "package auth\n")
	rs := Ranks{
		{Name: "coord", ReportsTo: Orchestrator},
		{Name: "domain", ReportsTo: "coord", HoldsGate: true, Dir: "domain", Prefix: "domain-"},
		{Name: "principal_domain_owner", ReportsTo: "coord", Base: "domain", Dir: "principal-domain", Prefix: "principal-domain-",
			PromotesWhen: "promote domain when reports >= 3 && passRate >= 100%"},
	}
	w := openWorkspaceRanks(t, root, rs)
	if w.Member("domain-auth") == nil {
		t.Fatalf("the domain owner must be on the roster: %v", namesOf(w))
	}
	// two reports do not yet cross
	w.FoldMetrics("domain-auth", "PASS", 0)
	w.FoldMetrics("domain-auth", "PASS", 0)
	if got := w.Promotions(); len(got) != 0 {
		t.Fatalf("two reports below the condition must not promote: %+v", got)
	}
	if w.Member("domain-auth") == nil {
		t.Fatal("the base stays put")
	}
	// the third clean report crosses: the harness registers the high agent itself
	w.FoldMetrics("domain-auth", "PASS", 0)
	events := w.Promotions()
	if len(events) != 1 {
		t.Fatalf("a crossing must register one promotion, got %+v holes %v", events, w.PromoteHoles())
	}
	ev := events[0]
	if ev.Base != "domain-auth" || ev.Rank != "principal_domain_owner" {
		t.Fatalf("event: %+v", ev)
	}
	if ev.New != "principal-domain-auth" {
		t.Fatalf("the promoted id rides its own prefix: %q", ev.New)
	}
	want := ".agent-one/principal-domain/auth/README.md"
	if filepath.ToSlash(ev.Doc) != want {
		t.Fatalf("the registered doc: %q, want %q", ev.Doc, want)
	}
	full := filepath.Join(root, filepath.FromSlash(want))
	text, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{"principal-domain-auth", "**Rank:** principal_domain_owner", "Promoted from:** `domain-auth`", "promote domain when"} {
		if !strings.Contains(string(text), frag) {
			t.Fatalf("the registered doc must carry %q:\n%s", frag, text)
		}
	}
	// the reloaded roster holds the registered agent, ranked as its promoted rank
	if c := w.Member("principal-domain-auth"); c == nil || c.Rank != "principal_domain_owner" {
		var rank string
		if c != nil {
			rank = c.Rank
		}
		t.Fatalf("the registered agent must be on the roster as principal_domain_owner, got %q — roster %v", rank, namesOf(w))
	}
	// the base's doc was not overwritten
	base, err := os.ReadFile(filepath.Join(root, ".agent-one/domain/auth/README.md"))
	if err != nil || !strings.Contains(string(base), "# domain-auth") {
		t.Fatalf("the base doc must stand: %v\n%s", err, base)
	}
	// the registered snapshot marks the crossing
	if m := w.Metrics("domain-auth"); m.Reports != 3 || m.Passes != 3 {
		t.Fatalf("snapshot: %+v", m)
	}
	// promoting again from the same agent does not double-register
	w.FoldMetrics("domain-auth", "PASS", 0)
	if got := w.Promotions(); len(got) != 1 {
		t.Fatalf("a second report from the promoted agent must not remint: %+v", got)
	}
}

func TestPromotionRegisterRefusesCollision(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, ".agent-one/AGENT-ONE.md", policyText)
	writeDoc(t, root, ".agent-one/domain/auth/README.md",
		"# domain-auth\n\n- **Rank:** Domain owner\n- **Owns:** `auth/`\n- **Reports to:** orchestrator\n")
	rs := Ranks{
		{Name: "coord", ReportsTo: Orchestrator},
		{Name: "domain", ReportsTo: "coord", HoldsGate: true, Dir: "domain", Prefix: "domain-"},
		{Name: "principal_domain_owner", ReportsTo: "coord", Base: "domain", Dir: "principal-domain", Prefix: "principal-domain-",
			PromotesWhen: "promote domain when reports >= 1"},
	}
	w := openWorkspaceRanks(t, root, rs)
	w.FoldMetrics("domain-auth", "PASS", 0)
	if got := w.Promotions(); len(got) != 1 {
		t.Fatalf("first crossing must register: %+v holes %v", got, w.PromoteHoles())
	}
	// a fresh workspace over the same disk: the doc stands, so a re-crossing must skip the register
	// and log a hole rather than overwrite
	w2 := openWorkspaceRanks(t, root, rs)
	if w2.Member("principal-domain-auth") == nil {
		t.Fatalf("the on-disk promoted agent must rejoin the roster: %v", namesOf(w2))
	}
	w2.FoldMetrics("domain-auth", "PASS", 0)
	if got := w2.Promotions(); len(got) != 0 {
		t.Fatalf("a doc-backed agent never remints: %+v", got)
	}
}

func TestPromotionNeverWithoutABaseRecord(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, ".agent-one/AGENT-ONE.md", policyText)
	writeDoc(t, root, ".agent-one/domain/auth/README.md",
		"# domain-auth\n\n- **Rank:** Domain owner\n- **Owns:** `auth/`\n- **Reports to:** orchestrator\n")
	rs := Ranks{
		{Name: "coord", ReportsTo: Orchestrator},
		{Name: "domain", ReportsTo: "coord", HoldsGate: true, Dir: "domain", Prefix: "domain-"},
		{Name: "principal_domain_owner", ReportsTo: "coord", Base: "domain", Dir: "principal-domain", Prefix: "principal-domain-",
			PromotesWhen: "promote domain when reports >= 1"},
	}
	w := openWorkspaceRanks(t, root, rs)
	// an agent that never filed a report reads zero across every metric — a zero record never crosses
	w.FoldMetrics("domain-ghost", "PASS", 0)
	if got := w.Promotions(); len(got) != 0 {
		t.Fatalf("nobody on the roster named domain-ghost: %+v", got)
	}
}

// helpers shared across workspace tests live in workspace_test.go; these two are local fakes to keep
// the promotion suite self-contained.

func writeDoc(t *testing.T, root, rel, text string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func namesOf(w *Workspace) []string {
	var out []string
	for _, c := range w.Members {
		out = append(out, c.Name)
	}
	return out
}

func openWorkspaceRanks(t *testing.T, root string, rs Ranks) *Workspace {
	t.Helper()
	w, err := Open(root, Default(), Options{Ranks: rs, Ontology: true})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// TestIntelligence is the coded evaluator the High promotion reads: the deterministic fold of
// the agent's passRate against its holeRate — zero at zero reports, floored at zero (never
// negative) when holes outnumber passes. It is a workspace metric — no prompt, no ask.
func TestIntelligence(t *testing.T) {
	// no record: nothing thinks, nothing promotes
	if got := (Metrics{}).Intelligence(); got != 0 {
		t.Fatalf("zero record must read zero: got %v", got)
	}
	// pass with no holes reads its full passRate
	if got := (Metrics{Reports: 10, Passes: 9}).Intelligence(); got != 90 {
		t.Fatalf("9/10 pass w/ 0 holes: want 90, got %v", got)
	}
	// holes discount the passes: 90 - 30 = 60
	if got := (Metrics{Reports: 10, Passes: 9, Holes: 3}).Intelligence(); got != 60 {
		t.Fatalf("9/10 pass w/ 3 holes: want 60, got %v", got)
	}
	// holes outnumbering passes floors at zero, never negative
	if got := (Metrics{Reports: 10, Passes: 3, Holes: 9}).Intelligence(); got != 0 {
		t.Fatalf("3/10 pass w/ 9 holes must floor at 0, got %v", got)
	}
	// the ASL reads it as a named operand
	cond, err := ParseCondition("intelligence >= 70 && reports >= 5")
	if err != nil {
		t.Fatal(err)
	}
	if !cond.Eval(Metrics{Reports: 10, Passes: 9, Holes: 1}) {
		t.Fatal("9/10 pass w/ 1 hole must cross intelligence >= 70 && reports >= 5")
	}
	if cond.Eval(Metrics{Reports: 10, Passes: 7, Holes: 4}) {
		t.Fatal("7/10 pass w/ 4 holes (intelligence 30) must stay under the band")
	}
}

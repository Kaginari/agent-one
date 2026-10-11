package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/config"
)

// promoteRanks is the policy's table with a short promotion band, so a fixture can earn it.
func promoteRanks(when string) Ranks {
	rs := Ranks(DefaultRanks(Default()))
	for i := range rs {
		if rs[i].Name == "principal_domain_owner" {
			rs[i].PromotesWhen = when
		}
	}
	return rs
}

// promoteWorkspace is one lane: coord-core → domain-auth (verify, ownership auth/) → zone-login.
func promoteWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeDoc(t, root, ".agent-one/AGENT-ONE.md", policyText)
	writeDoc(t, root, ".agent-one/log.md", "# Chronicle\n\nAppend-only.\n\n---\n")
	writeDoc(t, root, ".agent-one/coord/core/README.md", "# coord-core\n\n- **Rank:** Coordinator\n- **Reports to:** orchestrator\n")
	writeDoc(t, root, ".agent-one/domain/auth/README.md",
		"# domain-auth\n\n- **Rank:** Domain owner\n- **Owns:** `auth/`\n- **Reports to:** coord-core\n- **Verify:** `test -d auth`\n\n## Invariants\n\n1. tokens expire after one hour\n")
	writeDoc(t, root, ".agent-one/zone/login/README.md",
		"# zone-login\n\n- **Rank:** Zone worker\n- **Owns:** `auth/login/`\n- **Reports to:** domain-auth\n- **Verify:** `test -d auth/login`\n")
	writeDoc(t, root, "auth/login/login.go", "package login\n")
	return root
}

// A promotion carries the base whole into the promoted rank's own dir, retires the base's seat,
// re-points the base's zone workers to the risen agent, keeps the ontology valid, and is logged.
func TestPromotionCarriesTheAgentAndRetiresTheBase(t *testing.T) {
	root := promoteWorkspace(t)
	w := openWorkspaceRanks(t, root, promoteRanks("promote domain when reports >= 3 && passRate >= 90%"))
	for i := 0; i < 3; i++ {
		w.FoldMetrics("domain-auth", "DONE", 0)
	}
	if got := w.Promotions(); len(got) != 1 || got[0].New != "principal-domain-auth" {
		t.Fatalf("one promotion to principal-domain-auth: %+v holes %v", got, w.PromoteHoles())
	}
	doc, err := os.ReadFile(filepath.Join(root, ".agent-one/principal-domain/auth/README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{"# principal-domain-auth", "- **Rank:** principal_domain_owner", "**Promoted from:** `domain-auth`", "- **Owns:** `auth/`", "- **Reports to:** coord-core", "`test -d auth`", "tokens expire after one hour", "## Promotion", "reports 3, passes 3"} {
		if !strings.Contains(string(doc), frag) {
			t.Errorf("the promoted doc must carry %q:\n%s", frag, doc)
		}
	}
	base, _ := os.ReadFile(filepath.Join(root, ".agent-one/domain/auth/README.md"))
	if !strings.Contains(string(base), retiredMark) || !strings.Contains(string(base), "rose as principal-domain-auth") {
		t.Fatalf("the base's seat retires, naming its promoted:\n%s", base)
	}
	// the roster: one agent on the soil, the zone workers' line runs through it
	for _, n := range namesOf(w) {
		if n == "domain-auth" {
			t.Error("the retired base leaves the roster")
		}
	}
	hi := w.Member("principal-domain-auth")
	if hi == nil || hi.Rank != "principal_domain_owner" || strings.Join(hi.Ownership, ",") != "auth" || hi.Parent != "coord-core" || !hi.VerifyCoded() {
		t.Fatalf("the risen agent holds the base's ownership, line and verify: %+v", hi)
	}
	if sl := w.Member("zone-login"); sl == nil || sl.Parent != "principal-domain-auth" {
		t.Fatalf("the base's zone worker now reports to the promoted: %+v", sl)
	}
	if o := w.Owner("auth/session.go"); o == nil || o.Name != "principal-domain-auth" {
		t.Errorf("the gate-holder fallback owner is the risen agent: %v", o)
	}
	// it commands the base's zone workers
	if err := w.mayDispatch("principal-domain-auth", w.Member("zone-login"), SubagentOptions{StrictFlow: true}); err != nil {
		t.Errorf("the principal domain owner orders its zone workers: %v", err)
	}
	// the ontology stays valid
	if fs := w.Onto.Validate(); len(fs) != 0 {
		t.Errorf("the promotion must leave the ontology valid: %v", fs)
	}
	// log.md carries the promotion with the reading that earned it (Policy 4)
	logText, _ := os.ReadFile(w.LogPath())
	if !strings.Contains(string(logText), "principal-domain-auth — promotion — domain-auth rose as principal-domain-auth") || !strings.Contains(string(logText), "earned: reports 3, passes 3") {
		t.Errorf("log.md must record the promotion:\n%s", logText)
	}
	// the dispatcher hears it once
	lines := w.promoteLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "[promotion] domain-auth rose as principal-domain-auth") {
		t.Errorf("the dispatcher reads the promotion: %v", lines)
	}
	if again := w.promoteLines(); len(again) != 0 {
		t.Errorf("announced once: %v", again)
	}
}

// The tally is a ledger replayed at Open: a record earned across sessions counts, and an agent
// that promoted in an earlier session never registers again.
func TestPromotionTallyOutlivesTheSession(t *testing.T) {
	root := promoteWorkspace(t)
	rs := promoteRanks("promote domain when reports >= 3 && passRate >= 90%")
	w1 := openWorkspaceRanks(t, root, rs)
	w1.FoldMetrics("domain-auth", "DONE", 0)
	w1.FoldMetrics("domain-auth", "DONE", 0)
	if len(w1.Promotions()) != 0 {
		t.Fatal("two reports do not cross")
	}
	w2 := openWorkspaceRanks(t, root, rs)
	if m := w2.Metrics("domain-auth"); m.Reports != 2 || m.Passes != 2 {
		t.Fatalf("the tally replays at Open: %+v", m)
	}
	w2.FoldMetrics("domain-auth", "DONE", 0)
	if got := w2.Promotions(); len(got) != 1 {
		t.Fatalf("the third report, a session later, crosses: %+v holes %v", got, w2.PromoteHoles())
	}
	shelf, err := os.ReadFile(filepath.Join(root, ".agent-one", filepath.FromSlash(promotionShelf)))
	if err != nil || strings.Count(string(shelf), `"kind":"report"`) != 3 || strings.Count(string(shelf), `"kind":"promote"`) != 1 {
		t.Fatalf("the shelf is append-only, one row per report and per promotion: %v\n%s", err, shelf)
	}
	w3 := openWorkspaceRanks(t, root, rs)
	if m := w3.Metrics("domain-auth"); m.Reports != 3 {
		t.Fatalf("replayed again: %+v", m)
	}
	w3.FoldMetrics("domain-auth", "DONE", 0)
	if got := w3.Promotions(); len(got) != 0 {
		t.Fatalf("a promoted agent never registers twice: %+v", got)
	}
}

// A register that cannot land is a hole: logged, and surfaced to the dispatcher as @?.
func TestPromotionHoleSurfaces(t *testing.T) {
	root := promoteWorkspace(t)
	rs := promoteRanks("promote domain when reports >= 1")
	for i := range rs {
		if rs[i].Name == "principal_domain_owner" { // the old shape: the promoted rank on its base's dir
			rs[i].Dir, rs[i].Prefix = "domain", "domain-"
		}
	}
	w := openWorkspaceRanks(t, root, rs)
	w.FoldMetrics("domain-auth", "DONE", 0)
	if len(w.Promotions()) != 0 {
		t.Fatal("no register on the base's own dir")
	}
	lines := w.promoteLines()
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "@? promotion hole — promote domain-auth") || !strings.Contains(lines[0], "a dir and prefix of its own") {
		t.Fatalf("the hole reaches the dispatcher as @?: %v", lines)
	}
	logText, _ := os.ReadFile(w.LogPath())
	if !strings.Contains(string(logText), "promotion hole — domain-auth") {
		t.Errorf("the hole is logged:\n%s", logText)
	}
	if w.Member("domain-auth") == nil {
		t.Error("a failed register leaves the base standing")
	}
}

// An agent's own @S FAIL is no pass, whatever the loop's status said.
func TestPromotionCountsTheBodysVerdict(t *testing.T) {
	root := promoteWorkspace(t)
	w := openWorkspaceRanks(t, root, promoteRanks("promote domain when reports >= 2 && passRate >= 100%"))
	w.FoldMetrics("domain-auth", "DONE", 0)
	w.FoldMetrics("domain-auth", "FAIL", 0)
	if m := w.Metrics("domain-auth"); m.Passes != 1 || m.Fails != 1 || len(w.Promotions()) != 0 {
		t.Fatalf("a fail is a fail: %+v", m)
	}
}

// The binary's table (config) and the workspace's default table promote on the same conditions, in
// the promoted ranks' own dirs.
func TestDefaultPromotionAgrees(t *testing.T) {
	if config.DefaultPromoteCoord != DefaultPromoteCoord || config.DefaultPromoteDomain != DefaultPromoteDomain {
		t.Fatal("config and workspace disagree on the default promotion conditions")
	}
	for _, s := range []string{DefaultPromoteCoord, DefaultPromoteDomain} {
		if _, err := ParseASL(s); err != nil {
			t.Fatalf("%q: %v", s, err)
		}
	}
	rs := Ranks(DefaultRanks(Default()))
	for _, n := range []string{"principal_coordinator", "principal_domain_owner"} {
		r, _ := rs.Get(n)
		b, _ := rs.Get(r.Base)
		if r.Dir == b.Dir || r.Prefix == b.Prefix || r.Prefix != r.Dir+"-" {
			t.Errorf("%s needs its own dir and prefix: %+v", n, r)
		}
	}
	if r, _ := Ranks(DefaultRanks(Default())).Get("principal_domain_owner"); r.Prefix != "principal-domain-" {
		t.Errorf("agent-one's words: %+v", r)
	}
	// the conservative band: 19 clean reports do not cross, 20 do; one hole in five blocks it
	a, _ := ParseASL(DefaultPromoteDomain)
	if a.Cond.Eval(Metrics{Reports: 19, Passes: 19}) || !a.Cond.Eval(Metrics{Reports: 20, Passes: 20}) || a.Cond.Eval(Metrics{Reports: 20, Passes: 20, Holes: 4}) || a.Cond.Eval(Metrics{Reports: 20, Passes: 17}) {
		t.Error("the default band")
	}
}

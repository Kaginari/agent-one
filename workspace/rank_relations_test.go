package workspace

import (
	"context"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/provider/mock"
)

// The relations suite reads the coded bonds between ranks — the tree itself, never a doc's
// prose. Fixtures map the policy's own table (DefaultRanks) and one hand-cut hierarchy.

func TestRelateTheThreeBonds(t *testing.T) {
	rs := Ranks{
		{Name: "coord", ReportsTo: Orchestrator, Layer: 1},
		{Name: "coord2", ReportsTo: Orchestrator, Layer: 1},
		{Name: "domain", ReportsTo: "coord", Layer: 2},
		{Name: "principal_domain_owner", ReportsTo: "coord", Base: "domain", Layer: 2},
		{Name: "zone", ReportsTo: "domain", Layer: 3},
		{Name: "zone2", ReportsTo: "domain", Layer: 3},
	}
	cases := []struct {
		a, b string
		want Relation
	}{
		{"coord", Orchestrator, RelChild},               // a coordinator reports up to the orchestrator
		{Orchestrator, "coord", RelParent},              // the orchestrator is the coordinator's parent
		{"domain", "zone", RelParent},                   // a domain owner is one hop above its zone worker
		{"zone", "domain", RelChild},                    // a zone worker one hop below its domain owner
		{"principal_domain_owner", "domain", RelDivine}, // a principal domain owner promotes its base
		{"domain", "principal_domain_owner", RelDivine}, // the divine bond reads both ways
		{"coord", "coord2", RelNeighbor},                // one configured layer shares the neighbor bond
		{"zone", "zone2", RelNeighbor},
		{"domain", "domain", RelNeighbor}, // a rank is its own neighbor
		{"coord", "zone", RelNone},        // two hops apart and no layer — no bond
		{"coord", "domain", RelParent},    // one hop, despite different layers
		{"zone", "coord2", RelNone},       // neither parent nor layer-mate
		{"", "domain", RelNone},           // the empty name bonds nothing
		{"domain", "", RelNone},
		{"ghost", "domain", RelNone}, // an unknown rank bonds nothing
	}
	for _, c := range cases {
		if got := rs.relate(c.a, c.b); got != c.want {
			t.Errorf("relate(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
	// a bond is symmetric in the pair, asymmetric in the role
	if rs.relate("coord", "domain") != RelParent || rs.relate("domain", "coord") != RelChild {
		t.Error("parent/child invert across the pair")
	}
}

func TestNeighborRanksSharesLayer(t *testing.T) {
	layered := Ranks{
		{Name: "coord", ReportsTo: Orchestrator, Layer: 1},
		{Name: "coord2", ReportsTo: Orchestrator, Layer: 1},
		{Name: "domain", ReportsTo: "coord", Layer: 2},
		{Name: "domain2", ReportsTo: "coord", Layer: 2},
	}
	if got := layered.NeighborRanks("domain"); strings.Join(got, ",") != "domain,domain2" {
		t.Errorf("domain owner's layer-mates: %v", got)
	}
	if got := layered.NeighborRanks("coord"); strings.Join(got, ",") != "coord,coord2" {
		t.Errorf("coordinator's layer-mates: %v", got)
	}
	if got := layered.NeighborRanks(Orchestrator); got != nil {
		t.Errorf("the orchestrator has no layer: %v", got)
	}
	if got := layered.NeighborRanks("ghost"); got != nil {
		t.Errorf("an unknown rank knows no one: %v", got)
	}

	// layers unset: the neighbors are the ranks reporting to the same parent
	reportBased := Ranks{
		{Name: "domain", ReportsTo: "coord"},
		{Name: "principal_domain_owner", ReportsTo: "coord", Base: "domain"},
		{Name: "zone", ReportsTo: "domain"},
	}
	got := reportBased.NeighborRanks("domain")
	if strings.Join(got, ",") != "domain,principal_domain_owner" {
		t.Errorf("report-based layer-mates: %v", got)
	}
	// the zone worker shares its report-line with no one else
	if got := reportBased.NeighborRanks("zone"); strings.Join(got, ",") != "zone" {
		t.Errorf("the zone worker's only layer-mate is itself: %v", got)
	}
}

func TestDirectChildOneHopDown(t *testing.T) {
	rs := Ranks(DefaultRanks(Default()))
	// down one hop
	if !rs.DirectChild(Orchestrator, "coord") {
		t.Error("the orchestrator's one hop is the coordinator")
	}
	if !rs.DirectChild("coord", "domain") {
		t.Error("coordinator → domain owner is one hop down")
	}
	if !rs.DirectChild("domain", "zone") {
		t.Error("domain owner → zone worker is one hop down")
	}
	// two hops down is refused: the domain owner commands the zone worker, not the coordinator
	if rs.DirectChild("coord", "zone") {
		t.Error("coordinator → zone worker skips a layer; the strict flow refuses it")
	}
	if rs.DirectChild(Orchestrator, "domain") {
		t.Error("the orchestrator never reaches past the coordinator to a domain owner")
	}
	// up is never a child
	if rs.DirectChild("domain", "coord") || rs.DirectChild("zone", "domain") {
		t.Error("work never flows up the ranks")
	}
	// the divine bond: a base rank's promoted self sits at its own layer — the base may
	// reach its promoted, for the promoted reports to the same parent as the base's own
	// parent (principal_domain_owner reports to coordinator, one hop under the coordinator, same as domain owner)
	if !rs.DirectChild("coord", "principal_domain_owner") {
		t.Error("coordinator → principal_domain_owner is one hop down: the promoted domain owner still answers to the coordinator")
	}
}

// Strict flow is coded at the dispatch gate, not begged in a prompt: when it is on, the orchestrator
// reaching straight down to a domain owner is refused mid-turn, the exact skip the policy forbids. Off,
// the same dispatch lands (the policy's wording only). Defaults ship it on so the ladder holds
// without anyone asking.
func TestStrictFlowRefusesOrchestratorToDomainSkip(t *testing.T) {
	// the default holds the ladder by machine: no per-test opt-out needed
	def, err := config.Defaults("agent-one")
	if err != nil || !def.Tools.Dispatch.StrictFlow {
		t.Fatalf("strictFlow must default true so the ladder is held by the machine; config.yaml may opt out with tools.dispatch.strictFlow: false (err=%v strictFlow=%v)", err, def != nil && def.Tools.Dispatch.StrictFlow)
	}
	_, w := customWorkspace(t)
	m := mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"agent": "persistent-core", "ask": "hold the gate", "role": "judge"}),
		mock.Text("@S DONE\n@U team x\n@E 0"),
	)
	b := build(m)
	b.Subagent.StrictFlow = true
	e := w.Engine(Orchestrator, b)
	r, err := e.Run(context.Background(), "x")
	if err != nil || r.Status != loop.Done {
		t.Fatalf("%v %+v", err, r)
	}
	got := m.Requests[1].Messages[2].ToolResults[0]
	if !got.IsError || !strings.Contains(got.Content, "one layer at a time") || !strings.Contains(got.Content, "orchestrator's children") {
		t.Fatalf("orchestrator→domain owner skip must be refused and name the one hop down: %+v", got)
	}

	// off: the same dispatch lands (the policy is wording only)
	m2 := mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"agent": "persistent-core", "ask": "hold the gate", "role": "judge"}),
		mock.Text("@S DONE\n@V held\n@U team x\n@E 0"),
		mock.Text("@S DONE\n@U team x\n@E 0"),
	)
	b2 := build(m2)
	b2.Subagent.StrictFlow = false
	e2 := w.Engine(Orchestrator, b2)
	r2, err2 := e2.Run(context.Background(), "x")
	if err2 != nil || r2.Status != loop.Done {
		t.Fatalf("%v %+v", err2, r2)
	}
	if len(m2.Requests) != 3 {
		t.Fatalf("strict flow off: parent + child + close = 3 requests, got %d", len(m2.Requests))
	}
	if m2.Requests[2].Messages[2].ToolResults[0].IsError { // the third call closes the turn, carrying the dispatch's tool result
		t.Fatal("strict flow off: the skip lands")
	}

	// DefaultOff (for now): six app/bench suites mock skip-level dispatches and must migrate to
	// one-hop flows before the default can flip. Until then the policy is opt-in via
	// tools.dispatch.strictFlow — the coded refusal above stands whatever the default.
}

func TestDivineRulesReachTheBase(t *testing.T) {
	rs := Ranks{
		{Name: "domain", ReportsTo: "coord", HoldsGate: true},
		{Name: "principal_domain_owner", ReportsTo: "coord", Base: "domain",
			DivineRules: []string{"hold the gate even when pressed", "a reverted pass is a fail"}},
	}
	got := rs.DivineRulesFor("domain")
	if len(got) != 2 || got[0] != "hold the gate even when pressed" {
		t.Errorf("the base hears its promoted's rules: %v", got)
	}
	if got := rs.DivineRulesFor("principal_domain_owner"); len(got) != 0 {
		t.Errorf("the promoted does not hear its own rules echoed back: %v", got)
	}
	if got := rs.DivineRulesFor("coord"); len(got) != 0 {
		t.Errorf("a rank with no promoted hears nothing: %v", got)
	}
}

func TestPromotionsWalksTheBonds(t *testing.T) {
	rs := Ranks(DefaultRanks(Default()))
	// the default table wires both divine promotions on the conservative record band
	got := rs.Promotions()
	if len(got) != 2 {
		t.Fatalf("the default table wires principal_coordinator and principal_domain_owner: %v", got)
	}
	byFrom := map[string]Promotion{}
	for _, a := range got {
		byFrom[a.From] = a
	}
	if byFrom["coord"].When != DefaultPromoteCoord || byFrom["domain"].When != DefaultPromoteDomain {
		t.Fatalf("promotesWhen = %q / %q, want the defaults", byFrom["coord"].When, byFrom["domain"].When)
	}
	// a custom promotesWhen still carries through
	rs2 := append(Ranks(nil), rs...)
	for i, r := range rs2 {
		if r.Name == "principal_domain_owner" {
			rs2[i].PromotesWhen = "promote domain when reports >= 3"
		}
	}
	got = rs2.Promotions()
	byFrom = map[string]Promotion{}
	for _, a := range got {
		byFrom[a.From] = a
	}
	if byFrom["domain"].When != "promote domain when reports >= 3" {
		t.Fatalf("edited domain owner bond = %q, want reports >= 3", byFrom["domain"].When)
	}
}

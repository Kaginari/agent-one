package workspace

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/loop"
)

// endSite mirrors the subagent.go call-site policy (warrant, then lift the act arm): this is the
// "subagent is done, what happens" seam the tests can hold onto honestly.
func endSite(w *Workspace, agent string, verdict string, r *loop.Result, subagent SubagentOptions) (stamp string, relieved string) {
	stamp, _ = w.councilLedger(agent, verdict, r, subagent)
	if rel := subagent.StressRelieveAt; rel > 0 {
		w.mu.Lock()
		strainNow := w.stress[agent]
		w.mu.Unlock()
		if strainNow >= rel {
			if act, ok := w.relieve(agent); ok {
				relieved = "council relief — " + agent + " crosses relieveAt: " + act
			}
		}
	}
	return stamp, relieved
}

// TestCouncilReliefSplit: strain crossing the act layer splits the agent's zone into two
// same-race young, retires the parent, and the roster re-reads the truth — the graph keeps
// only memory, never a hidden claim.
func TestCouncilReliefSplit(t *testing.T) {
	dir := t.TempDir()
	for p, s := range map[string]string{
		".agent-one/AGENT-ONE.md":              policyText,
		".agent-one/log.md":                    "# Chronicle\n",
		".agent-one/domain/security/README.md": "# domain-security\n\n- **Rank:** Domain owner\n- **Owns:** `src/`\n",
		".agent-one/zone/two/README.md":        "# zone-two\n\n- **Rank:** Zone worker\n- **Owns:** `src/a/`, `src/b/`\n- **Reports to:** domain-security\n- **Verify:** `true`\n",
	} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w, err := Open(dir, Default(), Options{Ontology: true})
	if err != nil {
		t.Fatal(err)
	}
	c1 := w.Member("zone-two")
	if c1 == nil || len(c1.Ownership) != 2 {
		t.Fatalf("split agent must carry exactly 2 prefixes: %+v", c1)
	}
	parentBreadth := len(c1.Ownership)
	subagent := SubagentOptions{StressEscalateAt: 1, StressMaxConsults: 3, StressRelieveAt: 2}
	r := &loop.Result{Status: loop.Fail}
	if _, act := endSite(w, "zone-two", "FAIL", r, subagent); act != "" {
		t.Fatalf("strain below the layer must never act: %q", act)
	}
	_, act := endSite(w, "zone-two", "FAIL", r, subagent) // second miss: the strain hits the layer
	if !strings.Contains(act, "split into") || !strings.Contains(act, "zone-two-a") || !strings.Contains(act, "zone-two-b") {
		t.Fatalf("the act must be the split naming both young: %q", act)
	}
	if c := w.Member("zone-two"); c != nil {
		t.Fatalf("the retiree must drop off the roster: %+v", c)
	}
	ca, cb := w.Member("zone-two-a"), w.Member("zone-two-b")
	if ca == nil || cb == nil {
		t.Fatalf("the young never reached the roster: %v / %v", ca, cb)
	}
	if len(ca.Ownership)+len(cb.Ownership) != parentBreadth {
		t.Fatalf("halves must total the parent's breadth: %d + %d ≠ %d", len(ca.Ownership), len(cb.Ownership), parentBreadth)
	}
	if own := w.Owner("src/a/x.go"); own == nil || own.Name != "zone-two-a" {
		t.Fatalf("src/a rides with zone-two-a: %+v", own)
	}
	if own := w.Owner("src/b/x.go"); own == nil || own.Name != "zone-two-b" {
		t.Fatalf("src/b rides with zone-two-b: %+v", own)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".agent-one/instruments/ledgers.jsonl"))
	if !strings.Contains(string(b), `"kind":"relief"`) || !strings.Contains(string(b), "split") || !strings.Contains(string(b), "zone-two-a") || !strings.Contains(string(b), "zone-two-b") {
		t.Fatalf("the act must write the provisioning + the cut to the shelf:\n%s", b)
	}
	if w.Member("domain-security") == nil {
		t.Fatal("relief must not slip the dispatching rank up")
	}
}

// TestCouncilReliefRetire: a one-prefix zone with nothing under it to divide is relieved by
// retirement — the seat keeps its graph memory while the soil opens again for dispatches.
// (A one-prefix zone WITH tree content splits instead — TestCouncilReliefTreeSplit below.)
func TestCouncilReliefRetire(t *testing.T) {
	dir := t.TempDir()
	for p, s := range map[string]string{
		".agent-one/AGENT-ONE.md":          policyText,
		".agent-one/log.md":                "# Chronicle\n",
		".agent-one/domain/host/README.md": "# domain-host\n\n- **Rank:** Domain owner\n- **Owns:** `zone/`\n",
		".agent-one/zone/narrow/README.md": "# zone-narrow\n\n- **Rank:** Zone worker\n- **Owns:** `zone/narrow/`\n- **Reports to:** domain-host\n",
	} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w, err := Open(dir, Default(), Options{Ontology: true})
	if err != nil {
		t.Fatal(err)
	}
	subagent := SubagentOptions{StressEscalateAt: 1, StressMaxConsults: 3, StressRelieveAt: 1}
	r := &loop.Result{Status: loop.Fail}
	_, act := endSite(w, "zone-narrow", "FAIL", r, subagent)
	if !strings.Contains(act, "retired") || strings.Contains(act, "split") {
		t.Fatalf("a narrow zone relieves by retirement, never provisioning: %q", act)
	}
	if w.Member("zone-narrow") != nil {
		t.Fatal("the retiree keeps graph memory, never claims soil again")
	}
	if own := w.Owner("zone/narrow/file.go"); own != nil && own.Name == "zone-narrow" {
		t.Fatalf("retired soil must release the zone worker's claim (the domain owner's umbrella stays lawful): %+v", own)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".agent-one/zone/narrow/README.md"))
	if !strings.Contains(string(b), "**Retired:** true") {
		t.Fatalf("the retiree's doc owns its marker:\n%s", b)
	}
	// under rel=1 the second miss must not re-fire idempotently — one act per streak
	_, act = endSite(w, "zone-narrow", "FAIL", r, subagent)
	if act != "" {
		t.Fatalf("the already-retired must not re-fire: %q", act)
	}
}

// TestCouncilReliefTreeSplit: a single-prefix zone holding a real tree narrows by it —
// the 2026-10-09 zone-sizing policy (one wide dir, every new file on one persistent agent, and no
// count-split possible with a single prefix): sub-dirs and direct files halve into two
// same-race young; nothing under the prefix is orphaned.
//
// The fixture roots the zone at `src/` — the roster's derive admits src-rooted structured
// claims under every configuration state landed so far; an `app`-rooted single-prefix zone rides
// the census class-(f) commission in flight (its seat-slug admission policy) and re-verifies
// when that slice lands.
func TestCouncilReliefTreeSplit(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		".agent-one/AGENT-ONE.md":         policyText,
		".agent-one/log.md":               "# Chronicle\n",
		".agent-one/domain/app/README.md": "# domain-app\n\n- **Rank:** Domain owner\n- **Owns:** `src/`\n",
		".agent-one/zone/wide/README.md":  "# zone-wide\n\n- **Rank:** Zone worker\n- **Owns:** `src/`\n- **Reports to:** domain-app\n- **Verify:** `true`\n",
		"src/a/one.go":                    "package a\n",
		"src/b/two.go":                    "package b\n",
		"src/c/three.go":                  "package c\n",
		"src/d/four.go":                   "package d\n",
		"src/root.go":                     "package src\n",
	}
	for p, s := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w, err := Open(dir, Default(), Options{Ontology: true})
	if err != nil {
		t.Fatal(err)
	}
	subagent := SubagentOptions{StressEscalateAt: 1, StressMaxConsults: 3, StressRelieveAt: 1}
	r := &loop.Result{Status: loop.Fail}
	_, act := endSite(w, "zone-wide", "FAIL", r, subagent)
	if !strings.Contains(act, "split into") || !strings.Contains(act, "zone-wide-a") || !strings.Contains(act, "zone-wide-b") || !strings.Contains(act, "by the tree") {
		t.Fatalf("a one-prefix zone with a tree must split by it: %q", act)
	}
	ca, cb := w.Member("zone-wide-a"), w.Member("zone-wide-b")
	if ca == nil || cb == nil {
		t.Fatalf("the young never reached the roster: %v / %v", ca, cb)
	}
	// the union of the halves is the tree under src/ — no entry orphaned
	claims := append(append([]string{}, ca.Ownership...), cb.Ownership...)
	sort.Strings(claims)
	want := []string{"src/a", "src/b", "src/c", "src/d", "src/root.go"}
	sort.Strings(want)
	if strings.Join(claims, ",") != strings.Join(want, ",") {
		t.Fatalf("the halves must carry the whole tree: %v", claims)
	}
	for _, p := range []string{"src/a/one.go", "src/b/two.go", "src/c/three.go", "src/d/four.go", "src/root.go"} {
		own := w.Owner(p)
		if own == nil || (own.Name != "zone-wide-a" && own.Name != "zone-wide-b") {
			t.Fatalf("%s must ride with a young one: %+v", p, own)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".agent-one/instruments/ledgers.jsonl"))
	if !strings.Contains(string(b), `"lane":"split-tree"`) {
		t.Fatalf("the act must write the tree-split to the shelf:\n%s", b)
	}
}

package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/memory"
	"github.com/Kaginari/agent-one/onto"
	"github.com/Kaginari/agent-one/value"
)

// evoWorkspace writes a fixture workspace: a domain owner over one zone worker whose zone is src/pay/ (two
// sub-directories with code), the zone worker's doc carrying the given Notes.
func evoWorkspace(t *testing.T, notes []string, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	workingNotes := strings.Join(notes, "\n")
	files := map[string]string{
		".agent-one/AGENT-ONE.md":         policyText,
		".agent-one/log.md":               "# Chronicle\n",
		".agent-one/coord/core/README.md": "# coord-core\n\n- **Rank:** Coordinator\n- **Owns:** `docs/`\n- **Reports to:** orchestrator\n",
		".agent-one/domain/pay/README.md": "# domain-pay\n\n- **Rank:** Domain owner\n- **Owns:** `src/`\n- **Reports to:** coord-core\n",
		".agent-one/zone/pay/README.md":   "# zone-pay\n\n- **Rank:** Zone worker\n- **Owns:** `src/pay/`\n- **Reports to:** domain-pay\n- **Skills:** none\n- **Purpose:** the payments zone\n\n## Invariants\n- charges are idempotent\n\n## Verify\n- `true`\n\n## Working notes\n" + workingNotes + "\n",
		"docs/README.md":                  "docs\n",
		"src/pay/api/handler.go":          "package api\n\nfunc Handle() {}\n",
		"src/pay/api/routes.go":           "package api\n\nfunc Routes() {}\n",
		"src/pay/core/ledger.go":          "package core\n\nfunc Post() {}\n",
		"src/pay/core/ledger_test.go":     "package core\n\nimport \"testing\"\n\nfunc TestPost(t *testing.T) {}\n",
	}
	for p, s := range extra {
		files[p] = s
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
	return dir
}

func evoOpen(t *testing.T, dir string, mut func(*EvolutionOptions)) *Workspace {
	t.Helper()
	o := EvolutionOptions{Enabled: true, Mode: "auto", NotesMax: 7, StressAt: 5, FailStreak: 3, CooldownRuns: 10, Arms: []string{"split", "mutate", "rebirth"}, SplitLines: 0, SplitFiles: 100, SkillStatus: "trial"}
	if mut != nil {
		mut(&o)
	}
	w, err := Open(dir, Default(), Options{Ontology: true, Evolution: o})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func stress(w *Workspace, agent string, n int) {
	w.mu.Lock()
	w.stress[agent] = n
	w.mu.Unlock()
}

func ontoClean(t *testing.T, w *Workspace) {
	t.Helper()
	ow, err := onto.LoadLayout(w.Root, w.Ranks.Layout(w.Lex))
	if err != nil {
		t.Fatal(err)
	}
	if fs := ow.Validate(); len(fs) > 0 {
		t.Fatalf("onto check must stay at 0 findings, got %v", fs)
	}
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// eight Notes with no repeated lesson
func loose(n int) []string {
	words := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliet"}
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf("- 2026-10-%02d %s", i+1, words[i]))
	}
	return out
}

func TestEvolveTriggerNeedsStressAndAFullWorkingNotes(t *testing.T) {
	// seven Notes, stressed: the working notes is not past notesMax
	w := evoOpen(t, evoWorkspace(t, loose(7), nil), nil)
	stress(w, "zone-pay", 9)
	if ev, ok := w.EvolveMaybe("zone-pay"); ok {
		t.Fatalf("a working notes at notesMax must not evolve: %+v", ev)
	}
	// eight Notes, calm: not stressed
	w = evoOpen(t, evoWorkspace(t, loose(8), nil), nil)
	if ev, ok := w.EvolveMaybe("zone-pay"); ok {
		t.Fatalf("an unstressed agent must not evolve: %+v", ev)
	}
	if st := w.StressOf("zone-pay"); st.Stressed {
		t.Fatalf("no reading says stressed: %+v", st)
	}
	// eight Notes AND stressed: it evolves
	stress(w, "zone-pay", 5)
	ev, ok := w.EvolveMaybe("zone-pay")
	if !ok || ev.Status != "applied" {
		t.Fatalf("stressed with 8 Notes must evolve: %+v ok=%v", ev, ok)
	}
	// with no repetition and no other evidence, the working notes is still consolidated (Principle 5)
	if ev.Arm != "mutate" || !strings.Contains(ev.Reason, "Principle 5") {
		t.Fatalf("the fallback is the plain distillation: %+v", ev)
	}
	if n := w.WorkingNotesOf("zone-pay"); n > 1 {
		t.Fatalf("the working notes must be emptied but its dated line, got %d", n)
	}
	log := read(t, w.Root, ".agent-one/log.md")
	if !strings.Contains(log, "evolution — zone-pay mutated its configuration") || !strings.Contains(log, "wrap-up: alpha") {
		t.Fatalf("the evolution and its wrap-up are logged:\n%s", log)
	}
	ontoClean(t, w)
}

func TestEvolveStressReadings(t *testing.T) {
	w := evoOpen(t, evoWorkspace(t, loose(8), nil), nil)
	for i := 0; i < 3; i++ {
		w.FoldRun(RunFold{Agent: "zone-pay", Loop: "DONE", Word: "FAIL", Gate: "fail"})
	}
	st := w.StressOf("zone-pay")
	if !st.Stressed || st.Streak != 3 {
		t.Fatalf("three failed verdicts in a row read stressed: %+v", st)
	}
	w2 := evoOpen(t, evoWorkspace(t, loose(8), nil), nil)
	w2.FoldRun(RunFold{Agent: "zone-pay", Loop: "CHECKPOINT"})
	if st := w2.StressOf("zone-pay"); !st.Stressed || !st.Doom {
		t.Fatalf("a doom checkpoint reads stressed: %+v", st)
	}
}

func splitNotes() []string {
	return []string{
		"- 2026-10-01 `src/pay/api/handler.go` returns 500 on a bad currency",
		"- 2026-10-02 `src/pay/api/routes.go` mounts the refund route twice",
		"- 2026-10-03 api/handler.go swallows the timeout",
		"- 2026-10-04 `src/pay/api/handler.go` leaks the idempotency key",
		"- 2026-10-05 `src/pay/core/ledger.go` double posts on retry",
		"- 2026-10-06 core/ledger.go rounds half-down",
		"- 2026-10-07 `src/pay/core/ledger.go` lacks a lock",
		"- 2026-10-08 the zone feels wide",
	}
}

func TestEvolveSplitDividesTheZoneAndTheWorkingNotes(t *testing.T) {
	w := evoOpen(t, evoWorkspace(t, splitNotes(), nil), func(o *EvolutionOptions) { o.SplitFiles = 3 })
	stress(w, "zone-pay", 6)
	ev, ok := w.EvolveMaybe("zone-pay")
	if !ok || ev.Status != "applied" || ev.Arm != "split" {
		t.Fatalf("a large clustered zone splits: %+v", ev)
	}
	if strings.Join(ev.Into, ",") != "zone-pay-api,zone-pay-core" {
		t.Fatalf("the young are named for their halves: %v", ev.Into)
	}
	if w.Member("zone-pay") != nil {
		t.Fatal("the parent retires off the roster")
	}
	a, c := w.Member("zone-pay-api"), w.Member("zone-pay-core")
	if a == nil || c == nil {
		t.Fatalf("both young stand: %v %v", a, c)
	}
	if own := w.Owner("src/pay/api/handler.go"); own == nil || own.Name != "zone-pay-api" {
		t.Fatalf("api soil rides with zone-pay-api: %+v", own)
	}
	if own := w.Owner("src/pay/core/ledger.go"); own == nil || own.Name != "zone-pay-core" {
		t.Fatalf("core soil rides with zone-pay-core: %+v", own)
	}
	if a.Parent != "domain-pay" || c.Parent != "domain-pay" {
		t.Fatalf("the young report where the parent did: %s %s", a.Parent, c.Parent)
	}
	da, dc := read(t, w.Root, a.Doc), read(t, w.Root, c.Doc)
	if !strings.Contains(da, "routes.go") || strings.Contains(da, "ledger.go") {
		t.Fatalf("the api half carries the api Notes only:\n%s", da)
	}
	if !strings.Contains(dc, "lacks a lock") || strings.Contains(dc, "refund route") {
		t.Fatalf("the core half carries the core Notes only:\n%s", dc)
	}
	na, nc := len(memory.NotesOf(da)), len(memory.NotesOf(dc))
	if na+nc != 8 {
		t.Fatalf("every Note goes to one half: %d + %d", na, nc)
	}
	if !strings.Contains(dc, "go test") && !strings.Contains(dc, "`true`") {
		t.Fatalf("the young carry a verify line:\n%s", dc)
	}
	parent := read(t, w.Root, ".agent-one/zone/pay/README.md")
	if !docRetired(parent) || !strings.Contains(parent, "split into zone-pay-api + zone-pay-core") {
		t.Fatalf("the parent's doc carries the retired mark:\n%s", parent)
	}
	if _, err := os.Stat(filepath.Join(w.Root, ".agent-one", "attic")); err != nil {
		t.Fatal("the parent's configuration rests in the attic")
	}
	ontoClean(t, w)
	var row *value.Row
	for _, r := range w.Ledger() {
		if r.Kind == value.KindEvolve {
			r := r
			row = &r
		}
	}
	if row == nil || row.Arm != "split" || row.Status != "applied" || len(row.Into) != 2 {
		t.Fatalf("the ledger notes the evolution: %+v", row)
	}
	if !strings.Contains(read(t, w.Root, ".agent-one/log.md"), "zone-pay evolved: split → zone-pay-api, zone-pay-core") {
		t.Fatal("the split is logged")
	}
}

func TestEvolveSplitAnDomainDividesItsZones(t *testing.T) {
	dir := evoWorkspace(t, nil, map[string]string{
		".agent-one/domain/pay/README.md": "# domain-pay\n\n- **Rank:** Domain owner\n- **Owns:** `src/`\n- **Reports to:** coord-core\n\n## Working notes\n" + strings.Join([]string{"- 2026-10-01 `src/pay/` slow", "- 2026-10-02 `src/pay/` again", "- 2026-10-03 `src/web/` broke", "- 2026-10-04 `src/web/` css", "- 2026-10-05 `src/pay/` x", "- 2026-10-06 `src/web/` y", "- 2026-10-07 `src/pay/` z", "- 2026-10-08 `src/web/` w"}, "\n") + "\n",
		".agent-one/zone/web/README.md":   "# zone-web\n\n- **Rank:** Zone worker\n- **Owns:** `src/web/`\n- **Reports to:** domain-pay\n- **Verify:** `true`\n",
		"src/web/app.go":                  "package web\n",
	})
	w := evoOpen(t, dir, func(o *EvolutionOptions) { o.SplitFiles = 3 })
	stress(w, "domain-pay", 6)
	ev, ok := w.EvolveMaybe("domain-pay")
	if !ok || ev.Status != "applied" || ev.Arm != "split" {
		t.Fatalf("a domain owner splits: %+v", ev)
	}
	if w.Member("domain-pay-pay") == nil || w.Member("domain-pay-web") == nil {
		t.Fatalf("two domain owners stand: %v", ev.Into)
	}
	if p := w.Member("zone-pay").Parent; p != "domain-pay-pay" {
		t.Fatalf("zone-pay follows its soil: %s", p)
	}
	if p := w.Member("zone-web").Parent; p != "domain-pay-web" {
		t.Fatalf("zone-web follows its soil: %s", p)
	}
	ontoClean(t, w)
}

func mutateNotes() []string {
	return []string{
		"- 2026-10-01 retry: always run `go test ./src/pay/core/` before touching the retry loop",
		"- 2026-10-02 the retry loop needs the jitter first",
		"- 2026-10-03 retry storms: use the backoff helper, never sleep",
		"- 2026-10-04 retry counts must reset after a success",
		"- 2026-10-05 retry: run the ledger test then the api test",
		"- 2026-10-06 retry window comes from config",
		"- 2026-10-07 retry in `src/pay/core/ledger.go` must be idempotent",
		"- 2026-10-08 refunds settle in two days",
	}
}

func TestEvolveMutateDistilsAndRegistersTheHow(t *testing.T) {
	w := evoOpen(t, evoWorkspace(t, mutateNotes(), nil), func(o *EvolutionOptions) {
		o.Survey = func(p []string) OwnershipSurvey {
			sv := OwnershipSurvey{Files: 1, Lines: 3}
			if len(p) == 1 && strings.HasPrefix(p[0], "src/pay/core") {
				sv.Verify = []string{"test -f src/pay/core/ledger_test.go"}
			}
			return sv
		}
	})
	stress(w, "zone-pay", 5)
	ev, ok := w.EvolveMaybe("zone-pay")
	if !ok || ev.Status != "applied" || ev.Arm != "mutate" {
		t.Fatalf("a repeated lesson mutates: %+v", ev)
	}
	if !strings.Contains(ev.Reason, `name "retry"`) {
		t.Fatalf("the reason names the lesson: %s", ev.Reason)
	}
	doc := read(t, w.Root, ".agent-one/zone/pay/README.md")
	if !strings.Contains(doc, "- retry — ") || !strings.Contains(doc, "from 7 notes") {
		t.Fatalf("the lesson is an invariant:\n%s", doc)
	}
	if !strings.Contains(doc, "- `test -f src/pay/core/ledger_test.go`") {
		t.Fatalf("a verify line where tests cover the lesson's paths:\n%s", doc)
	}
	if ev.Skill != "zone-pay-retry" || !strings.Contains(doc, "- **Skills:** zone-pay-retry") {
		t.Fatalf("the how is registered and held: %q\n%s", ev.Skill, doc)
	}
	skill := read(t, w.Root, ".agent-one/crafted-skills/zone-pay-retry/SKILL.md")
	if !strings.Contains(skill, "status: trial") || !strings.Contains(skill, "name: zone-pay-retry") || !strings.Contains(skill, "backoff helper") {
		t.Fatalf("the skill file:\n%s", skill)
	}
	ts := memory.NotesOf(doc)
	if len(ts) != 1 || !strings.Contains(ts[0].Text, "consolidated into") {
		t.Fatalf("the working notes keeps only its dated consolidated line: %+v", ts)
	}
	if c := w.Member("zone-pay"); c == nil || !contains(c.Skills, "zone-pay-retry") || !sameSoil(c.Ownership, []string{"src/pay/"}) {
		t.Fatalf("same name, same ownership, the new hat held: %+v", c)
	}
	if !strings.Contains(read(t, w.Root, ".agent-one/log.md"), "wrap-up: refunds settle in two days") {
		t.Fatal("the single goes to the log's wrap-up")
	}
	ontoClean(t, w)
}

func TestEvolveMutateDistilsAHeldSkillsWorkingNotes(t *testing.T) {
	skill := "---\nname: zone-pay-wire\ndescription: the wire\n---\n# wire\n\n## Working notes\n" + strings.Join([]string{"- 2026-10-01 envelope cap trims findings", "- 2026-10-02 envelope cap hides holes", "- 2026-10-03 envelope cap again"}, "\n") + "\n"
	dir := evoWorkspace(t, loose(5), map[string]string{".agent-one/crafted-skills/zone-pay-wire/SKILL.md": skill})
	doc := read(t, dir, ".agent-one/zone/pay/README.md")
	_ = os.WriteFile(filepath.Join(dir, ".agent-one/zone/pay/README.md"), []byte(strings.Replace(doc, "- **Skills:** none", "- **Skills:** zone-pay-wire", 1)), 0o644)
	w := evoOpen(t, dir, nil)
	if n := w.WorkingNotesOf("zone-pay"); n != 8 {
		t.Fatalf("the working notes counts the doc's and the held skill's: %d", n)
	}
	stress(w, "zone-pay", 5)
	if ev, ok := w.EvolveMaybe("zone-pay"); !ok || ev.Status != "applied" {
		t.Fatalf("it evolves: %+v", ev)
	}
	m := read(t, dir, ".agent-one/crafted-skills/zone-pay-wire/SKILL.md")
	if ts := memory.NotesOf(m); len(ts) != 1 || !strings.Contains(ts[0].Text, "consolidated into zone-pay's configuration") {
		t.Fatalf("the skill's working notes is consolidated too:\n%s", m)
	}
	if !strings.Contains(read(t, dir, ".agent-one/zone/pay/README.md"), "envelope") {
		t.Fatal("the skill's lesson lands in the agent's invariants")
	}
	ontoClean(t, w)
}

func TestEvolveRebirthWritesAFreshConfiguration(t *testing.T) {
	dir := evoWorkspace(t, loose(8), nil)
	doc := read(t, dir, ".agent-one/zone/pay/README.md")
	_ = os.WriteFile(filepath.Join(dir, ".agent-one/zone/pay/README.md"), []byte(strings.Replace(doc, "- charges are idempotent", "- charges post through `src/pay/old/charge.go`", 1)), 0o644)
	w := evoOpen(t, dir, func(o *EvolutionOptions) {
		o.Survey = func(p []string) OwnershipSurvey {
			return OwnershipSurvey{Files: 4, Lines: 12, Invariants: []string{"sketch: Go packages api, core: 3 source files."}, Verify: []string{"test -d src/pay"}}
		}
	})
	for i := 0; i < 4; i++ {
		w.FoldRun(RunFold{Agent: "zone-pay", Loop: "DONE", Word: "FAIL", Gate: "fail", VerifyRun: 1, VerifyRed: 1})
	}
	if m := w.Metrics("zone-pay"); m["runs"] != 4 {
		t.Fatalf("the record holds 4 runs: %v", m["runs"])
	}
	ev, ok := w.EvolveMaybe("zone-pay")
	if !ok || ev.Status != "applied" || ev.Arm != "rebirth" {
		t.Fatalf("a stale configuration is reborn: %+v", ev)
	}
	if !strings.Contains(ev.Reason, "its gate failed on its own verify lines") || !strings.Contains(ev.Reason, "src/pay/old/charge.go") {
		t.Fatalf("the reason names the evidence: %s", ev.Reason)
	}
	fresh := read(t, w.Root, ".agent-one/zone/pay/README.md")
	for _, want := range []string{"- **Reborn:**", "sketch: Go packages api, core", "- `test -d src/pay`", "- **Reports to:** domain-pay", "- **Owns:** `src/pay/`"} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("the fresh doc carries %q:\n%s", want, fresh)
		}
	}
	if strings.Contains(strings.Join(sectionBullets(fresh, "Invariants"), "\n"), "src/pay/old/charge.go") {
		t.Fatal("the stale invariant does not come back")
	}
	atts, _ := filepath.Glob(filepath.Join(w.Root, ".agent-one", "attic", "zone-pay@*.md"))
	if len(atts) != 1 || !strings.Contains(read(t, w.Root, w.Rel(atts[0])), "src/pay/old/charge.go") {
		t.Fatalf("the old configuration rests in the attic: %v", atts)
	}
	if m := w.Metrics("zone-pay"); m["runs"] != 0 || m["fails"] != 0 {
		t.Fatalf("the value window starts again: runs %v fails %v", m["runs"], m["fails"])
	}
	reborn := false
	for _, r := range w.Ledger() {
		reborn = reborn || (r.Kind == value.KindRebirth && r.Agent == "zone-pay")
	}
	if !reborn {
		t.Fatal("a rebirth row is written")
	}
	ontoClean(t, w)
}

func TestEvolveFailedVerifyKeepsTheOldDoc(t *testing.T) {
	dir := evoWorkspace(t, loose(8), nil)
	w := evoOpen(t, dir, func(o *EvolutionOptions) {
		o.Survey = func(p []string) OwnershipSurvey { return OwnershipSurvey{Verify: []string{"exit 3"}} }
	})
	w.FoldRun(RunFold{Agent: "zone-pay", Loop: "DONE", Word: "FAIL", Gate: "fail", VerifyRun: 1, VerifyRed: 1})
	stress(w, "zone-pay", 5)
	before := read(t, dir, ".agent-one/zone/pay/README.md")
	ev, ok := w.EvolveMaybe("zone-pay")
	if !ok || ev.Status != "hole" || !strings.Contains(ev.Hole, "exit 3") {
		t.Fatalf("a red verify is a hole: %+v", ev)
	}
	if after := read(t, dir, ".agent-one/zone/pay/README.md"); after != before {
		t.Fatal("the old doc stands untouched")
	}
	if _, err := os.Stat(filepath.Join(dir, ".agent-one", "attic")); err == nil {
		t.Fatal("nothing was swapped in, not even the attic copy")
	}
	if !strings.Contains(read(t, dir, ".agent-one/log.md"), "evolution hole — zone-pay") {
		t.Fatal("the hole is logged")
	}
	if cd := w.CooldownOf("zone-pay"); cd.Left == 0 {
		t.Fatal("a failed try starts the cooldown too (no redraw on every landing)")
	}
	ontoClean(t, w)
}

func TestEvolveCooldown(t *testing.T) {
	dir := evoWorkspace(t, loose(8), nil)
	w := evoOpen(t, dir, func(o *EvolutionOptions) { o.CooldownRuns = 2 })
	stress(w, "zone-pay", 5)
	if _, ok := w.EvolveMaybe("zone-pay"); !ok {
		t.Fatal("first evolution")
	}
	// the working notes fills again and the strain returns
	doc := read(t, dir, ".agent-one/zone/pay/README.md")
	_ = os.WriteFile(filepath.Join(dir, ".agent-one/zone/pay/README.md"), []byte(doc+strings.Join(loose(8), "\n")+"\n"), 0o644)
	stress(w, "zone-pay", 5)
	if ev, ok := w.EvolveMaybe("zone-pay"); ok {
		t.Fatalf("inside the cooldown nothing evolves: %+v", ev)
	}
	w.FoldRun(RunFold{Agent: "zone-pay", Loop: "DONE", Word: "FAIL"})
	w.FoldRun(RunFold{Agent: "zone-pay", Loop: "DONE", Word: "FAIL"})
	if cd := w.CooldownOf("zone-pay"); cd.Left != 0 || cd.RunsSince != 2 {
		t.Fatalf("two runs later the cooldown is spent: %+v", cd)
	}
	// the cooldown is persisted: a reopened workspace remembers it
	w2 := evoOpen(t, dir, func(o *EvolutionOptions) { o.CooldownRuns = 3 })
	if cd := w2.CooldownOf("zone-pay"); cd.Last == nil || cd.Left != 1 {
		t.Fatalf("the ledger carries the cooldown across a restart: %+v", cd)
	}
	if ev, ok := w.EvolveMaybe("zone-pay"); !ok || ev.Status != "applied" {
		t.Fatalf("out of the cooldown it evolves again: %+v", ev)
	}
}

func TestEvolveProposeQueuesInsteadOfApplying(t *testing.T) {
	dir := evoWorkspace(t, mutateNotes(), nil)
	var heard []EvolveEvent
	w := evoOpen(t, dir, func(o *EvolutionOptions) {
		o.Mode = "propose"
		o.On = func(e EvolveEvent) { heard = append(heard, e) }
	})
	stress(w, "zone-pay", 5)
	before := read(t, dir, ".agent-one/zone/pay/README.md")
	ev, ok := w.EvolveMaybe("zone-pay")
	if !ok || ev.Status != "proposed" || ev.ID == "" {
		t.Fatalf("propose mode queues: %+v", ev)
	}
	if read(t, dir, ".agent-one/zone/pay/README.md") != before {
		t.Fatal("nothing is applied in propose mode")
	}
	if ev2, ok := w.EvolveMaybe("zone-pay"); ok {
		t.Fatalf("an open proposal is not proposed again: %+v", ev2)
	}
	p, err := w.Proposal(ev.ID[:5])
	if err != nil || p.Status != "open" || !strings.Contains(p.Diff(), "+- retry — ") {
		t.Fatalf("the queue holds the diff: %v\n%s", err, p.Diff())
	}
	got, err := w.ApplyProposal(context.Background(), ev.ID)
	if err != nil || got.Status != "applied" {
		t.Fatalf("apply lands it through the same proof: %v %+v", err, got)
	}
	if !strings.Contains(read(t, dir, ".agent-one/zone/pay/README.md"), "- retry — ") {
		t.Fatal("applied")
	}
	if p, _ := w.Proposal(ev.ID); p.Status != "applied" {
		t.Fatalf("the proposal is marked applied: %s", p.Status)
	}
	if len(heard) != 2 || heard[0].Status != "proposed" || heard[1].Status != "applied" {
		t.Fatalf("the surfaces hear both: %+v", heard)
	}
	ontoClean(t, w)
	// a second proposal, rejected
	dir2 := evoWorkspace(t, loose(8), nil)
	w2 := evoOpen(t, dir2, func(o *EvolutionOptions) { o.Mode = "propose" })
	stress(w2, "zone-pay", 5)
	ev, _ = w2.EvolveMaybe("zone-pay")
	if _, err := w2.RejectProposal(ev.ID, "not now"); err != nil {
		t.Fatal(err)
	}
	if _, err := w2.ApplyProposal(context.Background(), ev.ID); err == nil {
		t.Fatal("a rejected proposal never applies")
	}
}

func TestEvolveProposalRefusesAMovedWorkspace(t *testing.T) {
	dir := evoWorkspace(t, loose(8), nil)
	w := evoOpen(t, dir, func(o *EvolutionOptions) { o.Mode = "propose" })
	stress(w, "zone-pay", 5)
	ev, _ := w.EvolveMaybe("zone-pay")
	p := filepath.Join(dir, ".agent-one/zone/pay/README.md")
	b, _ := os.ReadFile(p)
	_ = os.WriteFile(p, append(b, []byte("- 2026-10-11 a human wrote here\n")...), 0o644)
	if _, err := w.ApplyProposal(context.Background(), ev.ID); err == nil || !strings.Contains(err.Error(), "changed since the plan") {
		t.Fatalf("a doc that moved under the plan refuses it: %v", err)
	}
}

func TestEvolveOffAndSweep(t *testing.T) {
	dir := evoWorkspace(t, loose(8), nil)
	w := evoOpen(t, dir, func(o *EvolutionOptions) { o.NotesMax = 0 })
	stress(w, "zone-pay", 9)
	if evs := w.EvolveSweep(); len(evs) != 0 {
		t.Fatalf("notesMax 0 is off: %+v", evs)
	}
	w = evoOpen(t, dir, nil)
	stress(w, "zone-pay", 9)
	evs := w.EvolveSweep()
	if len(evs) != 1 || evs[0].Agent != "zone-pay" {
		t.Fatalf("the run-start sweep weighs every agent: %+v", evs)
	}
	if lines := w.evolveLines(); len(lines) != 1 || !strings.HasPrefix(lines[0], "[evolution] zone-pay mutated its configuration") {
		t.Fatalf("the dispatcher's report carries it: %v", lines)
	}
	rep := w.ValueReport()
	if len(rep.Evolution.Recent) != 1 || rep.Evolution.NotesMax != 7 {
		t.Fatalf("/api/value carries the evolution: %+v", rep.Evolution)
	}
	for _, b := range rep.Agents {
		if b.Agent == "zone-pay" && (b.Evolution == nil || b.Evolution.Last == nil || b.Evolution.CooldownLeft != 10) {
			t.Fatalf("the agent's cooldown and last evolution: %+v", b.Evolution)
		}
	}
}

func TestNotesOfMatchesTheWorkingNotesCount(t *testing.T) {
	doc := "# x\n\n## Working notes\npreamble\n- 2026-10-01 one\n  more of one\n### [2026-10-02]\nprose of two\n- not dated\n\n## Next\n- 2026-10-03 outside\n"
	ts := memory.NotesOf(doc)
	if len(ts) != 2 || memory.CountNotes(doc) != 2 {
		t.Fatalf("two dated entries: %+v", ts)
	}
	if !strings.Contains(ts[0].Text, "more of one") || !strings.Contains(ts[1].Text, "- not dated") {
		t.Fatalf("an entry carries its continuation lines: %+v", ts)
	}
	cut := cutWorkingNotes(doc, ts, "- 2026-10-04 consolidated into 1 invariant (evolution)")
	if memory.CountNotes(cut) != 1 || !strings.Contains(cut, "preamble") || !strings.Contains(cut, "## Next\n- 2026-10-03 outside") {
		t.Fatalf("the cut leaves the rest whole:\n%s", cut)
	}
}

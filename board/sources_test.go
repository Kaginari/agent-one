package board

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/onto"
)

var fixedNow = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func fixtureRoot(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fixtureSources(t *testing.T, name string) Sources {
	return FileSources(fixtureRoot(t, name), ".agent-one", onto.Layout{}, func() time.Time { return fixedNow })
}

func TestUsageReadAndRollup(t *testing.T) {
	s := fixtureSources(t, "workspace")
	recs, rd := s.Usage()
	if !rd.Lit {
		t.Fatalf("usage should be lit: %s", rd)
	}
	if len(recs) != 5 {
		t.Fatalf("want 5 records (one bad line skipped), got %d", len(recs))
	}
	if !strings.Contains(rd.Why, "skipped: 1") {
		t.Errorf("the skipped line must be named: %q", rd.Why)
	}
	since, rng := RangeSince("7d", fixedNow)
	r := Rollup(recs, since, rng)
	if r.Records != 5 || r.Total.Calls != 5 {
		t.Errorf("total calls %d", r.Total.Calls)
	}
	if r.Total.Tokens() != 1200+300+20000+5000+800+900+500+100+2000+400+30000+1000+700+1500 {
		t.Errorf("total tokens %d", r.Total.Tokens())
	}
	if r.Total.Unpriced != 1 {
		t.Errorf("one call is unpriced (usd null), got %d", r.Total.Unpriced)
	}
	if len(r.ByDay) != 2 || r.ByDay[0].Key != "2026-09-25" {
		t.Errorf("by day: %+v", r.ByDay)
	}
	if r.ByAgent[0].Key != "orchestrator" {
		t.Errorf("biggest agent should be orchestrator: %+v", r.ByAgent)
	}
	if len(r.Sessions) != 2 {
		t.Errorf("sessions: %v", r.Sessions)
	}
	day, _ := RangeSince("24h", fixedNow)
	if today := Rollup(recs, day, "24h"); today.Records != 2 {
		t.Errorf("24h should hold today's 2 calls, got %d", today.Records)
	}
	if all, _ := RangeSince("all", fixedNow); !all.IsZero() {
		t.Error("all = zero time")
	}
}

func TestUsageSilentOnEmptyWorkspace(t *testing.T) {
	s := fixtureSources(t, "empty")
	recs, rd := s.Usage()
	if len(recs) != 0 || rd.Lit {
		t.Fatalf("empty workspace must be silent, not zero: %+v", rd)
	}
	if !strings.HasPrefix(rd.String(), "never lit") {
		t.Errorf("reading: %s", rd)
	}
	if _, rd := s.Loop(); rd.Lit {
		t.Error("loop should be silent")
	}
	if _, rd := s.Log(); rd.Lit {
		t.Error("log should be silent")
	}
	if c := s.Team(); c.Reading.Lit {
		t.Error("team should be silent with no members")
	}
	if m := s.Memory(); m.Reading.Lit {
		t.Error("memory should be silent")
	}
	if tb := s.Toolbox(); tb.Reading.Lit {
		t.Error("toolbox should be silent")
	}
}

func TestLogParse(t *testing.T) {
	s := fixtureSources(t, "workspace")
	entries, rd := s.Log()
	if !rd.Lit || len(entries) != 2 {
		t.Fatalf("log: %d entries, %s", len(entries), rd)
	}
	if entries[0].At != "2026-09-25T18:30:00+02:00" || !strings.HasPrefix(entries[0].Who, "ephemeral-subagent") {
		t.Errorf("newest first: %+v", entries[0])
	}
	if !strings.Contains(entries[0].Body, "token TTL is 15m") {
		t.Errorf("agent kept: %q", entries[0].Body)
	}
}

func TestLoopRuns(t *testing.T) {
	s := fixtureSources(t, "workspace")
	runs, rd := s.Loop()
	if !rd.Lit || len(runs) != 1 {
		t.Fatalf("runs: %+v %s", runs, rd)
	}
	if runs[0].Status != "PASS" || runs[0].As != "zone-auth" || runs[0].Steps != 1 {
		t.Errorf("run: %+v", runs[0])
	}
}

func TestTeamGraph(t *testing.T) {
	s := fixtureSources(t, "workspace")
	c := s.Team()
	if !c.Reading.Lit {
		t.Fatalf("team: %s", c.Reading)
	}
	byID := map[string]Node{}
	for _, n := range c.Nodes {
		byID[n.ID] = n
	}
	if byID["zone-auth"].Rank != "zone" || byID["domain-core"].Rank != "domain" || byID["coord-voice"].Rank != "coord" {
		t.Errorf("ranks: %+v", byID)
	}
	if byID["skill-zone-auth-skill"].Lane != "zone" {
		t.Errorf("zone-auth-skill lane: %q", byID["skill-zone-auth-skill"].Lane)
	}
	if byID["skill-tdd"].Lane != "shared" || !byID["skill-tdd"].Shared {
		t.Errorf("tdd (host skill, no rank prefix) is shared: %+v", byID["skill-tdd"])
	}
	if byID["zone-auth"].Doc != ".agent-one/zone/auth/README.md" {
		t.Errorf("doc: %q", byID["zone-auth"].Doc)
	}
	want := map[string]bool{"zone-auth truth domain-core": false, "domain-core verdict coord-voice": false, "coord-voice reports orchestrator": false, "zone-auth holds skill-tdd": false}
	for _, e := range c.Edges {
		k := e.From + " " + e.Bond + " " + e.To
		if _, ok := want[k]; ok {
			want[k] = true
		}
		if e.Bond == "above" {
			t.Errorf("transitive closure leaked into the drawing: %+v", e)
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("missing edge %s", k)
		}
	}
	if c.Lanes[0] != "skill:zone" || c.Lanes[len(c.Lanes)-1] != "skill:shared" {
		t.Errorf("lanes: %v", c.Lanes)
	}
}

func TestMemoryAndToolboxViews(t *testing.T) {
	s := fixtureSources(t, "workspace")
	m := s.Memory()
	if !m.Reading.Lit {
		t.Fatalf("memory: %s", m.Reading)
	}
	if len(m.Notes) != 3 || m.ByKind["team"] != 1 || m.ByKind["domain"] != 1 || m.ByKind["untyped"] != 1 {
		t.Errorf("notes by kind: %v (%d notes)", m.ByKind, len(m.Notes))
	}
	if m.Notes[0].Text != "an untyped note" {
		t.Errorf("newest first: %+v", m.Notes[0])
	}
	if len(m.WorkingNotes) == 0 {
		t.Error("the fixture skill has a workingNotes")
	}
	tb := s.Toolbox()
	if !tb.Reading.Lit {
		t.Fatalf("toolbox: %s", tb.Reading)
	}
	if !tb.Offered["tdd"] || !tb.Loaded["tdd"] || tb.Loaded["zone-auth-skill"] {
		t.Errorf("offered %v loaded %v", tb.Offered, tb.Loaded)
	}
	if len(tb.Entries) < 2 {
		t.Errorf("registry entries: %d", len(tb.Entries))
	}
}

func TestDocIsFenced(t *testing.T) {
	s := fixtureSources(t, "workspace")
	if _, err := s.Doc(".agent-one/zone/auth/README.md"); err != nil {
		t.Errorf("workspace doc readable: %v", err)
	}
	if _, err := s.Doc(".claude/skills/tdd/SKILL.md"); err != nil {
		t.Errorf("skill doc readable: %v", err)
	}
	for _, bad := range []string{"../../go.mod", ".agent-one/../.agent-one/../sources.go", "/etc/passwd", ".agent-one/name"} {
		if _, err := s.Doc(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestTeamIsOneTreeUnderTheOrchestrator(t *testing.T) {
	c := fixtureSources(t, "workspace").Team()
	byID := map[string]Node{}
	for _, n := range c.Nodes {
		byID[n.ID] = n
	}
	orchestrator, ok := byID[c.Orchestrator]
	if c.Orchestrator == "" || !ok || !orchestrator.Orchestrator || orchestrator.Kind != "member" {
		t.Fatalf("the orchestrator must be a node: %q %+v", c.Orchestrator, orchestrator)
	}
	for _, n := range c.Nodes {
		if n.Kind != "member" || n.Orchestrator {
			if n.Parent != "" {
				t.Errorf("%s (a %s) has a parent %q", n.ID, n.Kind, n.Parent)
			}
			continue
		}
		seen := map[string]bool{}
		at := n.ID
		for at != c.Orchestrator {
			if seen[at] {
				t.Fatalf("%s loops at %s", n.ID, at)
			}
			seen[at] = true
			p, ok := byID[byID[at].Parent]
			if !ok {
				t.Fatalf("%s: %s's parent %q is not a node", n.ID, at, byID[at].Parent)
			}
			at = p.ID
		}
	}
	if byID["zone-auth"].Parent != "domain-core" || byID["domain-core"].Parent != "coord-voice" || byID["coord-voice"].Parent != c.Orchestrator {
		t.Errorf("the asserted bonds make the tree: %q %q %q", byID["zone-auth"].Parent, byID["domain-core"].Parent, byID["coord-voice"].Parent)
	}
	// an orphan hangs under the orchestrator by an implied edge, and a note says so
	implied := 0
	for _, e := range c.Edges {
		if e.Implied {
			implied++
			if e.Bond != "reports" || e.To != c.Orchestrator || byID[e.From].Parent != c.Orchestrator {
				t.Errorf("implied edge: %+v", e)
			}
		}
	}
	if implied == 0 || !strings.Contains(strings.Join(c.Notes, "\n"), "drawn under "+c.Orchestrator) {
		t.Errorf("the fixture's orphans must hang under the orchestrator, said: %d %v", implied, c.Notes)
	}
	// a skill in the workspace's own base-skills is a node, and its holder is bonded to it
	if byID["skill-zone-auth-craft"].Kind != "skill" {
		t.Fatalf("base-skills skill missing: %+v", byID["skill-zone-auth-craft"])
	}
	holds := false
	for _, e := range c.Edges {
		holds = holds || (e.From == "zone-auth" && e.Bond == "holds" && e.To == "skill-zone-auth-craft")
	}
	if !holds {
		t.Error("zone-auth holds zone-auth-craft (its Skills field) — no holds edge")
	}
}

func TestTreeUpAddsAMissingOrchestrator(t *testing.T) {
	c := &Team{Nodes: []Node{{ID: "coord-a", Kind: "member", Rank: "coord"}, {ID: "domain-b", Kind: "member", Rank: "domain"}, {ID: "domain-c", Kind: "member", Rank: "domain"}, {ID: "m", Kind: "skill"}},
		Edges: []Edge{{From: "domain-b", To: "coord-a", Bond: "verdict"}, {From: "coord-a", To: "orchestrator", Bond: "reports"}, {From: "domain-c", To: "domain-c", Bond: "above"}, {From: "domain-b", To: "m", Bond: "holds"}}}
	treeUp(c)
	var orchestrator *Node
	for i := range c.Nodes {
		if c.Nodes[i].ID == c.Orchestrator {
			orchestrator = &c.Nodes[i]
		}
	}
	if orchestrator == nil || !orchestrator.Orchestrator || c.Orchestrator != "orchestrator" {
		t.Fatalf("no orchestrator node added: %+v", c.Nodes)
	}
	parents := map[string]string{}
	for _, n := range c.Nodes {
		parents[n.ID] = n.Parent
	}
	if parents["domain-b"] != "coord-a" || parents["coord-a"] != "orchestrator" || parents["domain-c"] != "orchestrator" || parents["m"] != "" {
		t.Errorf("parents: %v", parents)
	}
}

func TestActivityRecentDropsTheWireHeader(t *testing.T) {
	runs := []Run{{As: "zone-loop", Ask: "@ROOT /w\n@ASK findings +unsaid\n@CAP 2048\nread the loop README\n", Status: "DONE", At: "2026-09-26T10:00:00Z"}}
	a := BuildActivity(runs, nil, fixedNow, ActivityDays)
	if len(a.Recent) != 1 || a.Recent[0].Ask != "read the loop README" {
		t.Errorf("recent: %+v", a.Recent)
	}
}

func TestActivityByDay(t *testing.T) {
	s := fixtureSources(t, "workspace")
	runs, _ := s.Loop()
	recs, _ := s.Usage()
	a := BuildActivity(runs, recs, fixedNow, ActivityDays)
	if len(a.Days) != ActivityDays || a.Days[ActivityDays-1] != fixedNow.UTC().Format("2006-01-02") {
		t.Fatalf("days: %v", a.Days)
	}
	sa := a.Agents["zone-auth"]
	if sa == nil || sa.Total != 1 || sa.Last == "" {
		t.Fatalf("zone-auth's run: %+v", sa)
	}
	i := -1
	for k, d := range a.Days {
		if d == "2026-09-25" {
			i = k
		}
	}
	if i < 0 || sa.Runs[i] != 1 || sa.Tokens[i] != 800+900+500+100 {
		t.Errorf("zone-auth on 2026-09-25: runs %v tokens %v", sa.Runs, sa.Tokens)
	}
	if r := a.Agents["orchestrator"]; r == nil || r.Total != 0 || r.Tokens[i] == 0 {
		t.Errorf("orchestrator spends tokens without a journaled run here: %+v", r)
	}
	if len(a.Recent) != 1 || a.Recent[0].As != "zone-auth" || a.Recent[0].Status != "PASS" || a.Recent[0].Ask != "smoke" {
		t.Errorf("recent: %+v", a.Recent)
	}
	if _, ok := a.Agents["domain-core"]; ok {
		t.Error("an agent the instruments never saw must be absent, not zero")
	}
}

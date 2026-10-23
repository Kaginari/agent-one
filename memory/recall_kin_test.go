package memory

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// kin-first recall: an agent's own member doc and its dated notes outrank the chronicle's
// mass — and another rank's doc still beats log.md. Census drift 2026-10-03: agents re-derived
// context from the chronicle because kin sources were never on top.
func TestRecallKinFirst(t *testing.T) {
	root := t.TempDir()
	agentOneDir := root + "/.agent-one"
	w := &Workspace{Root: root, Home: root, Now: time.Now}
	// a coordinator with a doc and dated notes carrying the task's terms
	writeFile(agentOneDir+"/coord/agent-one/README.md",
		"# coord-agent-one\n\n- **Rank:** Coordinator\n- **Owns:** the whole subagent\n\n## Working notes\n\n- census ownership onto mapping lands through isowns\n")
	writeFile(agentOneDir+"/coord/agent-one/notes/2026-10-03-census-brief.md",
		"# census brief\n\nThe onto census derives isowns ownership claims from the structured Ownership field; the write-gate's Owner() reads the same structure. Land the derivation in onto/workspace.go.\n")
	// the chronicle drags matching terms by mass alone
	writeFile(agentOneDir+"/log.md",
		"# Chronicle\n\n### [2026-10-02] orchestrator — census ownership chatter\n- census ownership onto isowns mentioned at length census ownership onto isowns census ownership\n")
	writeFile(agentOneDir+"/AGENT-ONE.md", "# policy\n")
	if _, err := w.Index(); err != nil {
		t.Fatalf("index: %v", err)
	}
	res, err := w.Recall("how does the onto census land isowns ownership", RecallOpts{As: "coord-agent-one", K: 3, NoCache: true})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(res.Results) == 0 {
		t.Fatal("no results")
	}
	top := res.Results[0]
	if !strings.Contains(top.Src, "coord/agent-one") {
		t.Fatalf("kin-first rule: the coordinator's own ownership must top the recall, got %+v", res.Results)
	}
	// the chronicle only surfaces when kin has nothing to say
	res2, _ := w.Recall("how does the onto census land isowns ownership", RecallOpts{As: "zone-app-commands", K: 5, NoCache: true})
	logRank := -1
	for i, r := range res2.Results {
		if strings.HasSuffix(r.Src, "log.md") {
			logRank = i
		}
	}
	if logRank == 0 {
		t.Errorf("the chronicle must never be the highest-ranked anchor while kin memory exists, got %+v", res2.Results)
	}
}

// The recall window seats at most two memories per source: a chronicle whose every entry
// matches cannot take all five seats from a doc that also answers.
func TestRecallWindowDiversity(t *testing.T) {
	root := t.TempDir()
	w := &Workspace{Root: root, Home: root, Now: time.Now}
	var log strings.Builder
	log.WriteString("# Chronicle\n")
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&log, "\n### [2026-10-0%d] orchestrator — gate retries\n- the gate retries the verify gate retries again\n", i)
	}
	writeFile(root+"/.agent-one/log.md", log.String())
	writeFile(root+"/.agent-one/AGENT-ONE.md", "# policy\n\n## The gate\n\nThe gate holds"+strings.Repeat(" landing review verdict witness", 60)+".\n")
	if _, err := w.Index(); err != nil {
		t.Fatal(err)
	}
	srcs := func(window bool) map[string]int {
		res, err := w.Recall("gate retries", RecallOpts{K: 5, NoCache: true, Window: window})
		if err != nil {
			t.Fatal(err)
		}
		n := map[string]int{}
		for _, r := range res.Results {
			n[r.Src]++
		}
		return n
	}
	if raw := srcs(false); raw[".agent-one/log.md"] != 5 {
		t.Fatalf("unshaped (the JS ranking): the chronicle takes every seat, got %v", raw)
	}
	if got := srcs(true); got[".agent-one/log.md"] != 2 || got[".agent-one/AGENT-ONE.md"] != 1 {
		t.Fatalf("shaped: two seats for the chronicle, the policy seated, got %v", got)
	}
}

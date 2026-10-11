package board

import (
	"encoding/json"
	"testing"
)

// TestTreeUpBreaksATwoNodeCycle: two members that each name the other as the one above them
// never reach the orchestrator — the loop is broken where it is found, the member hangs under the
// orchestrator by an implied reports edge, a note says so, and no parent chain loops.
func TestTreeUpBreaksATwoNodeCycle(t *testing.T) {
	c := &Team{Nodes: []Node{{ID: "domain-a", Kind: "member", Rank: "domain"}, {ID: "domain-b", Kind: "member", Rank: "domain"}},
		Edges: []Edge{{From: "domain-a", To: "domain-b", Bond: "above"}, {From: "domain-b", To: "domain-a", Bond: "above"}}}
	treeUp(c)
	parent := map[string]string{}
	for _, n := range c.Nodes {
		parent[n.ID] = n.Parent
	}
	for _, id := range []string{"domain-a", "domain-b"} {
		seen := map[string]bool{}
		for at := id; at != c.Orchestrator; at = parent[at] {
			if seen[at] || at == "" {
				t.Fatalf("%s never reaches the orchestrator: parents %v", id, parent)
			}
			seen[at] = true
		}
	}
	implied := 0
	for _, e := range c.Edges {
		if e.Implied && e.To == c.Orchestrator && e.Bond == "reports" {
			implied++
		}
	}
	if implied != 1 || len(c.Notes) != 1 {
		t.Errorf("one of the pair hangs under the orchestrator by one implied edge and one note: %d edges, notes %v, parents %v", implied, c.Notes, parent)
	}
}

// TestAPITeamOnAnEmptyWorkspace: /api/team's HTTP face on a workspace with no members is still one
// tree — the orchestrator alone, named, with empty (never null) node, edge and label lists.
func TestAPITeamOnAnEmptyWorkspace(t *testing.T) {
	b := newBoard(t, "empty", Options{})
	code, agent := get(t, b, "/api/team")
	if code != 200 {
		t.Fatalf("/api/team on the empty workspace: %d %s", code, agent)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(agent), &raw); err != nil {
		t.Fatalf("not json: %v\n%s", err, agent)
	}
	for _, k := range []string{"nodes", "edges", "labels"} {
		if v, ok := raw[k]; !ok || string(v) == "null" {
			t.Errorf("%q must be a list on an empty workspace, got %s", k, v)
		}
	}
	var c struct {
		Orchestrator string `json:"orchestrator"`
		Nodes        []Node `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(agent), &c); err != nil {
		t.Fatal(err)
	}
	members := 0
	for _, n := range c.Nodes {
		if n.Kind == "member" {
			members++
			if n.ID != c.Orchestrator || !n.Orchestrator {
				t.Errorf("the empty workspace holds only the orchestrator, found %+v", n)
			}
		}
	}
	if c.Orchestrator == "" || members != 1 {
		t.Errorf("the orchestrator must stand alone: orchestrator %q, %d members\n%s", c.Orchestrator, members, agent)
	}
}

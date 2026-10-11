package app

import (
	"testing"

	"github.com/Kaginari/agent-one/value"
	"github.com/Kaginari/agent-one/workspace"
)

// The nearest promotion: the agent with the fewest conditions left, then the shortest distance;
// one left reads as its reading, several as their count and the nearest.
func TestNearestPromotion(t *testing.T) {
	leaf := func(text string, got, want float64, met bool) value.Leaf {
		d := 0.0
		if !met {
			d = want - got
		}
		return value.Leaf{Text: text, Got: got, Want: want, Met: met, Distance: d}
	}
	agents := []workspace.AgentValue{
		{Agent: "domain-a", Promotion: &workspace.PromoteProgress{To: "principal-domain-owner", Leaves: []value.Leaf{leaf("reports >= 20", 12, 20, false), leaf("passRate >= 90%", 84.333, 90, false)}}},
		{Agent: "domain-engine", Promotion: &workspace.PromoteProgress{To: "principal-domain-owner", Leaves: []value.Leaf{leaf("reports >= 20", 17, 20, false), leaf("passRate >= 90%", 95, 90, true)}}},
		{Agent: "domain-done", Promotion: &workspace.PromoteProgress{To: "principal-domain-owner", Met: true}},
		{Agent: "zone-x"},
	}
	if got := nearestPromotion(agents); got != "domain-engine · reports 17/20 to promote" {
		t.Fatalf("got %q", got)
	}
	if got := nearestPromotion(agents[:1]); got != "domain-a · 2 to promote · passRate 84.3/90" {
		t.Fatalf("got %q", got)
	}
	if got := nearestPromotion(agents[2:]); got != "" {
		t.Fatalf("no agent on its way: %q", got)
	}
}

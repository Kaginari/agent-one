package app

import (
	"strings"
	"testing"
)

// The never-list restates the code: the ownership policy and the gate leave a zone worker its own nest
// in the workspace's scratch (workspace.ZoneOfPath, SoilScratch), so the zone worker's never-line names that
// carve-out, and the gate section says what the scratch is.
func TestPolicyNeverListNamesTheScratch(t *testing.T) {
	policy := PolicyText()
	i := strings.Index(policy, "- **A zone worker** never writes outside its zone")
	if i < 0 {
		t.Fatal("the zone worker's never-line is gone")
	}
	line := policy[i:]
	if j := strings.Index(line[2:], "\n- "); j >= 0 {
		line = line[:j+2]
	}
	if !strings.Contains(line, "`.agent-one/tmp/`") {
		t.Errorf("the zone worker's never-line contradicts the scratch carve-out: %q", line)
	}
	gate := policy[strings.Index(policy, "## The review gate"):]
	gate = gate[:strings.Index(gate, "\n## ")]
	if !strings.Contains(gate, "The workspace scratch (`.agent-one/tmp/`") {
		t.Error("the gate section does not say what the scratch is")
	}
}

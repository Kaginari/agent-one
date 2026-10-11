package app

import (
	"strings"
	"testing"
)

// The attribute audit: every unowned file names its strongest suggestion — dir-majority
// first, ancestor ownership second, journal-class census last — and reports, never assigns.
func TestAuditOwnership(t *testing.T) {
	owners := map[string]string{
		"tool/dispatch.go": "zone-tool",
		"tool/tool.go":     "zone-tool",
		"tool/bash.go":     "zone-tool",
		"app":              "domain-app", // ancestors resolve dir-style, as under() normalizes
	}
	owner := func(rel string) string { return owners[rel] }
	files := []string{
		"tool/dispatch.go", // owned
		"tool/newkit.go",   // same-dir majority → zone-tool
		"app/x.go",         // ancestor ownership "app/" → domain-app
		"HANDOFF.md",       // journal class → coord/agent-one
		"vendor/x.c",       // nothing → no-trace, human decides
	}
	fs := auditOwnership(files, owner)
	if len(fs) != 4 {
		t.Fatalf("4 unowned, got %d: %+v", len(fs), fs)
	}
	byPath := map[string]ownershipFinding{}
	for _, f := range fs {
		byPath[f.Path] = f
	}
	if f := byPath["tool/newkit.go"]; f.ZonedBy != "same-dir majority" || f.Suggest != "zone-tool" {
		t.Fatalf("dir-majority arm: %+v", f)
	}
	if f := byPath["app/x.go"]; f.ZonedBy != "ancestor ownership" || f.Suggest != "domain-app" {
		t.Fatalf("ancestor arm: %+v", f)
	}
	if f := byPath["HANDOFF.md"]; f.ZonedBy != "journal class" || f.Suggest != "coord/agent-one" || !strings.Contains(f.Suggestion, "HANDOFF.md") {
		t.Fatalf("journal-class arm: %+v", f)
	}
	if f := byPath["vendor/x.c"]; f.ZonedBy != "no trace" || f.Suggestion != "" {
		t.Fatalf("no-trace stays with the human: %+v", f)
	}
}

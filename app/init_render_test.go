package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderInitStyledText(t *testing.T) {
	var buf bytes.Buffer
	rep := initReport{
		dist:  "agent-one",
		root:  "/tmp/demo",
		level: "medium",
		made:  []string{".agent-one/AGENT-ONE.md"},
		medium: &mediumRep{
			members: []plan{
				{Rank: "coord", Short: "core", Parent: "orchestrator", Ownership: []string{"go.mod"}},
				{Rank: "domain", Short: "app", Parent: "coord-core", Ownership: []string{"app/"}},
				{Rank: "zone", Short: "app", Parent: "domain-app", Ownership: []string{"app/"}, Verify: []string{"go test ./app/..."}},
				{Rank: "zone", Short: "cli", Parent: "domain-app", Ownership: []string{"cli/"}},
				{Rank: "service", Short: "ci", Parent: "orchestrator", Ownership: []string{".github/"}},
			},
			wrote:  []string{".agent-one/coord/core/README.md"},
			kept:   1,
			coords: 1, domains: 1, zones: 2, service: 1,
			prefix: map[string]string{"coord": "coord-", "domain": "domain-", "zone": "zone-", "service": "service-"},
		},
	}
	renderInit(&buf, rep, defaultInitWords())
	out := buf.String()
	for _, want := range []string{"agent-one", "scaffold", "AGENT-ONE.md", "team", "1 written", "1 kept", "1 coordinator · 1 domain owner · 2 zone workers · 1 service owner",
		"    coord-core", "      domain-app", "        zone-app", "zone-cli", "cli/", "(no tests seen)", "    service-ci", "next", "run \"<ask>\""} {
		if !strings.Contains(out, want) {
			t.Errorf("styled init output missing %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "@S") || strings.Contains(out, "@F") {
		t.Errorf("styled output must not leak wire tags, got:\n%s", out)
	}
}

func TestStyledInitGate(t *testing.T) {
	var buf bytes.Buffer
	if styledInit(&buf, false) {
		t.Error("a buffer is not a terminal — styled gate must close")
	}
	if styledInit(nil, true) {
		t.Error("--plain closes the styled face")
	}
}

func TestPadTo(t *testing.T) {
	if got := padTo("zone", 8); got != "zone    " {
		t.Errorf("padTo: got %q", got)
	}
	if got := padTo("orchestrator", 4); got != "orchestrator" {
		t.Errorf("padTo must not truncate: got %q", got)
	}
}

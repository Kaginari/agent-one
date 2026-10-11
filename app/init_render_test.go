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
				{Rank: "zone", Short: "app", Parent: "domain-app", Ownership: []string{"app/"}},
			},
			wrote: []string{".agent-one/coords/core/README.md"},
			kept:  1,
		},
	}
	renderInit(&buf, rep, defaultInitWords())
	out := buf.String()
	for _, want := range []string{"agent-one", "scaffold", "AGENT-ONE.md", "team", "1 written", "1 kept", "zone", "next", "run \"<ask>\""} {
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

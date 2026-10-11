package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPulseTool(t *testing.T) {
	p := PulseTool()
	if p.Name != "pulse" || p.Class != Read {
		t.Fatalf("pulse is a read tool: %+v", p)
	}
	r := p.Run(context.Background(), Env{Self: func() string {
		return "steps 198 · turns 199 · wall 20m9s\nbenches: `bash go test` ×23 (19 red / 0 green / 4 muted)"
	}}, json.RawMessage(`{}`))
	if r.Err || !strings.Contains(r.Output, "19 red") {
		t.Fatalf("the digest must be served verbatim: %+v", r)
	}
	// a nil Self (no session acting — a dispatcher preview, a dry context) degrades to a
	// note, never an error: the tool may sit in registries built outside a run.
	r2 := p.Run(context.Background(), Env{}, json.RawMessage(`{}`))
	if r2.Err || !strings.Contains(r2.Output, "no session is acting") {
		t.Fatalf("a nil Self must degrade to a note: %+v", r2)
	}
}

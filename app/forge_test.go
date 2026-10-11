package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// /forge is the working notes: it lists what subagents carried home (@FORGE → the team ledger) and registers
// by the working notes's yes — a skill lands as a workspace note, a tool as a tools.custom patch that
// reloads live. Registered drafts mark themselves; nothing registers twice.
func TestForgeWorkingNotesListsAndRegisters(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json", text("nothing"))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	a := w.open()
	// the team ledger, as filed by record beats
	notes := `{"at":"2026-09-28T21:00:00Z","by":"zone-auth","workspace":"x","tag":"forge","kind":"team","text":"tool:ui-wrangler | rewrite bare /v1 strings | ./wrangle {{path}}"}
{"at":"2026-09-28T21:01:00Z","by":"zone-auth","workspace":"x","tag":"forge","kind":"team","text":"skill:ttl-craft | one hour is the shelf life"}
{"at":"2026-09-28T21:02:00Z","by":"zone-auth","workspace":"x","tag":"forge-registered","kind":"team","text":"skill:ttl-craft"}
`
	path := filepath.Join(w.root, ".agent-one", "memory", "shared")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "notes.jsonl"), []byte(notes), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errw bytes.Buffer
	a.forgeCmd(&out, &errw, "")
	if s := out.String(); !strings.Contains(s, "1. zone-auth  tool:ui-wrangler") || strings.Contains(s, "ttl-craft") {
		t.Fatalf("the working notes lists only the unminted:\n%s", s)
	}
	out.Reset()
	// a tool register patches tools.custom and marks the draft
	a.forgeCmd(&out, &errw, "register 1")
	if !strings.Contains(out.String(), "registered tool:ui-wrangler") {
		t.Fatalf("register: %s / err: %s", out.String(), errw.String())
	}
	y, err := os.ReadFile(filepath.Join(w.root, ".agent-one", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if c := string(y); !strings.Contains(c, "ui-wrangler") || !strings.Contains(c, "- ./wrangle") || !strings.Contains(c, `"{{path}}"`) || !strings.Contains(c, "path:") || !strings.Contains(c, "crafted from zone-auth's run") {
		t.Fatalf("the register's config patch:\n%s", c)
	}
	out.Reset()
	a.forgeCmd(&out, &errw, "")
	if s := out.String(); !strings.Contains(s, "the forge ledger is quiet") {
		t.Fatalf("registered things leave the list:\n%s", s)
	}
	// a skill draft registers as a note riding kind team
	out.Reset()
	w2 := newTestWorkspace(t, "agent-one", agentOneMembers())
	w2.script(".agent-one/tmp/s.json", text("nothing"))
	w2.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	a2 := w2.open()
	if err := os.MkdirAll(filepath.Join(w2.root, ".agent-one", "memory", "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w2.root, ".agent-one", "memory", "shared", "notes.jsonl"), []byte(`{"at":"2026-09-28T21:00:00Z","by":"zone-auth","workspace":"x","tag":"forge","kind":"team","text":"skill:ttl-craft | one hour is the shelf life"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a2.forgeCmd(&out, &errw, "register 1")
	if !strings.Contains(out.String(), "registered skill:ttl-craft") {
		t.Fatalf("skill register: %s / err: %s", out.String(), errw.String())
	}
	b, err2 := os.ReadFile(filepath.Join(w2.root, ".agent-one", "memory", "shared", "notes.jsonl"))
	if err2 != nil {
		t.Fatal(err2)
	}
	if got := string(b); !strings.Contains(got, `"tag":"skill"`) || !strings.Contains(got, `"tag":"forge-registered"`) {
		t.Fatalf("the skill lands as a note, marked registered:\n%s", got)
	}
}

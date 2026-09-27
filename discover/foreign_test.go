package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForeignGlobalsAreSkipped(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	write := func(p, s string) {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(s), 0o644)
	}
	foreign := "---\ndescription: a body of the ISEKAI convention\n---\nwork in .isekai/\n"
	write(filepath.Join(home, ".config/opencode/agents/throne.md"), foreign)
	write(filepath.Join(home, ".config/opencode/agents/helper.md"), "---\ndescription: a plain helper\n---\nhelp\n")
	write(filepath.Join(home, ".claude/commands/genesis.md"), foreign)
	write(filepath.Join(home, ".claude/skills/old/SKILL.md"), foreign)
	write(filepath.Join(home, ".claude/CLAUDE.md"), foreign)
	write(filepath.Join(root, ".claude/agents/local.md"), foreign) // the workspace's own: kept
	opt := Options{Root: root, Home: home, Skills: true, Commands: true, Agents: true}
	var agents []string
	for _, a := range Agents(opt) {
		agents = append(agents, a.Name)
	}
	if len(agents) != 2 || agents[0] != "helper" && agents[1] != "helper" {
		t.Fatalf("agents = %v, want helper and local only", agents)
	}
	for _, a := range agents {
		if a == "throne" {
			t.Fatal("a foreign machine-wide agent was listed")
		}
	}
	if c := Commands(opt); len(c) != 0 {
		t.Fatalf("a foreign machine-wide command was listed: %v", c)
	}
	if s := Skills(opt); len(s) != 0 {
		t.Fatalf("a foreign machine-wide skill was listed: %v", s)
	}
	for _, in := range Instructions(opt) {
		if in.Source == "claude-global" {
			t.Fatal("a foreign machine-wide instruction file was read")
		}
	}
}

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/tool"
)

// The app sweeps the fold shelf at start: a run's drawer untouched past tools.output.foldKeepDays goes.
func TestAppSweepsTheFoldShelf(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.write(".agent-one/config.yaml", "tools: {bash: {sandbox: none}, output: {foldKeepDays: 2}}\nui: {board: {autostart: false}}\n")
	old, fresh := tool.FoldRunDir(w.root, ".agent-one", "old-run"), tool.FoldRunDir(w.root, ".agent-one", "fresh-run")
	tool.FoldKeep(old, "old")
	tool.FoldKeep(fresh, "fresh")
	stale := time.Now().Add(-3 * 24 * time.Hour)
	os.Chtimes(old, stale, stale)
	a, err := New(Options{Dist: "agent-one", Root: w.root, Cwd: w.root, Home: w.home, Env: func(k string) string { return map[string]string{"HOME": w.home}[k] },
		In: strings.NewReader(""), Out: &strings.Builder{}, Err: &strings.Builder{}, Quiet: true, IsTTY: func() bool { return false }, NoBoard: true})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("a drawer past keepDays survived the start")
	}
	if _, err := os.Stat(filepath.Join(fresh, tool.FoldDigest("fresh")+".txt")); err != nil {
		t.Fatal("a fresh drawer was swept")
	}
}

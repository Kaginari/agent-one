package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Kaginari/agent-one/tui"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// TestReadmeShots writes the README's terminal screens as ANSI frames when README_SHOTS names a
// directory: the real host on a scripted provider, each frame the renderer's own (truecolor, the
// default palette). A human turns them into pictures; the test only proves each scene is reached.
func TestReadmeShots(t *testing.T) {
	dir := os.Getenv("README_SHOTS")
	if dir == "" {
		t.Skip("README_SHOTS unset")
	}
	// a project's own path in the pictures: the workspace is ~/acme-shop under a temp HOME
	home := t.TempDir()
	t.Setenv("HOME", home)
	w := &testWorkspace{t: t, root: filepath.Join(home, "acme-shop"), home: home, dist: "agent-one"}
	for p, s := range agentOneMembers() {
		w.write(p, s)
	}
	w.script(".agent-one/tmp/s.json",
		when("tidy", map[string]interface{}{"text": "I'll look at the README first, then tidy its intro.\n\n", "calls": []interface{}{map[string]interface{}{"name": "bash", "input": map[string]string{"command": "sleep 2; wc -l README.md"}}}}),
		when("README.md", call("edit", map[string]string{"path": "README.md", "old": "hello", "new": "hello\n\nA workspace that remembers in documents."})),
		when("edited", text("Done — the intro now says what the workspace is for.\n\n- `README.md`: one line added under the title\n- the gate passed: right author, invariants, duties, the doc\n")),
		when("push", call("bash", map[string]string{"command": "git push origin main"})),
		when("denied", text("Understood — nothing was pushed.")),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", "policy: {ownership: {orchestratorNeverAuthors: false}}\n"))
	if b, err := os.ReadFile("../.agent-one/ontology/schema.ttl"); err == nil {
		w.write(".agent-one/ontology/schema.ttl", string(b)) // a founded workspace has its schema
	}

	a := w.open()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h, err := a.tuiHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	th := tui.DetectThemeFor(func(string) string { return "" }, "AGENT_ONE_", "cyberpunk", "dark")
	th.Color, th.TrueColor = true, true
	m := tui.New(h, th, h.words)
	tm := teatest.NewTestModel(t, shotModel{framed{m}}, teatest.WithInitialTermSize(132, 38))
	m.Attach(tm.Send)
	h.attach(tmSender{tm})
	sc := &screen{tm: tm}
	sc.follow(tm.Output())
	t.Cleanup(func() {
		tm.Send(tui.EvQuit{})
		tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
	})
	shot := func(name string) {
		req := make(rawReq, 1)
		go tm.Send(req)
		select {
		case f := <-req:
			if err := os.WriteFile(filepath.Join(dir, name+".ans"), []byte(f), 0o644); err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("no frame for %s", name)
		}
	}
	key := func(k tea.KeyPressMsg) { tm.Send(k) }

	sc.wait(t, "⏎ ask")
	time.Sleep(400 * time.Millisecond) // the welcome's zone worker settles; a picture, not a sync
	shot("welcome")

	tm.Type("tidy the README intro")
	enter(tm)
	sc.wait(t, "● bash  sleep 2; wc -l README.md")
	time.Sleep(600 * time.Millisecond)
	shot("thinking")
	sc.wait(t, "● edit  README.md  write")
	sc.wait(t, "the intro now says")
	key(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	sc.wait(t, "+ A workspace that remembers in documents.")
	sc.wait(t, "✓ gate · ")
	time.Sleep(300 * time.Millisecond)
	shot("tools")

	tm.Type("push it")
	enter(tm)
	sc.wait(t, "1 · Yes")
	time.Sleep(300 * time.Millisecond)
	shot("gating")
	tm.Type("3")
	sc.wait(t, "why not?")
	tm.Type("not yet")
	enter(tm)
	sc.wait(t, "nothing was pushed")

	key(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	time.Sleep(700 * time.Millisecond)
	shot("agents")
	key(tea.KeyPressMsg{Code: tea.KeyEscape})
	time.Sleep(300 * time.Millisecond)

	key(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	tm.Type("the")
	time.Sleep(500 * time.Millisecond)
	shot("overlay")
	key(tea.KeyPressMsg{Code: tea.KeyEscape})
	time.Sleep(300 * time.Millisecond)

	tm.Type("/theme")
	enter(tm)
	sc.wait(t, "Corporate Rebellion")
	key(tea.KeyPressMsg{Code: tea.KeyDown})
	key(tea.KeyPressMsg{Code: tea.KeyDown})
	time.Sleep(600 * time.Millisecond)
	shot("theme-picker")
	key(tea.KeyPressMsg{Code: tea.KeyEscape})
	time.Sleep(400 * time.Millisecond)

	tm.Type("/board")
	enter(tm)
	time.Sleep(900 * time.Millisecond)
	tm.Type("2")
	time.Sleep(900 * time.Millisecond)
	shot("board-graph")
}

// shotModel answers a raw frame request (the renderer's own ANSI) beside framed's.
type shotModel struct{ framed }

type rawReq chan string

func (s shotModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if req, ok := msg.(rawReq); ok {
		req <- s.m.Render()
		return s, nil
	}
	_, cmd := s.framed.Update(msg)
	return s, cmd
}

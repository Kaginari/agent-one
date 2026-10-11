package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/config"
)

// A look switched anywhere is kept in the workspace's config.local.yaml, the config reloaded, the
// dashboard's stream told; an unknown palette is refused with the list and writes nothing; the
// dashboard's ask can switch it with /theme too.
func TestThemeKeptAndAnnounced(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.write(".agent-one/config.yaml", "models: {default: mock/m}\nproviders: {mock: {enabled: true}}\nui: {board: {autostart: false}}\n")
	a := w.open()
	if n, m := a.look(); n != "cyberpunk" || m != "dark" {
		t.Fatalf("the default look: %s %s", n, m)
	}
	bus := a.webBus()
	replay, ch, cancel := bus.Subscribe()
	defer cancel()
	_ = replay
	if err := a.setTheme("synthwave", "light"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(w.root, ".agent-one", "config.local.yaml"))
	if err != nil || !strings.Contains(string(b), "theme: synthwave") || !strings.Contains(string(b), "mode: light") {
		t.Fatalf("config.local.yaml: %v\n%s", err, b)
	}
	if n, m := a.look(); n != "synthwave" || m != "light" || !strings.Contains(a.liveConfig().Where("ui.theme"), "config.local.yaml") {
		t.Fatalf("reloaded: %s %s", n, m)
	}
	if ev := string(<-ch); !strings.Contains(ev, `"type":"theme"`) || !strings.Contains(ev, `"name":"synthwave"`) || !strings.Contains(ev, `"mode":"light"`) {
		t.Fatalf("the stream: %s", ev)
	}
	if err := a.setTheme("vaporwave", ""); err == nil || !strings.Contains(err.Error(), "chromecore") {
		t.Fatalf("an unknown palette names the list: %v", err)
	}
	var out, errw strings.Builder
	a.themeSlash("glitch", &out, &errw)
	if n, m := a.look(); n != "glitch" || m != "light" || !strings.Contains(out.String(), "theme glitch · light") {
		t.Fatalf("/theme glitch: %s %s %q %q", n, m, out.String(), errw.String())
	}
	out.Reset()
	a.themeSlash("", &out, &errw)
	if !strings.Contains(out.String(), "▸ glitch") {
		t.Fatalf("/theme alone lists: %q", out.String())
	}
}

// A --set flag pins ui.theme above the local file: a switch is still held for the session (and
// kept in config.local.yaml for a start without the flag), and a mode alone keeps the switched
// palette.
func TestThemeSwitchOverAPinnedFlag(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.write(".agent-one/config.yaml", "models: {default: mock/m}\nproviders: {mock: {enabled: true}}\nui: {board: {autostart: false}}\n")
	a, err := New(Options{Dist: w.dist, Root: w.root, Cwd: w.root, Home: w.home, Env: func(string) string { return "" },
		In: strings.NewReader(""), Out: &strings.Builder{}, Err: &strings.Builder{}, Quiet: true, IsTTY: func() bool { return false }, NoBoard: true,
		Flags: []config.Override{{Path: "ui.theme", Value: "cyberpunk", Flag: "--set"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	if err := a.setTheme("rain-city", ""); err != nil {
		t.Fatal(err)
	}
	if err := a.setTheme("", "light"); err != nil {
		t.Fatal(err)
	}
	if n, m := a.look(); n != "rain-city" || m != "light" {
		t.Fatalf("the session holds %s %s", n, m)
	}
	if b := w.read(".agent-one/config.local.yaml"); !strings.Contains(b, "theme: rain-city") || !strings.Contains(b, "mode: light") {
		t.Fatalf("config.local.yaml:\n%s", b)
	}
}

package app

import (
	"encoding/json"
	"strings"
	"testing"
)

// Every tool an agent is offered declares a schema that is JSON: one that is not breaks the request
// encoding of every model call that carries the shelf (a mock never encodes, so it passes unseen).
func TestEveryShelfSchemaIsJSON(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.write(".agent-one/config.yaml", "tools: {bash: {sandbox: none}}\nui: {board: {autostart: false}}\n")
	a, err := New(Options{Dist: "agent-one", Root: w.root, Cwd: w.root, Home: w.home, Env: func(k string) string { return map[string]string{"HOME": w.home}[k] },
		In: strings.NewReader(""), Out: &strings.Builder{}, Err: &strings.Builder{}, Quiet: true, IsTTY: func() bool { return false }, NoBoard: true})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for _, agent := range []string{"orchestrator", "zone-auth"} {
		r, done := a.Shelf.Build(agent)
		for _, d := range r.Defs() {
			if len(d.Schema) > 0 && !json.Valid(d.Schema) {
				t.Errorf("%s's %s schema is not JSON: %s", agent, d.Name, d.Schema)
			}
		}
		if done != nil {
			done()
		}
	}
}

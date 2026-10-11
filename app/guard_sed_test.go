package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSedGuardHook(t *testing.T) {
	a := testTmpApp(t, "/Users/x/ws/agent-one")
	a.Cfg.Tools.TmpRedirect.Enabled = true
	g := a.sedGuardHook()
	if g == nil {
		t.Fatal("the sed guard must ride when the workspace's write discipline is on")
	}
	refuse := []string{
		`sed -i 's/old/new/' onto/workspace.go`,
		`sed -i.bak -e 's/old/new/' onto/workspace.go`,
		`sed --in-place 's/old/new/' onto/workspace.go`,
		`gofmt -w x.go && sed -i 's/a/b/' y.go`,
	}
	for _, cmd := range refuse {
		in, _ := json.Marshal(map[string]string{"command": cmd})
		msg := g(nil, "bash", in)
		if msg == "" {
			t.Errorf("in-place sed must be refused: %q", cmd)
			continue
		}
		if !strings.Contains(msg, "edit") || !strings.Contains(msg, "refused") {
			t.Errorf("refusal must name the refused act and the edit tool: %q", msg)
		}
	}
	allow := []string{
		`sed -n '100,260p' onto/workspace.go`,
		`sed 's/a/b/' < in.txt > out.txt`,
		`grep -r sed memory/ | sed 's/^/  /'`,
		`ls && go test ./app`,
	}
	for _, cmd := range allow {
		in, _ := json.Marshal(map[string]string{"command": cmd})
		if msg := g(nil, "bash", in); msg != "" {
			t.Errorf("streams and reads stay free; wrongly refused %q: %q", cmd, msg)
		}
	}
	// non-bash tools stay undisturbed
	in, _ := json.Marshal(map[string]string{"path": "x.go", "old": "a", "new": "b"})
	if msg := g(nil, "edit", in); msg != "" {
		t.Errorf("the sed guard only watches bash; wrongly refused edit: %q", msg)
	}
}

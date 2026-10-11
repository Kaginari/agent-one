package board

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/onto"
)

func newBoard(t *testing.T, workspace string, o Options) *Board {
	t.Helper()
	o.WorkspaceRoot = fixtureRoot(t, workspace)
	if o.Now == nil {
		o.Now = func() time.Time { return fixedNow }
	}
	b := New(o)
	t.Cleanup(b.Close)
	return b
}

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", path, nil)
	req.Host = "127.0.0.1:7411" // as the browser addresses the board; any other Host is refused
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSEEmitsOnUsageAppend(t *testing.T) {
	root := t.TempDir()
	copyTree(t, fixtureRoot(t, "workspace"), root)
	b := New(Options{WorkspaceRoot: root, Poll: 15 * time.Millisecond})
	t.Cleanup(b.Close)
	srv := httptest.NewServer(b)
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}
	rd := bufio.NewReader(resp.Body)
	line, _ := rd.ReadString('\n')
	if !strings.HasPrefix(line, "retry:") {
		t.Fatalf("first line %q", line)
	}
	time.Sleep(60 * time.Millisecond) // two polls: the watcher has a baseline
	f, err := os.OpenFile(filepath.Join(root, ".agent-one", "instruments", "usage", "s-2026-09-26.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"ts":"2026-09-26T09:00:00Z","session":"s-2026-09-26","agent":"orchestrator","rank":"orchestrator","model":"m","provider":"p","input":1,"output":1,"cacheRead":0,"cacheWrite":0,"usd":null}` + "\n")
	f.Close()
	got := make(chan string, 1)
	go func() {
		for {
			l, err := rd.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(l, "event: usage") {
				got <- l
				return
			}
		}
	}()
	select {
	case <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("no usage event within 3s of the append")
	}
}

func TestReadmeDocumentsTheUsageSchema(t *testing.T) {
	b, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"`ts`", "`session`", "`agent`", "`rank`", "`role`", "`model`", "`provider`", "`input`", "`cacheRead`", "`cacheWrite`", "`usd`"} {
		if !strings.Contains(string(b), f) {
			t.Errorf("README lacks usage field %s", f)
		}
	}
	_ = io.EOF
}

// A silent instrument on an agent-one workspace names that workspace's dir, never the other
// distribution's: the memory and toolbox feeds open the workspace dir the board was given.
func TestSilentReadingNamesTheWorkspaceDir(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".agent-one"), 0o755)
	f := FileSources(root, ".agent-one", onto.Layout{}, func() time.Time { return fixedNow })
	for name, why := range map[string]string{"memory": f.Memory().Reading.Why, "toolbox": f.Toolbox().Reading.Why} {
		if strings.Contains(why, ".agent-one") {
			t.Errorf("%s reading names the wrong workspace dir: %q", name, why)
		}
	}
}

func TestAPITeamCarriesTheNamedOrchestratorAndActivity(t *testing.T) {
	b := newBoard(t, "workspace", Options{Names: Names{"rank.orchestrator": "Sovereign"}})
	_, agent := get(t, b, "/api/team")
	var c struct {
		Orchestrator string `json:"orchestrator"`
		Nodes        []Node `json:"nodes"`
		Edges        []Edge `json:"edges"`
	}
	if err := json.Unmarshal([]byte(agent), &c); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range c.Nodes {
		if n.ID == c.Orchestrator {
			found = n.Orchestrator && n.Label == "Sovereign"
		}
	}
	if !found {
		t.Errorf("the orchestrator must ride the payload under the lexicon's word: %s", agent)
	}
	code, agent := get(t, b, "/api/activity")
	var a Activity
	if code != 200 || json.Unmarshal([]byte(agent), &a) != nil || len(a.Days) != ActivityDays || a.Agents["zone-auth"] == nil || !a.Lit {
		t.Errorf("activity: %d %s", code, agent)
	}
}

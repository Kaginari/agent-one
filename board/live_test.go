package board

import (
	"bufio"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/ui"
)

type fakeLive struct {
	asked  []string
	events chan []byte
}

func (f *fakeLive) Subscribe() ([][]byte, <-chan []byte, func()) {
	return [][]byte{[]byte(`{"type":"ask","text":"before"}`)}, f.events, func() {}
}
func (f *fakeLive) Ask(t string) error {
	if t == "boom" {
		return errors.New("refused by a hook")
	}
	f.asked = append(f.asked, t)
	return nil
}
func (f *fakeLive) Answer(string, int, string) error { return nil }
func (f *fakeLive) Interrupt()                       {}
func (f *fakeLive) Status() map[string]any           { return map[string]any{"model": "m"} }

func liveBoard(t *testing.T, live Live) *Board {
	t.Helper()
	b := New(Options{WorkspaceRoot: "testdata/workspace", Live: live})
	t.Cleanup(b.Close)
	return b
}

func ask(b *Board, host, origin, token, agent string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "http://"+host+"/api/ask", strings.NewReader(agent))
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if token != "" {
		r.Header.Set("X-Board-Token", token)
	}
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	return w
}

func TestDashActsNeedTokenLoopbackAndOrigin(t *testing.T) {
	f := &fakeLive{events: make(chan []byte)}
	b := liveBoard(t, f)
	ok := `{"text":"hello"}`
	cases := []struct {
		name, host, origin, token string
		code                      int
	}{
		{"no token", "127.0.0.1:7411", "", "", 403},
		{"wrong token", "127.0.0.1:7411", "", "nope", 403},
		{"another site", "127.0.0.1:7411", "http://evil.example", b.token, 403},
		{"rebound dns name", "evil.example:7411", "", b.token, 403},
		{"own page", "127.0.0.1:7411", "http://127.0.0.1:7411", b.token, 200},
		{"localhost", "localhost:7411", "", b.token, 200},
	}
	for _, c := range cases {
		if w := ask(b, c.host, c.origin, c.token, ok); w.Code != c.code {
			t.Errorf("%s: %d %s, want %d", c.name, w.Code, w.Body, c.code)
		}
	}
	if len(f.asked) != 2 {
		t.Fatalf("asks that landed: %v", f.asked)
	}
	if w := ask(b, "127.0.0.1:1", "", b.token, `{"text":"boom"}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "hook") {
		t.Fatalf("a refused ask: %d %s", w.Code, w.Body)
	}
	if w := ask(b, "127.0.0.1:1", "", b.token, `{"text":"  "}`); w.Code != http.StatusBadRequest {
		t.Fatalf("an empty ask: %d", w.Code)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/ask", nil)
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/ask: %d", w.Code)
	}
}

func TestDashWithoutSessionIsReadOnly(t *testing.T) {
	b := liveBoard(t, nil)
	if w := ask(b, "127.0.0.1:7411", "", b.token, `{"text":"hi"}`); w.Code != http.StatusConflict {
		t.Fatalf("an ask without a session: %d", w.Code)
	}
}

func TestDashPageCarriesTokenAndCSP(t *testing.T) {
	b := liveBoard(t, &fakeLive{events: make(chan []byte)})
	w := httptest.NewRecorder()
	b.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dash", nil))
	agent := w.Body.String()
	if w.Code != 200 || !strings.Contains(agent, `content="`+b.token+`"`) || strings.Contains(agent, "{{") {
		t.Fatalf("dash page: %d, token or placeholders wrong", w.Code)
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("CSP: %q", csp)
	}
	for _, asset := range []string{"/web/dash.js", "/web/tokens.css", "/web/components/net/net.css"} {
		w := httptest.NewRecorder()
		b.ServeHTTP(w, httptest.NewRequest(http.MethodGet, asset, nil))
		if w.Code != 200 {
			t.Errorf("%s: %d", asset, w.Code)
		}
	}
}

func TestDashStreamReplaysThenStreams(t *testing.T) {
	f := &fakeLive{events: make(chan []byte, 1)}
	b := liveBoard(t, f)
	srv := httptest.NewServer(b)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/dash/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	f.events <- []byte(`{"type":"delta","text":"after"}`)
	sc := bufio.NewScanner(resp.Body)
	var got []string
	deadline := time.After(3 * time.Second)
	for len(got) < 2 {
		line := make(chan string, 1)
		go func() {
			if sc.Scan() {
				line <- sc.Text()
			} else {
				close(line)
			}
		}()
		select {
		case l, ok := <-line:
			if !ok {
				t.Fatalf("stream ended: %v", got)
			}
			if strings.HasPrefix(l, "data: {\"type") {
				got = append(got, l)
			}
		case <-deadline:
			t.Fatalf("stream stalled: %v", got)
		}
	}
	if !strings.Contains(got[0], "before") || !strings.Contains(got[1], "after") {
		t.Fatalf("replay then live, got %v", got)
	}
}

// The dashboard is held to the ui system it teaches: tokens only, every class declared, every
// header true.
func TestDashboardHoldsTheUISystem(t *testing.T) {
	m, err := ui.Scan(".", "web")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ui.Lint(".", m) {
		t.Errorf("%s", f)
	}
	for _, name := range []string{"tokens.css", "palettes.css", "layout.css"} {
		have, _ := content.ReadFile("web/" + name)
		want, err := ui.FoundationFile(name)
		if err != nil || string(have) != string(want) {
			t.Errorf("web/%s drifted from the ui foundation's copy — copy it again", name)
		}
	}
}

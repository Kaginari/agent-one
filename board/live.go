package board

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/memory"
)

// Live is the running session as the dashboard drives it: its event stream, and the few acts a
// browser may take — ask, answer a choice, interrupt. The integrator (the app) implements it;
// nil means the board runs without a session and the dashboard is read-only.
type Live interface {
	// Subscribe replays the session so far and streams what follows; cancel ends it.
	Subscribe() (replay [][]byte, events <-chan []byte, cancel func())
	Ask(text string) error
	Answer(id string, index int, text string) error
	Interrupt()
	// Status is the header's reading: model, context, cost, state, whether choices are answered here.
	Status() map[string]any
}

// token guards every act: a page served by this board carries it; a page on another origin cannot
// read it, so it cannot make the session act (cross-site requests) — and the Host check below
// stops a rebound DNS name from reaching the board as if it were local.
func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// local says the request is addressed to this machine by a loopback name: a browser that
// resolved some other name to 127.0.0.1 (DNS rebinding) sends that name in Host.
func local(r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return false
	}
	if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host && o != "https://"+r.Host {
		return false
	}
	return true
}

func (b *Board) act(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return false
	}
	if !local(r) || r.Header.Get("X-Board-Token") != b.token {
		http.Error(w, "refused: acts come from this board's own page", http.StatusForbidden)
		return false
	}
	if b.opt.Live == nil {
		http.Error(w, "no session: start one with `"+b.names.get("bin")+"` or `"+b.names.get("bin")+" dash`", http.StatusConflict)
		return false
	}
	return true
}

func (b *Board) liveRoutes() {
	web, _ := fs.Sub(content, "web")
	files := http.StripPrefix("/web/", http.FileServer(http.FS(web)))
	b.mux.Handle("/web/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}))
	b.mux.HandleFunc("/dash", b.dash)
	b.mux.HandleFunc("/dash/stream", b.dashStream)
	// the workspace's two planes beside the ontology: the minted agents, every wearable skill, and
	// the desks (thoughts live in the worn skill)
	b.mux.HandleFunc("/api/workspace", func(w http.ResponseWriter, r *http.Request) {
		agents, skills := b.src.Agents(), b.src.Skills()
		if agents == nil {
			agents = []AgentInfo{}
		}
		if skills == nil {
			skills = []SkillInfo{}
		}
		notes := []memory.WorkingNotes{}
		if m := b.src.Memory(); m != nil && m.WorkingNotes != nil {
			notes = m.WorkingNotes
		}
		writeJSON(w, map[string]any{"agents": agents, "skills": skills, "workingNotes": notes})
	})
	b.mux.HandleFunc("/api/live", func(w http.ResponseWriter, r *http.Request) {
		st := map[string]any{"session": false}
		if b.opt.Live != nil {
			st = b.opt.Live.Status()
			st["session"] = true
		}
		st["workspace"] = b.workspaceName()
		writeJSON(w, st)
	})
	b.mux.HandleFunc("/api/ask", func(w http.ResponseWriter, r *http.Request) {
		if !b.act(w, r) {
			return
		}
		var in struct{ Text string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil || strings.TrimSpace(in.Text) == "" {
			http.Error(w, "an ask needs text", http.StatusBadRequest)
			return
		}
		if err := b.opt.Live.Ask(in.Text); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	b.mux.HandleFunc("/api/answer", func(w http.ResponseWriter, r *http.Request) {
		if !b.act(w, r) {
			return
		}
		var in struct {
			ID    string
			Index int
			Text  string
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			http.Error(w, "bad answer", http.StatusBadRequest)
			return
		}
		if err := b.opt.Live.Answer(in.ID, in.Index, in.Text); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	b.mux.HandleFunc("/api/interrupt", func(w http.ResponseWriter, r *http.Request) {
		if !b.act(w, r) {
			return
		}
		b.opt.Live.Interrupt()
		writeJSON(w, map[string]any{"ok": true})
	})
}

// dash is the dashboard page: the static shell with this board's token and words.
func (b *Board) dash(w http.ResponseWriter, r *http.Request) {
	page, err := content.ReadFile("web/dash.html")
	if err != nil {
		http.Error(w, "dashboard missing from the build", http.StatusInternalServerError)
		return
	}
	words, _ := json.Marshal(b.names)
	out := strings.NewReplacer("{{TOKEN}}", b.token, "{{WORDS}}", htmlAttr(string(words)), "{{DIST}}", htmlAttr(b.names.get("bin"))).Replace(string(page))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'")
	_, _ = w.Write([]byte(out))
}

func htmlAttr(s string) string {
	return strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// dashStream is the session's event stream: the replay, then every event as it happens.
func (b *Board) dashStream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	if !local(r) {
		http.Error(w, "refused", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = fmt.Fprint(w, "retry: 2000\n\n")
	if b.opt.Live == nil {
		_, _ = fmt.Fprint(w, "event: nosession\ndata: {}\n\n")
		fl.Flush()
		<-r.Context().Done()
		return
	}
	replay, ch, cancel := b.opt.Live.Subscribe()
	defer cancel()
	_, _ = fmt.Fprint(w, "event: reset\ndata: {}\n\n")
	for _, e := range replay {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", e)
	}
	fl.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-b.stop:
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", e)
			fl.Flush()
		case <-ping.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

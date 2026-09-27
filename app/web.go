package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Kaginari/agent-one/tui"
)

// webBus is the session seen from the browser: every event the host sends the terminal UI is
// also turned into JSON here, kept in a bounded replay for a page that joins late, and streamed
// to every open dashboard. It implements board.Live; the host attaches when the session starts.
type webBus struct {
	mu         sync.Mutex
	host       *tuiHost
	seq        int
	ring       [][]byte
	subs       map[chan []byte]struct{}
	choices    map[string]chan<- tui.ChoiceAnswer
	answerable bool // the browser answers choices (the dashboard is the only ui)
}

const webReplay = 4000

func newWebBus() *webBus {
	return &webBus{subs: map[chan []byte]struct{}{}, choices: map[string]chan<- tui.ChoiceAnswer{}}
}

func (w *webBus) attach(h *tuiHost) {
	w.mu.Lock()
	w.host = h
	w.mu.Unlock()
}

// Send takes one host event (tea.Msg) and publishes its JSON form.
func (w *webBus) Send(msg tea.Msg) {
	ev := webEvent(msg)
	if ev == nil {
		return
	}
	w.mu.Lock()
	if c, ok := msg.(tui.EvChoice); ok {
		if w.answerable {
			w.seq++
			id := "c" + strconv.Itoa(w.seq)
			w.choices[id] = c.Reply
			ev["id"] = id
		} else {
			ev["elsewhere"] = true // answered in the terminal
		}
	}
	w.publishLocked(ev)
	w.mu.Unlock()
}

// note publishes a bus-made event (an ask, an answer) the host never sends the terminal.
func (w *webBus) note(ev map[string]any) {
	w.mu.Lock()
	w.publishLocked(ev)
	w.mu.Unlock()
}

func (w *webBus) publishLocked(ev map[string]any) {
	w.seq++
	ev["seq"] = w.seq
	ev["at"] = time.Now().UnixMilli()
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	w.ring = append(w.ring, b)
	if len(w.ring) > webReplay {
		w.ring = append([][]byte(nil), w.ring[len(w.ring)-webReplay:]...)
	}
	for ch := range w.subs {
		select {
		case ch <- b:
		default: // a stalled page misses an event; a reload replays the session
		}
	}
}

// --- board.Live ---

func (w *webBus) Subscribe() ([][]byte, <-chan []byte, func()) {
	ch := make(chan []byte, 512)
	w.mu.Lock()
	replay := append([][]byte(nil), w.ring...)
	w.subs[ch] = struct{}{}
	w.mu.Unlock()
	return replay, ch, func() {
		w.mu.Lock()
		if _, ok := w.subs[ch]; ok {
			delete(w.subs, ch)
			close(ch)
		}
		w.mu.Unlock()
	}
}

func (w *webBus) Ask(text string) error {
	w.mu.Lock()
	h := w.host
	w.mu.Unlock()
	if h == nil {
		return errors.New("the session is not ready yet")
	}
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "/") {
		w.note(map[string]any{"type": "ask", "text": text, "from": "dashboard", "slash": true})
		lines, quit := h.Slash(text)
		if len(lines) > 0 {
			w.Send(tui.EvLines{Lines: lines})
		}
		if quit {
			w.Send(tui.EvNotice{Text: "the session ends from the terminal (/quit there)"})
		}
		return nil
	}
	h.mu.Lock()
	running := h.running
	h.mu.Unlock()
	if running {
		h.Queue(text)
		return nil
	}
	if why := h.Submit(text); why != "" {
		return errors.New(why)
	}
	return nil
}

func (w *webBus) Answer(id string, index int, text string) error {
	w.mu.Lock()
	reply, ok := w.choices[id]
	delete(w.choices, id)
	w.mu.Unlock()
	if !ok {
		return fmt.Errorf("choice %s is no longer open", id)
	}
	reply <- tui.ChoiceAnswer{Index: index, Text: text}
	w.note(map[string]any{"type": "answered", "id": id, "index": index, "text": text})
	return nil
}

func (w *webBus) Interrupt() {
	w.mu.Lock()
	h := w.host
	w.mu.Unlock()
	if h != nil {
		h.Interrupt()
	}
}

func (w *webBus) Status() map[string]any {
	w.mu.Lock()
	h, answerable := w.host, w.answerable
	w.mu.Unlock()
	st := map[string]any{"ready": h != nil, "answerable": answerable}
	if h == nil {
		return st
	}
	f := h.Footer()
	h.mu.Lock()
	running := h.running
	h.mu.Unlock()
	st["model"], st["ctx"], st["ctxPct"], st["ctxKnown"], st["cost"], st["tokens"], st["live"], st["running"] =
		f.Model, f.Ctx, f.CtxPct, f.CtxKnown, f.Cost, f.Tokens, f.Live, running
	st["session"], st["dist"] = h.a.SessionID, h.a.Cfg.Dist.Name
	return st
}

// close aborts every open choice and ends every stream (the session is over).
func (w *webBus) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, reply := range w.choices {
		select {
		case reply <- tui.ChoiceAnswer{Aborted: true}:
		default:
		}
		delete(w.choices, id)
	}
	for ch := range w.subs {
		delete(w.subs, ch)
		close(ch)
	}
}

// webEvent is a host event as the dashboard reads it; nil for the terminal's own events.
func webEvent(msg tea.Msg) map[string]any {
	switch e := msg.(type) {
	case tui.EvDelta:
		return map[string]any{"type": "delta", "text": e.Text}
	case tui.EvStream:
		return map[string]any{"type": "stream", "agent": e.Agent, "kind": e.Kind, "text": e.Text}
	case tui.EvToolStart:
		t := toolJSON(e.Tool)
		t["status"], t["why"] = "running", "" // a step opens pending; its outcome comes with the end
		return map[string]any{"type": "tool", "phase": "start", "tool": t}
	case tui.EvToolEnd:
		return map[string]any{"type": "tool", "phase": "end", "tool": toolJSON(e.Tool)}
	case tui.EvBodyStep:
		return map[string]any{"type": "bodystep", "agent": e.Agent, "end": e.End, "tool": toolJSON(e.Tool)}
	case tui.EvSubagent:
		c := e.Subagent
		return map[string]any{"type": "subagent", "name": c.Name, "rank": c.Rank, "role": c.Role, "model": c.Model, "state": c.State,
			"elapsedMs": c.Elapsed.Milliseconds(), "ask": c.Ask, "report": c.Report, "failed": c.Failed, "word": c.Word}
	case tui.EvState:
		return map[string]any{"type": "state", "agent": e.Agent, "state": e.State, "tool": e.Tool}
	case tui.EvTurnStart:
		return map[string]any{"type": "turnstart", "text": e.Text, "auto": e.Auto}
	case tui.EvTurnDone:
		return map[string]any{"type": "turndone", "status": e.Status, "text": e.Text, "streamed": e.Streamed, "holes": e.Holes, "verdict": e.Verdict,
			"log": e.LogRel, "interrupted": e.Interrupted, "hint": e.Hint}
	case tui.EvNotice:
		return map[string]any{"type": "notice", "text": e.Text}
	case tui.EvError:
		return map[string]any{"type": "error", "text": e.Text}
	case tui.EvLines:
		return map[string]any{"type": "lines", "lines": e.Lines}
	case tui.EvChoice:
		v := e.View
		return map[string]any{"type": "choice", "title": v.Title, "class": v.Class, "tool": v.Tool, "summary": v.Summary, "why": v.Why, "agent": v.Agent,
			"options": v.Options, "notes": v.Notes, "reasons": v.Reasons, "free": v.Free, "prompt": v.Prompt}
	}
	return nil
}

func toolJSON(t tui.ToolView) map[string]any {
	m := map[string]any{"id": t.ID, "name": t.Name, "summary": t.Summary, "class": t.Class, "status": t.Status, "ms": t.Ms, "why": t.Why, "wrote": t.Wrote, "agent": t.Agent}
	if out := t.Output; out != "" {
		if len(out) > 16<<10 {
			out = out[:16<<10] + "\n… (cut at 16 KiB)"
		}
		m["output"] = out
	}
	if d := t.Diff; d != nil {
		lines := make([]map[string]any, 0, min(len(d.Lines), 400))
		for i, l := range d.Lines {
			if i == 400 {
				break
			}
			lines = append(lines, map[string]any{"k": string(l.Kind), "o": l.Old, "n": l.New, "t": l.Text})
		}
		m["diff"] = map[string]any{"add": d.Add, "del": d.Del, "path": d.Path, "truncated": d.Truncated || len(d.Lines) > 400, "lines": lines}
	}
	return m
}

// fanout sends every event to each of its senders: the terminal and the dashboard.
type fanout []sender

func (f fanout) Send(msg tea.Msg) {
	for _, s := range f {
		if s != nil {
			s.Send(msg)
		}
	}
}

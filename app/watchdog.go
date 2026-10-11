package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Kaginari/agent-one/gate"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/tui"
)

// The heartbeat watchdog (freeze policy, 2026-10-07): a workspace silent while a gate/ask card is
// open is a wedge by definition — every channel the human could steer by drains through the
// one goroutine parked in that card. The watchdog runs once per turn (dies with it), reads
// the pulse stamped by every send, and moves in two layers:
//
// notice layer — the dashboard/terminal/hearing-journal all learn the card is old;
// suspend layer — the turn parks above its card (transcript saved, card durable on the
//
//	instruments shelf; the human's word, whenever it comes, resumes the run
//	at the step it died on) — the workspace holds the question, not the breath.
var (
	watchdogTick     = 30 * time.Second // checks are cheap; the pulse read costs a lock
	watchdogNotice   = 3 * time.Minute  // a slow human gets a nudge, never a surprise
	watchdogAutoDeny = 10 * time.Minute // the name survives: past it, the workspace suspends the wait
)

// pendingSuspend is a card the watchdog parked: its run's transcript holds the question at
// the step it died on, and the human's next typed line revives exactly that thread.
type pendingSuspend struct {
	RunID  string
	CardID string
}

// Suspend is the auto-deny layer's kinder hand (freeze review 2026-10-07): instead of denying
// the unanswered card, the turn parks above it — transcript saved, card durable, and the
// human's word revives the thread whenever it comes. Interrupt stays the human's own lawful
// denial; Suspend is the workspace's.
func (h *tuiHost) Suspend() {
	h.mu.Lock()
	h.suspending = true
	cancel := h.cancel
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// parkCard arms the pending suspension and lands the card on the instruments shelf: the
// question outlives the process. The ID composes run+step so the card survives a session
// boundary and finds its own transcript.
func (h *tuiHost) parkCard(r gate.Request, view tui.ChoiceView) string {
	runID := ""
	if h.s != nil {
		runID = h.s.RunID
	}
	id := runID + "-" + r.ID
	h.mu.Lock()
	h.pendingSuspend = &pendingSuspend{RunID: runID, CardID: id}
	h.mu.Unlock()
	rel := filepath.Join(h.a.Cfg.Dist.WorkspaceDir, "instruments", "cards", id+".json")
	rec := map[string]any{
		"id": id, "runId": runID, "step": r.ID, "agent": r.Agent,
		"title": view.Title, "class": r.Class.String(), "tool": r.Tool,
		"summary": r.Summary, "why": view.Why, "options": view.Options,
		"openedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if b, err := json.MarshalIndent(rec, "", "  "); err == nil {
		full := filepath.Join(h.a.Root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err == nil {
			if werr := os.WriteFile(full, b, 0o600); werr != nil {
				return rel + " (shelf unwritten: " + werr.Error() + ")"
			}
		}
	}
	return rel
}

// shelfCard lands a parked card on the instruments shelf: the question outlives the process —
// a restart replays instruments, never the RAM. ID composes run+step so the card survives a
// session boundary AND finds its transcript.
func (h *tuiHost) shelfCard(r gate.Request, view tui.ChoiceView) string {
	runID := ""
	if h.s != nil {
		runID = h.s.RunID
	}
	id := runID + "-" + r.ID
	dir := filepath.Join(h.a.Root, h.a.Cfg.Dist.WorkspaceDir, "instruments", "cards")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "(shelf failed: " + err.Error() + ")"
	}
	rec := map[string]any{
		"id":       id,
		"runId":    runID,
		"step":     r.ID,
		"agent":    r.Agent,
		"title":    view.Title,
		"class":    r.Class.String(),
		"tool":     r.Tool,
		"summary":  r.Summary,
		"why":      r.Why,
		"options":  view.Options,
		"openedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "(shelf failed: " + err.Error() + ")"
	}
	p := filepath.Join(dir, id+".json")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		return "(shelf failed: " + err.Error() + ")"
	}
	h.mu.Lock()
	h.pendingSuspend = &pendingSuspend{RunID: runID, CardID: id}
	h.mu.Unlock()
	return filepath.Join(h.a.Cfg.Dist.WorkspaceDir, "instruments", "cards", id+".json")
}

// watchdog arms the heartbeat for exactly one turn's lifetime (tctx ends with it).
func (h *tuiHost) watchdog(tctx context.Context) {
	go func() {
		tk := time.NewTicker(watchdogTick)
		defer tk.Stop()
		noticed := false
		for {
			select {
			case <-tctx.Done():
				return
			case <-h.ctx.Done():
				return
			case <-tk.C:
			}
			h.mu.Lock()
			running, open := h.running, h.choiceOpen
			silence := time.Since(h.pulse)
			h.mu.Unlock()
			if !running {
				return // the turn ended: this witness dies with it
			}
			if open == 0 {
				continue // nothing waits on a human right now; a card may still open mid-turn
			}
			switch {
			case silence >= watchdogAutoDeny:
				h.journalWatchdog("auto-suspend", silence)
				h.sendQuiet(tui.EvNotice{Text: fmt.Sprintf("watchdog: no answer for %s — the card parks on the instruments shelf and the thread waits for your word (journal carries the cause)", silence.Round(time.Second))})
				h.Suspend()
				return
			case silence >= watchdogNotice && !noticed:
				noticed = true
				h.journalWatchdog("notice", silence)
				h.sendQuiet(tui.EvNotice{Text: fmt.Sprintf("watchdog: a card has waited %s without an answer — answer it, or interrupt (esc / the dashboard stop) frees the workspace", silence.Round(time.Second))})
			case silence < watchdogNotice && noticed:
				noticed = false // activity resumed: the next stall earns its own nudge
			}
		}
	}()
}

func (h *tuiHost) journalWatchdog(act string, silence time.Duration) {
	if h.s == nil || h.s.Journal == nil {
		return
	}
	h.s.Journal.Log(loop.Event{"t": "watchdog", "act": act, "silentMs": silence.Milliseconds(), "openCards": 1})
}

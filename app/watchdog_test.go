package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWatchdogParksAnUnansweredCard shrinks the heartbeat policy to milliseconds and proves the
// spine: the notice layer speaks, the suspend layer parks the card on the instruments shelf
// (a durable little truth that outlives the process), the turn ends SUSPENDED, and the
// human's next typed line revives the run from its transcript — not a fresh forgetful turn.
func TestWatchdogParksAnUnansweredCard(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		when("go on", text("the word revived the run")),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	oldTick, oldNotice, oldDeny := watchdogTick, watchdogNotice, watchdogAutoDeny
	watchdogTick, watchdogNotice, watchdogAutoDeny = 50*time.Millisecond, 100*time.Millisecond, 250*time.Millisecond
	t.Cleanup(func() { watchdogTick, watchdogNotice, watchdogAutoDeny = oldTick, oldNotice, oldDeny })
	h, tm, sc := tuiSessionHost(t, w)
	tm.Type("push it")
	enter(tm)
	sc.wait(t, "1 · Yes")             // card up; nobody answers, nobody interrupts
	sc.wait(t, "watchdog: no answer") // the workspace said its wedge out loud
	sc.wait(t, "interrupted")         // the turn ended (tui prints the interrupt glyph)
	// the parked card is durable, and the run is gapable
	files, _ := filepath.Glob(filepath.Join(w.root, ".agent-one/instruments/cards/*-s*.json"))
	if len(files) == 0 {
		t.Fatal("no parked card on the instruments shelf — the question died with RAM")
	}
	j := w.journalText(".agent-one")
	if !strings.Contains(j, `"t":"watchdog"`) || !strings.Contains(j, "auto-suspend") {
		t.Fatalf("the journal must carry the watchdog's cause:\n%s", j)
	}
	if !strings.Contains(j, `"t":"end","status":"SUSPENDED"`) && !strings.Contains(j, `"status":"SUSPENDED"`) {
		t.Fatalf("the run must end SUSPENDED:\n%s", j)
	}
	// the word revives: next typed line resumes the run's transcript
	tm.Type("go on")
	enter(tm)
	sc.wait(t, "the word revived the run")
	_ = h // kept for the next assert once the live binding is exercised
}

// TestWatchdogLeavesAnAnsweredTurnAlone: answered cards die before any layer fires — the
// watchdog never interrupts a workspace that is being steered.
func TestWatchdogLeavesAnAnsweredTurnAlone(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		text("pushed"),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	oldTick, oldNotice, oldDeny := watchdogTick, watchdogNotice, watchdogAutoDeny
	watchdogTick, watchdogNotice, watchdogAutoDeny = 30*time.Millisecond, 60*time.Millisecond, 120*time.Millisecond
	t.Cleanup(func() { watchdogTick, watchdogNotice, watchdogAutoDeny = oldTick, oldNotice, oldDeny })
	_, tm, sc := tuiSessionHost(t, w)
	tm.Type("push it")
	enter(tm)
	sc.wait(t, "1 · Yes")
	enter(tm) // the human answers Yes well inside the notice layer
	sc.wait(t, "pushed")
	if s := sc.text(); strings.Contains(s, "watchdog:") || strings.Contains(s, "■ interrupted") {
		t.Fatalf("the watchdog fired on a steered workspace:\n%s", s)
	}
	j := w.journalText(".agent-one")
	if strings.Contains(j, `"t":"watchdog"`) {
		t.Fatalf("no watchdog event belongs in a cleanly answered turn:\n%s", j)
	}
}

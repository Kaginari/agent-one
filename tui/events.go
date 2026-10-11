package tui

import "time"

// The events the app sends the program (UI.Send). Every one is a tea.Msg; the model turns it
// into a block above the live area or a change to it.

// EvDelta is streamed session text.
type EvDelta struct{ Text string }

// EvToolStart opens a tool block; EvToolEnd closes it with the outcome (matched by ID).
type EvToolStart struct{ Tool ToolView }
type EvToolEnd struct{ Tool ToolView }

// EvSubagent upserts a Subagent block by name: its state while it runs, its report when it lands.
type EvSubagent struct{ Subagent SubagentView }

// EvState is a agent's state (thinking · tool · waiting on gate · done); the session's drives
// the spinner verb.
type EvState struct{ Agent, State, Tool string }

// EvTurnStart says the app started a turn on its own (a queued line, a Subagent's report).
type EvTurnStart struct {
	Text string
	Auto bool
}

// EvTurnDone closes a turn: the answer (already streamed or not), the holes, the gate verdict.
type EvTurnDone struct {
	Status      string
	Text        string
	Streamed    bool
	Holes       []string
	Verdict     string
	LogRel      string
	Interrupted bool
	Hint        string // e.g. the resume command after a checkpoint
}

// EvNotice is one dim line; EvError one in the error colour; EvLines several plain lines (a
// slash command's answer).
type EvNotice struct{ Text string }
type EvError struct{ Text string }
type EvLines struct{ Lines []string }

// EvChoice puts a choice to the operator — the gate's approval or a question — and waits on
// Reply. The model answers exactly once; a program that exits answers with Aborted.
type EvChoice struct {
	View  ChoiceView
	Reply chan<- ChoiceAnswer
}

// ChoiceAnswer is what the operator picked.
type ChoiceAnswer struct {
	Index   int    // the option, 0-based; -1 for free text
	Text    string // free text, or the reason given with a denial
	Aborted bool   // the program went away before an answer
}

// EvChoiceAnswered lands on the event bus (and its instruments tap) after every answered
// choice — the evidence ledger a review lane reads later to graduate rules from patterns
// ("you always allow class X" wants a rule, not a faster cursor). It never steers a live
// choice: the gate stays silent, and no value function touches the human's pacing.
type EvChoiceAnswered struct {
	Class, Tool, Answer, Recommended string
}

// EvPlanOutcome closes the scenario funnel's loop at a turn's end (crafted-skill
// scenario-3-2-1): each deck that updated during that turn lands as one row naming the agent,
// the end verdict, the cards it carried and the chosen slice. The scorecard's tomorrow reads
// yesterday's rows — instruments, never weights.
type EvPlanOutcome struct {
	Agent, Verdict string // the deck's owner, the turn's end verdict
	Cards          int    // deck items the deck carried
	Chosen         string // id of the in_progress/done item the turn worked ("" when none named)
	Why            string // the score's evidence, quoted ("" = no score carded)
}

// EvQuit ends the program.
type EvQuit struct{}

// evSlashDone carries a slash command's output back into the model.
type evSlashDone struct {
	lines []string
	quit  bool
}

// evTick refreshes the footer and the spinner's elapsed reading.
type evTick struct{ at time.Time }

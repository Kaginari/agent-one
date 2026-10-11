package value

import "github.com/Kaginari/agent-one/rank"

// Need is the capability a signal needs from a rank before it can read anything but zero.
type Need string

const (
	NeedNone     Need = ""          // every rank produces it
	NeedAuthors  Need = "authors"   // the rank authors changes (it meets the gate)
	NeedGate     Need = "holdsGate" // the rank holds a gate (it issues verdicts)
	NeedCommands Need = "commands"  // the rank dispatches (it has agents under it)
	NeedRoot     Need = "root"      // the orchestrator alone (it answers the human)
)

// Signal is one reading the value ledger folds to, per agent over the window.
type Signal struct {
	Name  string
	Scale string // percent | count | momentum
	Need  Need
	Doc   string
}

// signals is the one table: the ASL operand set, the score's vocabulary, the meter's choices.
var signals = []Signal{
	{"runs", "count", NeedNone, "every run the agent made in the window, neutral ones included"},
	{"reports", "count", NeedNone, "runs that were judged: a pass or a fail (policy.value.pass / .fail); neutral runs never count"},
	{"passes", "count", NeedNone, "judged runs that passed"},
	{"fails", "count", NeedNone, "judged runs that failed"},
	{"neutral", "count", NeedNone, "runs whose word is neutral (the human's DENIED, an INTERRUPTED, a DRY run) — never the agent's failure"},
	{"holes", "count", NeedNone, "harness holes: the discipline the harness caught (off-wire answers, a missing @U, a gate sent back) — never infra faults or an honest @?"},
	{"passRate", "percent", NeedNone, "passes over reports"},
	{"failRate", "percent", NeedNone, "fails over reports"},
	{"holeRate", "percent", NeedNone, "reports that carried at least one harness hole, over reports (≤ 100%)"},
	{"landRate", "percent", NeedNone, "reports that landed: the loop ended DONE, the agent's word is not a fail, and no gate failed"},
	{"intelligence", "percent", NeedNone, "passRate less holeRate, floored at zero"},
	{"gateRate", "percent", NeedAuthors, "gated runs that passed the gate on the first try"},
	{"verifyGreenRate", "percent", NeedAuthors, "verify lines that ran green at the gate, over verify lines run"},
	{"gateRight", "percent", NeedGate, "verdicts the agent held that were not overturned (a pass later failed on the same paths)"},
	{"verdicts", "count", NeedGate, "gate verdicts the agent held"},
	{"overturned", "count", NeedGate, "the agent's pass verdicts a later fail on the same paths overturned"},
	{"zoneHealth", "percent", NeedCommands, "100 less the mean strain (as a percent of policy.stress.escalateAt) of the agents it dispatched"},
	{"planLandRate", "percent", NeedCommands, "its dispatches that landed"},
	{"reaskRate", "percent", NeedCommands, "its dispatches that re-commissioned an agent after a council warrant"},
	{"askDoneRate", "percent", NeedRoot, "the human's turns closed with a pass and not reopened (the same paths rewritten) within policy.value.reopenTurns turns"},
	{"upheldRate", "percent", NeedNone, "reports that filed findings (@F) and landed"},
	{"findings", "count", NeedNone, "@F findings filed"},
	{"thrashRate", "percent", NeedNone, "runs whose guts priced a bench thrash"},
	{"momentum", "momentum", NeedNone, "the mean closing guts momentum of its runs"},
	{"strain", "count", NeedNone, "the council's strain on the agent now (0 after a clean landing)"},
	{"strainPct", "percent", NeedNone, "strain as a percent of policy.stress.escalateAt"},
	{"tokens", "count", NeedNone, "tokens spent in the window"},
	{"score", "percent", NeedNone, "the rank's value score (ranks.<r>.value.score), 0..100"},
}

// Table is the signal table, in the docs order.
func Table() []Signal { return append([]Signal(nil), signals...) }

// SignalNames lists every signal name.
func SignalNames() []string {
	out := make([]string, 0, len(signals))
	for _, s := range signals {
		out = append(out, s.Name)
	}
	return out
}

// Lookup finds one signal.
func Lookup(name string) (Signal, bool) {
	for _, s := range signals {
		if s.Name == name {
			return s, true
		}
	}
	return Signal{}, false
}

// IsSignal reports whether name is a signal.
func IsSignal(name string) bool { _, ok := Lookup(name); return ok }

// Produces reports whether a rank with these capabilities can read a signal as anything but a
// standing zero — a score built on a signal its rank never produces is a load-time hole.
func Produces(f rank.Flags, name string) bool {
	s, ok := Lookup(name)
	if !ok {
		return false
	}
	switch s.Need {
	case NeedAuthors:
		return f.Authors
	case NeedGate:
		return f.HoldsGate
	case NeedCommands:
		return f.Commands
	case NeedRoot:
		return f.Root
	}
	return true
}

package app

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/instrument"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/wire"
)

// LiveAgent is one agent the subagent can see: rank, role, model, state, elapsed, context, spend.
type LiveAgent struct {
	Name    string
	Rank    string
	Role    string
	Model   string
	State   string // thinking · tool · waiting on gate · done · queued
	Exit    string // the run journal's t:end verdict (DONE · FAIL · ESCALATE · DENIED · INTERRUPTED), lifted off the wire report at Finish — the subagent card's face
	Failed  bool   // Exit names a failed verdict
	Started time.Time
	Ended   time.Time
	Session *loop.Session
	Depth   int
	// two honest lanes (freeze review 2026-10-07): Send lines ride the running turn's next
	// tool batch; Enqueue lines sit until the turn drains. Before, both shared one inbox —
	// Enqueue's "never mid-turn" contract was a lie under loop/loop.go's Inbox hook.
	mid    []string
	next   []string
	report string
	orders []wire.Enqueue // @$ lines it asked for; drained as dispatches at the turn boundary
	done   chan struct{}
}

// Subagent is the live subagent (binary.md §Live session): every agent that runs or ran in this
// session, its state, its inbox for /send, and the reports of asynchronous Subagents waiting to
// wake their dispatcher.
type Subagent struct {
	app    *App
	mu     sync.Mutex
	agents map[string]*LiveAgent
	order  []string
	wake   []string // reports of finished background Subagents, for the session's next turn
	onWake func()
	// onFinish sees a background subagent's report as it lands (the TUI draws its block).
	onFinish func(name, report string)
	// relays holds the @$ orders a dispatched agent's commission declared it must return in its
	// report. Expectations register at dispatch-observe time (the ask carried @$ lines), clear
	// when the named agent is tracked (the relay booked), and close at Finish with either the
	// echoed order or an unbooked-relay wake for the orchestrator.
	relays map[string][]wire.Enqueue
}

func newSubagent(a *App) *Subagent {
	c := &Subagent{app: a, agents: map[string]*LiveAgent{}}
	prev := a.OnState
	a.OnState = func(agent, state string) {
		c.setState(agent, state)
		if prev != nil {
			prev(agent, state)
		}
	}
	return c
}

// Track registers an agent's session as it starts.
func (c *Subagent) Track(name, rank, role, model string, s *loop.Session, depth int) *LiveAgent {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.agents[name]
	if b == nil {
		b = &LiveAgent{Name: name, done: make(chan struct{})}
		c.agents[name] = b
		c.order = append(c.order, name)
	}
	b.Rank, b.Role, b.Model, b.Session, b.Depth = rank, role, model, s, depth
	b.Started, b.Ended, b.State = time.Now(), time.Time{}, "queued"
	if b.done == nil {
		b.done = make(chan struct{})
	}
	// the relay booked: an agent named in someone's outstanding relay expectation is now running
	for holder, ords := range c.relays {
		keep := ords[:0]
		for _, o := range ords {
			if o.Agent != name {
				keep = append(keep, o)
			}
		}
		if len(keep) == 0 {
			delete(c.relays, holder)
		} else {
			c.relays[holder] = keep
		}
	}
	return b
}

// ExpectRelay records the orders a commission's @$ lines ask a dispatched agent to echo back in
// its report. A relay not carried by the report and not booked meanwhile surfaces as a wake at
// the agent's Finish — the orchestrator hears the chain landed or that it owes the booking.
func (c *Subagent) ExpectRelay(agent string, orders []wire.Enqueue) {
	if len(orders) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.relays == nil {
		c.relays = map[string][]wire.Enqueue{}
	}
	c.relays[strings.ToLower(agent)] = append(c.relays[strings.ToLower(agent)], orders...)
}

func (c *Subagent) setState(agent, state string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.agents[agent]
	if b == nil {
		b = &LiveAgent{Name: agent, Started: time.Now(), done: make(chan struct{})}
		c.agents[agent] = b
		c.order = append(c.order, agent)
	}
	b.State = state
	if state == "done" {
		b.Ended = time.Now()
	}
}

// observe is the loop's State seam: the agent's state and its live session.
func (c *Subagent) observe(s *loop.Session, state string) {
	name := agentName(s)
	c.setState(name, state)
	c.mu.Lock()
	if b := c.agents[name]; b != nil && b.Session == nil {
		b.Session = s
	}
	c.mu.Unlock()
}

// Finish marks an agent done with its report. Before the report goes out the Subagent lifts its
// @$ lines into the agent's order book — drained as dispatches at the turn boundary, where
// the chain gate re-checks each one (fail-closed; a refused order surfaces as a hole).
func (c *Subagent) Finish(name, report string) {
	c.mu.Lock()
	b := c.agents[name]
	if b == nil {
		b = &LiveAgent{Name: name, done: make(chan struct{})}
		c.agents[name] = b
		c.order = append(c.order, name)
	}
	if rep, ok := wire.ParseReport(report); ok {
		b.orders = append(b.orders, rep.Enqueues...)
		// subagent-card-exit-verdict policy (leg 2a): the exit word is the first @S token — the loop
		// stamps a non-DONE exit as "FAIL — …", so token one is the journal's verdict.
		if rep.Status != "" {
			tok := strings.ToUpper(strings.Fields(rep.Status)[0])
			switch tok {
			case "DONE", "FAIL", "ESCALATE", "DENIED", "INTERRUPTED":
				b.Exit = tok
			}
		}
	}
	b.Failed = b.Exit == "FAIL" || b.Exit == "DENIED" || b.Exit == "ESCALATE"
	b.State, b.Ended, b.report = "done", time.Now(), report
	// the relay ledger: for each @$ the agent's commission declared, its own report must carry the
	// echoed order toward that agent, or the orchestrator hears "[relay unbooked]"
	if exp := c.relays[strings.ToLower(name)]; len(exp) > 0 {
		delete(c.relays, strings.ToLower(name))
		var echoed []string
		if rep, ok := wire.ParseReport(report); ok {
			for _, o := range rep.Enqueues {
				echoed = append(echoed, o.Agent)
			}
		}
		for _, e := range exp {
			found := false
			for _, eb := range echoed {
				if eb == e.Agent {
					found = true
					break
				}
			}
			foundTracked := false
			if tb := c.agents[e.Agent]; tb != nil && tb.State != "done" {
				foundTracked = true
			}
			if !found && !foundTracked {
				c.wake = append(c.wake, fmt.Sprintf("[relay unbooked] %s closed and its report left '%s' unbooked (its commission carried '@$ %s: %s') — the relay needs the orchestrator's hand", name, e.Agent, e.Agent, e.Ask))
			}
		}
	}
	select {
	case <-b.done:
	default:
		close(b.done)
	}
	f := c.onFinish
	c.mu.Unlock()
	if f != nil && report != "" {
		f(name, report)
	}
}

// Orders drains an agent's @$ order book — the dispatches it asked for at the boundary.
func (c *Subagent) Orders(name string) []wire.Enqueue {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.agents[name]
	if b == nil || len(b.orders) == 0 {
		return nil
	}
	out := b.orders
	b.orders = nil
	return out
}

// OrderBook drains every queued @$ order and returns each one with the report of any same-named
// Subagent that already ran and finished — so the REPL's turn-done drain can register the new dispatch
// with thread ("the same agent's earlier Subagent reported: @S …"). Orders are flat (no per-agent
// queue); the order book refills as reports land.
func (c *Subagent) OrderBook() []struct {
	Agent, Ask, Report string
} {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []struct {
		Agent, Ask, Report string
	}
	for _, b := range c.agents {
		for _, o := range b.orders {
			re := struct{ Agent, Ask, Report string }{Agent: o.Agent, Ask: o.Ask}
			if done := c.agents[o.Agent]; done != nil && done.State == "done" && done.report != "" {
				re.Report = done.report
			}
			out = append(out, re)
		}
		b.orders = nil
	}
	return out
}

// Send queues a line for a running agent (delivered at its next tool step).
// Mid-turn steering only — a human's queued line must NOT use this: it rides the
// running turn's context and breaks task isolation. Use Enqueue for FIFO steering
// (the B contract: complete the current task, then hear the next line).
func (c *Subagent) Send(name, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.agents[strings.ToLower(name)]
	if b == nil {
		return fmt.Errorf("no agent named %q in the subagent (%s)", name, strings.Join(c.order, ", "))
	}
	if b.State == "done" {
		return fmt.Errorf("%s is done — its context died with its task; dispatch it again", name)
	}
	b.mid = append(b.mid, text)
	return nil
}

// Enqueue FIFOs a line for an agent with no mid-turn delivery: it sits in the inbox
// until an end-of-turn drain (finish/woke/repl turn-done) starts it as the next turn.
// Unlike Send, it never rides the running turn's next tool step — a human's queued
// line uses this so the current task completes before the new one is heard.
func (c *Subagent) Enqueue(name, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.agents[strings.ToLower(name)]
	if b == nil {
		return fmt.Errorf("no agent named %q in the subagent (%s)", name, strings.Join(c.order, ", "))
	}
	b.next = append(b.next, text)
	return nil
}

// Inbox drains an agent's MID-TURN lines (the Send lane) — the loop's tool-batch seam reads
// exactly this; Enqueue lines never surface here.
func (c *Subagent) Inbox(name string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.agents[name]
	if b == nil || len(b.mid) == 0 {
		return nil
	}
	lines := b.mid
	b.mid = nil
	return lines
}

// DrainAll empties both lanes (mid leftovers then the FIFO) — the turn-end seam's word:
// "finish with everything that arrived while you worked".
func (c *Subagent) DrainAll(name string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.agents[name]
	if b == nil || len(b.mid)+len(b.next) == 0 {
		return nil
	}
	lines := append(b.mid, b.next...)
	b.mid, b.next = nil, nil
	return lines
}

// QueueDepth reports an agent's unread line count (both lanes) — the dashboard's wedge meter.
func (c *Subagent) QueueDepth(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.agents[name]
	if b == nil {
		return 0
	}
	return len(b.mid) + len(b.next)
}

// Wake queues a finished background Subagent's report for the dispatcher's next turn.
func (c *Subagent) Wake(report string) {
	c.mu.Lock()
	c.wake = append(c.wake, report)
	f := c.onWake
	c.mu.Unlock()
	if f != nil {
		f()
	}
}

// TakeWake returns the queued reports, emptying the queue.
func (c *Subagent) TakeWake() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.wake
	c.wake = nil
	return out
}

// Agents lists the subagent in start order.
func (c *Subagent) Agents() []LiveAgent {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []LiveAgent
	for _, n := range c.order {
		out = append(out, *c.agents[n])
	}
	return out
}

// Live counts the agents not done.
func (c *Subagent) Live() int {
	n := 0
	for _, b := range c.Agents() {
		if b.State != "done" && b.Depth > 0 { // the session itself is not a dispatched agent
			n++
		}
	}
	return n
}

// Lines renders /agents: rank, role, model, state, elapsed, context %, tokens, cost.
func (c *Subagent) Lines() []string {
	agents := c.Agents()
	if len(agents) == 0 {
		return []string{"the subagent is empty"}
	}
	sort.SliceStable(agents, func(i, j int) bool { return agents[i].Started.Before(agents[j].Started) })
	var out []string
	for _, b := range agents {
		end := time.Now()
		if !b.Ended.IsZero() {
			end = b.Ended
		}
		ctx := "ctx —"
		if b.Session != nil && b.Session.Reading().Available {
			ctx = fmt.Sprintf("ctx %d%%", b.Session.Reading().Percent())
		}
		sp := c.app.Journal.Agent(b.Name)
		cost := "unpriced"
		if sp.Calls > 0 && sp.Unpriced == 0 {
			cost = fmt.Sprintf("$%.4f", sp.USD)
		}
		out = append(out, fmt.Sprintf("%-20s %-8s %-10s %-32s %-15s %6s  %s  %6d tok  %s", b.Name, orStr(b.Rank, "orchestrator"), orStr(b.Role, "—"), orStr(b.Model, "—"), b.State, end.Sub(b.Started).Round(time.Second), ctx, sp.Tokens(), cost))
	}
	return out
}

// StatusLine is the one line above the prompt.
func (c *Subagent) StatusLine(s *loop.Session) string {
	tot := c.app.Journal.Total()
	cost := "unpriced"
	switch {
	case tot.Calls == 0:
		cost = "no calls yet"
	case tot.Unpriced == 0:
		cost = fmt.Sprintf("$%.4f", tot.USD)
	}
	ctx := "ctx —"
	if r := readingOf(s); r.Available {
		ctx = fmt.Sprintf("ctx %d%% (%s)", r.Percent(), r.Zone)
	}
	live := c.Live()
	subagents := ""
	if live > 0 {
		subagents = fmt.Sprintf(" · %d live", live)
	}
	return fmt.Sprintf("%s · %s · %s · %d tok · %s%s", c.app.Cfg.Dist.Name, c.app.mountModel.Ref.Model, ctx, tot.Tokens(), cost, subagents)
}

// Close marks every agent done.
func (c *Subagent) Close() {
	for _, b := range c.Agents() {
		if b.State != "done" {
			c.Finish(b.Name, "")
		}
	}
}

func readingOf(s *loop.Session) instrument.Context {
	if s == nil {
		return instrument.Context{}
	}
	return s.Reading()
}

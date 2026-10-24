package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/loop"
)

// Tracer turns the loop's own hooks into OpenTelemetry spans and ships them to a collector as
// OTLP/HTTP JSON — the standard library's POST, no SDK: a CHAIN span per turn (named rank · agent),
// an AGENT span per subagent (the dispatch that summoned it, parent of the subagent's turns) and a
// TOOL span per executed call, kinds as OpenInference spells them. Export is outward (Principle 7):
// NewTracer is nil unless the human's own config enabled it and named the endpoint, and every
// method is a no-op on nil. A failure never reaches the loop: it is spoken once per class through
// warn (the session's notice line), and the spans are dropped.
type Tracer struct {
	endpoint string
	headers  map[string]string
	unset    []string // headersEnv variables that resolved empty: named on the first send
	service  string
	rankOf   func(body string) string
	warn     func(string)
	client   *http.Client
	every    time.Duration // the batch's longest wait

	mu        sync.Mutex
	chains    map[*loop.Session]*span    // the open turn of each session
	tools     map[*loop.StepRecord]*span // calls between Observe start and end
	subagents map[string]*span           // agent → the open dispatch its next turn hangs under
	warned    map[string]bool
	dropped   bool // a span met a full queue
	closed    bool

	queue chan otlpSpan
	once  sync.Once
	done  chan struct{}
}

// span is one span in flight.
type span struct {
	trace, id, parent, name string
	start                   time.Time
	attrs                   []otlpAttr
}

const (
	traceBatch = 64      // spans per POST
	traceCut   = 4 << 10 // input.value / output.value are cut here
	traceQueue = 1 << 12 // spans waiting for the exporter; past it they are dropped
)

// NewTracer is the session's tracer, or nil when export is off or names no endpoint. env reads
// the headersEnv variables; rankOf names an agent's rank; warn carries the once-per-class notices.
func NewTracer(o config.OTLP, service string, env func(string) string, rankOf func(string) string, warn func(string)) *Tracer {
	if !o.Enabled || o.Endpoint == "" {
		return nil
	}
	if o.Service != "" {
		service = o.Service
	}
	t := &Tracer{endpoint: o.Endpoint, headers: map[string]string{}, service: service, rankOf: rankOf, warn: warn,
		client: &http.Client{Timeout: 10 * time.Second}, every: 2 * time.Second,
		chains: map[*loop.Session]*span{}, tools: map[*loop.StepRecord]*span{}, subagents: map[string]*span{}, warned: map[string]bool{},
		queue: make(chan otlpSpan, traceQueue), done: make(chan struct{})}
	for _, h := range sortedKeys(o.HeadersEnv) {
		if v := env(o.HeadersEnv[h]); v != "" {
			t.headers[h] = v
		} else {
			t.unset = append(t.unset, o.HeadersEnv[h])
		}
	}
	return t
}

// State opens the turn's CHAIN on its first model call and closes it on a panic (Ended never
// comes then).
func (t *Tracer) State(s *loop.Session, state string) {
	if t == nil {
		return
	}
	rank := t.rank(s)
	t.mu.Lock()
	defer t.mu.Unlock()
	switch state {
	case "thinking":
		t.chain(s, rank)
	case "panic":
		if c := t.chains[s]; c != nil {
			delete(t.chains, s)
			t.emit(c, true, "panic")
		}
	}
}

// Observe opens a call's span at start and ships it at end when the call ran (done or failed);
// a call the gate, a rule or a guard stopped never executed and leaves no span. A dispatch is
// the subagent's AGENT span: the summoned agent's next turn is its child, in the same trace.
func (t *Tracer) Observe(s *loop.Session, st *loop.StepRecord, phase string) {
	if t == nil {
		return
	}
	rank := t.rank(s)
	t.mu.Lock()
	defer t.mu.Unlock()
	sp := t.tools[st]
	if sp == nil {
		parent := t.chain(s, rank)
		sp = &span{trace: parent.trace, id: spanID(), parent: parent.id, name: st.Tool, start: time.Now(),
			attrs: []otlpAttr{attrStr("openinference.span.kind", "TOOL"), attrStr("tool.name", st.Tool), attrStr("input.value", traceClip(string(st.Input)))}}
		if body := dispatchAgent(st); body != "" {
			sp.name = "subagent · " + body
			sp.attrs[0] = attrStr("openinference.span.kind", "AGENT")
			sp.attrs = append(sp.attrs, attrStr("agent_one.subagent", body))
			t.subagents[body] = sp
		}
	}
	if phase == "start" {
		t.tools[st] = sp
		return
	}
	delete(t.tools, st)
	if st.Status != "done" && st.Status != "failed" {
		if body := dispatchAgent(st); body != "" && t.subagents[body] == sp {
			delete(t.subagents, body)
		}
		return
	}
	sp.attrs = append(sp.attrs, attrStr("output.value", traceClip(st.Result.Output)), attrStr("agent_one.status", st.Status))
	t.emit(sp, st.Status == "failed", "")
}

// Ended closes the turn's CHAIN with its status word.
func (t *Tracer) Ended(s *loop.Session, r *loop.Result) {
	if t == nil {
		return
	}
	rank := t.rank(s)
	t.mu.Lock()
	defer t.mu.Unlock()
	c := t.chain(s, rank)
	delete(t.chains, s)
	c.attrs = append(c.attrs, attrStr("agent_one.status", r.Status), attrInt("agent_one.steps", len(r.Steps)), attrInt("agent_one.holes", len(r.Holes)), attrStr("output.value", traceClip(r.Text)))
	t.emit(c, r.Status == loop.Fail, strings.Join(r.Holes, "; "))
}

// Close ships what is queued (a few seconds at most) and stops the exporter.
func (t *Tracer) Close() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	close(t.queue)
	t.mu.Unlock()
	t.once.Do(func() { close(t.done) }) // never started: nothing to wait for
	select {
	case <-t.done:
	case <-time.After(5 * time.Second):
	}
}

// rank names the session's agent's rank — read before t.mu, never under it (the workspace's lock).
func (t *Tracer) rank(s *loop.Session) string {
	if t.rankOf == nil {
		return ""
	}
	return t.rankOf(agentName(s))
}

// chain is the session's open turn, opened when absent: an ephemeral subagent's first turn hangs under
// the dispatch that summoned it, every other turn starts its own trace. Caller holds t.mu.
func (t *Tracer) chain(s *loop.Session, rank string) *span {
	if c := t.chains[s]; c != nil {
		return c
	}
	body := agentName(s)
	c := &span{trace: traceID(), id: spanID(), name: body, start: time.Now(),
		attrs: []otlpAttr{attrStr("openinference.span.kind", "CHAIN"), attrStr("agent_one.agent", body), attrStr("agent_one.rank", rank), attrStr("agent_one.run", s.RunID)}}
	if rank != "" {
		c.name = rank + " · " + body
	}
	if subagent := t.subagents[body]; subagent != nil {
		c.trace, c.parent = subagent.trace, subagent.id
		delete(t.subagents, body)
	}
	t.chains[s] = c
	return c
}

// emit queues a finished span; a full queue drops it and says so once. Caller holds t.mu.
func (t *Tracer) emit(sp *span, failed bool, msg string) {
	if t.closed {
		return
	}
	t.once.Do(func() { go t.run() })
	o := otlpSpan{TraceID: sp.trace, SpanID: sp.id, ParentSpanID: sp.parent, Name: sp.name, Kind: 1,
		Start: strconv.FormatInt(sp.start.UnixNano(), 10), End: strconv.FormatInt(time.Now().UnixNano(), 10), Attributes: sp.attrs}
	if failed {
		o.Status = otlpStatus{Code: 2, Message: traceClip(msg)}
	}
	select {
	case t.queue <- o:
	default:
		t.dropped = true // spoken by the exporter, never from under a hook's lock
	}
}

// run is the exporter: a batch leaves when it is full, when it has waited `every`, and at Close.
func (t *Tracer) run() {
	defer close(t.done)
	tick := time.NewTicker(t.every)
	defer tick.Stop()
	var batch []otlpSpan
	flush := func() {
		if len(batch) > 0 {
			t.post(batch)
			batch = nil
		}
	}
	for {
		select {
		case sp, ok := <-t.queue:
			if !ok {
				flush()
				return
			}
			if batch = append(batch, sp); len(batch) >= traceBatch {
				flush()
			}
		case <-tick.C:
			flush()
		}
	}
}

// post sends one batch as an ExportTraceServiceRequest. Every notice is spoken from here, the
// exporter's goroutine: a hook never waits on the notice line.
func (t *Tracer) post(batch []otlpSpan) {
	t.mu.Lock()
	dropped := t.dropped
	t.mu.Unlock()
	if dropped {
		t.warnOnce("full", "traces: the export queue is full — spans are dropped until the collector catches up")
	}
	for _, v := range t.unset {
		t.warnOnce("unset "+v, "traces: "+v+" is not set — spans are sent without that header (instruments.otlp.headersEnv)")
	}
	body, _ := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{
		"resource":   map[string]any{"attributes": []otlpAttr{attrStr("service.name", t.service)}},
		"scopeSpans": []any{map[string]any{"scope": map[string]string{"name": t.service}, "spans": batch}},
	}}})
	ctx, cancel := context.WithTimeout(context.Background(), t.client.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		t.warnOnce("request", "traces: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for h, v := range t.headers {
		req.Header.Set(h, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		t.warnOnce("send", fmt.Sprintf("traces: the collector at %s did not answer — spans are dropped while it is away", t.host()))
		return
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		t.warnOnce("http "+strconv.Itoa(resp.StatusCode), fmt.Sprintf("traces: the collector at %s answered %s — spans are dropped (check instruments.otlp)", t.host(), resp.Status))
	}
}

// warnOnce speaks one notice per failure class for the tracer's life: a dead collector is one
// line, never a line per batch.
func (t *Tracer) warnOnce(class, msg string) {
	t.mu.Lock()
	first := !t.warned[class]
	t.warned[class] = true
	t.mu.Unlock()
	if first && t.warn != nil {
		t.warn(msg)
	}
}

// host is the endpoint's host alone: a notice never repeats a path or query that may carry a key.
func (t *Tracer) host() string {
	if u, err := url.Parse(t.endpoint); err == nil {
		return u.Host
	}
	return "the endpoint"
}

// dispatchAgent is the agent a dispatch call summons ("" for any other call).
func dispatchAgent(st *loop.StepRecord) string {
	if st.Tool != "dispatch" {
		return ""
	}
	var in struct{ Agent string }
	_ = json.Unmarshal(st.Input, &in)
	return in.Agent
}

// The OTLP/HTTP JSON shapes (opentelemetry-proto, trace/v1): ids are hex, times are decimal
// strings of Unix nanoseconds, kind 1 is INTERNAL, status code 2 is ERROR.
type otlpSpan struct {
	TraceID      string     `json:"traceId"`
	SpanID       string     `json:"spanId"`
	ParentSpanID string     `json:"parentSpanId,omitempty"`
	Name         string     `json:"name"`
	Kind         int        `json:"kind"`
	Start        string     `json:"startTimeUnixNano"`
	End          string     `json:"endTimeUnixNano"`
	Attributes   []otlpAttr `json:"attributes"`
	Status       otlpStatus `json:"status"`
}

type otlpStatus struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type otlpAttr struct {
	Key   string            `json:"key"`
	Value map[string]string `json:"value"`
}

func attrStr(k, v string) otlpAttr { return otlpAttr{k, map[string]string{"stringValue": v}} }
func attrInt(k string, n int) otlpAttr {
	return otlpAttr{k, map[string]string{"intValue": strconv.Itoa(n)}}
}

func traceID() string { return randHex(16) }
func spanID() string  { return randHex(8) }

// traceClip keeps a value under traceCut bytes, on a rune boundary.
func traceClip(s string) string {
	if len(s) <= traceCut {
		return s
	}
	return strings.ToValidUTF8(s[:traceCut], "") + "…"
}

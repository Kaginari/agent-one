package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/provider"
)

// loadCfg writes a project config into a fresh workspace and loads it with a fake environment.
func loadCfg(t *testing.T, yamlText string, env map[string]string) (*config.Config, string) {
	t.Helper()
	return loadCfgAt(t, "config.yaml", yamlText, env)
}

// loadCfgAt writes the config as one layer's file: config.local.yaml is the machine's own word.
func loadCfgAt(t *testing.T, file, yamlText string, env map[string]string) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "workspace")
	home := filepath.Join(dir, "home")
	_ = os.MkdirAll(filepath.Join(root, ".agent-one"), 0o755)
	_ = os.MkdirAll(home, 0o755)
	if yamlText != "" {
		if err := os.WriteFile(filepath.Join(root, ".agent-one", file), []byte(yamlText), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := config.LoadWith(config.Options{Dist: "agent-one", Root: root, Home: home, Env: func(k string) string { return env[k] }})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return c, root
}

// anthropicStub answers every Messages call and records the last agent.
func anthropicStub(t *testing.T, status int) (*httptest.Server, *map[string]interface{}) {
	t.Helper()
	var last map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		last = nil
		_ = json.Unmarshal(b, &last)
		w.Header().Set("content-type", "application/json")
		if status != 200 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"type":"overloaded_error","message":"busy"}}`)
			return
		}
		model, _ := last["model"].(string)
		_, _ = io.WriteString(w, `{"model":"`+model+`","stop_reason":"end_turn","content":[{"type":"text","text":"@S DONE\n@E 12"}],"usage":{"input_tokens":100,"output_tokens":10,"cache_read_input_tokens":1000,"cache_creation_input_tokens":0}}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &last
}

func TestRoutingAndMeter(t *testing.T) {
	srv, last := anthropicStub(t, 200)
	cfg, root := loadCfg(t, `
models:
  default: anthropic/claude-opus-5
  roles:
    analyst: anthropic/claude-haiku-4-5
    judge: {model: anthropic/claude-opus-5, effort: high}
  ranks:
    zone: anthropic/claude-sonnet-5
  tasks:
    drain: anthropic/claude-haiku-4-5
providers:
  anthropic:
    baseURL: `+srv.URL+`
    models:
      claude-haiku-4-5: {tier: 1, price: {input: 1, output: 5, cacheRead: 0.1}}
      claude-sonnet-5: {tier: 2, price: {input: 2, output: 10}}
      claude-opus-5: {tier: 3}
budgets:
  session: {tokens: 2000}
`, map[string]string{"ANTHROPIC_API_KEY": "sk-test"})
	ps := NewProviders(cfg, func(k string) string { return map[string]string{"ANTHROPIC_API_KEY": "sk-test"}[k] })
	cases := []struct {
		route Route
		model string
		slot  string
	}{
		{Route{}, "claude-opus-5", "models.default"},
		{Route{Role: "analyst"}, "claude-haiku-4-5", "models.roles.analyst"},
		{Route{Role: "analyst"}, "claude-haiku-4-5", "models.roles.analyst"},
		// roles lens policy (member → role → task → default): the commission's role hat
		// decides — ranks never pitch a model, only name who an agent is
		{Route{Rank: "zone", Role: "judge"}, "claude-opus-5", "models.roles.judge"},
		{Route{Rank: "zone"}, "claude-opus-5", "models.default"},
		{Route{Task: "drain"}, "claude-haiku-4-5", "models.tasks.drain"},
		{Route{Task: "gate"}, "claude-opus-5", "models.default"},
	}
	for _, c := range cases {
		pr, m, err := ps.Route(c.route)
		if err != nil || m.ID != c.model || m.Slot != c.slot {
			t.Errorf("%+v → %s (%s) %v; want %s (%s)", c.route, m.ID, m.Slot, err, c.model, c.slot)
			continue
		}
		if _, err := pr.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}); err != nil {
			t.Fatal(err)
		}
		if (*last)["model"] != c.model {
			t.Errorf("%+v: agent model %v", c.route, (*last)["model"])
		}
	}
	// effort rides the role entry
	pr, _, _ := ps.Route(Route{Role: "judge"})
	_, _ = pr.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}})
	if oc, _ := (*last)["output_config"].(map[string]interface{}); oc["effort"] != "high" {
		t.Errorf("effort: %v", (*last)["output_config"])
	}
	// the meter journals every call, priced from config; unpriced models carry usd null
	j := NewUsageJournal(filepath.Join(root, ".agent-one", "instruments", "usage"), "sess-1")
	haiku, m, _ := ps.Route(Route{Role: "analyst", Member: "zone-auth", Rank: "zone"})
	price, _ := cfg.PriceFor(m.Ref.Model)
	meter := &Meter{Provider: haiku, Journal: j, Agent: "zone-auth", Rank: "zone", Role: "analyst", Model: m.Ref.Model, Price: price, Session: cfg.Budgets.Session}
	if _, err := meter.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	opus, m2, _ := ps.Route(Route{})
	meter2 := &Meter{Provider: opus, Journal: j, Agent: "orchestrator", Model: m2.Ref.Model, Session: cfg.Budgets.Session}
	if _, err := meter2.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(j.Path())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("journal lines: %q", lines)
	}
	var rec map[string]interface{}
	_ = json.Unmarshal([]byte(lines[0]), &rec)
	for _, k := range []string{"ts", "session", "agent", "rank", "role", "model", "provider", "input", "output", "cacheRead", "cacheWrite", "usd"} {
		if _, ok := rec[k]; !ok {
			t.Errorf("record lacks %s: %s", k, lines[0])
		}
	}
	// 100 in × $1/M + 10 out × $5/M + 1000 cache read × $0.1/M = 0.00025 — the role hat
	// (models.roles.analyst → haiku) lenses now: rank pins are dead by policy
	if usd, _ := rec["usd"].(float64); usd < 0.00024 || usd > 0.00026 || rec["agent"] != "zone-auth" || rec["role"] != "analyst" || rec["model"] != "anthropic/claude-haiku-4-5" || rec["cacheRead"].(float64) != 1000 {
		t.Errorf("priced record: %s", lines[0])
	}
	_ = json.Unmarshal([]byte(lines[1]), &rec)
	if rec["usd"] != nil || rec["agent"] != "orchestrator" {
		t.Errorf("unpriced record: %s", lines[1])
	}
	if tot := j.Total(); tot.Calls != 2 || tot.Unpriced != 1 || tot.Usage.Input != 200 {
		t.Errorf("totals: %+v", tot)
	}
	// the session budget (2000 tokens) is crossed: 2 × (100+10+1000) = 2220
	if why := meter.Over(); !strings.Contains(why, "session token budget") {
		t.Errorf("budget: %q", why)
	}
	if _, err := meter.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Errorf("over budget call: %v", err)
	}
	recs, _ := ReadUsage(j.Dir)
	roll := RollupOf(recs, timeZero(), "")
	if roll.Records != 2 || roll.ByAgent["zone-auth"].Calls != 1 || roll.ByModel["anthropic/claude-opus-5"].Unpriced != 1 || len(roll.Lines()) < 3 {
		t.Errorf("rollup: %+v", roll)
	}
}

func TestFallbackAndScript(t *testing.T) {
	srv, _ := anthropicStub(t, 529)
	root := t.TempDir()
	script := filepath.Join(root, "script.json")
	_ = os.WriteFile(script, []byte(`[
  {"when": "list", "calls": [{"name": "bash", "input": {"command": "ls"}}]},
  {"text": "@S DONE first\n@E 16"},
  {"when": "again", "text": "again", "repeat": true}
]`), 0o644)
	cfg, _ := loadCfg(t, `
models:
  default: {model: anthropic/claude-opus-5, fallback: mock/m}
providers:
  anthropic: {baseURL: `+srv.URL+`}
  mock: {enabled: true, script: `+script+`}
`, map[string]string{"ANTHROPIC_API_KEY": "sk-test"})
	ps := NewProviders(cfg, func(k string) string { return map[string]string{"ANTHROPIC_API_KEY": "sk-test"}[k] })
	pr, _, err := ps.Route(Route{})
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := pr.(*retrying); !ok {
		t.Fatalf("retry ladder expected outermost, got %T", pr)
	} else if _, ok := r.Provider.(*fallback); !ok {
		t.Fatalf("fallback wrapper expected inside the ladder, got %T", r.Provider)
	}
	ask := func(text string) provider.Response {
		r, err := pr.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: text}}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	// the primary answers 529 → the mock script answers: `when` first, then in order, repeat kept
	if r := ask("please list"); len(r.Message.ToolCalls) != 1 || r.Message.ToolCalls[0].Name != "bash" || r.Stop != provider.StopToolUse {
		t.Errorf("when step: %+v", r.Message)
	}
	if r := ask("anything"); r.Message.Text != "@S DONE first\n@E 16" {
		t.Errorf("ordered step: %q", r.Message.Text)
	}
	if r := ask("again"); r.Message.Text != "again" {
		t.Errorf("repeat: %q", r.Message.Text)
	}
	if r := ask("again"); r.Message.Text != "again" {
		t.Errorf("repeat twice: %q", r.Message.Text)
	}
	if r := ask("nothing left"); !strings.Contains(r.Message.Text, "@? mock script script.json exhausted") {
		t.Errorf("exhausted: %q", r.Message.Text)
	}
	// a mock with no script says so on the wire
	empty, err := newScriptProvider("mock/x", "", root)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := empty.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}}); !strings.Contains(r.Message.Text, "@S MOCK") {
		t.Errorf("no script: %q", r.Message.Text)
	}
	// a missing key is a hole, not a panic
	ps2 := NewProviders(cfg, func(string) string { return "" })
	if _, _, err := ps2.Route(Route{}); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") || len(ps2.Holes()) != 1 {
		t.Errorf("missing key: %v %v", err, ps2.Holes())
	}
}

func TestWindowAuto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		if r.URL.Path == "/v1/models" {
			_, _ = io.WriteString(w, `{"data":[{"id":"fake/vllm-coder","max_model_len":32768}]}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	cfg, _ := loadCfg(t, `
models: {default: vllm/fake/vllm-coder}
providers:
  vllm: {type: openai, baseURL: `+srv.URL+`/v1, contextWindow: auto, toolCalls: text, guidedDecoding: true}
  openrouter: {enabled: true, contextWindow: 128000}
`, nil)
	ps := NewProviders(cfg, func(string) string { return "" })
	m, _ := ps.Resolve(Route{})
	if m.Provider != "vllm" || m.ID != "fake/vllm-coder" {
		t.Fatalf("model id with a slash: %+v", m)
	}
	if n := ps.Window(context.Background(), m); n != 32768 {
		t.Errorf("auto window: %d %v", n, ps.Holes())
	}
	m2 := m
	m2.Entry = cfg.Providers["openrouter"]
	m2.Ref.Model, m2.Provider, m2.ID = "openrouter/x", "openrouter", "x"
	if n := ps.Window(context.Background(), m2); n != 128000 {
		t.Errorf("configured window: %d", n)
	}
}

func TestFinishReasonErrorFallsBack(t *testing.T) {
	if !retryable(fmt.Errorf("openai: the upstream ended the answer with finish_reason error (model m) — a failed call, not an answer")) {
		t.Fatal("an upstream error ending must move the call to the fallback")
	}
	// a missing model group on the gateway (HTTP 404) IS the fallback's purpose: availability —
	// the secondary rides on (2026-09-28: the bench workspace ran domain owner verdicts on a 404ing group)
	if !retryable(fmt.Errorf("openai: HTTP 404: model group z-ai/glm-5 not found")) {
		t.Fatal("a gateway 404 on the primary must move the call to the fallback")
	}
	// an upstream failure relayed in the stream moves on too; the request's own fault does not
	if !retryable(&provider.StreamError{Provider: "openai", Code: 502, Message: "Upstream error"}) || retryable(&provider.StreamError{Provider: "openai", Code: 400, Message: "prompt too long"}) {
		t.Fatal("a stream error event is classified by its code")
	}
}

// flaky fails with each of errs in turn, then answers.
type flaky struct {
	errs  []error
	calls int
}

func (f *flaky) Name() string { return "fake/flaky" }

func (f *flaky) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	f.calls++
	if f.calls <= len(f.errs) {
		return provider.Response{}, f.errs[f.calls-1]
	}
	return provider.Response{Message: provider.Message{Role: provider.Assistant, Text: "@S DONE"}}, nil
}

func TestRetryLadder(t *testing.T) {
	ask := func(ctx context.Context, f *flaky) error {
		_, err := (&retrying{f}).Complete(ctx, provider.Request{})
		return err
	}
	// a gateway restart and a stream cut mid-answer are asked again, and the answer lands
	for _, e := range []error{
		errors.New(`openai: Post "http://gw/v1/chat/completions": dial tcp: connect: connection refused`),
		errors.New("openai: HTTP 503: upstream restarting"),
		fmt.Errorf("openai: stream: %w: no byte for 2m0s", provider.ErrStalled),
		errors.New("openai: stream: unexpected EOF"),
		errors.New("anthropic: stream: the connection ended before message_stop — the answer is incomplete"),
		// a gateway's own timeout is its status, not the client's clock
		errors.New("openai: HTTP 504: gateway timeout"),
		errors.New("openai: HTTP 503: upstream connect error or disconnect/reset before headers. reset reason: connection timeout"),
		// an upstream's failure relayed inside the stream, by its kind — with a code and without
		fmt.Errorf("fallback: %w", &provider.StreamError{Provider: "openai", Code: 502, Message: "Upstream error from Darkbloom: inference generation failed"}),
		&provider.StreamError{Provider: "openai", Message: "Upstream error from Darkbloom: inference generation failed"},
	} {
		f := &flaky{errs: []error{e, e}}
		if err := ask(context.Background(), f); err != nil || f.calls != 3 {
			t.Errorf("%v: transient fault must be re-asked until it answers: err %v after %d calls", e, err, f.calls)
		}
	}
	// a refusal, a missing model, a client timeout and a cancelled run come back on the first ask
	for _, e := range []error{
		errors.New("openai: HTTP 400: context length exceeded"),
		errors.New("anthropic: HTTP 401: invalid x-api-key"),
		errors.New("openai: HTTP 404: model group not found"),
		errors.New(`openai: Post "http://gw": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`),
		fmt.Errorf("openai: %w", context.Canceled),
		// the request's own fault, reported in the stream, and the client's whole clock spent reading it
		&provider.StreamError{Provider: "openai", Code: 400, Message: "prompt too long"},
		&provider.StreamError{Provider: "openai", Type: "invalid_request_error", Message: "bad tool schema"},
		errors.New("openai: stream: context deadline exceeded (Client.Timeout or context cancellation while reading agent)"),
	} {
		f := &flaky{errs: []error{e}}
		if err := ask(context.Background(), f); err == nil || f.calls != 1 {
			t.Errorf("%v: must not be re-asked: err %v after %d calls", e, err, f.calls)
		}
	}
	// the ladder is bounded and says how often it asked
	down := errors.New("openai: HTTP 502: bad gateway")
	f := &flaky{errs: []error{down, down, down, down}}
	if err := ask(context.Background(), f); err == nil || f.calls != 3 || !strings.Contains(err.Error(), "HTTP 502") || !strings.Contains(err.Error(), "asked 3 times") {
		t.Errorf("exhausted ladder: err %v after %d calls", err, f.calls)
	}
	// a run cancelled while a layer waits returns at once, not after the wait
	saved := ladder
	ladder = []time.Duration{time.Hour}
	defer func() { ladder = saved }()
	ctx, cancel := context.WithCancel(context.Background())
	f = &flaky{errs: []error{down}}
	time.AfterFunc(50*time.Millisecond, cancel)
	done := make(chan error, 1)
	go func() { done <- ask(ctx, f) }()
	select {
	case err := <-done:
		if err == nil || f.calls != 1 {
			t.Errorf("cancelled wait: err %v after %d calls", err, f.calls)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the ladder waited past its context")
	}
}

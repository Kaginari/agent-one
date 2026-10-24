package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/tool"
)

// A span never carries a secret by default: a tool's input and output are redacted
// (instruments.otlp.content: redacted), shipped raw only on "full", and left off on "none".
func TestTracerRedactsSpanContent(t *testing.T) {
	secrets := []string{"sk-live0123456789abcdef", "ghp_0123456789abcdefABCDEF0123456789abcd", "hunter2-correct-horse", "eyJhbGciOiJIUzI1NiJ9.payload.sig"}
	in := json.RawMessage(`{"command":"curl -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig' -d password=hunter2-correct-horse"}`)
	out := "OPENAI_API_KEY=sk-live0123456789abcdef\ntoken: ghp_0123456789abcdefABCDEF0123456789abcd\nthe build is green\n"
	ship := func(content string) otlpSpan {
		col := newCollector(t, http.StatusOK)
		tr := NewTracer(config.OTLP{Enabled: true, Endpoint: col.URL + "/v1/traces", Content: content}, "agent-one", func(string) string { return "" }, nil, func(string) {})
		s := &loop.Session{RunID: "r1", Engine: &loop.Engine{As: "zone-auth"}}
		st := &loop.StepRecord{Tool: "bash", Input: in, Status: "done", Result: tool.Result{Output: out}}
		tr.Observe(s, st, "start")
		tr.Observe(s, st, "end")
		tr.Close()
		col.mu.Lock()
		defer col.mu.Unlock()
		for _, sp := range col.spans {
			if sp.attr("tool.name") == "bash" {
				return sp
			}
		}
		t.Fatalf("%s: no tool span: %+v", content, col.spans)
		return otlpSpan{}
	}
	for _, content := range []string{"", "redacted"} {
		sp := ship(content)
		got := sp.attr("input.value") + sp.attr("output.value")
		for _, s := range secrets {
			if strings.Contains(got, s) {
				t.Errorf("content %q shipped the secret %q: %s", content, s, got)
			}
		}
		if !strings.Contains(got, "the build is green") || !strings.Contains(got, "curl") {
			t.Errorf("content %q redacted more than the secrets: %s", content, got)
		}
	}
	if sp := ship("full"); !strings.Contains(sp.attr("output.value"), secrets[0]) {
		t.Errorf("full ships the bytes as they are: %+v", sp.Attributes)
	}
	if sp := ship("none"); sp.attr("input.value") != "" || sp.attr("output.value") != "" {
		t.Errorf("none ships no content: %+v", sp.Attributes)
	}
}

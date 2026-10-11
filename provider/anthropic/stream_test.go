package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/provider"
)

func sse(events ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		for _, ev := range events {
			_, _ = io.WriteString(w, "data: "+ev+"\n\n")
		}
	}
}

func streamOnce(t *testing.T, h http.Handler) (provider.Response, error) {
	t.Helper()
	srv := httptest.NewServer(h)
	defer srv.Close()
	c := &Client{APIKey: "k", Model: "m", BaseURL: srv.URL, HTTP: srv.Client(), Stream: true}
	return c.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}, OnDelta: func(string) {}})
}

// A tool with no arguments streams no input_json_delta: its input is the start block's, or {}.
func TestStreamZeroArgToolCall(t *testing.T) {
	resp, err := streamOnce(t, sse(
		`{"type":"message_start","message":{"model":"m","usage":{"input_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tu_1","name":"status","input":{}}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3}}`,
		`{"type":"message_stop"}`))
	if err != nil || len(resp.Message.ToolCalls) != 1 || string(resp.Message.ToolCalls[0].Input) != `{}` {
		t.Fatalf("zero-arg call: %v %+v", err, resp.Message.ToolCalls)
	}
}

// Redacted thinking carries its data at the block's start; it is replayed as it came.
func TestStreamRedactedThinkingKeepsData(t *testing.T) {
	resp, err := streamOnce(t, sse(
		`{"type":"message_start","message":{"model":"m","usage":{"input_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"EnCrYpTeD"}}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":"ok"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
		`{"type":"message_stop"}`))
	if err != nil || len(resp.Message.Opaque) != 1 || !strings.Contains(string(resp.Message.Opaque[0]), `"data":"EnCrYpTeD"`) {
		t.Fatalf("redacted thinking: %v %s", err, resp.Message.Opaque)
	}
}

// A stream that ends before message_stop is a fault, never a partial success.
func TestStreamCutShortIsAnError(t *testing.T) {
	_, err := streamOnce(t, sse(
		`{"type":"message_start","message":{"model":"m","usage":{"input_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"half an ans"}}`))
	if err == nil || !strings.Contains(err.Error(), "before message_stop") {
		t.Fatalf("want a cut-stream error, got %v", err)
	}
}

// 429 and 529 (overloaded) are retried; the answer after them is the turn's.
func TestRetriesOverloaded(t *testing.T) {
	provider.Backoff = func(int) time.Duration { return time.Millisecond }
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch n.Add(1) {
		case 1:
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow"}}`))
		case 2:
			w.WriteHeader(529)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`))
		default:
			_, _ = w.Write([]byte(`{"model":"m","stop_reason":"end_turn","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
		}
	}))
	defer srv.Close()
	c := &Client{APIKey: "k", Model: "m", BaseURL: srv.URL, HTTP: srv.Client()}
	resp, err := c.Complete(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.User, Text: "hi"}}})
	if err != nil || resp.Message.Text != "done" || n.Load() != 3 {
		t.Fatalf("retry: %v %q after %d calls", err, resp.Message.Text, n.Load())
	}
}

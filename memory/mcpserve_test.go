package memory

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The workspace's memory over MCP: initialize answers with the tools capability, tools/list names
// the three tools, a note filed with remember is recalled at once, an unknown tool is an error
// result and an unknown method a JSON-RPC error; a notification gets no answer.
func TestServeMCPRoundTrip(t *testing.T) {
	workspace, home := fixture(t)
	t.Setenv("HOME", home)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"remember","arguments":{"text":"the payments service retries webhooks three times with backoff","tag":"payments"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"recall","arguments":{"query":"how many times are payment webhooks retried","k":3}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"forget-everything","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"resources/list"}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := ServeMCP(workspace, "agent-one", "test", strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("6 answers (the notification gets none), got %d:\n%s", len(lines), out.String())
	}
	type resp struct {
		ID     int `json:"id"`
		Result struct {
			ProtocolVersion string                  `json:"protocolVersion"`
			Capabilities    map[string]any          `json:"capabilities"`
			Tools           []struct{ Name string } `json:"tools"`
			Content         []struct{ Text string } `json:"content"`
			IsError         bool                    `json:"isError"`
		} `json:"result"`
		Error *struct{ Code int } `json:"error"`
	}
	rs := make([]resp, len(lines))
	for i, l := range lines {
		if err := json.Unmarshal([]byte(l), &rs[i]); err != nil {
			t.Fatalf("line %d: %v: %s", i, err, l)
		}
	}
	if rs[0].Result.ProtocolVersion != "2025-06-18" || rs[0].Result.Capabilities["tools"] == nil {
		t.Fatalf("initialize: %s", lines[0])
	}
	var names []string
	for _, tl := range rs[1].Result.Tools {
		names = append(names, tl.Name)
	}
	if strings.Join(names, ",") != "recall,remember,memory_status" {
		t.Fatalf("tools: %v", names)
	}
	if rs[2].Result.IsError || !strings.Contains(rs[2].Result.Content[0].Text, "REMEMBERED") {
		t.Fatalf("remember: %s", lines[2])
	}
	if rs[3].Result.IsError || !strings.Contains(rs[3].Result.Content[0].Text, "#payments") {
		t.Fatalf("the note is recalled at once: %s", lines[3])
	}
	if !rs[4].Result.IsError {
		t.Fatalf("an unknown tool is an error result: %s", lines[4])
	}
	if rs[5].Error == nil || rs[5].Error.Code != -32601 {
		t.Fatalf("an unknown method is a JSON-RPC error: %s", lines[5])
	}
}

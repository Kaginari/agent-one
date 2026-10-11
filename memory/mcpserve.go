package memory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// ServeMCP serves the workspace's memory to an MCP client over stdio — Claude Code, OpenCode, any
// client that speaks the Model Context Protocol: newline-delimited JSON-RPC 2.0 on in and out.
// Three tools, the same instrument the binary's own loop uses:
//
//	recall          — the memories that answer a question (long and shared tiers, ranked)
//	remember        — file a note in the workspace's shared memory (or the machine's)
//	memory_status   — what the workspace remembers: the tiers, the index, the holes
//
// The workspace is opened at root for every call, so a note filed by the session is recalled at
// once. Nothing here reads or writes outside the workspace's memory dir (and the machine's shared
// notes when machine is asked).
func ServeMCP(root, name, version string, in io.Reader, out io.Writer) error {
	s := &mcpServer{root: root, name: name, version: version, out: out}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<24)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		s.handle(line)
	}
	return sc.Err()
}

type mcpServer struct {
	root, name, version string
	mu                  sync.Mutex
	out                 io.Writer
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// mcpProtocol is the protocol revision answered when a client names none.
const mcpProtocol = "2025-06-18"

func (s *mcpServer) send(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.out.Write(append(b, '\n'))
}

func (s *mcpServer) reply(id json.RawMessage, result any) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *mcpServer) fail(id json.RawMessage, code int, msg string) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}

func (s *mcpServer) handle(line []byte) {
	var r rpcReq
	if err := json.Unmarshal(line, &r); err != nil {
		s.fail(json.RawMessage("null"), -32700, "parse error: "+err.Error())
		return
	}
	notification := len(r.ID) == 0
	switch r.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(r.Params, &p)
		pv := p.ProtocolVersion
		if pv == "" {
			pv = mcpProtocol
		}
		s.reply(r.ID, map[string]any{
			"protocolVersion": pv,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.name + "-memory", "version": s.version},
			"instructions":    "The " + s.name + " workspace's memory: recall before you act on something the project may already know, remember what the next session should not have to rediscover.",
		})
	case "notifications/initialized", "notifications/cancelled":
		// nothing to answer
	case "ping":
		s.reply(r.ID, map[string]any{})
	case "tools/list":
		s.reply(r.ID, map[string]any{"tools": mcpTools})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(r.Params, &p); err != nil {
			s.fail(r.ID, -32602, "invalid params: "+err.Error())
			return
		}
		text, isErr := s.call(p.Name, p.Arguments)
		s.reply(r.ID, map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "isError": isErr})
	default:
		if !notification {
			s.fail(r.ID, -32601, "method not found: "+r.Method)
		}
	}
}

var mcpTools = []map[string]any{
	{
		"name":        "recall",
		"description": "Recall what the workspace remembers about a question: log entries, docs, the policy, notes from earlier sessions — ranked by meaning, then by relation. Use it before acting on something the project may already know.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "the question, in words"},
				"k":     map[string]any{"type": "integer", "description": "how many memories (default 5)"},
				"tier":  map[string]any{"type": "string", "enum": []string{"all", "long", "shared"}, "description": "which tier to search (default all)"},
				"kind":  map[string]any{"type": "string", "description": "narrow to one kind: episodic, procedural, semantic, policy, team, ownership"},
			},
			"required": []string{"query"},
		},
	},
	{
		"name":        "remember",
		"description": "File a note in the workspace's shared memory, for every later session to recall: a decision, a trap, a fact that is nowhere on disk. One fact per note.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":    map[string]any{"type": "string", "description": "the note"},
				"tag":     map[string]any{"type": "string", "description": "a short tag to find it by"},
				"kind":    map[string]any{"type": "string", "description": "policy, team or domain — what the note is about (optional)"},
				"machine": map[string]any{"type": "boolean", "description": "file it in the machine's shared notes (every workspace on this machine) instead of this workspace's"},
			},
			"required": []string{"text"},
		},
	},
	{
		"name":        "memory_status",
		"description": "What the workspace remembers: each tier's size, the index's age, and the holes.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
}

// call runs one tool; the text is what the CLI prints for the same command.
func (s *mcpServer) call(name string, args json.RawMessage) (string, bool) {
	w, err := Open(s.root)
	if err != nil {
		return "memory: " + err.Error(), true
	}
	var a struct {
		Query   string `json:"query"`
		K       int    `json:"k"`
		Tier    string `json:"tier"`
		Kind    string `json:"kind"`
		Text    string `json:"text"`
		Tag     string `json:"tag"`
		Machine bool   `json:"machine"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return "invalid arguments: " + err.Error(), true
		}
	}
	var b bytes.Buffer
	switch name {
	case "recall":
		if strings.TrimSpace(a.Query) == "" {
			return "recall needs a query", true
		}
		r, err := w.Recall(a.Query, RecallOpts{As: "mcp", Tier: a.Tier, Kind: a.Kind, K: a.K})
		if err != nil {
			return "recall: " + err.Error(), true
		}
		printRecall(&b, r)
	case "remember":
		if strings.TrimSpace(a.Text) == "" {
			return "remember needs a text", true
		}
		r, err := w.Remember(a.Text, RememberOpts{As: "mcp", Machine: a.Machine, Tag: a.Tag, Kind: a.Kind})
		if err != nil {
			return "remember: " + err.Error(), true
		}
		js, _ := json.Marshal(r)
		fmt.Fprintf(&b, "remembered: %s", js)
	case "memory_status":
		printStatus(&b, w.Status("mcp"))
	default:
		return "no tool " + name + " — recall, remember, memory_status", true
	}
	return strings.TrimRight(b.String(), "\n"), false
}

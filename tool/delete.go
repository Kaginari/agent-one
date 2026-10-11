package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ──── delete (archives, never destroys)

// DeleteTool moves a file out of the tree into the workspace's attic:
// <root>/<workspaceDir>/attic/<YYYY-MM-DD>/<its root-relative path>, with one ledger line in
// attic/attic.jsonl naming what left and where it rests. The outcome is the workspace's own
// "old sessions" archive — an agent (or the human) restores by moving it back; a file that was
// tracked in git also keeps its git record. Records (AGENT-ONE.md, log.md, design, notes.jsonl)
// refuse outright: Policy 4 truth is append-only, not movable. Inside the workspace = Destructive
// (gate decides); outside = Outward (the human approves explicitly, or it refuses).
func DeleteTool() *Tool {
	return &Tool{
		Name: "delete",
		Description: "Archive a file out of the tree (never destroys): the file moves to <workspace>/attic/<today>/<its path>, logged in attic/attic.jsonl — anyone restores by moving it back. " +
			"Records (AGENT-ONE.md, log.md, design, notes.jsonl) refuse: append-only truth does not move.",
		Schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		// floor write: inside the workspace an archive-delete is a reversible write — free like its
		// siblings; outside rises to Outward (the human approves), records to Destructive (the
		// gate refuses) — settled per call.
		Class: Write,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct {
				Path string
			}
			_ = decode(in, &a)
			abs := env.Resolve(a.Path)
			rel := env.Rel(abs)
			if !env.Inside(abs) {
				return Classification{Class: Outward, Why: "path outside the workspace: " + abs, Paths: []string{abs}}
			}
			if env.isRecord(abs) {
				return Classification{Class: Destructive, Why: "archive of a record " + rel + " — Policy 4 truth does not move; the gate refuses", Paths: []string{abs}}
			}
			return Classification{Class: Destructive, Why: "archives " + rel + " to the attic (restorable)", Paths: []string{abs}}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Path string
			}
			if err := decode(in, &a); err != nil || a.Path == "" {
				return fail("delete: path is required")
			}
			abs := env.Resolve(a.Path)
			if !env.Inside(abs) {
				return fail("delete: %s is outside the workspace — outward needs the human's word, not a tool agent's", abs)
			}
			if env.isRecord(abs) {
				return fail("delete: %s is a record (Policy 4, append-only truth) — it does not move; append the correction instead", env.Rel(abs))
			}
			st, err := os.Stat(abs)
			if err != nil {
				return fail("delete: %v", err)
			}
			if st.IsDir() {
				return fail("delete: %s is a directory — the attic takes files, not trees (archive its members one at a time if the workspace must)", env.Rel(abs))
			}
			rel := env.Rel(abs)
			today := time.Now().UTC().Format("2006-01-02")
			dst := filepath.Join(env.Root, env.workspaceDir(), "attic", today, rel)
			dst = filepath.Clean(dst)
			if abs == dst {
				return fail("delete: %s already rests in the attic", rel)
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return fail("delete: %v", err)
			}
			if err := os.Rename(abs, dst); err != nil {
				return fail("delete: %v", err)
			}
			ledger := filepath.Join(env.Root, env.workspaceDir(), "attic", "attic.jsonl")
			entry, _ := json.Marshal(map[string]any{
				"at": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), "path": rel, "attic": env.Rel(dst),
			})
			if f, err := os.OpenFile(ledger, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
				defer f.Close()
				_, _ = f.Write(append(entry, '\n'))
			}
			out := fmt.Sprintf("archived %s → %s (ledger attic/attic.jsonl; restore by moving it back)", rel, env.Rel(dst))
			if z := zoneOwner(env, rel); z != "" {
				out += " · was zone of " + z
			}
			return Result{Output: out, Wrote: []string{dst}}
		},
	}
}

// zoneOwner names the member that authors the zone a relative path falls under ("" when no
// member claims it or no Owner is wired). The kit's write/edit/delete results quote it so
// every landing names its soil's persistent agent.
func zoneOwner(env Env, rel string) string {
	if env.ZoneOwner == nil {
		return ""
	}
	return env.ZoneOwner(rel)
}

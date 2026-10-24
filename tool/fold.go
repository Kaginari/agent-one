package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The fold shelf: a tool result the replay folds off the wire keeps its bytes here, named by
// their digest, so the caption it leaves can point at the exact payload instead of asking for
// a re-run. Each run keeps its own drawer (<shelf>/<run id>/): a fold is that run's raw output,
// and the fold tool restores only the acting run's — never another agent's. Content-addressed
// within a drawer: a kept file is never rewritten. FoldSweep drops drawers left untouched past
// the keep window (tools.output.foldKeepDays).

// FoldDir is the shelf's directory: <root>/<workspaceDir>/tmp/fold.
func FoldDir(root, workspaceDir string) string {
	if workspaceDir == "" {
		workspaceDir = DefaultWorkspaceDir
	}
	return filepath.Join(root, workspaceDir, "tmp", "fold")
}

// FoldRunDir is one run's drawer on the shelf: <shelf>/<run id>. A run id never walks off the
// shelf; "" (no run) has no drawer.
func FoldRunDir(root, workspaceDir, runID string) string {
	id := strings.TrimSpace(runID)
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return ""
	}
	return filepath.Join(FoldDir(root, workspaceDir), id)
}

// FoldSweep removes what the shelf kept longer than keep: every drawer (and any loose file a
// workspace-wide shelf left) last touched before now-keep. keep <= 0 sweeps nothing.
func FoldSweep(shelf string, keep time.Duration, now time.Time) (removed int) {
	if keep <= 0 {
		return 0
	}
	ents, err := os.ReadDir(shelf)
	if err != nil {
		return 0
	}
	for _, e := range ents {
		fi, err := e.Info()
		if err != nil || now.Sub(fi.ModTime()) < keep {
			continue
		}
		if os.RemoveAll(filepath.Join(shelf, e.Name())) == nil {
			removed++
		}
	}
	return removed
}

// FoldDigest names a payload by its content (sha256, the first 16 hex digits).
func FoldDigest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:8])
}

// FoldKeep shelves a payload under its digest and returns the digest and the kept path. A
// payload already kept is left alone; a new one lands whole or not at all (write, then rename).
func FoldKeep(dir, content string) (digest, path string, err error) {
	digest = FoldDigest(content)
	path = filepath.Join(dir, digest+".txt")
	if _, err := os.Stat(path); err == nil {
		return digest, path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	f, err := os.CreateTemp(dir, ".keep-*")
	if err != nil {
		return "", "", err
	}
	_, err = f.WriteString(content)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		os.Remove(f.Name())
		return "", "", err
	}
	return digest, path, nil
}

// FoldTool restores a folded payload: by its digest, or by the kept-at path its caption quoted.
// It reads only the acting run's drawer (env.RunID), and only a file whose bytes still hash to
// its name — what comes back is exactly what that run folded.
func FoldTool() *Tool {
	return &Tool{
		Name:        "fold",
		Description: "Restore a tool output the replay folded away, byte for byte: `digest` (or the `path` its caption says it is kept at).",
		Schema:      json.RawMessage(`{"type":"object","properties":{"digest":{"type":"string"},"path":{"type":"string"}},"anyOf":[{"required":["digest"]},{"required":["path"]}]}`),
		Class:       Read,
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct{ Digest, Path string }
			if err := decode(in, &a); err != nil || (a.Digest == "" && a.Path == "") {
				return fail("fold: digest or path is required")
			}
			dir := FoldRunDir(env.Root, env.WorkspaceDir, env.RunID)
			if dir == "" {
				return fail("fold: no run is acting — a fold is restored only by the run that folded it")
			}
			name := strings.TrimSuffix(strings.TrimSpace(a.Digest), ".txt")
			if a.Path != "" {
				p := env.Resolve(a.Path)
				if filepath.Dir(p) != dir {
					return fail("fold: %s is not on this run's fold shelf (%s) — a run restores only its own folds", a.Path, env.Rel(dir))
				}
				name = strings.TrimSuffix(filepath.Base(p), ".txt")
			}
			data, err := os.ReadFile(filepath.Join(dir, filepath.Base(name)+".txt"))
			if err != nil {
				return fail("fold: no payload %s is kept on this run's shelf (%s) — a run restores only its own folds", name, env.Rel(dir))
			}
			if FoldDigest(string(data)) != name {
				return fail("fold: the payload kept as %s no longer hashes to its name — not restored", name)
			}
			return Result{Output: string(data)}
		},
	}
}

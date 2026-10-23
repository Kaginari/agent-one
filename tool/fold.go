package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// The fold shelf: a tool result the replay folds off the wire keeps its bytes here, named by
// their digest, so the caption it leaves can point at the exact payload instead of asking for
// a re-run. Content-addressed: the same payload folded by two sessions keeps once, a kept file
// is never rewritten, and nothing here is pruned — a fold never loses bytes.

// FoldDir is the shelf's directory: <root>/<workspaceDir>/tmp/fold.
func FoldDir(root, workspaceDir string) string {
	if workspaceDir == "" {
		workspaceDir = DefaultWorkspaceDir
	}
	return filepath.Join(root, workspaceDir, "tmp", "fold")
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
// It reads only the shelf, and only a file whose bytes still hash to its name — what comes back
// is exactly what was folded.
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
			dir := FoldDir(env.Root, env.WorkspaceDir)
			name := strings.TrimSuffix(strings.TrimSpace(a.Digest), ".txt")
			if a.Path != "" {
				p := env.Resolve(a.Path)
				if filepath.Dir(p) != dir {
					return fail("fold: %s is not on the fold shelf (%s)", a.Path, env.Rel(dir))
				}
				name = strings.TrimSuffix(filepath.Base(p), ".txt")
			}
			data, err := os.ReadFile(filepath.Join(dir, filepath.Base(name)+".txt"))
			if err != nil {
				return fail("fold: no payload %s is kept on the shelf (%s)", name, env.Rel(dir))
			}
			if FoldDigest(string(data)) != name {
				return fail("fold: the payload kept as %s no longer hashes to its name — not restored", name)
			}
			return Result{Output: string(data)}
		},
	}
}

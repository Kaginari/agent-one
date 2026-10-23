package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
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

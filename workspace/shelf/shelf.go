// Package shelf appends to the harness's own line files — the work queue, the ledger shelf, the
// value ledger: one JSON row a line, appended, never rewritten (Policy 4) — and keeps them the
// harness's.
//
// An agent's write onto these files is refused at the ownership gate when the classifier can read
// it; it cannot read every program (a script, an interpreter, a path built in a variable). The
// seal is the backstop: the files are written only by Append, and beside each Append keeps
// `<file>.seal` — the size and mtime the harness left the file at and the running SHA-256 of
// every byte it wrote. Check reads a file against its seal (a stat, and the hash only when the
// stat moved); what the harness did not write is set aside in `<dir>/attic/`, never deleted, so
// no fold reads it. One lock a dir (`<dir>/.lock`, flock) serializes every process's Append and
// Check, so the human's q command and a second session seal their own lines too.
package shelf

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Append writes one line to the file at path (created with its dirs, perm on creation) in a
// single O_APPEND write: concurrent writers interleave lines, never bytes of lines. A crash may
// have left half a line at the end: the new line then starts on a line of its own, in the same
// write, so the torn bytes stay a skipped line and swallow nothing. What the harness did not
// write is set aside first (Check), so the seal Append leaves never covers it.
func Append(path string, line []byte, perm os.FileMode) error {
	unlock, err := lock(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := check(path); err != nil {
		return err
	}
	s, _ := readSeal(path) // the file stands as sealed now (a missing file: the empty seal)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, perm)
	if err != nil {
		return err
	}
	b := make([]byte, 0, len(line)+2)
	if st, serr := f.Stat(); serr == nil && st.Size() > 0 {
		last := make([]byte, 1)
		if _, rerr := f.ReadAt(last, st.Size()-1); rerr == nil && last[0] != '\n' {
			b = append(b, '\n')
		}
	}
	b = append(append(b, line...), '\n')
	_, werr := f.Write(b)
	st, serr := f.Stat()
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return werr
	}
	h := hashOf(s.Hash)
	h.Write(b)
	next := seal{Size: s.Size + int64(len(b)), Hash: stateOf(h)}
	if serr == nil && st.Size() == next.Size {
		next.Mtime = st.ModTime().UnixNano() // else a hand wrote beside ours: the next Check reads it
	}
	return writeSeal(path, next)
}

// Check reads the file at path against its seal and sets aside what the harness did not write,
// naming it ("" when the file stands as the harness left it): bytes after the sealed end move to
// attic/<file>; a file whose sealed bytes changed moves whole to attic/<file>.<unix nanos> and an
// empty sealed file takes its place. A file with no seal yet (a workspace older than the seal, a file
// moved onto the shelf) is sealed as it stands.
func Check(path string) (string, error) {
	s, sealed := readSeal(path)
	st, err := os.Stat(path)
	switch {
	case sealed && err == nil && st.Size() == s.Size && st.ModTime().UnixNano() == s.Mtime:
		return "", nil
	case !sealed && errors.Is(err, os.ErrNotExist):
		return "", nil
	}
	unlock, err := lock(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	defer unlock()
	return check(path)
}

// check is Check under the dir's lock.
func check(path string) (string, error) {
	s, sealed := readSeal(path)
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		if !sealed || s.Size == 0 {
			return "", nil
		}
		return fmt.Sprintf("%s was removed by a hand other than the harness's", name(path)), os.Remove(sealPath(path))
	}
	if err != nil {
		return "", err
	}
	if !sealed {
		return "", adopt(path)
	}
	if st.Size() == s.Size && st.ModTime().UnixNano() == s.Mtime {
		return "", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	n, _ := io.CopyN(h, f, s.Size)
	if n == s.Size && bytes.Equal(h.Sum(nil), hashOf(s.Hash).Sum(nil)) {
		if st.Size() == s.Size {
			s.Mtime = st.ModTime().UnixNano() // touched, not written
			return "", writeSeal(path, s)
		}
		tail, err := io.ReadAll(f)
		if err != nil {
			return "", err
		}
		attic := filepath.Join(filepath.Dir(path), "attic", filepath.Base(path))
		if err := setAside(attic, tail); err != nil {
			return "", err
		}
		if err := os.Truncate(path, s.Size); err != nil {
			return "", err
		}
		if st, err := os.Stat(path); err == nil {
			s.Mtime = st.ModTime().UnixNano()
		}
		return fmt.Sprintf("%d bytes on %s the harness did not write — set aside in %s", len(tail), name(path), name(attic)), writeSeal(path, s)
	}
	// the sealed bytes themselves changed: no line of the file can be trusted to the fold
	f.Close()
	aside := filepath.Join(filepath.Dir(path), "attic", fmt.Sprintf("%s.%d", filepath.Base(path), time.Now().UnixNano()))
	if err := os.MkdirAll(filepath.Dir(aside), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(path, aside); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, nil, st.Mode().Perm()); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s was rewritten by a hand other than the harness's — the whole file set aside in %s, an empty one in its place", name(path), name(aside)), adopt(path)
}

// setAside appends foreign bytes to the attic file under a line naming when they were found.
func setAside(attic string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(attic), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(attic, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	out := append([]byte(fmt.Sprintf("# %s: %d bytes the harness did not write\n", time.Now().UTC().Format(time.RFC3339), len(b))), b...)
	if len(b) > 0 && b[len(b)-1] != '\n' {
		out = append(out, '\n')
	}
	_, werr := f.Write(out)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

// seal is a file as the harness left it.
type seal struct {
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
	Hash  []byte `json:"hash"` // the SHA-256 state after Size bytes (its MarshalBinary)
}

func sealPath(path string) string { return path + ".seal" }

// name is a file as a finding names it: from the shelf's dir down (shared/queue.jsonl,
// shared/attic/queue.jsonl).
func name(path string) string {
	dir := filepath.Dir(path)
	if filepath.Base(dir) == "attic" {
		dir = filepath.Dir(dir)
	}
	rel, _ := filepath.Rel(filepath.Dir(dir), path)
	return filepath.ToSlash(rel)
}

func readSeal(path string) (seal, bool) {
	var s seal
	b, err := os.ReadFile(sealPath(path))
	if err != nil || json.Unmarshal(b, &s) != nil || hashOf(s.Hash) == nil {
		return seal{}, false
	}
	return s, true
}

func writeSeal(path string, s seal) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := sealPath(path) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, sealPath(path))
}

// adopt seals the file as it stands.
func adopt(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return err
	}
	s := seal{Size: n, Hash: stateOf(h)}
	if st, err := f.Stat(); err == nil && st.Size() == n {
		s.Mtime = st.ModTime().UnixNano()
	}
	return writeSeal(path, s)
}

// hashOf resumes a SHA-256 from its saved state (a fresh one for none); nil when the state is
// not one.
func hashOf(state []byte) hash.Hash {
	h := sha256.New()
	if len(state) > 0 {
		if err := h.(encoding.BinaryUnmarshaler).UnmarshalBinary(state); err != nil {
			return nil
		}
	}
	return h
}

func stateOf(h hash.Hash) []byte {
	b, _ := h.(encoding.BinaryMarshaler).MarshalBinary()
	return b
}

// lock takes the dir's lock (made with the dir), held across processes until the answer is
// called.
func lock(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

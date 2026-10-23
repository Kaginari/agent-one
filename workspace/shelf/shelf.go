// Package shelf appends to the harness's own line files — the work queue, the ledger shelf, the
// value ledger: one JSON row a line, appended, never rewritten (Policy 4).
package shelf

import (
	"os"
	"path/filepath"
)

// Append writes one line to the file at path (created with its dirs, perm on creation) in a
// single O_APPEND write: concurrent writers interleave lines, never bytes of lines. A crash may
// have left half a line at the end: the new line then starts on a line of its own, in the same
// write, so the torn bytes stay a skipped line and swallow nothing.
func Append(path string, line []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
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
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

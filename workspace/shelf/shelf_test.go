package shelf

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAppendStartsAfterATornTail: a line appended after a torn tail starts on a line of its
// own; the torn bytes stay as they were and history is never rewritten.
func TestAppendStartsAfterATornTail(t *testing.T) {
	p := filepath.Join(t.TempDir(), "shared", "x.jsonl")
	if err := Append(p, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0o644)
	f.WriteString(`{"a":`)
	f.Close()
	if err := Append(p, []byte(`{"a":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "{\"a\":1}\n{\"a\":\n{\"a\":2}\n" {
		t.Fatalf("%q", b)
	}
}

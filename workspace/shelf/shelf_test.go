package shelf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAppendStartsAfterATornTail: a line appended after a torn tail the seal adopted starts on
// a line of its own; the torn bytes stay as they were and history is never rewritten.
func TestAppendStartsAfterATornTail(t *testing.T) {
	p := filepath.Join(t.TempDir(), "shared", "x.jsonl")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("{\"a\":1}\n{\"a\":"), 0o644) // a file from before the seal
	if err := Append(p, []byte(`{"a":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "{\"a\":1}\n{\"a\":\n{\"a\":2}\n" {
		t.Fatalf("%q", b)
	}
}

// TestCheckSetsAsideForeignBytes: bytes after the harness's last line are moved to the attic
// (kept whole, never deleted) and the file stands as the harness left it; a touch is no finding;
// an Append after foreign bytes sets them aside first, so its seal never covers them.
func TestCheckSetsAsideForeignBytes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "shared", "q.jsonl")
	Append(p, []byte(`{"a":1}`), 0o644)
	if found, err := Check(p); found != "" || err != nil {
		t.Fatalf("the harness's own line is no finding: %q %v", found, err)
	}
	later := time.Now().Add(time.Minute)
	os.Chtimes(p, later, later)
	if found, _ := Check(p); found != "" {
		t.Fatalf("a touch is no finding: %q", found)
	}
	f, _ := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0o644)
	f.WriteString(`{"forged":1}` + "\n")
	f.Close()
	found, err := Check(p)
	if err != nil || !strings.Contains(found, "13 bytes on shared/q.jsonl") || !strings.Contains(found, "shared/attic/q.jsonl") {
		t.Fatalf("the foreign bytes are named: %q %v", found, err)
	}
	if b, _ := os.ReadFile(p); string(b) != "{\"a\":1}\n" {
		t.Fatalf("the file stands as the harness left it: %q", b)
	}
	f, _ = os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0o644)
	f.WriteString(`{"forged":2}`)
	f.Close()
	Append(p, []byte(`{"a":2}`), 0o644)
	if b, _ := os.ReadFile(p); string(b) != "{\"a\":1}\n{\"a\":2}\n" {
		t.Fatalf("an append sets foreign bytes aside before it writes: %q", b)
	}
	if found, _ := Check(p); found != "" {
		t.Fatalf("the harness's lines after the attic: %q", found)
	}
	attic, _ := os.ReadFile(filepath.Join(filepath.Dir(p), "attic", "q.jsonl"))
	if !strings.Contains(string(attic), `{"forged":1}`+"\n") || !strings.Contains(string(attic), `{"forged":2}`+"\n") {
		t.Fatalf("both forgeries are kept: %q", attic)
	}
}

// TestCheckRewriteSetsTheFileAside: a file whose sealed bytes changed cannot be trusted line by
// line — it is moved whole to the attic and an empty sealed file takes its place; a file with no
// seal yet is sealed as it stands, and a sealed file removed by another hand is named.
func TestCheckRewriteSetsTheFileAside(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shared")
	p := filepath.Join(dir, "q.jsonl")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(p, []byte("{\"old\":1}\n"), 0o644)
	if found, err := Check(p); found != "" || err != nil {
		t.Fatalf("an unsealed file is adopted: %q %v", found, err)
	}
	Append(p, []byte(`{"a":1}`), 0o644)
	os.WriteFile(p, []byte("{\"old\":1}\n{\"a\":9}\n"), 0o644)
	later := time.Now().Add(time.Minute) // a later write: the stat moved
	os.Chtimes(p, later, later)
	found, _ := Check(p)
	if !strings.Contains(found, "rewritten") {
		t.Fatalf("a rewrite is named: %q", found)
	}
	if b, err := os.ReadFile(p); err != nil || len(b) != 0 {
		t.Fatalf("an empty file stands in its place: %q %v", b, err)
	}
	aside, _ := filepath.Glob(filepath.Join(dir, "attic", "q.jsonl.*"))
	if len(aside) != 1 {
		t.Fatalf("the whole file is kept: %v", aside)
	}
	if b, _ := os.ReadFile(aside[0]); string(b) != "{\"old\":1}\n{\"a\":9}\n" {
		t.Fatalf("kept whole: %q", b)
	}
	Append(p, []byte(`{"a":2}`), 0o644)
	os.Remove(p)
	if found, _ := Check(p); !strings.Contains(found, "removed") {
		t.Fatalf("a sealed file removed by another hand is named: %q", found)
	}
}

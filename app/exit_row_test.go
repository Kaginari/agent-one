package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/loop"
)

// the veil-exit row: when the hand reaches for stop between turns, the journal's exit row names
// the lexicon's word for the human.
func TestLogExit(t *testing.T) {
	a := testTmpApp(t, t.TempDir())
	a.Lex.Human = "operator"
	dir := t.TempDir()
	jpath := filepath.Join(dir, "x.jsonl")
	s := &loop.Session{Started: time.Now().Add(-time.Minute), Journal: &loop.Journal{Path: jpath}}
	a.logExit(s)
	data, err := os.ReadFile(jpath)
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]interface{}
	for _, l := range splitLines(string(data)) {
		if json.Unmarshal([]byte(l), &row) == nil && row["t"] == "exit" {
			break
		}
		row = nil
	}
	if row == nil {
		t.Fatal("no exit row in journal: ", string(data))
	}
	if row["cause"] != "operator" {
		t.Error("the exit row must name the human's word: ", row)
	}
	if _, ok := row["ms"]; !ok {
		t.Error("elapsed rides along: ", row)
	}
	// a lexicon-less workspace still names the hand honestly
	dir2 := t.TempDir()
	jpath2 := filepath.Join(dir2, "y.jsonl")
	a.Lex.Human = ""
	a.logExit(&loop.Session{Started: time.Now(), Journal: &loop.Journal{Path: jpath2}})
	data2, _ := os.ReadFile(jpath2)
	if !strings.Contains(string(data2), `"cause":"the human"`) {
		t.Error("lexicon-less workspace falls back to 'the human': ", string(data2))
	}
	// nil session / nil journal ride free (the harness can die anywhere)
	a.logExit(nil)
	a.logExit(&loop.Session{})
}

func splitLines(s string) []string {
	out := []string{}
	cur := ""
	for _, c := range s {
		if c == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

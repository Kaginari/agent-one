package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestScrubDryVsApply: the cure deck must find the marker stains in all three surface kinds
// (log = episodic record, notes = shared record, short cache = free), count them once per
// family, drop only cache hulls, write backups when applying, and reseed the index — and a
// second dry-run after apply must find nothing.
func TestScrubDryVsApply(t *testing.T) {
	workspace, home := fixture(t)
	w, err := Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	w.Home = home

	// stains: one in the episode stream, one on the shared bus, two in the cache
	log := filepath.Join(workspace, ".agent-one", "log.md")
	rawLog, _ := os.ReadFile(log)
	stainedLog := string(rawLog) + "\n### [2026-10-03] orchestrator — relayed the report\n- **Maintenance:** ⚙️ relayed the commission down.\n"
	if err := os.WriteFile(log, []byte(stainedLog), 0o644); err != nil {
		t.Fatal(err)
	}
	sharedDir := filepath.Join(workspace, ".agent-one", "memory", "shared")
	os.MkdirAll(sharedDir, 0o755)
	os.WriteFile(filepath.Join(sharedDir, "notes.jsonl"), []byte(`{"at":"2026-10-03T00:00:00.000Z","by":"orchestrator","text":"**Evolution:** 🚀 landed the pairing"}`+"\n"), 0o644)
	shortDir := filepath.Join(workspace, ".agent-one", "memory", "short")
	os.MkdirAll(shortDir, 0o755)
	cache := `{"id":"keep","as":"orchestrator","q":"relay **Maintenance:** ⚙️ the commission","results":[{"src":".agent-one/AGENT-ONE.md"}]}` + "\n" +
		`{"id":"drop","as":"orchestrator","q":"⚙️ ","results":[]}` + "\n"
	cachePath := filepath.Join(shortDir, "orchestrator.jsonl")
	os.WriteFile(cachePath, []byte(cache), 0o644)

	beforeLog, _ := os.ReadFile(log)
	beforeCache, _ := os.ReadFile(cachePath)

	// dry-run measures, touches nothing
	dry, err := w.Scrub(false)
	if err != nil {
		t.Fatal(err)
	}
	if dry.Applied || dry.Hits != 4 || dry.Dropped != 1 {
		t.Fatalf("dry: applied=%v hits=%d dropped=%d want false/4/1 — %+v", dry.Applied, dry.Hits, dry.Dropped, dry)
	}
	if a, _ := os.ReadFile(log); strings.EqualFold(string(a), "") || !strings.EqualFold(string(a), string(beforeLog)) {
		t.Fatal("dry-run wrote the log")
	}
	if a, _ := os.ReadFile(cachePath); string(a) != string(beforeCache) {
		t.Fatal("dry-run wrote the cache")
	}
	if len(dry.Families) != 3 {
		names := []string{}
		for _, f := range dry.Families {
			names = append(names, f.Name)
		}
		t.Fatalf("families = %v, want [evolution-marker maintenance-marker vs16]", names)
	}

	// apply writes through backups and reseeds the index
	applied, err := w.Scrub(true)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || applied.Hits != 4 || applied.Dropped != 1 || applied.Indexed == 0 {
		t.Fatalf("applied: %+v", applied)
	}
	afterLog, _ := os.ReadFile(log)
	if strings.Contains(string(afterLog), "**Maintenance:**") || strings.Contains(string(afterLog), "️") {
		t.Fatalf("log still stained: %q", afterLog)
	}
	if !strings.Contains(string(afterLog), "Maintenance:  relayed") && !strings.Contains(string(afterLog), "Maintenance: relayed") {
		t.Fatalf("log lost the sentence with the marker: %q", afterLog)
	}
	afterCache, _ := os.ReadFile(cachePath)
	if strings.Contains(string(afterCache), "drop") {
		t.Fatalf("empty-hull cache entry survived: %q", afterCache)
	}
	if !strings.Contains(string(afterCache), `"id":"keep"`) || strings.Contains(string(afterCache), "️") {
		t.Fatalf("kept entry wrong: %q", afterCache)
	}
	if strings.Contains(string(afterCache), "**Maintenance:**") {
		t.Fatalf("kept entry still stained: %q", afterCache)
	}
	seen := 0
	for _, f := range applied.Files {
		if f.Backup == "" {
			t.Fatalf("applied file without backup: %+v", f)
		}
		if _, err := os.Stat(strings.Replace(f.Backup, "~", home, 1)); err == nil {
			seen++
		}
	}
	if seen != len(applied.Files) {
		t.Fatalf("backups on disk: %d of %d files", seen, len(applied.Files))
	}

	// idempotent: a second dry-run finds a clean memory
	again, err := w.Scrub(false)
	if err != nil {
		t.Fatal(err)
	}
	if again.Hits != 0 || len(again.Families) != 0 {
		t.Fatalf("second dry-run: %+v", again)
	}
}

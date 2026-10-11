package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRecallNoRebuildBargainNagsOnce covers the degraded-marker policy (freeze review
// 2026-10-07): a workspace that declines on-path index rebuilds (the 2026-10-01 stall cure)
// must not repeat the raw drift text as a recall hole every turn — that echo taught
// agents to ignore the holes channel. The drift surfaces once, as a degraded marker.
func TestRecallNoRebuildBargainNagsOnce(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	opts := RecallOptions{Enabled: true, Memory: true, K: 5}          // RebuildOnStale: false — the bargain
	rc := w.Recall("zone-auth", "tokens expire after one hour", opts) // lands the index
	if !rc.Rebuilt {
		t.Fatal("the first recall must build the missing index — that is the policy, not the bargain")
	}
	// drift: a gated turn appended to the chronicle (the everyday stall source)
	logPath := filepath.Join(dir, ".agent-one/log.md")
	b, _ := os.ReadFile(logPath)
	if err := os.WriteFile(logPath, append(b, []byte("\n\n---\n\nmore\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // the stamp resolution must SEE the drift
	rc = w.Recall("zone-auth", "tokens expire after one hour", opts)
	if rc.Rebuilt {
		t.Fatal("the bargain forbids rebuilding on the recall path")
	}
	for _, h := range rc.Holes {
		if strings.Contains(h, "index older than its sources") {
			t.Fatalf("the raw drift echo survived:: %v", rc.Holes)
		}
	}
	var degraded int
	for _, h := range rc.Holes {
		if strings.HasPrefix(h, "degraded: memory recall rides") {
			degraded++
		}
	}
	if degraded != 1 {
		t.Fatalf("exactly one degraded marker per recall; got %d in %v", degraded, rc.Holes)
	}
	// and a workspace that kept rebuilds (the default) still carries the honest raw text
	rc = w.Recall("zone-auth", "tokens expire after one hour", RecallOptions{Enabled: true, Memory: true, K: 5, RebuildOnStale: true})
	if rc.Rebuilt != true {
		t.Fatal("with rebuilds on, drift heals on the spot")
	}
}

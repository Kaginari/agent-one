package queue

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func store(t *testing.T) *Store {
	t.Helper()
	return Open(filepath.Join(t.TempDir(), "shared", "queue.jsonl"), nil)
}

// TestQueueLifecycle: add holds the work, claim hands it to the agent it was ordered to, touch
// and requeue move only what that agent holds, close ends it — and every move is one line.
func TestQueueLifecycle(t *testing.T) {
	s := store(t)
	it, err := s.Add(Item{For: "zone-auth", From: "domain-x", Ask: "fix the token check"}, "domain-x")
	if err != nil || it.ID != "q1" || it.State != Held || it.Rev != 1 {
		t.Fatalf("add: %+v %v", it, err)
	}
	if _, err := s.Claim("q1", "zone-db"); !IsRefusal(err) {
		t.Fatalf("an agent the work is not ordered to claimed it: %v", err)
	}
	if it, err = s.Claim("q1", "zone-auth"); err != nil || it.State != Claimed || it.Owner != "zone-auth" {
		t.Fatalf("claim: %+v %v", it, err)
	}
	if _, err := s.Claim("q1", "zone-auth"); !IsRefusal(err) {
		t.Fatal("claimed work claimed twice")
	}
	if _, err := s.Touch("q1", "zone-db", "mine now"); !IsRefusal(err) {
		t.Fatal("an agent touched work another holds")
	}
	if it, err = s.Touch("q1", "zone-auth", "halfway"); err != nil || it.Note != "halfway" || it.State != Claimed {
		t.Fatalf("touch: %+v %v", it, err)
	}
	if it, err = s.Requeue("q1", "zone-auth", "blocked on the schema"); err != nil || it.State != Held || it.Owner != "" {
		t.Fatalf("requeue: %+v %v", it, err)
	}
	if _, err := s.Claim("q1", "zone-auth"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Close("q1", "zone-auth", Claimed, ""); !IsRefusal(err) {
		t.Fatal("close took a non-terminal state")
	}
	if it, err = s.Close("q1", "zone-auth", Done, "landed"); err != nil || it.State != Done || it.Rev != 6 {
		t.Fatalf("close: %+v %v", it, err)
	}
	for _, mv := range []func() error{
		func() error { _, e := s.Claim("q1", "zone-auth"); return e },
		func() error { _, e := s.Requeue("q1", "zone-auth", ""); return e },
		func() error { _, e := s.Close("q1", "zone-auth", Dropped, ""); return e },
	} {
		if err := mv(); !IsRefusal(err) {
			t.Fatalf("a terminal item moved: %v", err)
		}
	}
	_, hist, err := s.Get("q1")
	if err != nil || len(hist) != 6 {
		t.Fatalf("history: %d %v", len(hist), err)
	}
	if _, _, err := s.Get("q9"); !IsRefusal(err) {
		t.Fatal("an unknown item read")
	}
}

// TestQueueMayLetsARankAbove: the holder rule bends only where May says — a rank above the
// holder moves its work; a stranger stays refused.
func TestQueueMayLetsARankAbove(t *testing.T) {
	s := store(t)
	s.May = func(by, owner string) bool { return by == "domain-x" && (owner == "zone-auth" || owner == "") }
	s.Add(Item{For: "zone-auth", Ask: "a"}, "human")
	if _, err := s.Claim("q1", "domain-x"); err != nil {
		t.Fatalf("the rank above could not claim for its zone worker: %v", err)
	}
	if _, err := s.Requeue("q1", "domain-x", "reroute"); err != nil {
		t.Fatalf("the rank above could not requeue: %v", err)
	}
	if _, err := s.Close("q1", "zone-db", Dropped, ""); !IsRefusal(err) {
		t.Fatal("a stranger closed held work")
	}
	if _, err := s.Close("q1", "domain-x", Dropped, "not needed"); err != nil {
		t.Fatalf("the rank above could not drop held work: %v", err)
	}
	s.Add(Item{From: "coord-y", Ask: "b"}, "coord-y")
	if _, err := s.Close("q2", "coord-y", Dropped, ""); err != nil {
		t.Fatalf("whoever queued held work could not drop it: %v", err)
	}
}

// TestQueueAppendOnly: Policy 4 — every move appends one line and leaves the bytes before it
// untouched; the queue as it stands is the fold.
func TestQueueAppendOnly(t *testing.T) {
	s := store(t)
	var prev []byte
	moves := []func() error{
		func() error { _, e := s.Add(Item{Ask: "one"}, "human"); return e },
		func() error { _, e := s.Claim("q1", "human"); return e },
		func() error { _, e := s.Touch("q1", "human", "n"); return e },
		func() error { _, e := s.Close("q1", "human", Done, ""); return e },
	}
	for i, mv := range moves {
		if err := mv(); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(s.Path())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(b, prev) {
			t.Fatalf("move %d rewrote history", i)
		}
		if n := bytes.Count(b, []byte("\n")); n != i+1 {
			t.Fatalf("move %d: %d lines", i, n)
		}
		prev = b
	}
}

// TestQueueRankLosesNeverDoubles: many hands claim one item at once through their own stores;
// the fold seats exactly one, every other hand reads a refusal.
func TestQueueRankLosesNeverDoubles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.jsonl")
	if _, err := Open(path, nil).Add(Item{Ask: "contested"}, "human"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	won, refused := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := Open(path, nil).Claim("q1", "agent-"+string(rune('a'+i)))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				won++
			case IsRefusal(err):
				refused++
			default:
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if won != 1 || refused != 7 {
		t.Fatalf("won %d refused %d", won, refused)
	}
	// a stale line appended by hand (a racer that lost) never folds
	stale, _ := json.Marshal(Item{ID: "q1", Rev: 2, Op: OpClaim, State: Claimed, Owner: "ghost", By: "ghost"})
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.Write(append(stale, '\n'))
	f.Close()
	it, _, _ := Open(path, nil).Get("q1")
	if it.Owner == "ghost" {
		t.Fatal("a stale revision folded")
	}
}

// TestQueueRanked: claimed work reads first, then held work by rank (P0 first) and age, then
// closed work.
func TestQueueRanked(t *testing.T) {
	s := store(t)
	clock := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { clock = clock.Add(time.Second); return clock }
	s.Add(Item{Ask: "late p1", Rank: 1}, "h")
	s.Add(Item{Ask: "p0", Rank: 0}, "h")
	s.Add(Item{Ask: "later p1", Rank: 1}, "h")
	s.Add(Item{Ask: "claimed p2", Rank: 2}, "h")
	s.Add(Item{Ask: "closed", Rank: 0}, "h")
	s.Claim("q4", "h")
	s.Close("q5", "h", Dropped, "")
	items, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.ID)
	}
	if strings.Join(got, ",") != "q4,q2,q1,q3,q5" {
		t.Fatalf("order %v", got)
	}
	held, _ := s.Held()
	if len(held) != 3 || held[0].ID != "q2" {
		t.Fatalf("held %+v", held)
	}
}

// TestQueueCLI: reads and moves exit 0, a refusal exits 2 with the reason on stderr, --json
// prints the items.
func TestQueueCLI(t *testing.T) {
	s := store(t)
	run := func(asJSON bool, args ...string) (int, string, string) {
		var out, errw bytes.Buffer
		code := CLI(s, "operator", args, asJSON, &out, &errw)
		return code, out.String(), errw.String()
	}
	if code, out, _ := run(false, "add", "--for", "Zone-Auth", "--rank", "0", "fix", "the", "check"); code != 0 || !strings.Contains(out, "queued q1") {
		t.Fatalf("add: %d %q", code, out)
	}
	if code, _, errw := run(false, "claim", "q1"); code != 2 || !strings.Contains(errw, "ordered to zone-auth") {
		t.Fatalf("a refusal: %d %q", code, errw)
	}
	if code, out, _ := run(false, "claim", "q1", "--as", "zone-auth"); code != 0 || !strings.Contains(out, "held by zone-auth") {
		t.Fatalf("claim --as: %d %q", code, out)
	}
	code, out, _ := run(true, "list")
	var items []Item
	if code != 0 || json.Unmarshal([]byte(out), &items) != nil || len(items) != 1 || items[0].State != Claimed {
		t.Fatalf("list --json: %d %q", code, out)
	}
	if code, out, _ := run(false, "close", "q1", "--as", "zone-auth", "--drop", "not", "needed"); code != 0 || !strings.Contains(out, "dropped q1") {
		t.Fatalf("close --drop: %d %q", code, out)
	}
	if code, out, _ := run(false, "list"); code != 0 || strings.Contains(out, "q1 ") {
		t.Fatalf("list hides closed work: %q", out)
	}
	if code, out, _ := run(false, "show", "q1"); code != 0 || !strings.Contains(out, "not needed") || !strings.Contains(out, "r3") {
		t.Fatalf("show: %q", out)
	}
	if code, _, _ := run(false, "bogus"); code != 2 {
		t.Fatal("an unknown q command passed")
	}
}

// TestQueueTornTailSwallowsNothing: a crash that left half a line at the file's end does not
// swallow the next move — the torn bytes the harness never sealed are set aside whole in the
// attic beside the file (workspace/shelf), the move lands on a line of its own, and every line left
// is a whole one.
func TestQueueTornTailSwallowsNothing(t *testing.T) {
	s := store(t)
	if _, err := s.Add(Item{For: "zone-auth", Ask: "fix"}, "domain-x"); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(s.Path(), os.O_WRONLY|os.O_APPEND, 0o644)
	f.WriteString(`{"id":"q1","rev":2,"op":"to`)
	f.Close()
	it, err := s.Claim("q1", "zone-auth")
	if err != nil || it.Owner != "zone-auth" {
		t.Fatalf("the move after a torn tail: %+v %v", it, err)
	}
	if got, _, _ := s.Get("q1"); got.State != Claimed {
		t.Fatalf("the claim folded: %+v", got)
	}
	b, _ := os.ReadFile(s.Path())
	for _, l := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
		if !json.Valid(l) {
			t.Fatalf("every line left is whole: %q", b)
		}
	}
	if attic, _ := os.ReadFile(filepath.Join(filepath.Dir(s.Path()), "attic", "queue.jsonl")); !bytes.Contains(attic, []byte(`"op":"to`)) {
		t.Fatalf("the torn bytes are kept: %q", attic)
	}
}

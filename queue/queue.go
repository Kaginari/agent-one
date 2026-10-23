// Package queue is the workspace's shared work queue: work ordered down the ranks (a report's @$
// lines, the human's `q add`) held where every session, every agent and the human can see it,
// until a wake claims it for the agent it was ordered to and that agent closes it.
//
// The file is the history. Every move appends the item's whole snapshot as one JSON line and
// no line is ever rewritten (Policy 4): the queue as it stands is a fold over the lines. Each
// snapshot carries its revision, and a line is folded only when it follows its item's last
// folded revision — so two processes moving one item at once both append, the first line in
// the file wins and the second reads back as a lost rank: a Refusal, never a double claim.
// No lock file, no database, nothing outside the standard library.
package queue

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/workspace/shelf"
)

// State is where an item stands. Done and Dropped are terminal: nothing moves them again.
type State string

const (
	Held    State = "held"    // waiting for a wake to claim it
	Claimed State = "claimed" // an agent holds it
	Done    State = "done"    // closed, the work landed
	Dropped State = "dropped" // closed, the work will not be done
)

// Terminal reports whether no move leaves this state.
func (s State) Terminal() bool { return s == Done || s == Dropped }

// The moves a line records.
const (
	OpAdd     = "add"
	OpClaim   = "claim"
	OpTouch   = "touch"
	OpRequeue = "requeue"
	OpClose   = "close"
)

// Item is one piece of work, as one snapshot: every line in the file is a whole Item.
type Item struct {
	ID    string    `json:"id"`
	Rev   int       `json:"rev"` // 1 at add, one more per move; a line that does not follow its item's last rev lost a rank
	Op    string    `json:"op"`  // the move this snapshot records
	State State     `json:"state"`
	Rank  int       `json:"rank"`          // the order: rank 0 runs before rank 1 (P0 first)
	For   string    `json:"for,omitempty"` // the agent the work is ordered to ("" = whoever the wake names)
	From  string    `json:"from,omitempty"`
	Ask   string    `json:"ask"`
	Owner string    `json:"owner,omitempty"` // the claimant while claimed
	By    string    `json:"by"`              // who made this move
	Note  string    `json:"note,omitempty"`  // the move's word (a touch's progress, a close's why)
	Added time.Time `json:"added"`
	At    time.Time `json:"at"`
}

// Refusal is a move the queue's policy does not allow: an unknown item, a closed one, one held by
// another agent, a rank lost. The CLI exits 2 on it; an unreadable file is an error, not this.
type Refusal struct {
	Op, ID, Why string
}

func (r *Refusal) Error() string {
	if r.ID == "" {
		return "queue " + r.Op + ": " + r.Why
	}
	return "queue " + r.Op + " " + r.ID + ": " + r.Why
}

// IsRefusal reports whether err is (or wraps) a Refusal.
func IsRefusal(err error) bool {
	var r *Refusal
	return errors.As(err, &r)
}

// Store is one queue file. May answers whether `by` may move work `owner` holds (a rank above
// it); nil means only the holder may. An item ordered to no one counts as held by "".
type Store struct {
	path string
	May  func(by, owner string) bool
	Now  func() time.Time
	mu   sync.Mutex
}

// Open names the queue at path; the file is made by the first move.
func Open(path string, may func(by, owner string) bool) *Store {
	return &Store{path: path, May: may}
}

// Path is the queue file.
func (s *Store) Path() string { return s.path }

// Fold replays snapshot lines into the queue as it stands, with each item's history (the
// lines that folded, oldest first). A line that does not follow its item's revision is a lost
// rank and is skipped, as is a line that moves a terminal item.
func Fold(lines []Item) (cur map[string]Item, hist map[string][]Item) {
	cur, hist = map[string]Item{}, map[string][]Item{}
	for _, l := range lines {
		if l.ID == "" {
			continue
		}
		prev, ok := cur[l.ID]
		switch {
		case !ok && l.Rev == 1 && l.Op == OpAdd:
		case ok && l.Rev == prev.Rev+1 && !prev.State.Terminal():
		default:
			continue
		}
		cur[l.ID] = l
		hist[l.ID] = append(hist[l.ID], l)
	}
	return cur, hist
}

// Ranked orders items for reading: claimed work first, then held work, then closed; within
// each, lower rank first, then oldest first.
func Ranked(items []Item) {
	group := func(s State) int {
		switch s {
		case Claimed:
			return 0
		case Held:
			return 1
		}
		return 2
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if ga, gb := group(a.State), group(b.State); ga != gb {
			return ga < gb
		}
		if a.Rank != b.Rank {
			return a.Rank < b.Rank
		}
		if !a.Added.Equal(b.Added) {
			return a.Added.Before(b.Added)
		}
		return idNum(a.ID) < idNum(b.ID)
	})
}

func idNum(id string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(id, "q"))
	return n
}

// read loads every line; a missing file is an empty queue, a malformed line is skipped.
func (s *Store) read() ([]Item, error) {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Item
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var it Item
		if json.Unmarshal(sc.Bytes(), &it) == nil {
			out = append(out, it)
		}
	}
	return out, sc.Err()
}

func (s *Store) fold() (map[string]Item, map[string][]Item, error) {
	lines, err := s.read()
	if err != nil {
		return nil, nil, err
	}
	cur, hist := Fold(lines)
	return cur, hist, nil
}

// List is the queue as it stands, ranked.
func (s *Store) List() ([]Item, error) {
	cur, _, err := s.fold()
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(cur))
	for _, it := range cur {
		out = append(out, it)
	}
	Ranked(out)
	return out, nil
}

// Held is the open work, ranked — what a wake may claim.
func (s *Store) Held() ([]Item, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var out []Item
	for _, it := range all {
		if it.State == Held {
			out = append(out, it)
		}
	}
	return out, nil
}

// Get is one item and its history.
func (s *Store) Get(id string) (Item, []Item, error) {
	cur, hist, err := s.fold()
	if err != nil {
		return Item{}, nil, err
	}
	it, ok := cur[id]
	if !ok {
		return Item{}, nil, &Refusal{Op: "show", ID: id, Why: "no such item"}
	}
	return it, hist[id], nil
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// may is the holder rule: the holder itself, or whoever May lets over it.
func (s *Store) may(by, owner string) bool {
	return (owner != "" && by == owner) || (s.May != nil && s.May(by, owner))
}

// Add queues new work, held until a wake claims it.
func (s *Store) Add(it Item, by string) (Item, error) {
	it.Ask = strings.TrimSpace(it.Ask)
	if it.Ask == "" {
		return Item{}, &Refusal{Op: OpAdd, Why: "an item needs an ask"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// a lost rank for an id (another process added the same number) retries on the next one
	for try := 0; try < 3; try++ {
		cur, _, err := s.fold()
		if err != nil {
			return Item{}, err
		}
		n := 0
		for id := range cur {
			if k := idNum(id); k > n {
				n = k
			}
		}
		at := s.now()
		next := Item{ID: "q" + strconv.Itoa(n+1), Rev: 1, Op: OpAdd, State: Held, Rank: it.Rank, For: it.For, From: or(it.From, by), Ask: it.Ask, By: by, Note: it.Note, Added: at, At: at}
		won, err := s.commit(next)
		if err != nil || won {
			return next, err
		}
	}
	return Item{}, &Refusal{Op: OpAdd, Why: "lost three ranks for an id — the queue is busy, try again"}
}

// Claim gives held work to `by`. Work ordered to an agent is claimed by that agent, or by one
// May lets over it (the wake claims on the agent's behalf by naming it).
func (s *Store) Claim(id, by string) (Item, error) {
	return s.move(OpClaim, id, by, "", func(it *Item) string {
		if it.State != Held {
			return fmt.Sprintf("it is %s%s — only held work is claimed", it.State, holder(*it))
		}
		if it.For != "" && by != it.For && !s.may(by, it.For) {
			return "it is ordered to " + it.For
		}
		it.State, it.Owner = Claimed, by
		return ""
	})
}

// Touch records progress on claimed work (its note, its clock) without moving it.
func (s *Store) Touch(id, by, note string) (Item, error) {
	return s.move(OpTouch, id, by, note, func(it *Item) string {
		if it.State != Claimed {
			return fmt.Sprintf("it is %s — only claimed work is touched", it.State)
		}
		if !s.may(by, it.Owner) {
			return "it is held by " + it.Owner
		}
		return ""
	})
}

// Requeue hands claimed work back to the held queue.
func (s *Store) Requeue(id, by, note string) (Item, error) {
	return s.move(OpRequeue, id, by, note, func(it *Item) string {
		if it.State != Claimed {
			return fmt.Sprintf("it is %s — only claimed work goes back", it.State)
		}
		if !s.may(by, it.Owner) {
			return "it is held by " + it.Owner
		}
		it.State, it.Owner = Held, ""
		return ""
	})
}

// Close ends work as Done or Dropped. Claimed work is closed by its holder (or one over it);
// held work by whoever queued it, or one over the agent it is ordered to.
func (s *Store) Close(id, by string, to State, note string) (Item, error) {
	if !to.Terminal() {
		return Item{}, &Refusal{Op: OpClose, ID: id, Why: fmt.Sprintf("%q is not a closing state (done or dropped)", to)}
	}
	return s.move(OpClose, id, by, note, func(it *Item) string {
		switch {
		case it.State.Terminal():
			return fmt.Sprintf("it is already %s", it.State)
		case it.State == Claimed && !s.may(by, it.Owner):
			return "it is held by " + it.Owner
		case it.State == Held && by != it.From && !s.may(by, it.For):
			return "it was queued by " + or(it.From, "another hand") + " for " + or(it.For, "no one yet")
		}
		it.State = to
		return ""
	})
}

func holder(it Item) string {
	if it.Owner != "" {
		return " by " + it.Owner
	}
	return ""
}

func or(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// move folds, lets the rule shape the next snapshot (a non-empty answer refuses), appends it,
// and reads back whether it won.
func (s *Store) move(op, id, by, note string, rule func(*Item) string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, _, err := s.fold()
	if err != nil {
		return Item{}, err
	}
	it, ok := cur[id]
	if !ok {
		return Item{}, &Refusal{Op: op, ID: id, Why: "no such item"}
	}
	if why := rule(&it); why != "" {
		return Item{}, &Refusal{Op: op, ID: id, Why: why}
	}
	it.Rev++
	it.Op, it.By, it.Note, it.At = op, by, note, s.now()
	won, err := s.commit(it)
	if err != nil {
		return Item{}, err
	}
	if !won {
		return Item{}, &Refusal{Op: op, ID: id, Why: "another hand moved it first — read it again"}
	}
	return it, nil
}

// commit appends one snapshot and reports whether the fold took it (a racing line with the
// same revision may have landed first).
func (s *Store) commit(it Item) (bool, error) {
	b, err := json.Marshal(it)
	if err != nil {
		return false, err
	}
	// one line in one write, on a line of its own after a torn tail (shelf.Append)
	if err := shelf.Append(s.path, b, 0o644); err != nil {
		return false, err
	}
	_, hist, err := s.fold()
	if err != nil {
		return false, err
	}
	// the line that folded at this revision is ours, whatever moved the item since
	h := hist[it.ID]
	if len(h) < it.Rev {
		return false, nil
	}
	got := h[it.Rev-1]
	return got.By == it.By && got.Op == it.Op && got.At.Equal(it.At), nil
}

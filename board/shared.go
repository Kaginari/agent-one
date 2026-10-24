package board

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/queue"
)

// SharedView is the workspace's shared board — what every session, every agent and the human can
// read alike: the work ordered down and still open on the shared queue (the wire briefs: a
// report's @$ lines, the human's `q add`), every agent's plan deck, and the last report each
// agent returned. The overview's board panel draws it; /api/board serves it.
type SharedView struct {
	Queue   SharedQueue    `json:"queue"`
	Decks   []SharedDeck   `json:"decks"`
	Reports []SharedReport `json:"reports"`
}

// SharedQueue is the shared queue as it stands: the open items (claimed first, then held, by
// rank) and how many stand in each state.
type SharedQueue struct {
	Reading Reading       `json:"reading"`
	Held    int           `json:"held"`
	Claimed int           `json:"claimed"`
	Closed  int           `json:"closed"`
	Items   []SharedBrief `json:"items"`
}

// SharedBrief is one open item: its brief cut to one line, whom it is ordered to, who holds it.
type SharedBrief struct {
	ID    string    `json:"id"`
	State string    `json:"state"`
	Rank  int       `json:"rank"`
	For   string    `json:"for,omitempty"`
	From  string    `json:"from,omitempty"`
	Owner string    `json:"owner,omitempty"`
	Ask   string    `json:"ask"`
	At    time.Time `json:"at"`
}

// SharedDeck is one agent's plan deck at a glance: its progress and the step in hand.
type SharedDeck struct {
	Agent   string `json:"agent"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Blocked int    `json:"blocked"`
	Current string `json:"current,omitempty"`
	Updated string `json:"updated,omitempty"`
}

// SharedReport is the last report one agent returned: its verdict and the ask it answered.
type SharedReport struct {
	As     string `json:"as"`
	Status string `json:"status"`
	Ask    string `json:"ask"`
	At     string `json:"at"`
}

// sharedCaps bound the board: the panel is a glance, never the whole record.
const (
	sharedBriefs  = 12
	sharedDecks   = 12
	sharedReports = 8
)

// shared reads the shared board off the files: the queue on the shared shelf, the decks the
// plan tool pens, the loop journal's runs. Read-only — nothing is sealed, moved or written.
func (f *fileSources) shared() *SharedView {
	v := &SharedView{Decks: []SharedDeck{}, Reports: []SharedReport{}}
	v.Queue = f.sharedQueue()
	v.Decks = f.sharedDecks()
	runs, _ := f.loop()
	v.Reports = lastReports(runs)
	return v
}

func (f *fileSources) sharedQueue() SharedQueue {
	path := filepath.Join(f.dir, "shared", "queue.jsonl")
	src, now := f.rel(path), f.now()
	q := SharedQueue{Items: []SharedBrief{}}
	st, err := os.Stat(path)
	if err != nil {
		q.Reading = silent(src, "no shared queue yet — nothing was ordered down", time.Time{}, now)
		return q
	}
	items, err := queue.Open(path, nil).List()
	if err != nil {
		q.Reading = silent(src, "the shared queue is unreadable: "+err.Error(), st.ModTime(), now)
		return q
	}
	q.Reading = lit(src, st.ModTime(), now)
	for _, it := range items {
		switch it.State {
		case queue.Held:
			q.Held++
		case queue.Claimed:
			q.Claimed++
		default:
			q.Closed++
			continue
		}
		if len(q.Items) < sharedBriefs {
			q.Items = append(q.Items, SharedBrief{ID: it.ID, State: string(it.State), Rank: it.Rank, For: it.For, From: it.From, Owner: it.Owner, Ask: briefLine(it.Ask), At: it.At})
		}
	}
	return q
}

// sharedDecks reads every deck under tmp/plans (not the archive), a deck in hand first (one
// with a step in progress or blocked), then the newest; an empty deck is not on the board.
func (f *fileSources) sharedDecks() []SharedDeck {
	dir := filepath.Join(f.dir, "tmp", "plans")
	ents, err := os.ReadDir(dir)
	out := []SharedDeck{}
	if err != nil {
		return out
	}
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var d struct {
			Agent   string `json:"agent"`
			Updated string `json:"updated"`
			Items   []struct {
				Text   string `json:"text"`
				Status string `json:"status"`
			} `json:"items"`
		}
		if json.Unmarshal(raw, &d) != nil || len(d.Items) == 0 {
			continue
		}
		sd := SharedDeck{Agent: d.Agent, Total: len(d.Items), Updated: d.Updated}
		if sd.Agent == "" {
			sd.Agent = strings.TrimSuffix(e.Name(), ".json")
		}
		pending := ""
		for _, it := range d.Items {
			switch it.Status {
			case "done":
				sd.Done++
			case "blocked":
				sd.Blocked++
			case "in_progress":
				if sd.Current == "" {
					sd.Current = briefLine(it.Text)
				}
			default:
				if pending == "" {
					pending = briefLine(it.Text)
				}
			}
		}
		if sd.Current == "" && sd.Done < sd.Total {
			sd.Current = pending
		}
		out = append(out, sd)
	}
	inHand := func(d SharedDeck) bool { return d.Done < d.Total }
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := inHand(out[i]), inHand(out[j]); a != b {
			return a
		}
		if out[i].Updated != out[j].Updated {
			return out[i].Updated > out[j].Updated
		}
		return out[i].Agent < out[j].Agent
	})
	if len(out) > sharedDecks {
		out = out[:sharedDecks]
	}
	return out
}

// lastReports is each agent's newest journaled run that ended with a verdict, newest first.
func lastReports(runs []Run) []SharedReport {
	seen := map[string]bool{}
	sorted := append([]Run(nil), runs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].At > sorted[j].At })
	out := []SharedReport{}
	for _, r := range sorted {
		if r.As == "" || r.Status == "" || seen[r.As] {
			continue
		}
		seen[r.As] = true
		out = append(out, SharedReport{As: r.As, Status: r.Status, Ask: briefLine(r.Ask), At: r.At})
		if len(out) == sharedReports {
			break
		}
	}
	return out
}

// briefLine is a brief as one line: its first line of prose (the wire's own @-lines are the
// envelope, not the ask), else the first line with its @WORD dropped, cut to 160 runes.
func briefLine(s string) string {
	var first string
	for _, l := range strings.Split(s, "\n") {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "@") {
			first = t
			break
		}
		if first == "" {
			if _, rest, ok := strings.Cut(t, " "); ok {
				first = strings.TrimSpace(rest)
			}
		}
	}
	if r := []rune(first); len(r) > 160 {
		first = string(r[:159]) + "…"
	}
	return first
}

// apiBoard serves the shared board (SharedView).
func (b *Board) apiBoard(w http.ResponseWriter, r *http.Request) {
	v := b.src.Shared()
	if v == nil {
		v = &SharedView{Queue: SharedQueue{Items: []SharedBrief{}}, Decks: []SharedDeck{}, Reports: []SharedReport{}}
	}
	writeJSON(w, v)
}

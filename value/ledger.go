package value

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Shelf is the value ledger's path under the workspace dir: one append-only JSON line per row,
// always kept — a workspace with no promotion still measures its agents.
const Shelf = "instruments/value.jsonl"

// LegacyShelf is the promotion tally the ledger replaced; Seed reads it once.
const LegacyShelf = "instruments/promotion.jsonl"

// Row kinds.
const (
	KindRun      = "run"       // one run of one agent (a Subagent's report, or an orchestrator turn)
	KindVerdict  = "verdict"   // a gate verdict, credited to the agent that held the gate
	KindOverturn = "overturn"  // a held pass a later fail on the same paths undid
	KindPromote  = "promote"   // a promotion registered (the base agent is done promoting)
	KindSeed     = "seed"      // the ledger was seeded from the legacy shelf
	KindEvolve   = "evolution" // a stressed agent evolved (split, mutate, rebirth) — its cooldown counts from here
	KindRebirth  = "rebirth"   // an agent was reborn: its record starts again from this row
)

// Holes splits a run's holes by whose they are: the workspace's (infra — a provider refusal, a
// drain, a recall, a cut at max_tokens), the harness's catch of the agent's discipline (harness —
// an off-wire answer, a missing @U, a gate sent back), and the agent's own honest @? (self). Only
// harness holes cost the agent.
type Holes struct {
	Infra   int `json:"infra,omitempty"`
	Harness int `json:"harness,omitempty"`
	Self    int `json:"self,omitempty"`
}

// UnmarshalJSON accepts the legacy bare count (a promotion tally's holes) as harness holes.
func (h *Holes) UnmarshalJSON(b []byte) error {
	var n int
	if json.Unmarshal(b, &n) == nil {
		*h = Holes{Harness: n}
		return nil
	}
	type plain Holes
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*h = Holes(p)
	return nil
}

// Row is one line of the ledger.
type Row struct {
	Kind  string    `json:"kind"`
	At    time.Time `json:"at"`
	Agent string    `json:"agent"`
	Rank  string    `json:"rank,omitempty"`
	By    string    `json:"by,omitempty"` // the dispatcher (a run) — the agent that commissioned it

	// a run
	Loop      string   `json:"loop,omitempty"`   // the loop's lifecycle status
	Word      string   `json:"word,omitempty"`   // the agent's own @S word
	Status    string   `json:"status,omitempty"` // the run's word: the loop's when it did not end DONE, else the agent's
	Gate      string   `json:"gate,omitempty"`   // pass | fail | off | "" (nothing gated)
	GateTries int      `json:"gateTries,omitempty"`
	VerifyRun int      `json:"verifyRun,omitempty"`
	VerifyRed int      `json:"verifyRed,omitempty"`
	Holes     Holes    `json:"holes,omitempty"`
	Findings  int      `json:"findings,omitempty"`
	Strain    int      `json:"strain,omitempty"`  // the council's strain on the agent after this run
	Warrant   bool     `json:"warrant,omitempty"` // the council stamped a warrant on this run
	Thrash    int      `json:"thrash,omitempty"`  // guts readings that priced a bench thrash
	Momentum  int      `json:"momentum,omitempty"`
	Guts      bool     `json:"guts,omitempty"` // the run journaled guts readings (momentum is meaningful)
	Tokens    int      `json:"tokens,omitempty"`
	Paths     []string `json:"paths,omitempty"`
	Turn      bool     `json:"turn,omitempty"` // an orchestrator turn: the human's ask

	// a verdict / an overturn
	Of   string `json:"of,omitempty"`   // the authoring agent the verdict judged
	Ref  string `json:"ref,omitempty"`  // an overturn: the overturned verdict's time (RFC3339Nano)
	New  string `json:"new,omitempty"`  // a promotion: the promoted agent
	From string `json:"from,omitempty"` // a seed: the shelf it was read from

	// an evolution: the arm, why it was chosen, and the agents it left (a split's young)
	Arm    string   `json:"arm,omitempty"`
	Reason string   `json:"reason,omitempty"`
	Into   []string `json:"into,omitempty"`
}

// Append writes one row to the ledger at path (created with its dirs). The write is one
// O_APPEND line: concurrent writers interleave rows, never bytes of rows.
func Append(path string, r Row) error {
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// Read replays a ledger. A missing file is an empty ledger; an unreadable line is skipped.
func Read(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Row
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var r Row
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Agent == "" && r.Kind != KindSeed {
			continue
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

// legacyRow is one line of the promotion tally.
type legacyRow struct {
	Kind   string    `json:"kind"`
	Agent  string    `json:"agent"`
	Status string    `json:"status"`
	Holes  int       `json:"holes"`
	New    string    `json:"new"`
	At     time.Time `json:"at"`
}

// Seed writes the ledger once from the legacy promotion tally: every report becomes a run row
// (its holes kept as harness holes — the old tally could not tell them apart), every promotion
// a promote row, and a seed row records the act. A ledger that already exists is never seeded;
// a workspace with no legacy tally gets no ledger until its first run. It answers how many rows it
// carried over.
func Seed(workspaceDir string) (int, error) {
	dst := filepath.Join(workspaceDir, filepath.FromSlash(Shelf))
	if _, err := os.Stat(dst); err == nil {
		return 0, nil
	}
	src := filepath.Join(workspaceDir, filepath.FromSlash(LegacyShelf))
	f, err := os.Open(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()
	var rows []Row
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var l legacyRow
		if json.Unmarshal(sc.Bytes(), &l) != nil || l.Agent == "" {
			continue
		}
		switch l.Kind {
		case "report":
			word := strings.ToUpper(strings.Fields(l.Status + " ")[0])
			loop := "DONE"
			switch word {
			case "FAIL", "ESCALATE", "CHECKPOINT", "DENIED", "DRY", "SUSPENDED":
				loop = word
			}
			rows = append(rows, Row{Kind: KindRun, At: l.At, Agent: l.Agent, Loop: loop, Word: word, Status: word, Holes: Holes{Harness: l.Holes}})
		case "promote":
			rows = append(rows, Row{Kind: KindPromote, At: l.At, Agent: l.Agent, New: l.New})
		}
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	var b strings.Builder
	for _, r := range rows {
		j, _ := json.Marshal(r)
		b.Write(j)
		b.WriteByte('\n')
	}
	seed, _ := json.Marshal(Row{Kind: KindSeed, At: time.Now().UTC(), From: LegacyShelf})
	b.Write(seed)
	b.WriteByte('\n')
	if err := os.WriteFile(dst, []byte(b.String()), 0o644); err != nil {
		return 0, fmt.Errorf("seed %s: %w", Shelf, err)
	}
	return len(rows), nil
}

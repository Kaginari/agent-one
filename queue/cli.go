package queue

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Usage is the q command's one line.
const Usage = "q list [--all] | show <id> | add [--for <body>] [--rank N] <ask> | claim <id> [--as <body>] | touch <id> [note] | requeue <id> [note] | close <id> [--drop] [note]"

// CLI runs one q command against the store as `by` (the human, unless --as names an agent).
// Exit 0 on a move or a read, 2 on a refusal or a usage error, 1 when the file cannot be read
// or written. asJSON prints items as JSON (output.format json); the text form is for people.
func CLI(s *Store, by string, args []string, asJSON bool, out, errw io.Writer) int {
	fail := func(code int, err error) int {
		fmt.Fprintf(errw, "@S FAIL\n@? %v\n", err)
		return code
	}
	errCode := func(err error) int {
		if IsRefusal(err) {
			return fail(2, err)
		}
		return fail(1, err)
	}
	sub := "list"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	var all, drop bool
	var forAgent, rank string
	var words []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--all", "-a":
			all = true
		case "--drop":
			drop = true
		case "--for", "--as", "--rank":
			if i+1 >= len(args) {
				return fail(2, fmt.Errorf("%s needs a value — %s", a, Usage))
			}
			i++
			switch a {
			case "--for":
				forAgent = strings.ToLower(args[i])
			case "--as":
				by = strings.ToLower(args[i])
			default:
				rank = args[i]
			}
		default:
			words = append(words, a)
		}
	}
	id := func() (string, bool) {
		if len(words) == 0 {
			return "", false
		}
		return words[0], true
	}
	rest := func() string {
		if len(words) < 2 {
			return ""
		}
		return strings.Join(words[1:], " ")
	}
	emit := func(it Item, verb string) int {
		if asJSON {
			return printJSON(out, it)
		}
		fmt.Fprintf(out, "@S OK %s %s — %s\n", verb, it.ID, line(it))
		return 0
	}
	switch sub {
	case "list", "ls":
		items, err := s.List()
		if err != nil {
			return errCode(err)
		}
		if !all {
			kept := items[:0]
			for _, it := range items {
				if !it.State.Terminal() {
					kept = append(kept, it)
				}
			}
			items = kept
		}
		if asJSON {
			if items == nil {
				items = []Item{}
			}
			return printJSON(out, items)
		}
		held, claimed := 0, 0
		for _, it := range items {
			switch it.State {
			case Held:
				held++
			case Claimed:
				claimed++
			}
		}
		fmt.Fprintf(out, "queue · %d held · %d claimed (%s)\n", held, claimed, s.Path())
		for _, it := range items {
			fmt.Fprintln(out, "  "+line(it))
		}
		return 0
	case "show":
		i, ok := id()
		if !ok {
			return fail(2, fmt.Errorf("q show <id>"))
		}
		it, hist, err := s.Get(i)
		if err != nil {
			return errCode(err)
		}
		if asJSON {
			return printJSON(out, struct {
				Item    Item   `json:"item"`
				History []Item `json:"history"`
			}{it, hist})
		}
		fmt.Fprintf(out, "%s\nask: %s\n", line(it), it.Ask)
		for _, h := range hist {
			note := ""
			if h.Note != "" {
				note = " — " + h.Note
			}
			fmt.Fprintf(out, "  r%d %s %-8s by %s%s\n", h.Rev, h.At.Local().Format("2006-01-02 15:04"), h.Op, h.By, note)
		}
		return 0
	case "add":
		n := 0
		if rank != "" {
			var err error
			if n, err = strconv.Atoi(rank); err != nil || n < 0 {
				return fail(2, fmt.Errorf("--rank takes a number, 0 first: %q", rank))
			}
		}
		it, err := s.Add(Item{For: forAgent, From: by, Rank: n, Ask: strings.Join(words, " ")}, by)
		if err != nil {
			return errCode(err)
		}
		return emit(it, "queued")
	case "claim", "touch", "requeue", "close":
		i, ok := id()
		if !ok {
			return fail(2, fmt.Errorf("q %s <id>", sub))
		}
		var it Item
		var err error
		switch sub {
		case "claim":
			it, err = s.Claim(i, by)
		case "touch":
			it, err = s.Touch(i, by, rest())
		case "requeue":
			it, err = s.Requeue(i, by, rest())
		default:
			to := Done
			if drop {
				to = Dropped
			}
			it, err = s.Close(i, by, to, rest())
		}
		if err != nil {
			return errCode(err)
		}
		return emit(it, map[string]string{"claim": "claimed", "touch": "touched", "requeue": "requeued", "close": string(it.State)}[sub])
	}
	return fail(2, fmt.Errorf("unknown q command %q — %s", sub, Usage))
}

// line is one item for people: id, state, rank, who it is for and who holds it, the ask's head.
func line(it Item) string {
	who := or(it.For, "anyone")
	if it.Owner != "" {
		who += " · held by " + it.Owner
	}
	ask := strings.Join(strings.Fields(it.Ask), " ")
	if r := []rune(ask); len(r) > 72 {
		ask = string(r[:71]) + "…"
	}
	return fmt.Sprintf("%-4s %-8s P%d  %s — %s", it.ID, it.State, it.Rank, who, ask)
}

func printJSON(out io.Writer, v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return 1
	}
	fmt.Fprintln(out, string(b))
	return 0
}

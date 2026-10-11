package wire

import (
	"fmt"
	"strings"
	"testing"
)

func TestForgeRoundTrip(t *testing.T) {
	r, ok := ParseReport("@S DONE\n@FORGE tool:ui-wrangler | rewrite bare /v1 strings safe | python3 wrangle.py {{path}}\n@FORGE skill:ttl-craft\n@FORGE nonsense\n@E 99")
	if !ok || len(r.Forges) != 2 {
		t.Fatalf("forges: %v (ok %v)", r.Forges, ok)
	}
	tool, skill := r.Forges[0], r.Forges[1]
	if tool.Kind != "tool" || tool.Name != "ui-wrangler" || tool.Text != "rewrite bare /v1 strings safe | python3 wrangle.py {{path}}" {
		t.Fatalf("tool draft the pipe keeps whole: %+v", tool)
	}
	if skill.Kind != "skill" || skill.Name != "ttl-craft" || skill.Text != "" {
		t.Fatalf("a name may carry the phrase alone: %+v", skill)
	}
	if len(r.Other) != 1 || r.Other[0] != "@FORGE nonsense" {
		t.Fatalf("a draft with no kind is prose, not poison: %+v", r.Other)
	}
	back, ok := ParseReport(r.String())
	if !ok || len(back.Forges) != 2 || back.Forges[0] != tool {
		t.Fatalf("round trip: %+v", back.Forges)
	}
}

func TestCommission(t *testing.T) {
	c := ParseCommission("@ROOT /w/x\n@SCOPE pkgs a, b\n@ASK draft +unsaid wire:raw\n@CAP 512\n@DUMP .agent-one/tmp/out.md\n@SIZE 4000\nBuild the thing.\n\nSecond line.")
	if c.Root != "/w/x" || c.Scope != "pkgs a, b" || c.Ask != "draft" || !c.Unsaid || !c.Raw || c.Cap != 512 || c.Dump != ".agent-one/tmp/out.md" || c.Size != 4000 || len(c.Agent) != 2 {
		t.Fatalf("%+v", c)
	}
	if ParseCommission("just prose").CapOrDefault() != DefaultCap {
		t.Fatal("default cap")
	}
	back := ParseCommission(c.String())
	if back.Ask != "draft" || !back.Unsaid || !back.Raw || back.Cap != 512 || len(back.Agent) != 2 {
		t.Fatalf("round trip %+v\n%s", back, c.String())
	}
}

func TestReport(t *testing.T) {
	r, ok := ParseReport("Some preamble.\n@S PASS 3 checks\n@F a.go:12 the fact\n@V claim holds — evidence\n@? what about x\n@U team the team knows\n@U untyped thing\n@T skill x — path — load≈10tok\n@E 123\n")
	if !ok || r.Status != "PASS 3 checks" || len(r.Findings) != 1 || len(r.Verdicts) != 1 || len(r.Holes) != 1 || len(r.Tools) != 1 || r.Bytes != 123 || len(r.Other) != 1 {
		t.Fatalf("%+v", r)
	}
	if r.Unsaid[0].Kind != "team" || r.Unsaid[0].Text != "the team knows" || r.Unsaid[1].Kind != "" || r.Unsaid[1].Text != "untyped thing" {
		t.Fatalf("unsaid %+v", r.Unsaid)
	}
	if kinds := r.SortedKinds(); strings.Join(kinds, ",") != ",team" {
		t.Fatalf("kinds %v", kinds)
	}
	if _, ok := ParseReport("no envelope here"); ok {
		t.Fatal("no @S means not on the wire")
	}
	raw, ok := ParseReport("S|DONE\nF|a.go:3|one fact\n?|a hole\nE|40")
	if !ok || raw.Status != "DONE" || raw.Findings[0] != "a.go:3 — one fact" || raw.Holes[0] != "a hole" || raw.Bytes != 40 {
		t.Fatalf("raw %+v", raw)
	}
	if _, ok := ParseReport("@Something odd"); ok {
		t.Fatal("unknown tags are not @S")
	}
}

func TestEmitAndCap(t *testing.T) {
	r := Report{Status: "PASS", Findings: []string{"a.go:1 one"}, Holes: []string{"h"}, Unsaid: []Unsaid{{Kind: "policy", Text: "l"}}}
	out := r.String()
	want := "@S PASS\n@? h\n@U policy l\n@F a.go:1 one\n@E "
	if !strings.HasPrefix(out, want) {
		t.Fatalf("order: %q", out)
	}
	if !strings.HasSuffix(out, fmt.Sprint(len(out))) {
		t.Fatalf("@E must equal the byte length: %q", out)
	}
	back, _ := ParseReport(out)
	if back.Bytes != len(out) {
		t.Fatal("@E parses back to the length")
	}
	big := Report{Status: "PASS", Unsaid: []Unsaid{{Kind: "domain", Text: "kept"}}, Holes: []string{"kept too"}}
	for i := 0; i < 100; i++ {
		big.Findings = append(big.Findings, fmt.Sprintf("f%d.go:%d a finding of some length", i, i))
	}
	for _, cap := range []int{200, 500, 1000, 2048} {
		out := big.Emit(cap, ".agent-one/tmp/full.md")
		if len(out) > cap {
			t.Fatalf("cap %d: %d bytes", cap, len(out))
		}
		if !strings.Contains(out, "@U domain kept") || !strings.Contains(out, "@? kept too") || !strings.Contains(out, "@? @CAP "+fmt.Sprint(cap)+": ") || !strings.Contains(out, "full report at .agent-one/tmp/full.md") {
			t.Fatalf("cap %d: %q", cap, out)
		}
		if !strings.HasSuffix(out, fmt.Sprint(len(out))) {
			t.Fatalf("cap %d: @E wrong: %q", cap, out)
		}
	}
	if len(big.Emit(0, "")) < 3000 {
		t.Fatal("cap 0 means no ceiling")
	}
	tiny := Report{Status: "PASS"}.Emit(10, "")
	if !strings.HasPrefix(tiny, "@S PASS") {
		t.Fatalf("a report that cannot fit still opens with @S: %q", tiny)
	}
}

func TestEnqueueTag(t *testing.T) {
	// @$ carries an order one layer down; the pipe-dense form parses too; a @$ with no
	// `<body>:` is prose and stays in Other; Emit round-trips what Parse collected.
	r, ok := ParseReport("@S DONE\n@$ domain-auth: gate the login flow\n$|zone-db: migrate the users table\n@$ stray line with no agent\n@E 96")
	if !ok {
		t.Fatal("a report with @$ must parse")
	}
	if len(r.Enqueues) != 2 {
		t.Fatalf("enqueues: %v", r.Enqueues)
	}
	if r.Enqueues[0].Agent != "domain-auth" || r.Enqueues[0].Ask != "gate the login flow" {
		t.Fatalf("@$ parse: %+v", r.Enqueues[0])
	}
	if r.Enqueues[1].Agent != "zone-db" || r.Enqueues[1].Ask != "migrate the users table" {
		t.Fatalf("pipe-dense @$: %+v", r.Enqueues[1])
	}
	if len(r.Other) != 1 || !strings.Contains(r.Other[0], "stray") {
		t.Fatalf("a colonless @$ is prose: %v", r.Other)
	}
	out := r.String()
	if !strings.Contains(out, "@$ domain-auth: gate the login flow") {
		t.Fatalf("Emit lost the order lines:\n%s", out)
	}
	again, _ := ParseReport(out)
	if len(again.Enqueues) != len(r.Enqueues) {
		t.Fatalf("round-trip: %d != %d", len(again.Enqueues), len(r.Enqueues))
	}
}

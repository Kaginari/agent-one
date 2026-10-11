package workspace

import (
	"strings"
	"testing"
)

// TestGateCircuitBreaker covers the council breaker (freeze review 2026-10-07): the same
// gate failing on the same file-set for the same agent hardens into a spec-rewrite directive
// after the threshold — retrying identically was the onto/app-commands thrash class.
func TestGateCircuitBreaker(t *testing.T) {
	w := open(t, fixture(t))
	fail := func(set ...string) *Verdict {
		v := &Verdict{Word: "fail", Reasons: []string{"x"}}
		w.breaker("zone-auth", set, v)
		return v
	}
	a := fail("src/auth/a.go")
	if strings.Contains(strings.Join(a.Reasons, " "), "circuit-breaker") {
		t.Fatalf("the first fail is a fail, not a conspiracy: %v", a.Reasons)
	}
	b := fail("src/auth/a.go")
	c := fail("src/auth/a.go")
	if c.Reasons[len(c.Reasons)-1] == "" || !strings.Contains(strings.Join(c.Reasons, " "), "circuit-breaker") || b.Reasons[len(b.Reasons)-1] == c.Reasons[len(c.Reasons)-1] {
		_ = a
		t.Fatalf("the third identical fail must carry the directive, first two must not: %v / %v / %v", a.Reasons, b.Reasons, c.Reasons)
	}
	// a different file-set restarts the streak
	d := fail("src/api/other.go")
	if strings.Contains(strings.Join(d.Reasons, " "), "circuit-breaker") {
		t.Fatalf("a different wave has no streak: %v", d.Reasons)
	}
	// and a pass clears whatever stood
	v := &Verdict{Word: "pass"}
	w.breaker("zone-auth", []string{"src/auth/a.go"}, v)
	e := fail("src/auth/a.go")
	if strings.Contains(strings.Join(e.Reasons, " "), "circuit-breaker") {
		t.Fatalf("a pass resets the streak: %v", e.Reasons)
	}
}

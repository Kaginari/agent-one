package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFooterTokensZeroRendersAsToday(t *testing.T) {
	th := NewTheme(true)

	// today: no token data — both zero — the footer says exactly what it always said
	zero := FooterView{Model: "kimi", Ctx: "ctx …gathering", CtxKnown: false}
	got := ansi.Strip(th.Footer(zero, 200))
	if !strings.Contains(got, "ctx …gathering") {
		t.Fatalf("zero CtxTokens/CtxLimit keeps the gathering hint: %q", got)
	}
	if strings.Contains(got, "▰") || strings.Contains(got, "k/") {
		t.Fatalf("zero values must not surface the meter or counts: %q", got)
	}

	known := FooterView{Model: "kimi", CtxKnown: true, CtxPct: 42}
	got = ansi.Strip(th.Footer(known, 200))
	if !strings.Contains(got, "ctx ") || !strings.Contains(got, "42%") {
		t.Fatalf("a known ctx still renders its meter: %q", got)
	}
	if strings.Contains(got, "/") && strings.Contains(got, "k") {
		t.Fatalf("zero CtxTokens/CtxLimit keeps the raw counts hidden: %q", got)
	}
}

func TestFooterTokensHumanized(t *testing.T) {
	th := NewTheme(true)
	f := FooterView{Model: "kimi", CtxKnown: true, CtxPct: 42, CtxTokens: 84_000, CtxLimit: 200_000}

	got := ansi.Strip(th.Footer(f, 200))
	if !strings.Contains(got, "84k/200k") {
		t.Fatalf("the footer's token segment is humanized: %q", got)
	}

	// below the width gate: meter survives, counts drop
	got = ansi.Strip(th.Footer(f, 80))
	if !strings.Contains(got, "42%") {
		t.Fatalf("ctx meter survives at width 80: %q", got)
	}
	if strings.Contains(got, "84k") {
		t.Fatalf("raw counts are width-88-gated, not shown at 80: %q", got)
	}

	// below the meter gate both drop
	got = ansi.Strip(th.Footer(f, 60))
	if strings.Contains(got, "42%") || strings.Contains(got, "84k") {
		t.Fatalf("below width 72 the whole ctx segment drops: %q", got)
	}

	// big values humanize too
	f.CtxTokens, f.CtxLimit, f.CtxPct = 1_200_000, 1_000_000, 120
	got = ansi.Strip(th.Footer(f, 200))
	if !strings.Contains(got, "1.2M/1.0M") {
		t.Fatalf("million-scale humanizes with M: %q", got)
	}
}

func TestHumanTokensForms(t *testing.T) {
	for i, tc := range []struct {
		n    int
		want string
	}{
		{0, "0"}, {42, "42"}, {999, "999"},
		{1000, "1.0k"}, {9_999, "10.0k"},
		{10_000, "10k"}, {84_000, "84k"}, {200_000, "200k"},
		{1_000_000, "1.0M"}, {1_200_000, "1.2M"},
	} {
		if got := humanTokens(tc.n); got != tc.want {
			t.Fatalf("case %d: humanTokens(%d) = %q, want %q", i, tc.n, got, tc.want)
		}
	}
}

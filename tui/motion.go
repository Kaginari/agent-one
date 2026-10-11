package tui

import (
	"image/color"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/x/ansi"
)

// Motion and colour: the intro (the mascot drops in on a spring, lands where the welcome box
// holds it, blinks), gradients along a line of text, the shimmer that sweeps the spinner's verb.

const (
	introFPS  = 60
	introDrop = 6 // pixels the mascot falls (3 rows)
	introMax  = 1400 * time.Millisecond
	introHold = 380 * time.Millisecond // after landing: settle, blink, then the welcome
)

type introState struct {
	spring  harmonica.Spring
	pos     float64 // pixels above its resting place
	vel     float64
	started time.Time
	landed  time.Time
}

type evIntro struct{}

func introTick() tea.Cmd {
	return tea.Tick(time.Second/introFPS, func(time.Time) tea.Msg { return evIntro{} })
}

// SetIntro turns the startup animation on; Start does it for a real terminal.
func (m *Model) SetIntro(on bool) {
	if !on {
		m.intro = nil
		return
	}
	m.intro = &introState{spring: harmonica.NewSpring(harmonica.FPS(introFPS), 7.5, 0.28), pos: introDrop}
}

// stepIntro advances the spring; false when the intro is over.
func (m *Model) stepIntro() bool {
	in := m.intro
	now := time.Now()
	if in.started.IsZero() {
		in.started = now
	}
	in.pos, in.vel = in.spring.Update(in.pos, in.vel, 0)
	if in.landed.IsZero() && math.Abs(in.pos) < 0.3 && math.Abs(in.vel) < 0.8 {
		in.landed = now
	}
	done := (!in.landed.IsZero() && now.Sub(in.landed) > introHold) || now.Sub(in.started) > introMax
	return !done
}

// introView draws the mascot at its spring position, two pixels per row, where the welcome box
// will hold it (the box's border and padding: two columns in).
func (m *Model) introView() string {
	in := m.intro
	off := int(math.Round(introDrop - in.pos))
	if off < 0 {
		off = 0
	}
	sp := mascotFor(m.words.Dist)
	pixels := sp.pixels
	if off > introDrop {
		// past its resting place the mascot squashes instead of sinking: a row shorter, the base
		// a pixel wider each side
		off = introDrop + 1
		pixels = squash(pixels)
	}
	px := make([]string, 0, introDrop+len(sp.pixels)+2)
	blank := strings.Repeat(".", len(pixels[0]))
	for i := 0; i < off; i++ {
		px = append(px, blank)
	}
	blink := !in.landed.IsZero() && time.Since(in.landed) > 150*time.Millisecond && time.Since(in.landed) < 260*time.Millisecond
	for _, row := range pixels {
		if blink {
			row = strings.Map(func(r rune) rune {
				if r == 'E' {
					return rune(sp.body)
				}
				return r
			}, row)
		}
		px = append(px, row)
	}
	for len(px) < introDrop+len(sp.pixels) {
		px = append(px, blank)
	}
	rows := halfBlocks(px, sp.palette)
	for i := range rows {
		rows[i] = "  " + rows[i]
	}
	return strings.Join(rows, "\n")
}

// squash drops the sprite's top row and widens its bottom row by a pixel each side.
func squash(px []string) []string {
	out := append([]string(nil), px[1:]...)
	last := []byte(out[len(out)-1])
	for i := range last {
		if last[i] != '.' {
			if i > 0 {
				last[i-1] = last[i]
			}
			break
		}
	}
	for i := len(last) - 1; i >= 0; i-- {
		if last[i] != '.' {
			if i < len(last)-1 {
				last[i+1] = last[i]
			}
			break
		}
	}
	out[len(out)-1] = string(last)
	return out
}

// Gradient colours s rune by rune along the stops.
func Gradient(s string, bold bool, stops ...color.Color) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	cols := lipgloss.Blend1D(max(len(runes), 2), stops...)
	var sb strings.Builder
	for i, r := range runes {
		st := lipgloss.NewStyle().Foreground(cols[i])
		if bold {
			st = st.Bold(true)
		}
		sb.WriteString(st.Render(string(r)))
	}
	return sb.String()
}

// Shimmer draws s with a soft highlight centred on phase (a rune index that sweeps past both
// ends), the base colour elsewhere.
func (t Theme) Shimmer(s string, phase int) string {
	return t.shimmerText(s, phase, t.text.GetForeground(), t.accent.GetForeground())
}

// shimmerText is Shimmer over an explicit base and principles — a Subagent row's verb sweeps its rank's
// lane instead of the global accent.
func (t Theme) shimmerText(s string, phase int, base, hi color.Color) string {
	runes := []rune(s)
	if base == nil || base == (lipgloss.NoColor{}) {
		base = lipgloss.Color("#c9d1d9")
		if !t.Dark {
			base = lipgloss.Color("#24292f")
		}
	}
	if hi == nil || hi == (lipgloss.NoColor{}) {
		hi = t.accent.GetForeground()
	}
	ramp := lipgloss.Blend1D(5, base, hi)
	var sb strings.Builder
	for i, r := range runes {
		d := i - phase
		if d < 0 {
			d = -d
		}
		c := base
		if d < 4 {
			c = ramp[4-d]
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(c).Render(string(r)))
	}
	return sb.String()
}

// shimmerPhase maps a tick count onto a sweep that runs a few runes past each end.
func shimmerPhase(tick, n int) int {
	span := n + 8
	return tick%span - 4
}

// Meter is a context meter: a bubbles progress bar at the given cell width, half-block
// precise, the fill tinted with the 3-stop zone blend (green → amber → red) so the gauge
// still says where the run stands. pct < 0 renders the empty track (ctx unknown).
func (t Theme) Meter(pct, cells int) string {
	// a 101-stop zone ramp (green → amber → red); the fill takes the colour of how full the
	// gauge is, so a soothing low run and a sweating high run read at a glance
	ramp := lipgloss.Blend1D(101, t.ok.GetForeground(), t.warn.GetForeground(), t.bad.GetForeground())
	bar := progress.New(
		progress.WithColorFunc(func(total, current float64) color.Color {
			if total <= 0 {
				return ramp[0]
			}
			i := int(current / total * 100)
			if i > 100 {
				i = 100
			}
			return ramp[i]
		}),
		progress.WithWidth(cells),
		progress.WithoutPercentage(),
	)
	bar.FullColor = t.ok.GetForeground()
	bar.EmptyColor = t.border.GetForeground()
	return bar.ViewAs(float64(pct) / 100)
}

func visibleLen(s string) int { return ansi.StringWidth(s) }

// ── the modal (N16 slice 3) ──

const (
	// the choice card floats over the darkened frame given this much room (crush's fullscreen
	// tripwire, one cell of slack); below it the card flows inline as always
	choiceMinW = 78
	choiceMinH = 21
	// choiceDrop: the enter animation's fall, in rows, over the first two ticks of the card's
	// life — coarse at 5fps by design: a bounce, not a film
	choiceDrop = 2
)

// centerRect places a bw×bh box in a w×h frame, clamped to the frame (crush's CenterRect math).
func centerRect(w, h, bw, bh int) (x, y int) {
	x = (w - bw) / 2
	if x < 0 {
		x = 0
	}
	y = (h - bh) / 2
	if y < 0 {
		y = 0
	}
	return x, y
}

// dropOffset is the enter animation's remaining fall at the card's age: 2 rows, then 1, then
// settled. It always ends — the 8s motion policy never gets a say past 400ms.
func dropOffset(since time.Duration) int {
	step := time.Second / pulseFPS
	switch {
	case since < step:
		return choiceDrop
	case since < 2*step:
		return 1
	}
	return 0
}

// ── the verdict burst (N16 slice 4) ──
//
// A gate verdict after Subagents ran rains the subagent glyph over the frame for a beat: pass in
// the ok colour, fail in the bad. Self-arming, self-expiring — six ticks, far under the 8s
// motion policy.

const (
	burstTicks = 6 // the fall's life in evTicks (1.2s)
	burstGlyph = "◆"
)

// ── the subagent pulse (N15/N16, slice 1) ──
//
// A live Subagent row's motion rides the tick that already ages its "· Ns" field: no new event
// flow. Each carried state holds its rank's lane — thinking sweeps the shimmer, a tool keeps
// the spinner's frames, waiting on the gate is a slow spring bob, and a landing flashes once
// full-row before the tombstone prints. Past 8s without a word from the agent every continuous
// motion cools to a quarter-rate breath; the landing flash is an event, not idle decor, and
// always fires.

const (
	pulseFPS = 5 // the evTick cadence the pulse rides
	// pulseIdleCool: an agent with no fresh event for this long is cooled — motion drops to a
	// quarter of its rate instead of firing at full speed forever.
	pulseIdleCool = 8 * time.Second
	// flashHold: a landed Subagent's row flashes in the live area this long, then the tick sweep
	// owes it one tombstone (the turn's end may print it sooner).
	flashHold = 700 * time.Millisecond
)

// the gate-wait bob: a slow underdamped spring, re-kicked while it rests so a long wait keeps
// breathing; cooled waits settle at the edge.
const (
	gateBobFreq = 4.5  // rad/s
	gateBobDamp = 0.32 // underdamped: one visible overshoot, then rest
	gateBobKick = 6.0  // entry velocity, ≈1.2 columns of travel
	gateBobCols = 2.0  // the bob's max displacement off the right edge
)

// subagentPulse is one Subagent's motion state; provisioned with its card, dies when the tombstone prints.
type subagentPulse struct {
	state string
	tool  string // the tool's name while state is "tool"
	since time.Time
	// active is the agent's last word (any event); the idle cool reads it.
	active time.Time
	spring harmonica.Spring
	bob    bool
	bobPos float64
	bobVel float64
	doneAt time.Time // the first terminal sighting: the flash clock
}

// enter records a (possibly changed) state; transitions restart the phase clock and arm the
// state's own motion — the spring kicks on gate-wait, the flash clock starts on done.
func (p *subagentPulse) enter(state, tool string, now time.Time) {
	if p.doneAt.IsZero() && state == "done" {
		p.doneAt = now
	}
	if p.state == state && (state != "tool" || p.tool == tool) {
		return
	}
	p.state, p.tool = state, tool
	if state != "done" {
		p.since = now
	}
	switch state {
	case "waiting on gate":
		p.spring = harmonica.NewSpring(harmonica.FPS(pulseFPS), gateBobFreq, gateBobDamp)
		p.bob, p.bobPos, p.bobVel = true, 0, gateBobKick
	default:
		p.bob, p.bobPos, p.bobVel = false, 0, 0
	}
}

func (m *Model) pulseOf(name string) *subagentPulse {
	if m.pulses == nil {
		m.pulses = map[string]*subagentPulse{}
	}
	p := m.pulses[name]
	if p == nil {
		now := time.Now()
		p = &subagentPulse{since: now, active: now}
		m.pulses[name] = p
	}
	return p
}

// notePulse records a Subagent's state for its live row's motion; tool names the tool of "tool".
func (m *Model) notePulse(name, state, tool string) {
	if name == "" || m.subagents[name] == nil {
		return
	}
	now := time.Now()
	p := m.pulseOf(name)
	p.enter(state, tool, now)
	p.active = now
}

// stepPulses advances the gate-wait springs on the tick; a cooled agent's bob settles at rest.
func (m *Model) stepPulses(now time.Time) {
	for _, p := range m.pulses {
		if !p.bob {
			continue
		}
		if now.Sub(p.active) > pulseIdleCool {
			p.bobPos, p.bobVel = 0, 0
			continue
		}
		p.bobPos, p.bobVel = p.spring.Update(p.bobPos, p.bobVel, 0)
		if math.Abs(p.bobPos) < 0.05 && math.Abs(p.bobVel) < 0.08 {
			p.bobVel = gateBobKick // settled while the gate still holds: bob again
		}
	}
}

// pulseFor is the model's half of the policy: it turns a live Subagent's motion state into the Pulse
// the block holds. The block stays a pure function of (view, width, pulse).
func (m *Model) pulseFor(c *SubagentView) Pulse {
	p := m.pulses[c.Name]
	if p == nil {
		return Pulse{}
	}
	lane := m.theme.lane(c.Rank)
	if lane == nil {
		lane = m.theme.accent.GetForeground()
	}
	now := time.Now()
	out := Pulse{Lane: lane}
	if c.State == "done" {
		if !p.doneAt.IsZero() {
			if age := now.Sub(p.doneAt); age < flashHold {
				out.Flash = float64(flashHold-age) / float64(flashHold)
			}
		}
		return out
	}
	// phases are the state's age at the tick's rate; past the idle cool they advance at a
	// quarter rate — the row breathes instead of sweeping, and the cooled frame itself says so.
	age := now.Sub(p.since)
	if age < 0 {
		age = 0
	}
	step := time.Second / pulseFPS
	frames := int(age / step)
	if now.Sub(p.active) > pulseIdleCool {
		cooled := age - pulseIdleCool
		if cooled < 0 {
			cooled = 0
		}
		frames = int(pulseIdleCool/step) + int(cooled/(4*step))
	}
	out.Phase = frames
	if ic := m.theme.Icon(m.words.Dist, c.Rank, frames/4); len(ic) == 2 {
		out.Principles = [2]string{ic[0], ic[1]} // the rank's sprite beside the live card (slice 4)
	}
	switch c.State {
	case "tool":
		out.Tool = p.tool
		if f := m.spin.Spinner.Frames; len(f) > 0 {
			out.Frames = f
		}
	case "waiting on gate":
		out.Bob = p.bobPos
	}
	return out
}

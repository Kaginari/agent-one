package value

import (
	"fmt"
	"math"
)

// Bands are a meter's thresholds: at or past OK the reading is ok, at or past Warn it warns,
// short of Warn it is bad (direction up); direction down reverses the walk (ok at or under OK).
type Bands struct {
	OK   float64 `json:"ok"`
	Warn float64 `json:"warn"`
}

// MeterDef is a meter as config writes it (ranks.<r>.value.meter, ui.meters.<name>).
type MeterDef struct {
	Signal    string `json:"signal"`
	Label     string `json:"label"`
	Scale     string `json:"scale"` // percent | count | momentum
	Bands     Bands  `json:"bands"`
	Direction string `json:"direction"` // up (more is better) | down (less is better)
}

// Meter is one rendered reading — what every surface draws, computed once here.
type Meter struct {
	Signal string  `json:"signal"`
	Label  string  `json:"label"`
	Scale  string  `json:"scale"`
	Value  float64 `json:"value"` // the signal's reading
	Pct    float64 `json:"pct"`   // the fill, 0..100
	Band   string  `json:"band"`  // ok | warn | bad | none (no basis)
	Text   string  `json:"text"`  // the reading as a surface prints it (82%, 14, +1.5)
	Bands  Bands   `json:"bands"`
	Dir    string  `json:"direction"`
}

// Band names.
const (
	BandOK   = "ok"
	BandWarn = "warn"
	BandBad  = "bad"
	BandNone = "none"
)

// CheckMeter refuses a meter config cannot honour.
func CheckMeter(m MeterDef) error {
	if m.Signal == "" {
		return fmt.Errorf("a meter names its signal")
	}
	if m.Signal != "score" && m.Signal != "ctx" && m.Signal != "plan" && !IsSignal(m.Signal) {
		return fmt.Errorf("unknown signal %q", m.Signal)
	}
	switch m.Scale {
	case "", "percent", "count", "momentum":
	default:
		return fmt.Errorf("scale %q is not percent | count | momentum", m.Scale)
	}
	switch m.Direction {
	case "", "up", "down":
	default:
		return fmt.Errorf("direction %q is not up | down", m.Direction)
	}
	if m.Direction == "down" && m.Bands.OK > m.Bands.Warn {
		return fmt.Errorf("bands: a falling meter's ok (%g) sits at or under its warn (%g)", m.Bands.OK, m.Bands.Warn)
	}
	if m.Direction != "down" && m.Bands.OK < m.Bands.Warn {
		return fmt.Errorf("bands: a rising meter's ok (%g) sits at or over its warn (%g)", m.Bands.OK, m.Bands.Warn)
	}
	return nil
}

// Render reads one meter against a value. basis false (no record) bands it none.
func Render(d MeterDef, v float64, basis bool) Meter {
	scale := d.Scale
	if scale == "" {
		scale = "percent"
		if s, ok := Lookup(d.Signal); ok {
			scale = s.Scale
		}
	}
	m := Meter{Signal: d.Signal, Label: d.Label, Scale: scale, Value: v, Bands: d.Bands, Dir: d.Direction}
	if m.Label == "" {
		m.Label = d.Signal
	}
	if m.Dir == "" {
		m.Dir = "up"
	}
	switch scale {
	case "percent":
		m.Pct = Clamp(v)
		m.Text = fmt.Sprintf("%.0f%%", v)
	case "momentum":
		// momentum lives around zero: -5..+5 maps to the bar
		m.Pct = Clamp(50 + v*10)
		m.Text = fmt.Sprintf("%+.1f", v)
	default:
		top := math.Max(d.Bands.OK, d.Bands.Warn)
		if top <= 0 {
			top = 1
		}
		m.Pct = Clamp(100 * v / top)
		m.Text = fmt.Sprintf("%.0f", v)
	}
	switch {
	case !basis:
		m.Band = BandNone
	case m.Dir == "down":
		switch {
		case v <= d.Bands.OK:
			m.Band = BandOK
		case v <= d.Bands.Warn:
			m.Band = BandWarn
		default:
			m.Band = BandBad
		}
	default:
		switch {
		case v >= d.Bands.OK:
			m.Band = BandOK
		case v >= d.Bands.Warn:
			m.Band = BandWarn
		default:
			m.Band = BandBad
		}
	}
	return m
}

package worldcore

import (
	"math"
	"slices"
	"strings"
)

// Glacier is the world:glacier payload (01 bus) and its persisted state: Mass ∈ [0, 1] (snowfall from gitops
// commits, a daily melt), one tongue per allowlisted app (advances per successful sync, calves on app.gone),
// and the ash bands in the ice (the ash days, newest first).
type Glacier struct {
	Mass    float64  `json:"mass"`
	Tongues []Tongue `json:"tongues"`
	Ash     []string `json:"ash"`
}

type Tongue struct {
	App string  `json:"app"`
	Len float64 `json:"len"` // metres down-slope from the cap
}

// Ash is one ash line (world:ash payload): an ADR merge day, Title only when the relay's title allowlist passed it.
type Ash struct {
	Day   string `json:"day"`
	Title string `json:"title,omitempty"`
}

// ponytail: tuning knobs, not physics. ~10 commits/day holds the mass near .8; a quiet month melts it to ~.2.
const (
	snowPerCommit = 0.004
	meltPerDay    = 0.95 // mass and tongue length kept per UTC day
	tongueStep    = 1.5  // m per successful sync
	tongueMax     = 12.0
	AshMax        = 32 // ash lines and ice bands kept (strata draws ≤ 32)
	VeinsMax      = 32 // capsules kept (veins draws ≤ 32)
)

func NewGlacier() Glacier { return Glacier{Mass: 0.5, Tongues: []Tongue{}, Ash: []string{}} }

// Snowline is the metrics snowline (world y) for a glacier mass: 0.8 × hmax at the starting mass .5, lower when it grows.
func Snowline(hmax, mass float64) float64 { return round2(hmax * (0.9 - 0.2*mass)) }

func (g Glacier) clone() Glacier {
	g.Tongues, g.Ash = slices.Clone(g.Tongues), slices.Clone(g.Ash)
	return g
}

// Snow adds n commits of snowfall.
func (g Glacier) Snow(n int) Glacier {
	g = g.clone()
	g.Mass = round2(math.Min(1, g.Mass+snowPerCommit*float64(n)))
	return g
}

// Advance grows app's tongue (a new one starts at one step).
func (g Glacier) Advance(app string) Glacier {
	g = g.clone()
	if i := slices.IndexFunc(g.Tongues, func(t Tongue) bool { return t.App == app }); i >= 0 {
		g.Tongues[i].Len = round2(math.Min(tongueMax, g.Tongues[i].Len+tongueStep))
	} else {
		g.Tongues = append(g.Tongues, Tongue{App: app, Len: tongueStep})
	}
	return g
}

// Calve drops app's tongue; ok = it had one (the client calves the chunks).
func (g Glacier) Calve(app string) (Glacier, bool) {
	g = g.clone()
	n := len(g.Tongues)
	g.Tongues = slices.DeleteFunc(g.Tongues, func(t Tongue) bool { return t.App == app })
	return g, len(g.Tongues) < n
}

// Melt is one UTC day of ablation: mass and tongues shrink, tongues under half a step are gone.
func (g Glacier) Melt() Glacier {
	g = g.clone()
	g.Mass = round2(g.Mass * meltPerDay)
	for i := range g.Tongues {
		g.Tongues[i].Len = round2(g.Tongues[i].Len * meltPerDay)
	}
	g.Tongues = slices.DeleteFunc(g.Tongues, func(t Tongue) bool { return t.Len < tongueStep/2 })
	return g
}

// AddAsh lays an ash band for day (newest first, ≤ AshMax); ok = it was new.
func (g Glacier) AddAsh(day string) (Glacier, bool) {
	if slices.Contains(g.Ash, day) {
		return g, false
	}
	g = g.clone()
	g.Ash = append(g.Ash, day)
	slices.SortFunc(g.Ash, func(a, b string) int { return strings.Compare(b, a) })
	g.Ash = g.Ash[:min(len(g.Ash), AshMax)]
	return g, slices.Contains(g.Ash, day) // a day older than a full list is trimmed: no change
}

// AddAshLine adds a to the ash lines (newest day first, ≤ AshMax; same day + title once); ok = it was new.
func AddAshLine(l []Ash, a Ash) ([]Ash, bool) {
	if slices.Contains(l, a) {
		return l, false
	}
	l = append(slices.Clone(l), a)
	slices.SortStableFunc(l, func(x, y Ash) int { return strings.Compare(y.Day, x.Day) })
	l = l[:min(len(l), AshMax)]
	return l, slices.Contains(l, a) // a day older than a full list is trimmed: no change
}

// AddVein adds v (the same fp keeps its seam and the thicker radius; ≤ VeinsMax, oldest dropped).
func AddVein(l []Vein, v Vein) []Vein {
	if i := slices.IndexFunc(l, func(x Vein) bool { return x.FP == v.FP }); i >= 0 {
		v.A, v.B, v.Thick = l[i].A, l[i].B, math.Max(v.Thick, l[i].Thick)
		l = slices.Delete(slices.Clone(l), i, i+1)
	}
	l = append(slices.Clone(l), v)
	return l[max(0, len(l)-VeinsMax):]
}

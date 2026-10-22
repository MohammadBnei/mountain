package main

import (
	"math"
	"testing"
	"time"
)

// TestSunElev: the Go port matches www/js/sun.js (vectors printed by sun() for these instants), and the plant item
// follows the ±6° bands.
func TestSunElev(t *testing.T) {
	for iso, want := range map[string]float64{
		"2026-10-22T12:00:00Z": 29.906613,
		"2026-10-22T16:30:00Z": 2.143272,
		"2026-06-21T03:30:00Z": -2.986227,
		"2026-12-21T23:00:00Z": -62.803617,
		"2026-03-20T06:10:00Z": 1.634810,
	} {
		at, _ := time.Parse(time.RFC3339, iso)
		if got := sunElev(at, parisLat, parisLon); math.Abs(got-want) > 1e-5 {
			t.Errorf("%s: elev %.6f, js %.6f", iso, got, want)
		}
	}
	for iso, want := range map[string]int{"2026-10-22T12:00:00Z": itemTree, "2026-10-22T16:30:00Z": itemFlag, "2026-12-21T23:00:00Z": itemLantern} {
		at, _ := time.Parse(time.RFC3339, iso)
		if got := plantItem(at); got != want {
			t.Errorf("%s: item %d, want %d", iso, got, want)
		}
	}
}

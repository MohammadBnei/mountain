package main

// The real sun over Paris, server side (R7 plant toy): the same SunCalc low-precision formula as www/js/sun.js
// (mourner/suncalc, BSD-2), elevation only. sun_test.go pins it to vectors from the JS.

import (
	"math"
	"time"
)

const (
	rad       = math.Pi / 180
	obliquity = rad * 23.4397
	parisLat  = 48.8566
	parisLon  = 2.3522
)

// sunNow is the clock the plant toy reads the sun from: its own, so a pinned sun (tests, fixtures) never moves the
// rate-limit clock.
var sunNow = time.Now

// sunElev is the sun's elevation in degrees at t over (lat, lon).
func sunElev(t time.Time, lat, lon float64) float64 {
	d := float64(t.UnixMilli())/864e5 - 10957.5 // days since J2000
	m := rad * (357.5291 + 0.98560028*d)        // solar mean anomaly
	l := m + rad*(1.9148*math.Sin(m)+0.02*math.Sin(2*m)+0.0003*math.Sin(3*m)) + rad*102.9372 + math.Pi
	ra := math.Atan2(math.Sin(l)*math.Cos(obliquity), math.Cos(l))
	dec := math.Asin(math.Sin(obliquity) * math.Sin(l))
	h := rad*(280.16+360.9856235*d+lon) - ra
	phi := rad * lat
	return math.Asin(math.Sin(phi)*math.Sin(dec)+math.Cos(phi)*math.Cos(dec)*math.Cos(h)) / rad
}

// Plant items (R7): what a visitor plants depends on the real sun at Paris.
const (
	itemTree    = 0 // day: elevation above +6°
	itemFlag    = 1 // dawn / dusk: −6°..+6° (civil twilight to golden hour)
	itemLantern = 2 // night: below −6°
)

func plantItem(t time.Time) int {
	switch e := sunElev(t, parisLat, parisLon); {
	case e > 6:
		return itemTree
	case e >= -6:
		return itemFlag
	}
	return itemLantern
}

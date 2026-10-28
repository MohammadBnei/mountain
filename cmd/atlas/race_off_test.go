//go:build !race

package main

// raceSlowdown: see race_on_test.go. Plain builds keep the bound as written.
const raceSlowdown = 1

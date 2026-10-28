//go:build race

package main

// raceSlowdown scales a test's wall-clock bound. The race detector runs this package's parser work about twelve
// times slower (TestRenderBounds: 0.5 s plain, 6.4 s under -race), so a bound written for the plain build fails
// CI's -race run with nothing wrong in the code; 15 leaves room for a slower runner.
const raceSlowdown = 15

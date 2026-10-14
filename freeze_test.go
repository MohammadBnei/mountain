package mountain

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// TestFrozen pins the W2 freeze (plan/README W2, 01 "FROZEN W2"): contract.js, world.js (bus registry, world.rig,
// event enum), layout.js, units.json, the kit (kit.js + tokens), the wire schema (metrics frame included). Each
// line of FROZEN.sha256 is `shasum -a 256` output. A change to one of these files is an announced, deliberate
// change, never a side effect: regenerate the list in the same commit and say so in the PR.
func TestFrozen(t *testing.T) {
	b, err := os.ReadFile("FROZEN.sha256")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		want, file, ok := strings.Cut(l, "  ")
		if !ok {
			t.Fatalf("FROZEN.sha256: bad line %q", l)
		}
		files = append(files, file)
		src, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("frozen file %s: %v", file, err)
			continue
		}
		if s := sha256.Sum256(src); hex.EncodeToString(s[:]) != want {
			t.Errorf("%s is frozen (W2) and has changed", file)
		}
	}
	if t.Failed() {
		t.Logf("if the change is deliberate and announced, update the list in the same commit:\n  shasum -a 256 %s > FROZEN.sha256", strings.Join(files, " "))
	}
}

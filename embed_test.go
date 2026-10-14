package mountain

import (
	"bytes"
	"os"
	"testing"
)

// The browser imports byte copies of the frozen JSON (embed only reaches www/).
func TestCopiesInSync(t *testing.T) {
	for src, dst := range map[string]string{"units.json": "www/js/units.json", "plan/style/tokens.json": "www/js/tokens.json"} {
		a, err1 := os.ReadFile(src)
		b, err2 := os.ReadFile(dst)
		if err1 != nil || err2 != nil || !bytes.Equal(a, b) {
			t.Errorf("%s is not a byte copy of %s (%v %v); run: cp %s %s", dst, src, err1, err2, src, dst)
		}
	}
}

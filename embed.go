// Package mountain holds the files embedded into the relay binary.
package mountain

import "embed"

// WWW is the static site (www/), served by the relay at /.
//
//go:embed www
var WWW embed.FS

// Units is units.json: shared Go/JS constants (frozen W2). www/js/units.json is a byte copy.
//
//go:embed units.json
var Units []byte

// Package mountain holds the files embedded into the atlas binary.
package mountain

import "embed"

// WWW is the static site (www/), served by atlas at /.
//
//go:embed www
var WWW embed.FS

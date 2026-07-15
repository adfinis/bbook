package static

import "embed"

//go:embed *.png *.css *.js
var FS embed.FS

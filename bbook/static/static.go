package static

import (
	"embed"
	"io/fs"
)

// assets/ holds the tracked images plus tailwind.css and the vendored js built
// by `npm run build`. Embedding the directory (rather than naming extensions)
// keeps a checkout without those build outputs compiling.
//
//go:embed assets
var embedded embed.FS

// FS serves the assets at the root, so /static/<file> keeps working.
var FS = func() fs.FS {
	sub, err := fs.Sub(embedded, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}()

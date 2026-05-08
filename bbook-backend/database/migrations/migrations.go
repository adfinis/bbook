package migrations

import "embed"

//go:embed *.up.sql *.down.sql
var FS embed.FS

package docs

import "embed"

//go:embed *.md
var Content embed.FS

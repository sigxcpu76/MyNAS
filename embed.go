package nas

//go:generate go run generate.go

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var embeddedWebDist embed.FS

// WebUIFS returns the embedded Vue.js frontend filesystem rooted at web/dist.
func WebUIFS() (fs.FS, error) {
	return fs.Sub(embeddedWebDist, "web/dist")
}

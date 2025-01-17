package static

// https://pkg.go.dev/embed
import "embed"

// wget https://andybrewer.github.io/mvp/mvp.css

//go:embed index.html app.js mvp.css public-config.json
var FS embed.FS
var Prefix = ""

// Package web serves the embedded environment directory UI.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:public
var files embed.FS

// Handler serves built files and falls back to the SPA shell
func Handler() http.Handler {
	root, _ := fs.Sub(files, "public")
	shell, _ := fs.ReadFile(root, "200.html")
	static := http.FileServerFS(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(root, name); err == nil {
			if strings.HasPrefix(name, "_app/immutable/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			static.ServeHTTP(w, r)
			return
		}
		if shell == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(shell)
	})
}

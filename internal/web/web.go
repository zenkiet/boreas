// Package web serves the embedded environment directory UI.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
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
	etags := map[string]string{}
	_ = fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			if body, err := fs.ReadFile(root, name); err == nil {
				sum := sha256.Sum256(body)
				etags[name] = `"` + hex.EncodeToString(sum[:12]) + `"`
			}
		}
		return nil
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(root, name); err == nil {
			if strings.HasPrefix(name, "_app/immutable/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else if etag, ok := etags[name]; ok {
				// Embedded files have no modification time, so without an ETag every page load downloads them again.
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("ETag", etag)
			}
			static.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(root, name+".html"); err == nil {
			http.ServeFileFS(w, r, root, name+".html")
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

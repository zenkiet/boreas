// Package web serves the embedded environment directory UI.
package web

import (
	"cmp"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed all:public
var files embed.FS

// encodings are the build's precompressed siblings, best first.
var encodings = [...]struct{ token, ext string }{{"br", ".br"}, {"gzip", ".gz"}}

// Handler serves built files and falls back to the SPA shell
func Handler() http.Handler {
	root, _ := fs.Sub(files, "public")
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

	send := func(w http.ResponseWriter, r *http.Request, name string) {
		header := w.Header()
		if strings.HasPrefix(name, "_app/immutable/") {
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// Embedded files have no modification time, so without an ETag every page load downloads them again.
			header.Set("Cache-Control", "no-cache")
		}
		file := name
		for _, e := range encodings {
			if etags[name+e.ext] == "" {
				continue
			}
			header.Set("Vary", "Accept-Encoding")
			if file == name && strings.Contains(r.Header.Get("Accept-Encoding"), e.token) {
				file = name + e.ext
				header.Set("Content-Encoding", e.token)
				header.Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
			}
		}
		header.Set("ETag", etags[file])
		http.ServeFileFS(w, r, root, file)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := cmp.Or(strings.TrimPrefix(path.Clean(r.URL.Path), "/"), "index.html")
		switch {
		case etags[name] != "":
		case etags[name+".html"] != "":
			name += ".html"
		// A file-like path can never be a page (project slugs have no dots), so it gets a real 404, not the shell.
		case strings.HasPrefix(name, ".well-known/"), !strings.Contains(name, "/") && strings.Contains(name, "."), etags["200.html"] == "":
			http.NotFound(w, r)
			return
		default:
			name = "200.html"
		}
		send(w, r, name)
	})
}

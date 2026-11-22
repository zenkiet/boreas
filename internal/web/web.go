// Package web serves the embedded environment directory UI.
package web

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:public
var files embed.FS

// encodings are the build's precompressed siblings, best first.
var encodings = [...]struct{ token, ext string }{{"br", ".br"}, {"gzip", ".gz"}}

const SupportTag = `<script src="/_boreas/support.js" async fetchpriority="low"></script>`

type SupportConfig struct {
	Desk    string            `json:"desk"`
	Inboxes map[string]string `json:"inboxes"`
}

func etag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:12]) + `"`
}

//go:embed support.js
var supportScript []byte

func Support(cfg SupportConfig) http.Handler {
	var body []byte
	if len(cfg.Inboxes) > 0 {
		config, _ := json.Marshal(cfg)
		body = fmt.Appendf(nil, "self.boreasSupport=%s;\n%s", config, supportScript)
	}
	tag := etag(body)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", tag)
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
	})
}

// Handler serves built files and falls back to the SPA shell
func Handler() http.Handler {
	root, _ := fs.Sub(files, "public")
	etags := map[string]string{}
	_ = fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			if body, err := fs.ReadFile(root, name); err == nil {
				etags[name] = etag(body)
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

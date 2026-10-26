package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zenkiet/boreas/internal/core"
)

// The bucket is addressed by path and every request is signed with the payload hash S3 requires.
func TestStoreRoundTrip(t *testing.T) {
	objects := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=key/") ||
			!strings.Contains(r.Header.Get("Authorization"), "x-amz-content-sha256") {
			http.Error(w, "unsigned", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodPut:
			objects[r.URL.Path], _ = io.ReadAll(r.Body)
		case http.MethodGet:
			body, ok := objects[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(body)
		case http.MethodDelete:
			delete(objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	s, ctx := New(srv.URL+"/", "shots", "", "key", "secret"), context.Background()

	if err := s.Put(ctx, "bora/u/1", "image/png", []byte("png")); err != nil {
		t.Fatal(err)
	}
	if body, mime, err := s.Get(ctx, "bora/u/1"); err != nil || !bytes.Equal(body, []byte("png")) || mime != "image/png" {
		t.Fatalf("get %q %q %v", body, mime, err)
	}
	if _, ok := objects["/shots/bora/u/1"]; !ok {
		t.Fatalf("stored under %v, want the bucket in the path", objects)
	}
	if err := s.Delete(ctx, "bora/u/1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get(ctx, "bora/u/1"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
}

package proxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"html/template"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/time/rate"
)

const (
	lockCookie = "boreas_lock"
	lockTTL    = 24 * time.Hour
)

//go:embed lock
var lockFiles embed.FS

var lockPage = template.Must(template.ParseFS(lockFiles, "lock/*"))

type lock struct {
	task, path string
	hash       []byte
	tries      *rate.Limiter
}

func (r *Registry) SetLock(project, task, hash string) {
	var l *lock
	if hash != "" {
		l = &lock{
			task: task, path: "/" + project + "/" + task + "/", hash: []byte(hash),
			tries: rate.NewLimiter(rate.Every(30*time.Second), 10),
		}
	}
	r.update(project, task, func(s *site) { s.lock = l })
}

func (l *lock) admits(w http.ResponseWriter, request *http.Request) bool {
	if cookie, err := request.Cookie(lockCookie); err == nil && l.valid(cookie.Value, time.Now()) {
		return true
	}
	status, message, mood := http.StatusUnauthorized, "", ""
	if request.Method == http.MethodPost {
		request.Body = http.MaxBytesReader(w, request.Body, 1<<10)
		if password := request.PostFormValue("boreas_password"); password != "" {
			switch {
			case !l.tries.Allow():
				status, message, mood = http.StatusTooManyRequests, "Too many tries. Wait a minute, then try again.", "wait"
			case bcrypt.CompareHashAndPassword(l.hash, []byte(password)) == nil:
				http.SetCookie(w, &http.Cookie{
					Name: lockCookie, Value: l.sign(time.Now().Add(lockTTL)), Path: l.path, MaxAge: int(lockTTL.Seconds()),
					HttpOnly: true, SameSite: http.SameSiteLaxMode,
					Secure: request.TLS != nil || request.Header.Get("X-Forwarded-Proto") == "https",
				})
				http.Redirect(w, request, request.URL.RequestURI(), http.StatusSeeOther)
				return false
			default:
				message, mood = "Wrong password.", "wrong"
			}
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = lockPage.ExecuteTemplate(w, "index.html", map[string]string{"Task": l.task, "Message": message, "Mood": mood})
	return false
}

func (l *lock) sign(expires time.Time) string {
	at := strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, l.hash)
	mac.Write([]byte(at))
	return at + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (l *lock) valid(value string, now time.Time) bool {
	at, _, _ := strings.Cut(value, ".")
	expires, err := strconv.ParseInt(at, 10, 64)
	return err == nil && now.Unix() < expires && hmac.Equal([]byte(value), []byte(l.sign(time.Unix(expires, 0))))
}

func dropLockCookie(header http.Header) {
	line := strings.Join(header["Cookie"], "; ")
	if !strings.Contains(line, lockCookie+"=") {
		return
	}
	pairs := slices.DeleteFunc(strings.Split(line, ";"), func(pair string) bool {
		pair = strings.TrimSpace(pair)
		return pair == "" || strings.HasPrefix(pair, lockCookie+"=")
	})
	header.Del("Cookie")
	if len(pairs) > 0 {
		header.Set("Cookie", strings.TrimSpace(strings.Join(pairs, ";")))
	}
}

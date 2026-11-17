package httptransport

import (
	"context"
	"net/http"
	"strings"

	"github.com/zenkiet/boreas/internal/core"
)

type contextKey int

const (
	userContextKey contextKey = iota
	accessContextKey
)

func userFrom(ctx context.Context) core.User {
	user, _ := ctx.Value(userContextKey).(core.User)
	return user
}

// accessFrom returns what authorize resolved; only project-scoped routes populate it.
func accessFrom(ctx context.Context) core.ProjectAccess {
	acc, _ := ctx.Value(accessContextKey).(core.ProjectAccess)
	return acc
}

func bearerToken(r *http.Request) string {
	if value, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); found {
		return strings.TrimSpace(value)
	}
	return ""
}

func (h *Handler) authorize(required access, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, kind, err := h.auth.Authenticate(r.Context(), bearerToken(r))
		if err != nil {
			writeServiceError(w, h.logger, err)
			return
		}
		if (required == accessAdmin && !user.IsAdmin()) || (required == accessSession && kind != core.TokenKindSession) {
			writeServiceError(w, h.logger, core.ErrForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		if kind == core.TokenKindSession {
			ctx = core.WithActor(ctx, user.ID)
		}
		if need := projectRoles[required]; need != "" {
			acc, err := h.projects.Access(ctx, user, r.PathValue("project"), r.PathValue("name"))
			if err == nil && acc.Role.Rank() < need.Rank() {
				err = core.ErrForbidden
			}
			if err != nil {
				writeServiceError(w, h.logger, err)
				return
			}
			ctx = context.WithValue(ctx, accessContextKey, acc)
		}
		next(w, r.WithContext(ctx))
	}
}

func deviceLabel(ua string) string {
	system := firstKnown(ua, [][2]string{
		{"iPhone", "iPhone"},
		{"iPad", "iPad"},
		{"Android", "Android"},
		{"CrOS", "ChromeOS"},
		{"Macintosh", "Mac"},
		{"Windows", "Windows"},
		{"Linux", "Linux"},
	})
	browser := firstKnown(ua, [][2]string{
		{"Edg/", "Edge"},
		{"Firefox/", "Firefox"},
		{"FxiOS/", "Firefox"},
		{"CriOS/", "Chrome"},
		{"Chrome/", "Chrome"},
		{"Safari/", "Safari"},
	})
	switch {
	case strings.Contains(ua, "AppleWebKit") && browser == "" && system == "Mac":
		return "iPad"
	case browser == "" || strings.Contains(ua, "; wv)"):
		return system
	case system == "":
		return browser
	}
	return browser + " on " + system
}

func firstKnown(ua string, known [][2]string) string {
	for _, k := range known {
		if strings.Contains(ua, k[0]) {
			return k[1]
		}
	}
	return ""
}

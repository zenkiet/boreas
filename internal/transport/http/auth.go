package httptransport

import (
	"context"
	"errors"
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
	header := r.Header.Get("Authorization")
	if value, found := strings.CutPrefix(header, "Bearer "); found {
		return strings.TrimSpace(value)
	}
	return ""
}

func (h *Handler) authorize(required access, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			writeUnauthorized(w)
			return
		}
		user, kind, err := h.auth.Authenticate(r.Context(), token)
		if err != nil {
			if errors.Is(err, core.ErrUnauthorized) {
				writeUnauthorized(w)
				return
			}
			writeServiceError(w, h.logger, err)
			return
		}
		if required == accessAdmin && !user.IsAdmin() {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if required == accessSession && kind != core.TokenKindSession {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		if need := required.projectRole(); need != "" {
			acc, err := h.projects.Access(ctx, user, r.PathValue("project"), r.PathValue("name"))
			if err != nil {
				writeServiceError(w, h.logger, err)
				return
			}
			if acc.Role.Rank() < need.Rank() {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}
			ctx = context.WithValue(ctx, accessContextKey, acc)
		}
		next(w, r.WithContext(ctx))
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="Boreas"`)
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
}

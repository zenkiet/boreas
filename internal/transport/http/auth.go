package httptransport

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/zenkiet/boreas/internal/core"
)

type contextKey int

const userContextKey contextKey = iota

func userFrom(ctx context.Context) core.User {
	user, _ := ctx.Value(userContextKey).(core.User)
	return user
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
		user, err := h.auth.Authenticate(r.Context(), token)
		if err != nil {
			if errors.Is(err, core.ErrUnauthorized) {
				writeUnauthorized(w)
				return
			}
			writeServiceError(w, h.options.Logger, err)
			return
		}
		if required == accessAdmin && !user.IsAdmin() {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if required == accessMember || required == accessOwner {
			role, err := h.projects.Access(r.Context(), user, r.PathValue("project"))
			if err != nil {
				writeServiceError(w, h.options.Logger, err)
				return
			}
			if required == accessOwner && role != core.ProjectRoleOwner {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="Boreas"`)
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
}

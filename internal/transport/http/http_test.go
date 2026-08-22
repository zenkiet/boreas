package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
	"github.com/zenkiet/boreas/internal/service"
)

func testHandler(tasks TaskService, auth AuthService, projects ProjectService) http.Handler {
	if auth == nil {
		auth = &stubAuth{user: testAdmin}
	}
	if projects == nil {
		projects = &stubProjects{}
	}
	return APIHandler(tasks, auth, projects, log.New(io.Discard, "", 0))
}

func authed(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.Header.Set("Authorization", "Bearer "+testToken)
	return r
}

func do(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

func TestUnauthenticatedRequestsRejected(t *testing.T) {
	h := testHandler(stubTasks{}, nil, nil)
	paths := []string{
		"/api/v1/stats",
		"/api/v1/projects",
		"/api/v1/projects/team/tasks",
		"/api/v1/users",
	}
	for _, path := range paths {
		rr := do(h, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want 401", path, rr.Code)
		}
		if rr.Header().Get("WWW-Authenticate") == "" {
			t.Fatalf("%s missing WWW-Authenticate header", path)
		}
	}

	rr := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("health should stay public, got %d", rr.Code)
	}
}

func TestBadTokenRejected(t *testing.T) {
	h := testHandler(stubTasks{}, nil, nil)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	r.Header.Set("Authorization", "Bearer wrong")
	if rr := do(h, r); rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestLoginReturnsTokenAndHidesHash(t *testing.T) {
	auth := &stubAuth{user: core.User{
		ID: uuid.New(), Username: "alice", Email: "a@example.com",
		Role: core.RoleAdmin, PasswordHash: "$2a$10$topsecrethash",
	}}
	h := testHandler(stubTasks{}, auth, nil)
	rr := do(h, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"username":"alice","password":"pw"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), testToken) {
		t.Fatalf("token missing: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "topsecrethash") || strings.Contains(rr.Body.String(), "password_hash") {
		t.Fatalf("password hash leaked: %s", rr.Body.String())
	}
}

func TestLoginFailureReturns401(t *testing.T) {
	auth := &stubAuth{login: func(context.Context, string, string) (string, core.User, error) {
		return "", core.User{}, core.ErrUnauthorized
	}}
	h := testHandler(stubTasks{}, auth, nil)
	rr := do(h, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"username":"x","password":"y"}`)))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestAdminOnlyRoutesRejectRegularUsers(t *testing.T) {
	h := testHandler(stubTasks{}, &stubAuth{user: testMember}, nil)
	for _, path := range []string{"/api/v1/users", "/api/v1/registry-credentials"} {
		if rr := do(h, authed(http.MethodGet, path, nil)); rr.Code != http.StatusForbidden {
			t.Fatalf("%s status = %d, want 403", path, rr.Code)
		}
	}

	admin := testHandler(stubTasks{}, &stubAuth{user: testAdmin}, nil)
	for _, path := range []string{"/api/v1/users", "/api/v1/registry-credentials"} {
		if rr := do(admin, authed(http.MethodGet, path, nil)); rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200 for an admin", path, rr.Code)
		}
	}
}

func TestOwnerOnlyRoutesRejectMembers(t *testing.T) {
	projects := &stubProjects{role: core.ProjectRoleMember}
	h := testHandler(stubTasks{}, &stubAuth{user: testMember}, projects)

	if rr := do(h, authed(http.MethodDelete, "/api/v1/projects/team", nil)); rr.Code != http.StatusForbidden {
		t.Fatalf("delete project status = %d, want 403", rr.Code)
	}
	if rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/members", nil)); rr.Code != http.StatusForbidden {
		t.Fatalf("list members status = %d, want 403", rr.Code)
	}
	if rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks", nil)); rr.Code != http.StatusOK {
		t.Fatalf("list tasks status = %d, want 200", rr.Code)
	}
}

// An unreachable project must look missing, or its existence leaks through the status code.
func TestNonMemberSeesProjectAsMissing(t *testing.T) {
	projects := &stubProjects{accessErr: core.ErrNotFound}
	h := testHandler(stubTasks{}, &stubAuth{user: testMember}, projects)
	if rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks", nil)); rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestRouteAccessLadder(t *testing.T) {
	cases := []struct {
		role   core.ProjectRole
		method string
		path   string
		want   int
	}{
		{core.ProjectRoleViewer, http.MethodGet, "/api/v1/projects/team/tasks", http.StatusOK},
		{core.ProjectRoleViewer, http.MethodGet, "/api/v1/projects/team/tasks/web", http.StatusOK},
		{core.ProjectRoleViewer, http.MethodPut, "/api/v1/projects/team/tasks/web/state", http.StatusForbidden},
		{core.ProjectRoleViewer, http.MethodDelete, "/api/v1/projects/team/tasks/web", http.StatusForbidden},

		{core.ProjectRoleOperator, http.MethodGet, "/api/v1/projects/team/tasks", http.StatusOK},
		{core.ProjectRoleOperator, http.MethodPut, "/api/v1/projects/team/tasks/web/state", http.StatusOK},
		{core.ProjectRoleOperator, http.MethodPost, "/api/v1/projects/team/tasks", http.StatusForbidden},

		{core.ProjectRoleMember, http.MethodDelete, "/api/v1/projects/team/tasks/web", http.StatusOK},
		{core.ProjectRoleMember, http.MethodGet, "/api/v1/projects/team/members", http.StatusForbidden},

		{core.ProjectRoleOwner, http.MethodGet, "/api/v1/projects/team/members", http.StatusOK},
		{core.ProjectRoleOwner, http.MethodGet, "/api/v1/projects/team/tasks/web/grants", http.StatusOK},
		{core.ProjectRoleMember, http.MethodGet, "/api/v1/projects/team/tasks/web/grants", http.StatusForbidden},
	}
	for _, tc := range cases {
		h := testHandler(stubTasks{}, &stubAuth{user: testMember}, &stubProjects{role: tc.role})
		body := io.Reader(nil)
		if tc.method == http.MethodPut {
			body = strings.NewReader(`{"action":"start"}`)
		}
		rr := do(h, authed(tc.method, tc.path, body))
		if rr.Code != tc.want {
			t.Fatalf("%s %s as %s = %d, want %d", tc.method, tc.path, tc.role, rr.Code, tc.want)
		}
	}
}

// Task form defaults may carry project secrets, so only members receive them.
func TestGranteeDoesNotReceiveProjectDefaultEnv(t *testing.T) {
	project := core.Project{Slug: "team", DefaultEnv: map[string]string{"API_KEY": "s3cret"}}
	grantee := false
	h := testHandler(stubTasks{}, &stubAuth{user: testMember},
		&stubProjects{role: core.ProjectRoleViewer, project: project, allTasks: &grantee})
	body := do(h, authed(http.MethodGet, "/api/v1/projects/team", nil)).Body.String()
	if strings.Contains(body, "s3cret") || strings.Contains(body, "API_KEY") {
		t.Fatalf("project defaults leaked to a grantee: %s", body)
	}

	member := true
	h = testHandler(stubTasks{}, &stubAuth{user: testMember},
		&stubProjects{role: core.ProjectRoleMember, project: project, allTasks: &member})
	body = do(h, authed(http.MethodGet, "/api/v1/projects/team", nil)).Body.String()
	if !strings.Contains(body, "s3cret") {
		t.Fatalf("a member must still see task form defaults: %s", body)
	}
}

// The middleware resolves access once; handlers must use it rather than re-querying.
func TestListRoutesReceiveResolvedAccess(t *testing.T) {
	var gotTasks, gotNotifications core.ProjectAccess
	grantee := false
	projects := &stubProjects{
		role: core.ProjectRoleViewer, allTasks: &grantee,
		project: core.Project{Slug: "team"},
		notifications: func(_ context.Context, acc core.ProjectAccess, _ int) ([]core.Notification, error) {
			gotNotifications = acc
			return nil, nil
		},
	}
	tasks := stubTasks{list: func(_ context.Context, acc core.ProjectAccess) ([]core.Task, error) {
		gotTasks = acc
		return nil, nil
	}}
	h := testHandler(tasks, &stubAuth{user: testMember}, projects)
	do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks", nil))
	do(h, authed(http.MethodGet, "/api/v1/projects/team/notifications", nil))

	for name, acc := range map[string]core.ProjectAccess{"tasks": gotTasks, "notifications": gotNotifications} {
		if acc.AllTasks || acc.UserID != testMember.ID {
			t.Fatalf("%s got %+v; a grantee must be scoped by identity", name, acc)
		}
	}
}

func TestServiceErrorMapping(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"invalid", errors.Join(core.ErrInvalidInput, errors.New("detail")), 400, "invalid request"},
		{"missing", core.ErrNotFound, 404, "not found"},
		{"exists", core.ErrAlreadyExists, 409, "conflict"},
		{"conflict", core.ErrConflict, 409, "conflict"},
		{"forbidden", core.ErrForbidden, 403, "forbidden"},
		{"internal hidden", errors.New("database password is secret"), 500, "internal server error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := testHandler(stubTasks{
				get: func(context.Context, string, string) (core.Task, error) { return core.Task{}, tt.err },
			}, nil, nil)
			rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks/web", nil))
			if rr.Code != tt.status {
				t.Fatalf("status = %d, want %d", rr.Code, tt.status)
			}
			if !strings.Contains(rr.Body.String(), tt.body) {
				t.Fatalf("body = %q", rr.Body.String())
			}
			if tt.status == 500 && strings.Contains(rr.Body.String(), "password") {
				t.Fatal("internal error leaked")
			}
		})
	}
}

func TestTaskRouteAndDTO(t *testing.T) {
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	id, projectID := uuid.New(), uuid.New()
	task := core.Task{
		ID: id, ProjectID: projectID, Name: "T-1", Image: "image",
		Status: core.StatusRunning, Port: 80, Error: "warning",
		PendingRecreate: true, CreatedAt: now, UpdatedAt: now,
	}
	h := testHandler(stubTasks{get: func(_ context.Context, project, name string) (core.Task, error) {
		if project != "team" || name != "T-1" {
			t.Fatalf("path values = %q %q", project, name)
		}
		return task, nil
	}}, nil, nil)

	rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks/T-1", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body map[string]map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	dto := body["task"]
	if dto["id"] != id.String() || dto["name"] != "T-1" || dto["pending_recreate"] != true {
		t.Fatalf("unexpected DTO: %#v", dto)
	}
	if _, ok := dto["cpu_nano"]; ok {
		t.Fatal("cpu_nano must no longer be returned")
	}
	if _, ok := dto["memory_bytes"]; ok {
		t.Fatal("memory_bytes must no longer be returned")
	}
	if _, ok := dto["last_accessed"]; ok {
		t.Fatal("last_accessed must no longer be returned")
	}
}

func TestTaskDTOAlwaysIncludesEnv(t *testing.T) {
	h := testHandler(stubTasks{
		get: func(context.Context, string, string) (core.Task, error) {
			return core.Task{Name: "T"}, nil
		},
	}, nil, nil)
	rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks/T", nil))
	if !strings.Contains(rr.Body.String(), `"env":{}`) {
		t.Fatalf("task response must include empty env: %s", rr.Body.String())
	}
}

func TestRemovedEnvEndpointsReturnNotFound(t *testing.T) {
	h := testHandler(stubTasks{}, nil, nil)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		rr := do(h, authed(method, "/api/v1/projects/team/tasks/T/env", nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", method, rr.Code)
		}
	}
}

func TestCreateTaskPassesProject(t *testing.T) {
	var seen string
	var input service.CreateTaskInput
	h := testHandler(stubTasks{
		create: func(_ context.Context, project string, in service.CreateTaskInput) (core.Task, error) {
			seen, input = project, in
			return core.Task{Name: in.Name}, nil
		},
	}, nil, nil)
	rr := do(h, authed(http.MethodPost, "/api/v1/projects/team/tasks",
		strings.NewReader(`{"name":"web","image":"nginx","port":8080}`)))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	if seen != "team" || input.Name != "web" || input.Image != "nginx" || input.Port != 8080 {
		t.Fatalf("project=%q input=%+v", seen, input)
	}
}

func TestCreateTaskRejectsRemovedFields(t *testing.T) {
	h := testHandler(stubTasks{}, nil, nil)
	rr := do(h, authed(http.MethodPost, "/api/v1/projects/team/tasks",
		strings.NewReader(`{"name":"web","image":"nginx","cpu_nano":500}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown field", rr.Code)
	}
}

func TestStatsDTO(t *testing.T) {
	h := testHandler(stubTasks{stats: func(context.Context) (core.SystemStats, error) {
		return core.SystemStats{TotalTasks: 4, RunningTasks: 2, TotalProjects: 3, TotalMemoryBytes: 8 * 1024 * 1024}, nil
	}}, nil, nil)
	rr := do(h, authed(http.MethodGet, "/api/v1/stats", nil))
	body := rr.Body.String()
	if !strings.Contains(body, `"total_memory_mb":8`) || !strings.Contains(body, `"total_projects":3`) {
		t.Fatalf("body = %s", body)
	}
	if strings.Contains(body, "max_containers") || strings.Contains(body, "container_memory_mb") {
		t.Fatalf("removed statistics returned: %s", body)
	}
}

func TestCredentialTokenNeverReturned(t *testing.T) {
	h := testHandler(stubTasks{}, &stubAuth{user: testAdmin}, &stubProjects{
		listCredentials: func(context.Context) ([]core.RegistryCredential, error) {
			return []core.RegistryCredential{{
				ID: uuid.New(), Name: "ghcr", Registry: core.RegistryGHCR,
				Username: "bot", Token: "ghp_supersecret",
			}}, nil
		},
	})
	rr := do(h, authed(http.MethodGet, "/api/v1/registry-credentials", nil))
	if strings.Contains(rr.Body.String(), "ghp_supersecret") || strings.Contains(rr.Body.String(), `"token"`) {
		t.Fatalf("credential token leaked: %s", rr.Body.String())
	}
}

func TestUserListNeverReturnsPasswordHash(t *testing.T) {
	h := testHandler(stubTasks{}, &stubAuth{
		user: testAdmin,
		listUsers: func(context.Context) ([]core.User, error) {
			return []core.User{{ID: uuid.New(), Username: "u", PasswordHash: "$2a$10$leak"}}, nil
		},
	}, nil)
	rr := do(h, authed(http.MethodGet, "/api/v1/users", nil))
	if strings.Contains(rr.Body.String(), "$2a$10$leak") || strings.Contains(rr.Body.String(), "password_hash") {
		t.Fatalf("password hash leaked: %s", rr.Body.String())
	}
}

func TestDeleteOwnAccountRejected(t *testing.T) {
	h := testHandler(stubTasks{}, &stubAuth{user: testAdmin}, nil)
	rr := do(h, authed(http.MethodDelete, "/api/v1/users/"+testAdmin.ID.String(), nil))
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
}

func TestMalformedUUIDRejected(t *testing.T) {
	h := testHandler(stubTasks{}, &stubAuth{user: testAdmin}, nil)
	if rr := do(h, authed(http.MethodDelete, "/api/v1/users/not-a-uuid", nil)); rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestLogoutRevokesCallerToken(t *testing.T) {
	auth := &stubAuth{user: testAdmin}
	h := testHandler(stubTasks{}, auth, nil)
	if rr := do(h, authed(http.MethodPost, "/api/v1/auth/logout", nil)); rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if len(auth.loggedOut) != 1 || auth.loggedOut[0] != testToken {
		t.Fatalf("revoked tokens = %v", auth.loggedOut)
	}
}

func TestCreateAPITokenReturnsSecretOnce(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	tokenID := uuid.New()
	var gotUser uuid.UUID
	var got service.CreateAPITokenInput
	auth := &stubAuth{
		user: testMember,
		createAPI: func(_ context.Context, userID uuid.UUID, in service.CreateAPITokenInput) (string, core.AuthToken, error) {
			gotUser, got = userID, in
			return "plain-secret", core.AuthToken{
				ID: tokenID, UserID: userID, Name: in.Name, Kind: core.TokenKindAPI,
				TokenHash: "must-not-leak", ValidFrom: in.ValidFrom, ExpiresAt: in.ValidTo, CreatedAt: now,
			}, nil
		},
	}
	h := testHandler(stubTasks{}, auth, nil)
	body := `{"name":"ci","valid_from":"` + now.Format(time.RFC3339) +
		`","valid_to":"` + now.Add(24*time.Hour).Format(time.RFC3339) + `"}`
	rr := do(h, authed(http.MethodPost, "/api/v1/auth/tokens", strings.NewReader(body)))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotUser != testMember.ID || got.Name != "ci" || !got.ValidFrom.Equal(now) {
		t.Fatalf("arguments: user=%s input=%+v", gotUser, got)
	}
	response := rr.Body.String()
	if !strings.Contains(response, `"token":"plain-secret"`) || !strings.Contains(response, tokenID.String()) {
		t.Fatalf("body=%s", response)
	}
	if strings.Contains(response, "must-not-leak") || strings.Contains(response, "token_hash") {
		t.Fatalf("token hash leaked: %s", response)
	}
}

func TestListAPITokensReturnsMetadataAndStatuses(t *testing.T) {
	now := time.Now().UTC()
	revokedAt := now.Add(-time.Minute)
	auth := &stubAuth{
		user: testMember,
		listAPI: func(_ context.Context, userID uuid.UUID) ([]core.AuthToken, error) {
			if userID != testMember.ID {
				t.Fatalf("userID=%s", userID)
			}
			return []core.AuthToken{
				{ID: uuid.New(), Name: "future", Kind: core.TokenKindAPI, TokenHash: "hidden-1", ValidFrom: now.Add(time.Hour), ExpiresAt: now.Add(2 * time.Hour), CreatedAt: now},
				{ID: uuid.New(), Name: "active", Kind: core.TokenKindAPI, TokenHash: "hidden-2", ValidFrom: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), CreatedAt: now},
				{ID: uuid.New(), Name: "expired", Kind: core.TokenKindAPI, TokenHash: "hidden-3", ValidFrom: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour), CreatedAt: now},
				{ID: uuid.New(), Name: "revoked", Kind: core.TokenKindAPI, TokenHash: "hidden-4", ValidFrom: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt, CreatedAt: now},
			}, nil
		},
	}
	h := testHandler(stubTasks{}, auth, nil)
	rr := do(h, authed(http.MethodGet, "/api/v1/auth/tokens", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	response := rr.Body.String()
	for _, status := range []string{"scheduled", "active", "expired", "revoked"} {
		if !strings.Contains(response, `"status":"`+status+`"`) {
			t.Fatalf("missing %s: %s", status, response)
		}
	}
	if strings.Contains(response, "hidden-") || strings.Contains(response, "token_hash") {
		t.Fatalf("secret metadata leaked: %s", response)
	}
}

func TestRevokeAPITokenUsesCurrentUser(t *testing.T) {
	tokenID := uuid.New()
	var gotUser, gotToken uuid.UUID
	auth := &stubAuth{
		user: testMember,
		revokeAPI: func(_ context.Context, userID, id uuid.UUID) error {
			gotUser, gotToken = userID, id
			return nil
		},
	}
	h := testHandler(stubTasks{}, auth, nil)
	rr := do(h, authed(http.MethodDelete, "/api/v1/auth/tokens/"+tokenID.String(), nil))
	if rr.Code != http.StatusOK || gotUser != testMember.ID || gotToken != tokenID {
		t.Fatalf("status=%d user=%s token=%s", rr.Code, gotUser, gotToken)
	}
	if !strings.Contains(rr.Body.String(), `"success":true`) {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestAPITokenCannotManageAPITokens(t *testing.T) {
	called := false
	auth := &stubAuth{
		user: testMember,
		auth: func(_ context.Context, token string) (core.User, core.TokenKind, error) {
			if token != testToken {
				return core.User{}, "", core.ErrUnauthorized
			}
			return testMember, core.TokenKindAPI, nil
		},
		listAPI: func(context.Context, uuid.UUID) ([]core.AuthToken, error) {
			called = true
			return nil, nil
		},
	}
	h := testHandler(stubTasks{}, auth, nil)
	for _, request := range []*http.Request{
		authed(http.MethodGet, "/api/v1/auth/tokens", nil),
		authed(http.MethodPost, "/api/v1/auth/tokens", strings.NewReader(`{}`)),
		authed(http.MethodDelete, "/api/v1/auth/tokens/"+uuid.New().String(), nil),
	} {
		rr := do(h, request)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s %s status=%d, want 403", request.Method, request.URL.Path, rr.Code)
		}
	}
	if called {
		t.Fatal("API token management service was called")
	}
}

func TestAPITokenCanDeploy(t *testing.T) {
	digest := "ghcr.io/acme/web@sha256:" + strings.Repeat("a", 64)
	deployed := false
	auth := &stubAuth{
		user: testMember,
		auth: func(context.Context, string) (core.User, core.TokenKind, error) {
			return testMember, core.TokenKindAPI, nil
		},
	}
	h := testHandler(stubTasks{
		deploy: func(_ context.Context, project, name, image string) (core.Task, error) {
			deployed = project == "team" && name == "web" && image == digest
			return core.Task{Name: name, Image: image, Status: core.StatusRunning}, nil
		},
	}, auth, &stubProjects{role: core.ProjectRoleMember})
	rr := do(h, authed(http.MethodPost, "/api/v1/projects/team/tasks/web/deploy",
		strings.NewReader(`{"image":"`+digest+`"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !deployed {
		t.Fatal("API token did not reach the deploy service")
	}
}

func TestCreateAPITokenRejectsMalformedTime(t *testing.T) {
	called := false
	auth := &stubAuth{
		user: testMember,
		createAPI: func(context.Context, uuid.UUID, service.CreateAPITokenInput) (string, core.AuthToken, error) {
			called = true
			return "", core.AuthToken{}, nil
		},
	}
	h := testHandler(stubTasks{}, auth, nil)
	rr := do(h, authed(http.MethodPost, "/api/v1/auth/tokens",
		strings.NewReader(`{"name":"ci","valid_from":"tomorrow","valid_to":"later"}`)))
	if rr.Code != http.StatusBadRequest || called {
		t.Fatalf("status=%d called=%v", rr.Code, called)
	}
}

func frame(stream byte, payload string) []byte {
	b := make([]byte, 8, 8+len(payload))
	b[0] = stream
	n := len(payload)
	b[4], b[5], b[6], b[7] = byte(n>>24), byte(n>>16), byte(n>>8), byte(n)
	return append(b, payload...)
}

func TestLogsDecodeSplitDockerFrame(t *testing.T) {
	wired := append(frame(1, "hello "), frame(2, "error\n")...)
	opened := false
	h := testHandler(stubTasks{
		logs: func(_ context.Context, _, _ string, opts core.LogOptions) (io.ReadCloser, error) {
			opened = true
			if opts.Tail != 7 || opts.Follow {
				t.Fatalf("options = %#v", opts)
			}
			return io.NopCloser(iotest.OneByteReader(bytes.NewReader(wired))), nil
		},
	}, nil, nil)
	rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks/T/logs?tail=7", nil))
	if rr.Body.String() != "hello error\n" {
		t.Fatalf("decoded = %q", rr.Body.String())
	}
	if !opened {
		t.Fatal("reader not opened")
	}
}

func TestInvalidTailDoesNotOpenLogs(t *testing.T) {
	opened := false
	h := testHandler(stubTasks{
		logs: func(context.Context, string, string, core.LogOptions) (io.ReadCloser, error) {
			opened = true
			return nil, nil
		},
	}, nil, nil)
	rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks/T/logs?tail=-1", nil))
	if rr.Code != http.StatusBadRequest || opened {
		t.Fatalf("status=%d opened=%v", rr.Code, opened)
	}
}

func TestUpdateTaskSendsOnlySuppliedFields(t *testing.T) {
	var got service.UpdateTaskInput
	var recreate bool
	h := testHandler(stubTasks{
		update: func(_ context.Context, _, _ string, in service.UpdateTaskInput, r bool) (core.Task, error) {
			got, recreate = in, r
			return core.Task{Name: "T", Description: "new"}, nil
		},
	}, nil, nil)

	body := strings.NewReader(`{"description":"new"}`)
	rr := do(h, authed(http.MethodPatch, "/api/v1/projects/team/tasks/T", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body)
	}
	if got.Description == nil || *got.Description != "new" {
		t.Fatalf("description = %v", got.Description)
	}
	if got.Image != nil || got.Port != nil || got.Labels != nil || got.Env != nil {
		t.Fatalf("omitted fields were sent: %+v", got)
	}
	if !recreate {
		t.Fatal("auto_restart must default to true")
	}
}

func TestUpdateTaskEnvDefaultsAutoRestartTrue(t *testing.T) {
	called := false
	h := testHandler(stubTasks{
		update: func(_ context.Context, project, name string, in service.UpdateTaskInput, recreate bool) (core.Task, error) {
			called = true
			if project != "team" || name != "T-1" || in.Env == nil || (*in.Env)["A"] != "B" || !recreate {
				t.Fatalf("arguments: %q %q %#v %v", project, name, in.Env, recreate)
			}
			return core.Task{Status: core.StatusRunning, Env: *in.Env}, nil
		},
	}, nil, nil)
	rr := do(h, authed(http.MethodPatch, "/api/v1/projects/team/tasks/T-1",
		strings.NewReader(`{"env":{"A":"B"}}`)))
	if rr.Code != http.StatusOK || !called {
		t.Fatalf("status/body = %d %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateTaskHonoursAutoRestartFalse(t *testing.T) {
	var recreate bool
	h := testHandler(stubTasks{
		update: func(_ context.Context, _, _ string, _ service.UpdateTaskInput, r bool) (core.Task, error) {
			recreate = r
			return core.Task{}, nil
		},
	}, nil, nil)

	body := strings.NewReader(`{"image":"nginx:alpine","auto_restart":false}`)
	if rr := do(h, authed(http.MethodPatch, "/api/v1/projects/team/tasks/T", body)); rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if recreate {
		t.Fatal("auto_restart=false was ignored")
	}
}

func TestUpdateTaskDistinguishesEmptyFromOmitted(t *testing.T) {
	var got service.UpdateTaskInput
	h := testHandler(stubTasks{
		update: func(_ context.Context, _, _ string, in service.UpdateTaskInput, _ bool) (core.Task, error) {
			got = in
			return core.Task{}, nil
		},
	}, nil, nil)

	body := strings.NewReader(`{"env":{}}`)
	if rr := do(h, authed(http.MethodPatch, "/api/v1/projects/team/tasks/T", body)); rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got.Env == nil {
		t.Fatal("an explicit empty object must not be read as omitted")
	}
	if len(*got.Env) != 0 {
		t.Fatalf("env = %v", *got.Env)
	}
}

func TestDeployTaskPassesImage(t *testing.T) {
	digest := "ghcr.io/acme/web@sha256:" + strings.Repeat("a", 64)
	var seenProject, seenName, seenImage string
	h := testHandler(stubTasks{
		deploy: func(_ context.Context, project, name, image string) (core.Task, error) {
			seenProject, seenName, seenImage = project, name, image
			return core.Task{Name: name, Image: image, Status: core.StatusRunning}, nil
		},
	}, nil, nil)

	rr := do(h, authed(http.MethodPost, "/api/v1/projects/team/tasks/T-1/deploy",
		strings.NewReader(`{"image":"`+digest+`"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	if seenProject != "team" || seenName != "T-1" || seenImage != digest {
		t.Fatalf("arguments: %q %q %q", seenProject, seenName, seenImage)
	}
	if !strings.Contains(rr.Body.String(), digest) {
		t.Fatalf("body = %s", rr.Body.String())
	}
}

func TestDeployTaskMapsInvalidImageTo400(t *testing.T) {
	h := testHandler(stubTasks{
		deploy: func(context.Context, string, string, string) (core.Task, error) {
			return core.Task{}, errors.Join(core.ErrInvalidInput, errors.New("deploy image must be immutable"))
		},
	}, nil, nil)
	rr := do(h, authed(http.MethodPost, "/api/v1/projects/team/tasks/T/deploy",
		strings.NewReader(`{"image":"ghcr.io/acme/web:staging"}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestDeployTaskRejectsUnknownFields(t *testing.T) {
	called := false
	h := testHandler(stubTasks{
		deploy: func(context.Context, string, string, string) (core.Task, error) {
			called = true
			return core.Task{}, nil
		},
	}, nil, nil)
	rr := do(h, authed(http.MethodPost, "/api/v1/projects/team/tasks/T/deploy",
		strings.NewReader(`{"image":"x","digest":"sha256:abc"}`)))
	if rr.Code != http.StatusBadRequest || called {
		t.Fatalf("status=%d called=%v", rr.Code, called)
	}
}

func TestUpdateTaskRejectsUnknownFields(t *testing.T) {
	called := false
	h := testHandler(stubTasks{
		update: func(context.Context, string, string, service.UpdateTaskInput, bool) (core.Task, error) {
			called = true
			return core.Task{}, nil
		},
	}, nil, nil)

	body := strings.NewReader(`{"name":"renamed"}`)
	rr := do(h, authed(http.MethodPatch, "/api/v1/projects/team/tasks/T", body))
	if rr.Code != http.StatusBadRequest || called {
		t.Fatalf("status=%d called=%v: rename is not supported and must be rejected", rr.Code, called)
	}
}

// Preflight must advertise every routed method because browsers reject missing methods.
func TestCORSAdvertisesEveryRoutedMethod(t *testing.T) {
	h := APIHandler(stubTasks{}, &stubAuth{user: testAdmin}, &stubProjects{}, log.New(io.Discard, "", 0))
	r := httptest.NewRequest(http.MethodOptions, "/api/v1/projects/team/tasks/T", nil)
	r.Header.Set("Origin", "http://localhost:4200")
	headers := do(h, r).Header()
	allowed := headers.Get("Access-Control-Allow-Methods")
	if headers.Get("Access-Control-Allow-Origin") != "*" ||
		headers.Get("Access-Control-Allow-Headers") != allowedHeaders {
		t.Fatalf("unexpected CORS headers: %v", headers)
	}

	for _, r := range routeTable {
		if !strings.Contains(allowed, r.method) {
			t.Fatalf("%s is routed but missing from %q", r.method, allowed)
		}
	}
}

func TestSSELogEntries(t *testing.T) {
	wired := append(frame(1, "2025-01-02T03:04:05Z hello\n"), frame(2, "bad\n")...)
	h := testHandler(stubTasks{
		logs: func(_ context.Context, _, _ string, opts core.LogOptions) (io.ReadCloser, error) {
			if !opts.Follow {
				t.Fatal("SSE logs must follow")
			}
			return io.NopCloser(bytes.NewReader(wired)), nil
		},
	}, nil, nil)
	rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks/T/logs/stream", nil))
	body := rr.Body.String()
	if rr.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type = %q", rr.Header().Get("Content-Type"))
	}
	if !strings.Contains(body, `"stream":"stdout","message":"hello"`) ||
		!strings.Contains(body, `"stream":"stderr","message":"bad"`) {
		t.Fatalf("SSE body = %q", body)
	}
}

func TestApplicationHandlerRoutes(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	var seen string
	proxy := http.NewServeMux()
	proxy.HandleFunc("/team/T-1/", func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		w.WriteHeader(http.StatusTeapot)
	})
	h := ApplicationHandler(api, proxy, logger)

	for path, want := range map[string]int{
		"/api/v1/health": http.StatusNoContent,
		"/team/T-1/app":  http.StatusTeapot,
	} {
		if rr := do(h, httptest.NewRequest(http.MethodGet, path, nil)); rr.Code != want {
			t.Fatalf("%s status=%d want=%d", path, rr.Code, want)
		}
	}
	if seen != "/team/T-1/app" {
		t.Fatalf("proxy path = %q", seen)
	}

	rr := do(h, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK || rr.Body.String() != `{"service":"boreas","status":"healthy"}` {
		t.Fatalf("root status=%d body=%q", rr.Code, rr.Body.String())
	}

	for _, path := range []string{"/T-1/app", "/missing", "/team/other/app"} {
		if rr := do(h, httptest.NewRequest(http.MethodGet, path, nil)); rr.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d, want 404", path, rr.Code)
		}
	}
}

func TestUpdateProjectDistinguishesNullFromOmittedCredential(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantSet    bool
		wantDetach bool
	}{
		{name: "omitted", body: `{"name":"Renamed"}`},
		{name: "null", body: `{"registry_credential_id":null}`, wantSet: true, wantDetach: true},
		{name: "value", body: `{"registry_credential_id":"` + uuid.New().String() + `"}`, wantSet: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got service.UpdateProjectInput
			projects := &stubProjects{update: func(_ context.Context, _ string, in service.UpdateProjectInput) (core.Project, error) {
				got = in
				return core.Project{}, nil
			}}
			h := testHandler(stubTasks{}, nil, projects)
			rr := do(h, authed(http.MethodPatch, "/api/v1/projects/team", strings.NewReader(tc.body)))
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
			}
			if (got.RegistryCredentialID != nil) != tc.wantSet {
				t.Fatalf("RegistryCredentialID set = %v, want %v", got.RegistryCredentialID != nil, tc.wantSet)
			}
			if tc.wantSet && (*got.RegistryCredentialID == nil) != tc.wantDetach {
				t.Fatalf("detach = %v, want %v", *got.RegistryCredentialID == nil, tc.wantDetach)
			}
		})
	}
}

func TestUpdateProjectForwardsTaskDefaults(t *testing.T) {
	var got service.UpdateProjectInput
	projects := &stubProjects{update: func(_ context.Context, _ string, in service.UpdateProjectInput) (core.Project, error) {
		got = in
		return core.Project{}, nil
	}}
	h := testHandler(stubTasks{}, nil, projects)
	rr := do(h, authed(http.MethodPatch, "/api/v1/projects/team",
		strings.NewReader(`{"default_image":"nginx:alpine","default_port":8080,"default_env":{}}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	if got.DefaultImage == nil || *got.DefaultImage != "nginx:alpine" {
		t.Fatalf("default_image = %v", got.DefaultImage)
	}
	if got.DefaultPort == nil || *got.DefaultPort != 8080 {
		t.Fatalf("default_port = %v", got.DefaultPort)
	}
	// An explicit empty object must survive as a clear instruction, not as omission.
	if got.DefaultEnv == nil || len(*got.DefaultEnv) != 0 {
		t.Fatalf("default_env = %v", got.DefaultEnv)
	}

	rr = do(h, authed(http.MethodPatch, "/api/v1/projects/team", strings.NewReader(`{"name":"Renamed"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got.DefaultImage != nil || got.DefaultPort != nil || got.DefaultEnv != nil {
		t.Fatalf("omitted defaults leaked: %+v", got)
	}
}

func TestProjectDTOAlwaysIncludesTaskDefaults(t *testing.T) {
	h := testHandler(stubTasks{}, nil, &stubProjects{})
	rr := do(h, authed(http.MethodGet, "/api/v1/projects/team", nil))
	body := rr.Body.String()
	for _, want := range []string{`"default_image":""`, `"default_port":0`, `"default_env":{}`} {
		if !strings.Contains(body, want) {
			t.Fatalf("project response must include %s: %s", want, body)
		}
	}
}

func TestUpdateProjectRejectsMalformedCredentialID(t *testing.T) {
	h := testHandler(stubTasks{}, nil, nil)
	rr := do(h, authed(http.MethodPatch, "/api/v1/projects/team",
		strings.NewReader(`{"registry_credential_id":"not-a-uuid"}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestPathParamsAreNotBodyFields(t *testing.T) {
	h := testHandler(stubTasks{}, nil, nil)
	bodies := []string{
		`{"name":"web","image":"nginx","Project":"evil"}`,
		`{"name":"web","image":"nginx","project":"evil"}`,
	}
	for _, body := range bodies {
		rr := do(h, authed(http.MethodPost, "/api/v1/projects/team/tasks", strings.NewReader(body)))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400", body, rr.Code)
		}
	}
}

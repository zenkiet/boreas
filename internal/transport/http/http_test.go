package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	return APIHandler(tasks, auth, projects, &stubPush{}, NewHub(), "test", slog.New(slog.DiscardHandler))
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

// Only these three routes are public; every other one challenges a missing or bad token.
func TestOnlyPublicRoutesSkipAuthentication(t *testing.T) {
	public := map[string]bool{"GET /api/v1/health": true, "POST /api/v1/auth/login": true, "GET /api/v1/public/projects": true}
	h := testHandler(stubTasks{}, nil, nil)
	for _, route := range routeTable {
		pattern, path := route.method+" "+route.path, strings.NewReplacer("{", "", "}", "").Replace(route.path)
		for _, header := range []string{"", "Bearer wrong"} {
			r := httptest.NewRequest(route.method, path, nil)
			r.Header.Set("Authorization", header)
			rr := do(h, r)
			if challenged := rr.Code == http.StatusUnauthorized && rr.Header().Get("WWW-Authenticate") != ""; challenged == public[pattern] {
				t.Fatalf("%s with %q = %d", pattern, header, rr.Code)
			}
		}
	}
}

func TestPasswordHashNeverReturned(t *testing.T) {
	alice := core.User{ID: uuid.New(), Username: "alice", Role: core.RoleAdmin, PasswordHash: "$2a$10$topsecrethash"}
	auth := &stubAuth{user: alice, listUsers: func(context.Context) ([]core.User, error) { return []core.User{alice}, nil }}
	h := testHandler(stubTasks{}, auth, nil)
	login := do(h, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"alice","password":"pw"}`)))
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), testToken) {
		t.Fatalf("login = %d %s", login.Code, login.Body.String())
	}
	for _, body := range []string{login.Body.String(), do(h, authed(http.MethodGet, "/api/v1/users", nil)).Body.String()} {
		if !strings.Contains(body, "alice") || strings.Contains(body, "topsecrethash") || strings.Contains(body, "password_hash") {
			t.Fatalf("user missing or hash leaked: %s", body)
		}
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
		{core.ProjectRoleViewer, http.MethodGet, "/api/v1/projects/team/metrics/stream", http.StatusOK},
		{core.ProjectRoleViewer, http.MethodGet, "/api/v1/projects/team/tasks/web/metrics/stream", http.StatusOK},
		{core.ProjectRoleViewer, http.MethodPut, "/api/v1/projects/team/tasks/web/state", http.StatusForbidden},
		{core.ProjectRoleViewer, http.MethodDelete, "/api/v1/projects/team/tasks/web", http.StatusForbidden},

		{core.ProjectRoleOperator, http.MethodGet, "/api/v1/projects/team/tasks", http.StatusOK},
		{core.ProjectRoleOperator, http.MethodPut, "/api/v1/projects/team/tasks/web/state", http.StatusOK},
		{core.ProjectRoleOperator, http.MethodPost, "/api/v1/projects/team/tasks", http.StatusForbidden},
		// Only a member of the task, or an administrator, reports development progress.
		{core.ProjectRoleOperator, http.MethodPatch, "/api/v1/projects/team/tasks/web", http.StatusForbidden},

		{core.ProjectRoleMember, http.MethodPatch, "/api/v1/projects/team/tasks/web", http.StatusOK},
		{core.ProjectRoleMember, http.MethodDelete, "/api/v1/projects/team/tasks/web", http.StatusOK},
		{core.ProjectRoleMember, http.MethodGet, "/api/v1/projects/team/members", http.StatusForbidden},
		{core.ProjectRoleMember, http.MethodDelete, "/api/v1/projects/team", http.StatusForbidden},

		{core.ProjectRoleOwner, http.MethodGet, "/api/v1/projects/team/members", http.StatusOK},
		{core.ProjectRoleOwner, http.MethodGet, "/api/v1/projects/team/tasks/web/grants", http.StatusOK},
		{core.ProjectRoleMember, http.MethodGet, "/api/v1/projects/team/tasks/web/grants", http.StatusForbidden},
	}
	bodies := map[string]string{http.MethodPut: `{"action":"start"}`, http.MethodPatch: `{"dev_status":"ready"}`}
	for _, tc := range cases {
		h := testHandler(stubTasks{}, &stubAuth{user: testMember}, &stubProjects{role: tc.role})
		rr := do(h, authed(tc.method, tc.path, strings.NewReader(bodies[tc.method])))
		if rr.Code != tc.want {
			t.Fatalf("%s %s as %s = %d, want %d", tc.method, tc.path, tc.role, rr.Code, tc.want)
		}
	}
}

// Task form defaults may carry project secrets, so only members receive them.
func TestGranteeDoesNotReceiveProjectDefaultEnv(t *testing.T) {
	project := core.Project{Slug: "team", DefaultEnv: map[string]string{"API_KEY": "s3cret"}, Repositories: []string{"github.com/acme/private-api"}}
	grantee := false
	h := testHandler(stubTasks{}, &stubAuth{user: testMember},
		&stubProjects{role: core.ProjectRoleViewer, project: project, allTasks: &grantee})
	body := do(h, authed(http.MethodGet, "/api/v1/projects/team", nil)).Body.String()
	if strings.Contains(body, "s3cret") || strings.Contains(body, "API_KEY") || strings.Contains(body, "private-api") {
		t.Fatalf("project defaults or repositories leaked to a grantee: %s", body)
	}

	member := true
	h = testHandler(stubTasks{}, &stubAuth{user: testMember},
		&stubProjects{role: core.ProjectRoleMember, project: project, allTasks: &member})
	body = do(h, authed(http.MethodGet, "/api/v1/projects/team", nil)).Body.String()
	if !strings.Contains(body, "s3cret") {
		t.Fatalf("a member must still see task form defaults: %s", body)
	}
}

// One handler serves both feeds, so a regression silently widens the project feed.
func TestNotificationFeeds(t *testing.T) {
	project := core.Project{ID: uuid.New(), Slug: "team"}
	var gotProject, gotBefore *uuid.UUID
	var gotSeen []uuid.UUID
	h := testHandler(stubTasks{}, &stubAuth{user: testMember}, &stubProjects{
		project: project,
		notifications: func(_ context.Context, actor core.User, projectID, before *uuid.UUID, _ int) ([]core.Notification, error) {
			if actor.ID != testMember.ID {
				t.Fatalf("feed read as %+v", actor)
			}
			gotProject, gotBefore = projectID, before
			return nil, nil
		},
		markSeen: func(_ context.Context, _ core.User, ids []uuid.UUID) error {
			gotSeen = ids
			return nil
		},
	})
	last := uuid.New()
	do(h, authed(http.MethodGet, "/api/v1/notifications?before="+last.String(), nil))
	if gotProject != nil || gotBefore == nil || *gotBefore != last {
		t.Fatalf("cross-project feed scoped to %v, before %v", gotProject, gotBefore)
	}
	do(h, authed(http.MethodGet, "/api/v1/projects/team/notifications", nil))
	if gotProject == nil || *gotProject != project.ID {
		t.Fatalf("project feed scoped to %v", gotProject)
	}
	do(h, authed(http.MethodPost, "/api/v1/projects/team/notifications/"+last.String()+"/seen", nil))
	if len(gotSeen) != 1 || gotSeen[0] != last {
		t.Fatalf("per-row seen marked %v", gotSeen)
	}
}

func TestProjectListIsTheFleet(t *testing.T) {
	project := core.Project{ID: uuid.New(), Slug: "team"}
	projects := &stubProjects{fleet: func(context.Context, core.User) ([]core.ProjectAccess, map[uuid.UUID][]core.FleetTask, error) {
		return []core.ProjectAccess{{Project: project, Role: core.ProjectRoleViewer}},
			map[uuid.UUID][]core.FleetTask{project.ID: {{
				Task: core.Task{Name: "web", Env: map[string]string{"SECRET": "s3cret"}},
				Role: core.ProjectRoleOperator,
				LastDeploy: &core.Notification{
					Status: core.NotificationSuccess, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				},
			}}}, nil
	}}
	body := do(testHandler(stubTasks{}, nil, projects), authed(http.MethodGet, "/api/v1/projects", nil)).Body.String()
	for _, want := range []string{
		`"my_role":"viewer","tasks":[{"name":"web"`, `"my_role":"operator"`,
		`"last_deploy":{"status":"success","at":"2026-01-02T03:04:05Z"}`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("fleet body missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, "s3cret") {
		t.Fatalf("the fleet leaked task env: %s", body)
	}
}

// Handlers use what the middleware resolved: its identity scope, and my_role raised by the caller's grants.
func TestMyRoleFollowsResolvedAccess(t *testing.T) {
	web, db := core.Task{ID: uuid.New(), Name: "web"}, core.Task{ID: uuid.New(), Name: "db"}
	var listed core.ProjectAccess
	grantee := false
	h := testHandler(stubTasks{
		list: func(_ context.Context, acc core.ProjectAccess) ([]core.Task, error) {
			listed = acc
			return []core.Task{web, db}, nil
		},
		get: func(context.Context, string, string) (core.Task, error) { return db, nil },
	}, &stubAuth{user: testMember}, &stubProjects{
		role: core.ProjectRoleViewer, allTasks: &grantee,
		grantedRoles: map[uuid.UUID]core.ProjectRole{web.ID: core.ProjectRoleOperator},
	})

	var list tasksResponse
	if err := json.Unmarshal(do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks", nil)).Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if listed.AllTasks || listed.UserID != testMember.ID {
		t.Fatalf("listed with %+v; a grantee must be scoped by identity", listed)
	}
	if len(list.Tasks) != 2 || list.Tasks[0].MyRole != core.ProjectRoleOperator || list.Tasks[1].MyRole != core.ProjectRoleViewer {
		t.Fatalf("list roles = %+v", list.Tasks)
	}
	for _, path := range []string{"/api/v1/projects/team", "/api/v1/projects/team/tasks/db"} {
		if body := do(h, authed(http.MethodGet, path, nil)).Body.String(); !strings.Contains(body, `"my_role":"viewer"`) {
			t.Fatalf("%s lost the resolved role: %s", path, body)
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

// Malformed input and body-borne path params stop at 400; default stubs succeed, so a 400 means no service ran.
func TestMalformedInputRejected(t *testing.T) {
	h := testHandler(stubTasks{}, nil, nil)
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/projects/team/tasks", `{"name":"web","image":"nginx","project":"evil"}`},
		{http.MethodPatch, "/api/v1/projects/team/tasks/T", `{"name":"renamed"}`},
		{http.MethodPost, "/api/v1/projects/team/tasks/T/deploy", `{"image":"x","digest":"sha256:abc"}`},
		{http.MethodPost, "/api/v1/auth/tokens", `{"name":"ci","valid_from":"tomorrow","valid_to":"later"}`},
		{http.MethodPatch, "/api/v1/projects/team", `{"registry_credential_id":"not-a-uuid"}`},
		{http.MethodDelete, "/api/v1/users/not-a-uuid", ""},
		{http.MethodGet, "/api/v1/projects/team/tasks/T/logs?tail=-1", ""},
		{http.MethodGet, "/api/v1/projects/team/tasks/T/logs/stream?since=yesterday", ""},
		{http.MethodGet, "/api/v1/notifications?before=nope", ""},
		{http.MethodGet, "/api/v1/notifications?limit=201", ""},
		{http.MethodPost, "/api/v1/notifications/seen", `{"ids":[]}`},
	} {
		if rr := do(h, authed(tc.method, tc.path, strings.NewReader(tc.body))); rr.Code != http.StatusBadRequest {
			t.Fatalf("%s %s %s = %d, want 400", tc.method, tc.path, tc.body, rr.Code)
		}
	}
}

func TestTaskRouteAndDTO(t *testing.T) {
	id := uuid.New()
	h := testHandler(stubTasks{get: func(_ context.Context, project, name string) (core.Task, error) {
		if project != "team" || name != "T-1" {
			t.Fatalf("path values = %q %q", project, name)
		}
		return core.Task{ID: id, Name: "T-1", DevStatus: core.DevReady, PendingRecreate: true}, nil
	}}, nil, nil)
	rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks/T-1", nil))
	for _, want := range []string{`"id":"` + id.String() + `"`, `"name":"T-1"`, `"dev_status":"ready"`, `"env":{}`, `"pending_recreate":true`} {
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("status %d, body missing %s: %s", rr.Code, want, rr.Body.String())
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

func TestStatsDTO(t *testing.T) {
	h := testHandler(stubTasks{stats: func(context.Context) (core.SystemStats, error) {
		return core.SystemStats{TotalTasks: 4, RunningTasks: 2, TotalProjects: 3, TotalMemoryBytes: 8 * 1024 * 1024}, nil
	}}, nil, nil)
	body := do(h, authed(http.MethodGet, "/api/v1/stats", nil)).Body.String()
	if !strings.Contains(body, `"total_memory_mb":8`) || !strings.Contains(body, `"total_projects":3`) {
		t.Fatalf("body = %s", body)
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

func TestDeleteOwnAccountRejected(t *testing.T) {
	h := testHandler(stubTasks{}, &stubAuth{user: testAdmin}, nil)
	rr := do(h, authed(http.MethodDelete, "/api/v1/users/"+testAdmin.ID.String(), nil))
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
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
	h := testHandler(stubTasks{}, &stubAuth{user: testMember, kind: core.TokenKindAPI}, nil)
	for _, request := range []*http.Request{
		authed(http.MethodGet, "/api/v1/auth/tokens", nil),
		authed(http.MethodPost, "/api/v1/auth/tokens", strings.NewReader(`{}`)),
		authed(http.MethodDelete, "/api/v1/auth/tokens/"+uuid.New().String(), nil),
	} {
		if rr := do(h, request); rr.Code != http.StatusForbidden {
			t.Fatalf("%s %s status=%d, want 403", request.Method, request.URL.Path, rr.Code)
		}
	}
}

// Build pipelines deploy with API tokens: operator level must reach the service with path and image intact.
func TestAPITokenDeploysImage(t *testing.T) {
	digest := "ghcr.io/acme/web@sha256:" + strings.Repeat("a", 64)
	var got string
	h := testHandler(stubTasks{deploy: func(_ context.Context, project, name, image string) (core.Task, error) {
		got = project + " " + name + " " + image
		return core.Task{Name: name, Image: image}, nil
	}}, &stubAuth{user: testMember, kind: core.TokenKindAPI}, &stubProjects{role: core.ProjectRoleOperator})
	rr := do(h, authed(http.MethodPost, "/api/v1/projects/team/tasks/T-1/deploy", strings.NewReader(`{"image":"`+digest+`"}`)))
	if rr.Code != http.StatusOK || got != "team T-1 "+digest || !strings.Contains(rr.Body.String(), digest) {
		t.Fatalf("status=%d forwarded=%q body=%s", rr.Code, got, rr.Body.String())
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
	h := testHandler(stubTasks{
		logs: func(_ context.Context, _, _ string, opts core.LogOptions) (io.ReadCloser, error) {
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
}

// Only fields sent reach the service; {} is not omission; auto_restart defaults to true.
func TestUpdateTaskForwardsOnlySuppliedFields(t *testing.T) {
	desc, image, ready := "new", "nginx:alpine", core.DevReady
	env, empty := map[string]string{"A": "B"}, map[string]string{}
	for _, tc := range []struct {
		body     string
		want     service.UpdateTaskInput
		recreate bool
	}{
		{`{"description":"new"}`, service.UpdateTaskInput{Description: &desc}, true},
		{`{"dev_status":"ready"}`, service.UpdateTaskInput{DevStatus: &ready}, true},
		{`{"env":{"A":"B"}}`, service.UpdateTaskInput{Env: &env}, true},
		{`{"env":{}}`, service.UpdateTaskInput{Env: &empty}, true},
		{`{"image":"nginx:alpine","auto_restart":false}`, service.UpdateTaskInput{Image: &image}, false},
	} {
		var got service.UpdateTaskInput
		var recreate bool
		h := testHandler(stubTasks{update: func(_ context.Context, project, name string, in service.UpdateTaskInput, r bool) (core.Task, error) {
			if project != "team" || name != "T" {
				t.Fatalf("path values = %q %q", project, name)
			}
			got, recreate = in, r
			return core.Task{}, nil
		}}, nil, nil)
		rr := do(h, authed(http.MethodPatch, "/api/v1/projects/team/tasks/T", strings.NewReader(tc.body)))
		if rr.Code != http.StatusOK || !reflect.DeepEqual(got, tc.want) || recreate != tc.recreate {
			t.Fatalf("%s: status %d, forwarded %+v, recreate %v", tc.body, rr.Code, got, recreate)
		}
	}
}

// Preflight must advertise every routed method because browsers reject missing methods.
func TestCORSAdvertisesEveryRoutedMethod(t *testing.T) {
	h := APIHandler(stubTasks{}, &stubAuth{user: testAdmin}, &stubProjects{}, &stubPush{}, NewHub(), "test", slog.New(slog.DiscardHandler))
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
			if !opts.Follow || !opts.Since.Equal(time.Date(2026, 1, 2, 3, 4, 5, 2, time.UTC)) {
				t.Fatalf("SSE logs must follow and resume strictly after since: %+v", opts)
			}
			return io.NopCloser(bytes.NewReader(wired)), nil
		},
	}, nil, nil)
	rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks/T/logs/stream?since=2026-01-02T03:04:05.000000001Z", nil))
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
	logger := slog.New(slog.DiscardHandler)
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
}

// registry_credential_id null detaches, a value attaches, omission leaves it; {} clears default_env.
func TestUpdateProjectForwardsOnlySuppliedFields(t *testing.T) {
	id, name, image, port, empty := uuid.New(), "Renamed", "nginx:alpine", 8080, map[string]string{}
	var detach *uuid.UUID
	attach := &id
	for body, want := range map[string]service.UpdateProjectInput{
		`{"name":"Renamed"}`:                               {Name: &name},
		`{"registry_credential_id":null}`:                  {RegistryCredentialID: &detach},
		`{"registry_credential_id":"` + id.String() + `"}`: {RegistryCredentialID: &attach},
		`{"default_image":"nginx:alpine","default_port":8080,"default_env":{}}`: {
			DefaultImage: &image, DefaultPort: &port, DefaultEnv: &empty,
		},
	} {
		var got service.UpdateProjectInput
		h := testHandler(stubTasks{}, nil, &stubProjects{update: func(_ context.Context, _ string, in service.UpdateProjectInput) (core.Project, error) {
			got = in
			return core.Project{}, nil
		}})
		rr := do(h, authed(http.MethodPatch, "/api/v1/projects/team", strings.NewReader(body)))
		if rr.Code != http.StatusOK || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: status %d, forwarded %+v", body, rr.Code, got)
		}
	}
}

func TestProjectDTOAlwaysIncludesTaskDefaults(t *testing.T) {
	body := do(testHandler(stubTasks{}, nil, nil), authed(http.MethodGet, "/api/v1/projects/team", nil)).Body.String()
	for _, want := range []string{`"default_image":""`, `"default_port":0`, `"default_env":{}`} {
		if !strings.Contains(body, want) {
			t.Fatalf("project response must include %s: %s", want, body)
		}
	}
}

func TestSSEMetricEntries(t *testing.T) {
	gotTask := "unset"
	h := testHandler(stubTasks{
		metrics: func(_ context.Context, acc core.ProjectAccess, name string) (<-chan core.TaskMetric, error) {
			gotTask = name
			// Must forward what the middleware resolved, not re-derive it.
			if acc.Role != core.ProjectRoleOwner || !acc.AllTasks {
				t.Fatalf("handler passed the wrong access: %+v", acc)
			}
			out := make(chan core.TaskMetric, 1)
			out <- core.TaskMetric{
				TaskName: "web", CPUPercent: 12.5, MemoryBytes: 1024, MemoryLimit: 4096,
				NetworkRXBytes: 7, NetworkTXBytes: 9,
				ObservedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			}
			close(out)
			return out, nil
		},
	}, nil, nil)

	// The empty wildcard is what widens the project stream; a regression here silently narrows it.
	if rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/metrics/stream", nil)); rr.Code != http.StatusOK || gotTask != "" {
		t.Fatalf("project stream = %d for task %q, want the empty wildcard", rr.Code, gotTask)
	}
	rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks/web/metrics/stream", nil))
	if rr.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type = %q", rr.Header().Get("Content-Type"))
	}
	if gotTask != "web" {
		t.Fatalf("task name = %q, want web", gotTask)
	}
	body := rr.Body.String()
	for _, want := range []string{
		`"task":"web"`, `"cpu_percent":12.5`, `"memory_bytes":1024`,
		`"memory_limit":4096`, `"network_rx_bytes":7`, `"network_tx_bytes":9`,
		`"observed_at":"2026-01-02T03:04:05Z"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSE body missing %s: %q", want, body)
		}
	}
	if !strings.HasPrefix(body, "data: ") || !strings.HasSuffix(body, "\n\n") {
		t.Fatalf("body is not SSE framed: %q", body)
	}
}

func TestMetricsStreamPropagatesServiceErrors(t *testing.T) {
	h := testHandler(stubTasks{
		metrics: func(_ context.Context, _ core.ProjectAccess, _ string) (<-chan core.TaskMetric, error) {
			return nil, errors.Join(core.ErrConflict, errors.New("task has no container"))
		},
	}, nil, nil)

	rr := do(h, authed(http.MethodGet, "/api/v1/projects/team/tasks/web/metrics/stream", nil))
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); strings.Contains(ct, "event-stream") {
		t.Fatalf("a failed stream must not claim to be SSE: %q", ct)
	}
}

func TestEventsStream(t *testing.T) {
	hub := NewHub()
	events, unsubscribe := hub.subscribe()
	defer unsubscribe()
	<-events // sent on connect
	hub.Publish()
	hub.Publish() // must not block: the pending event already covers it
	<-events
	select {
	case <-events:
		t.Fatal("two publishes left two events")
	default:
	}
	hub.Close()
	h := APIHandler(stubTasks{}, &stubAuth{user: testMember}, &stubProjects{}, &stubPush{}, hub, "test", slog.New(slog.DiscardHandler))
	if rr := do(h, authed(http.MethodGet, "/api/v1/events/stream", nil)); rr.Body.String() != "data: {}\n\n" {
		t.Fatalf("stream after close = %d %q", rr.Code, rr.Body.String())
	}
}

func TestOnlySuccessfulAuthenticatedWritesPublish(t *testing.T) {
	hub := NewHub()
	events, unsubscribe := hub.subscribe()
	defer unsubscribe()
	<-events
	h := APIHandler(stubTasks{}, &stubAuth{user: testAdmin}, &stubProjects{}, &stubPush{}, hub, "test", slog.New(slog.DiscardHandler))
	member := `{"user_id":"` + uuid.NewString() + `","role":"operator"}`
	for _, tc := range []struct {
		name string
		r    *http.Request
		want bool
	}{
		{"write", authed(http.MethodPost, "/api/v1/projects/demo/members", strings.NewReader(member)), true},
		{"read", authed(http.MethodGet, "/api/v1/projects", nil), false},
		{"private chat", authed(http.MethodPost, "/api/v1/projects/demo/chats", strings.NewReader(`{"message":"How?"}`)), false},
		{"rejected write", authed(http.MethodPost, "/api/v1/projects/demo/members", strings.NewReader(`{`)), false},
		{"public write", httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"alice","password":"pw"}`)), false},
	} {
		code := do(h, tc.r).Code
		select {
		case <-events:
			if !tc.want {
				t.Errorf("%s (%d) published", tc.name, code)
			}
		default:
			if tc.want {
				t.Errorf("%s (%d) did not publish", tc.name, code)
			}
		}
	}
}

func TestOnlyAdminsPickProjectRepositories(t *testing.T) {
	projects := &stubProjects{role: core.ProjectRoleOwner, update: func(context.Context, string, service.UpdateProjectInput) (core.Project, error) {
		return core.Project{}, nil
	}}
	for _, tc := range []struct {
		user core.User
		want int
	}{{testMember, http.StatusForbidden}, {testAdmin, http.StatusOK}} {
		h := testHandler(stubTasks{}, &stubAuth{user: tc.user}, projects)
		rr := do(h, authed(http.MethodPatch, "/api/v1/projects/demo", strings.NewReader(`{"repositories":["github.com/acme/web"]}`)))
		if rr.Code != tc.want {
			t.Errorf("%s sets repositories: %d, want %d", tc.user.Username, rr.Code, tc.want)
		}
		if rr := do(h, authed(http.MethodGet, "/api/v1/code/repositories?query=acme", nil)); rr.Code != tc.want {
			t.Errorf("%s searches repositories: %d, want %d", tc.user.Username, rr.Code, tc.want)
		}
	}
}

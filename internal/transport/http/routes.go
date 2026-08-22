package httptransport

import (
	"net/http"
)

type access int

const (
	accessPublic access = iota
	accessAuthed
	accessSession
	accessAdmin
	accessMember
	accessOwner
)

type route struct {
	method      string
	path        string
	access      access
	handler     func(*Handler, http.ResponseWriter, *http.Request)
	tag         string
	summary     string
	description string
	req         any
	resp        any
	status      int
	contentType string
	extraErrors []int
}

var routeTable = [...]route{
	{
		method: http.MethodGet, path: "/api/v1/health", access: accessPublic,
		handler: (*Handler).health,
		tag:     "system", summary: "Health check",
		resp: new(healthResponse), status: http.StatusOK,
	},
	{
		method: http.MethodGet, path: "/api/v1/stats", access: accessAuthed,
		handler: (*Handler).stats,
		tag:     "system", summary: "Service statistics",
		resp: new(systemStatsDTO), status: http.StatusOK,
	},

	{
		method: http.MethodPost, path: "/api/v1/auth/login", access: accessPublic,
		handler: (*Handler).login,
		tag:     "auth", summary: "Exchange credentials for a token",
		description: "The token is returned once and expires after 30 days.",
		req:         new(loginRequest), resp: new(loginResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest, http.StatusUnauthorized},
	},
	{
		method: http.MethodPost, path: "/api/v1/auth/logout", access: accessAuthed,
		handler: (*Handler).logout,
		tag:     "auth", summary: "Revoke the current token",
		resp: new(successResponse), status: http.StatusOK,
	},
	{
		method: http.MethodGet, path: "/api/v1/auth/me", access: accessAuthed,
		handler: (*Handler).me,
		tag:     "auth", summary: "Current user",
		resp: new(userResponse), status: http.StatusOK,
	},
	{
		method: http.MethodGet, path: "/api/v1/auth/tokens", access: accessSession,
		handler: (*Handler).listAPITokens,
		tag:     "auth", summary: "List your API tokens",
		description: "Returns metadata only; plaintext tokens and token hashes are never returned.",
		resp:        new(apiTokensResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPost, path: "/api/v1/auth/tokens", access: accessSession,
		handler: (*Handler).createAPIToken,
		tag:     "auth", summary: "Create an API token",
		description: "The plaintext token is returned once. Its validity window must not exceed 90 days. " +
			"Only a login session, not another API token, can create it.",
		req: new(createAPITokenRequest), resp: new(createAPITokenResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/auth/tokens/{id}", access: accessSession,
		handler: (*Handler).revokeAPIToken,
		tag:     "auth", summary: "Revoke one of your API tokens",
		description: "Only a login session can revoke an API token, and users can revoke only their own tokens.",
		req:         new(apiTokenPath), resp: new(successResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest, http.StatusNotFound},
	},

	{
		method: http.MethodGet, path: "/api/v1/users", access: accessAdmin,
		handler: (*Handler).listUsers,
		tag:     "users", summary: "List users",
		resp: new(usersResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPost, path: "/api/v1/users", access: accessAdmin,
		handler: (*Handler).createUser,
		tag:     "users", summary: "Create a user",
		req: new(createUserRequest), resp: new(userResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodPatch, path: "/api/v1/users/{id}", access: accessAdmin,
		handler: (*Handler).updateUser,
		tag:     "users", summary: "Update a user",
		description: "Changing the password or role, or disabling the account, revokes that user's tokens.",
		req:         new(updateUserRequest), resp: new(userResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict},
	},
	{
		method: http.MethodDelete, path: "/api/v1/users/{id}", access: accessAdmin,
		handler: (*Handler).deleteUser,
		tag:     "users", summary: "Delete a user",
		description: "A user cannot delete their own account.",
		req:         new(userPath), resp: new(successResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict},
	},

	{
		method: http.MethodGet, path: "/api/v1/registry-credentials", access: accessAdmin,
		handler: (*Handler).listCredentials,
		tag:     "registry-credentials", summary: "List registry credentials",
		description: "Credential tokens are never returned.",
		resp:        new(credentialsResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPost, path: "/api/v1/registry-credentials", access: accessAdmin,
		handler: (*Handler).createCredential,
		tag:     "registry-credentials", summary: "Create a registry credential",
		req: new(createCredentialRequest), resp: new(credentialResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodDelete, path: "/api/v1/registry-credentials/{id}", access: accessAdmin,
		handler: (*Handler).deleteCredential,
		tag:     "registry-credentials", summary: "Delete a registry credential",
		req: new(credentialPath), resp: new(successResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict},
	},

	{
		method: http.MethodGet, path: "/api/v1/projects", access: accessAuthed,
		handler: (*Handler).listProjects,
		tag:     "projects", summary: "List reachable projects",
		description: "Administrators see every project; other users see only their memberships.",
		resp:        new(projectsResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPost, path: "/api/v1/projects", access: accessAuthed,
		handler: (*Handler).createProject,
		tag:     "projects", summary: "Create a project",
		description: "The creator becomes the project owner. The slugs api, health, metrics, static and admin are reserved. " +
			"default_image, default_port and default_env only prefill the task creation form; " +
			"task creation never applies them on its own.",
		req: new(createProjectRequest), resp: new(projectResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}", access: accessMember,
		handler: (*Handler).getProject,
		tag:     "projects", summary: "Get a project",
		req: new(projectPath), resp: new(projectResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPatch, path: "/api/v1/projects/{project}", access: accessOwner,
		handler: (*Handler).updateProject,
		tag:     "projects", summary: "Update a project",
		description: "Send registry_credential_id as null to detach the credential; omit it to leave it unchanged. " +
			"An empty default_image or default_env clears that task form default; " +
			"existing tasks and their containers are never touched.",
		req: new(updateProjectRequest), resp: new(projectResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/projects/{project}", access: accessOwner,
		handler: (*Handler).deleteProject,
		tag:     "projects", summary: "Delete a project",
		description: "A project that still owns tasks cannot be deleted.",
		req:         new(projectPath), resp: new(successResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusConflict},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/members", access: accessOwner,
		handler: (*Handler).listMembers,
		tag:     "projects", summary: "List project members",
		req: new(projectPath), resp: new(membersResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPost, path: "/api/v1/projects/{project}/members", access: accessOwner,
		handler: (*Handler).addMember,
		tag:     "projects", summary: "Add or promote a member",
		req: new(addMemberRequest), resp: new(successResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/projects/{project}/members/{userID}", access: accessOwner,
		handler: (*Handler).removeMember,
		tag:     "projects", summary: "Remove a member",
		description: "The last owner cannot be removed.",
		req:         new(memberPath), resp: new(successResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},

	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/notifications", access: accessMember,
		handler: (*Handler).listNotifications,
		tag:     "projects", summary: "List deploy notifications",
		description: "Newest first. Recorded when a deploy succeeds or fails; a retried callback for " +
			"the image a task already runs records nothing.",
		req: new(notificationsRequest), resp: new(notificationsResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest},
	},

	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks", access: accessMember,
		handler: (*Handler).listTasks,
		tag:     "tasks", summary: "List tasks",
		req: new(projectPath), resp: new(tasksResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPost, path: "/api/v1/projects/{project}/tasks", access: accessMember,
		handler: (*Handler).createTask,
		tag:     "tasks", summary: "Create a task",
		description: "Task names are unique within a project and must match ^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$.",
		req:         new(createTaskRequest), resp: new(taskResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}", access: accessMember,
		handler: (*Handler).getTask,
		tag:     "tasks", summary: "Get a task",
		req: new(taskPath), resp: new(taskResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPatch, path: "/api/v1/projects/{project}/tasks/{name}", access: accessMember,
		handler: (*Handler).updateTask,
		tag:     "tasks", summary: "Update a task",
		description: "Only the fields sent are changed. Changing image, port, labels, or env " +
			"needs a new container: auto_restart applies it immediately and defaults to true, " +
			"otherwise it is applied by the next start or restart. Editing only the description " +
			"leaves a running container untouched.",
		req: new(updateTaskRequest), resp: new(taskResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodDelete, path: "/api/v1/projects/{project}/tasks/{name}", access: accessMember,
		handler: (*Handler).deleteTask,
		tag:     "tasks", summary: "Delete a task",
		description: "Removes the task and its container.",
		req:         new(taskPath), resp: new(taskDeletedResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPost, path: "/api/v1/projects/{project}/tasks/{name}/deploy", access: accessMember,
		handler: (*Handler).deployTask,
		tag:     "tasks", summary: "Deploy an image built elsewhere",
		description: "For a build pipeline to call once it has pushed an image. The image must be " +
			"immutable, of the form repository@sha256:<64 hex digits>, so the exact artifact that was " +
			"built is the one that runs. Boreas pulls it and recreates the container, leaving a stopped " +
			"task stopped. Deploying the image a task already runs changes nothing, so a callback may be " +
			"retried safely.",
		req: new(deployTaskRequest), resp: new(taskResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodPut, path: "/api/v1/projects/{project}/tasks/{name}/state", access: accessMember,
		handler: (*Handler).updateState,
		tag:     "tasks", summary: "Start, stop, or restart a task",
		req: new(updateStateRequest), resp: new(taskStateResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}/logs", access: accessMember,
		handler: (*Handler).logs,
		tag:     "tasks", summary: "Read task logs",
		req: new(logsRequest), resp: new(string), status: http.StatusOK,
		contentType: "text/plain", extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}/logs/stream", access: accessMember,
		handler: (*Handler).streamLogs,
		tag:     "tasks", summary: "Stream task logs",
		description: "Server-Sent Events. Each event carries a JSON object " +
			`{"timestamp": string, "stream": "stdout"|"stderr", "message": string}, ` +
			"and the server sends a comment heartbeat while idle. Use EventSource rather than a generated client method.",
		req: new(streamLogsRequest), resp: new(string), status: http.StatusOK,
		contentType: "text/event-stream", extraErrors: []int{http.StatusBadRequest},
	},
}

func (r route) errorStatuses() []int {
	var statuses []int
	if r.access != accessPublic {
		statuses = append(statuses, http.StatusUnauthorized)
	}
	switch r.access {
	case accessSession, accessAdmin, accessMember, accessOwner:
		statuses = append(statuses, http.StatusForbidden)
	}
	statuses = append(statuses, r.extraErrors...)
	if r.access == accessMember || r.access == accessOwner {
		statuses = append(statuses, http.StatusNotFound)
	}
	statuses = append(statuses, http.StatusInternalServerError)
	return statuses
}

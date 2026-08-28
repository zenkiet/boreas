package httptransport

import (
	"net/http"

	"github.com/zenkiet/boreas/internal/core"
)

type access int

const (
	accessPublic access = iota
	accessAuthed
	accessSession
	accessAdmin
	accessViewer
	accessOperator
	accessMember
	accessOwner
)

// projectRole reports the least project role an access level accepts, or "" when the level is not project-scoped.
func (a access) projectRole() core.ProjectRole {
	switch a {
	case accessViewer:
		return core.ProjectRoleViewer
	case accessOperator:
		return core.ProjectRoleOperator
	case accessMember:
		return core.ProjectRoleMember
	case accessOwner:
		return core.ProjectRoleOwner
	}
	return ""
}

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
		description: "The token is returned once and expires in 30 days.",
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
		description: "Metadata only; tokens and hashes are never returned.",
		resp:        new(apiTokensResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPost, path: "/api/v1/auth/tokens", access: accessSession,
		handler: (*Handler).createAPIToken,
		tag:     "auth", summary: "Create an API token",
		description: "The token is returned once. Validity must not exceed 90 days. " +
			"Requires a login session, not another API token.",
		req: new(createAPITokenRequest), resp: new(createAPITokenResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/auth/tokens/{id}", access: accessSession,
		handler: (*Handler).revokeAPIToken,
		tag:     "auth", summary: "Revoke one of your API tokens",
		description: "Requires a login session. Users revoke only their own tokens.",
		req:         new(apiTokenPath), resp: new(successResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest, http.StatusNotFound},
	},

	{
		method: http.MethodPost, path: "/api/v1/push/subscriptions", access: accessAuthed,
		handler: (*Handler).subscribePush,
		tag:     "push", summary: "Subscribe this device to deploy notifications",
		description: "Takes an FCM registration token. The device receives the deploys the caller can list; " +
			"re-registering moves the subscription to the current caller.",
		req: new(pushSubscriptionRequest), resp: new(successResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/push/subscriptions/{token}", access: accessAuthed,
		handler: (*Handler).unsubscribePush,
		tag:     "push", summary: "Unsubscribe this device",
		description: "Users remove only tokens they registered themselves.",
		req:         new(pushSubscriptionPath), resp: new(successResponse), status: http.StatusOK,
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
		description: "Changing password or role, or disabling the account, revokes that user's tokens.",
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
		description: "Administrators see every project, others only their memberships.",
		resp:        new(projectsResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPost, path: "/api/v1/projects", access: accessAdmin,
		handler: (*Handler).createProject,
		tag:     "projects", summary: "Create a project",
		description: "The creator becomes owner. The slugs api, health, metrics, static and admin " +
			"are reserved. default_image, default_port and default_env only prefill the task " +
			"creation form; task creation never applies them on its own.",
		req: new(createProjectRequest), resp: new(projectResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}", access: accessViewer,
		handler: (*Handler).getProject,
		tag:     "projects", summary: "Get a project",
		req: new(projectPath), resp: new(projectResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPatch, path: "/api/v1/projects/{project}", access: accessOwner,
		handler: (*Handler).updateProject,
		tag:     "projects", summary: "Update a project",
		description: "registry_credential_id null detaches the credential, omitted leaves it unchanged. " +
			"An empty default_image or default_env clears that form default. " +
			"Existing tasks and containers are never touched.",
		req: new(updateProjectRequest), resp: new(projectResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/projects/{project}", access: accessOwner,
		handler: (*Handler).deleteProject,
		tag:     "projects", summary: "Delete a project",
		description: "Refused while the project still owns tasks.",
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
		method: http.MethodGet, path: "/api/v1/projects/{project}/notifications", access: accessViewer,
		handler: (*Handler).listNotifications,
		tag:     "projects", summary: "List deploy notifications",
		description: "Newest first. Recorded when a deploy succeeds or fails; a retried callback " +
			"for the image a task already runs records nothing.",
		req: new(notificationsRequest), resp: new(notificationsResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodPost, path: "/api/v1/projects/{project}/notifications/{id}/seen", access: accessViewer,
		handler: (*Handler).markNotificationSeen,
		tag:     "projects", summary: "Mark a notification seen",
		description: "Seen is tracked per user. Idempotent; an id outside the caller's visibility is a no-op.",
		req:         new(notificationSeenPath), resp: new(successResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/projects/{project}/notifications/{id}/seen", access: accessViewer,
		handler: (*Handler).markNotificationUnseen,
		tag:     "projects", summary: "Mark a notification unseen",
		description: "Clears the caller's own seen mark. Idempotent.",
		req:         new(notificationSeenPath), resp: new(successResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest},
	},

	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks", access: accessViewer,
		handler: (*Handler).listTasks,
		tag:     "tasks", summary: "List tasks",
		req: new(projectPath), resp: new(tasksResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPost, path: "/api/v1/projects/{project}/tasks", access: accessMember,
		handler: (*Handler).createTask,
		tag:     "tasks", summary: "Create a task",
		description: "Names are unique per project and must match ^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$.",
		req:         new(createTaskRequest), resp: new(taskResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}", access: accessViewer,
		handler: (*Handler).getTask,
		tag:     "tasks", summary: "Get a task",
		req: new(taskPath), resp: new(taskResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPatch, path: "/api/v1/projects/{project}/tasks/{name}", access: accessMember,
		handler: (*Handler).updateTask,
		tag:     "tasks", summary: "Update a task",
		description: "Only the fields sent are changed. image, port, labels and env need a new " +
			"container: auto_restart applies it at once and defaults to true, otherwise the next " +
			"start or restart does. description and dev_status leave a running container untouched. " +
			"dev_status tracks the code, not the container: in_progress on create, blocked is not " +
			"fit for QA, ready is.",
		req: new(updateTaskRequest), resp: new(taskResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodDelete, path: "/api/v1/projects/{project}/tasks/{name}", access: accessMember,
		handler: (*Handler).deleteTask,
		tag:     "tasks", summary: "Delete a task",
		description: "The container is removed too.",
		req:         new(taskPath), resp: new(taskDeletedResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPost, path: "/api/v1/projects/{project}/tasks/{name}/deploy", access: accessOperator,
		handler: (*Handler).deployTask,
		tag:     "tasks", summary: "Deploy an image built elsewhere",
		description: "For a build pipeline to call after pushing an image. The image must be immutable, " +
			"of the form repository@sha256:<64 hex digits>, so the artifact that was built is the one " +
			"that runs. Pulls it and recreates the container, leaving a stopped task stopped. " +
			"Deploying the image a task already runs changes nothing, so callbacks are safe to retry.",
		req: new(deployTaskRequest), resp: new(taskResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodPut, path: "/api/v1/projects/{project}/tasks/{name}/state", access: accessOperator,
		handler: (*Handler).updateState,
		tag:     "tasks", summary: "Start, stop, or restart a task",
		req: new(updateStateRequest), resp: new(taskStateResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}/logs", access: accessViewer,
		handler: (*Handler).logs,
		tag:     "tasks", summary: "Read task logs",
		req: new(logsRequest), resp: new(string), status: http.StatusOK,
		contentType: "text/plain", extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}/logs/stream", access: accessViewer,
		handler: (*Handler).streamLogs,
		tag:     "tasks", summary: "Stream task logs",
		description: "Server-Sent Events, each carrying " +
			`{"timestamp": string, "stream": "stdout"|"stderr", "message": string}, ` +
			"plus a comment heartbeat while idle. Use EventSource, not a generated client method.",
		req: new(streamLogsRequest), resp: new(string), status: http.StatusOK,
		contentType: "text/event-stream", extraErrors: []int{http.StatusBadRequest},
	},

	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/metrics/stream", access: accessViewer,
		handler: (*Handler).streamMetrics,
		tag:     "projects", summary: "Stream resource metrics for every running task",
		description: "Server-Sent Events, roughly one per task per second, each carrying " +
			`{"task": string, "cpu_percent": number, "memory_bytes": integer, "memory_limit": integer, ` +
			`"network_rx_bytes": integer, "network_tx_bytes": integer, "observed_at": string}. ` +
			"Tasks that are not running are omitted, and a comment heartbeat fills idle time. " +
			"Use EventSource, not a generated client method.",
		req: new(projectPath), resp: new(string), status: http.StatusOK,
		contentType: "text/event-stream",
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}/metrics/stream", access: accessViewer,
		handler: (*Handler).streamMetrics,
		tag:     "tasks", summary: "Stream resource metrics for one task",
		description: "The project-wide stream's payload, limited to this task. 409 when the task " +
			"has no container yet. Use EventSource, not a generated client method.",
		req: new(taskPath), resp: new(string), status: http.StatusOK,
		contentType: "text/event-stream", extraErrors: []int{http.StatusConflict},
	},

	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}/grants", access: accessOwner,
		handler: (*Handler).listGrants,
		tag:     "tasks", summary: "List task grants",
		req: new(taskPath), resp: new(grantsResponse), status: http.StatusOK,
	},
	{
		method: http.MethodPost, path: "/api/v1/projects/{project}/tasks/{name}/grants", access: accessOwner,
		handler: (*Handler).grantTask,
		tag:     "tasks", summary: "Grant a user access to one task",
		description: "Raises the user's role on this task above what the project gives them, never " +
			"lowers it. A grant reaches the project without membership and dies with the task. " +
			"Owner cannot be granted here.",
		req: new(grantRequest), resp: new(successResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/projects/{project}/tasks/{name}/grants/{userID}", access: accessOwner,
		handler: (*Handler).revokeGrant,
		tag:     "tasks", summary: "Revoke a task grant",
		req: new(grantPath), resp: new(successResponse), status: http.StatusOK,
		extraErrors: []int{http.StatusBadRequest},
	},
}

func (r route) errorStatuses() []int {
	var statuses []int
	if r.access != accessPublic {
		statuses = append(statuses, http.StatusUnauthorized)
	}
	projectScoped := r.access.projectRole() != ""
	if projectScoped || r.access == accessSession || r.access == accessAdmin {
		statuses = append(statuses, http.StatusForbidden)
	}
	statuses = append(statuses, r.extraErrors...)
	if projectScoped {
		statuses = append(statuses, http.StatusNotFound)
	}
	statuses = append(statuses, http.StatusInternalServerError)
	return statuses
}

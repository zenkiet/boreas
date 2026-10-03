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

// The least project role each project-scoped access level accepts; other levels are absent.
var projectRoles = map[access]core.ProjectRole{
	accessViewer: core.ProjectRoleViewer, accessOperator: core.ProjectRoleOperator,
	accessMember: core.ProjectRoleMember, accessOwner: core.ProjectRoleOwner,
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

const readSSE = "Read it with fetch and a stream reader: EventSource cannot send the Authorization header."

var routeTable = [...]route{
	{
		method: http.MethodGet, path: "/api/v1/health", access: accessPublic, handler: (*Handler).health,
		tag: "system", summary: "Health check",
		resp: new(healthResponse),
	},
	{
		method: http.MethodGet, path: "/api/v1/stats", access: accessAuthed, handler: (*Handler).stats,
		tag: "system", summary: "Service statistics",
		resp: new(systemStatsDTO),
	},
	{
		method: http.MethodGet, path: "/api/v1/events/stream", access: accessSession, handler: (*Handler).streamEvents,
		tag: "system", summary: "Stream change signals",
		description: "Server-Sent Events carrying {} once on connect and after any task or notification change, " +
			"with nothing about what changed: refetch what you show, such as GET /api/v1/projects. " +
			"Bursts arrive as one event, and a comment heartbeat fills idle time. " + readSSE,
		resp: new(string), contentType: "text/event-stream",
	},

	{
		method: http.MethodPost, path: "/api/v1/auth/login", access: accessPublic, handler: (*Handler).login,
		tag: "auth", summary: "Exchange credentials for a token",
		description: "The token is returned once and expires in 30 days.",
		req:         new(loginRequest), resp: new(loginResponse),
		extraErrors: []int{http.StatusBadRequest, http.StatusUnauthorized},
	},
	{
		method: http.MethodPost, path: "/api/v1/auth/logout", access: accessAuthed, handler: (*Handler).logout,
		tag: "auth", summary: "Revoke the current token",
		resp: new(successResponse),
	},
	{
		method: http.MethodGet, path: "/api/v1/auth/me", access: accessAuthed, handler: (*Handler).me,
		tag: "auth", summary: "Current user",
		resp: new(userResponse),
	},
	{
		method: http.MethodGet, path: "/api/v1/auth/tokens", access: accessSession, handler: (*Handler).listAPITokens,
		tag: "auth", summary: "List your API tokens",
		description: "Metadata only; tokens and hashes are never returned.",
		resp:        new(apiTokensResponse),
	},
	{
		method: http.MethodPost, path: "/api/v1/auth/tokens", access: accessSession, handler: (*Handler).createAPIToken,
		tag: "auth", summary: "Create an API token",
		description: "The token is returned once. Validity must not exceed 90 days. " +
			"Requires a login session, not another API token.",
		req: new(createAPITokenRequest), resp: new(createAPITokenResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/auth/tokens/{id}", access: accessSession, handler: (*Handler).revokeAPIToken,
		tag: "auth", summary: "Revoke one of your API tokens",
		description: "Requires a login session. Users revoke only their own tokens.",
		req:         new(idPath), resp: new(successResponse),
		extraErrors: []int{http.StatusBadRequest, http.StatusNotFound},
	},

	{
		method: http.MethodPost, path: "/api/v1/push/subscriptions", access: accessAuthed, handler: (*Handler).subscribePush,
		tag: "push", summary: "Subscribe this device to deploy notifications",
		description: "Takes an FCM registration token. The device receives the deploys the caller can list; " +
			"re-registering moves the subscription to the current caller.",
		req: new(pushSubscriptionRequest), resp: new(successResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/push/subscriptions/{token}", access: accessAuthed, handler: (*Handler).unsubscribePush,
		tag: "push", summary: "Unsubscribe this device",
		description: "Users remove only tokens they registered themselves.",
		req:         new(pushSubscriptionPath), resp: new(successResponse),
		extraErrors: []int{http.StatusBadRequest, http.StatusNotFound},
	},

	{
		method: http.MethodGet, path: "/api/v1/users", access: accessAdmin, handler: (*Handler).listUsers,
		tag: "users", summary: "List users",
		resp: new(usersResponse),
	},
	{
		method: http.MethodPost, path: "/api/v1/users", access: accessAdmin, handler: (*Handler).createUser,
		tag: "users", summary: "Create a user",
		req: new(createUserRequest), resp: new(userResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodPatch, path: "/api/v1/users/{id}", access: accessAdmin, handler: (*Handler).updateUser,
		tag: "users", summary: "Update a user",
		description: "Changing password or role, or disabling the account, revokes that user's tokens.",
		req:         new(updateUserRequest), resp: new(userResponse),
		extraErrors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict},
	},
	{
		method: http.MethodDelete, path: "/api/v1/users/{id}", access: accessAdmin, handler: (*Handler).deleteUser,
		tag: "users", summary: "Delete a user",
		description: "A user cannot delete their own account.",
		req:         new(idPath), resp: new(successResponse),
		extraErrors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict},
	},

	{
		method: http.MethodGet, path: "/api/v1/registry-credentials", access: accessAdmin, handler: (*Handler).listCredentials,
		tag: "registry-credentials", summary: "List registry credentials",
		description: "Credential tokens are never returned.",
		resp:        new(credentialsResponse),
	},
	{
		method: http.MethodPost, path: "/api/v1/registry-credentials", access: accessAdmin, handler: (*Handler).createCredential,
		tag: "registry-credentials", summary: "Create a registry credential",
		req: new(createCredentialRequest), resp: new(credentialResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodDelete, path: "/api/v1/registry-credentials/{id}", access: accessAdmin, handler: (*Handler).deleteCredential,
		tag: "registry-credentials", summary: "Delete a registry credential",
		req: new(idPath), resp: new(successResponse),
		extraErrors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict},
	},

	{
		method: http.MethodGet, path: "/api/v1/public/projects", access: accessPublic, handler: (*Handler).publicProjects,
		tag: "system", summary: "Public environment directory",
		description: "Every project with its tasks, reduced to what the public directory pages show. " +
			"Image, port, environment and container identifiers are never included.",
		resp: new(publicProjectsResponse),
	},

	{
		method: http.MethodGet, path: "/api/v1/projects", access: accessAuthed, handler: (*Handler).listProjects,
		tag: "projects", summary: "List reachable projects with their tasks",
		description: "Administrators see every project, others their memberships and grants. my_role is the " +
			"caller's role, per project and per task, where a grant can raise it; last_deploy is the newest deploy.",
		resp: new(projectsResponse),
	},
	{
		method: http.MethodPost, path: "/api/v1/projects", access: accessAdmin, handler: (*Handler).createProject,
		tag: "projects", summary: "Create a project",
		description: "The creator becomes owner. The slugs api, health, metrics, static and admin " +
			"are reserved. default_image, default_port and default_env only prefill the task " +
			"creation form; task creation never applies them on its own.",
		req: new(createProjectRequest), resp: new(projectResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}", access: accessViewer, handler: (*Handler).getProject,
		tag: "projects", summary: "Get a project",
		req: new(projectPath), resp: new(projectResponse),
	},
	{
		method: http.MethodPatch, path: "/api/v1/projects/{project}", access: accessOwner, handler: (*Handler).updateProject,
		tag: "projects", summary: "Update a project",
		description: "registry_credential_id null detaches the credential, omitted leaves it unchanged. " +
			"An empty default_image or default_env clears that form default. " +
			"Existing tasks and containers are never touched.",
		req: new(updateProjectRequest), resp: new(projectResponse), extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/projects/{project}", access: accessOwner, handler: (*Handler).deleteProject,
		tag: "projects", summary: "Delete a project",
		description: "Refused while the project still owns tasks.",
		req:         new(projectPath), resp: new(successResponse), extraErrors: []int{http.StatusConflict},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/members", access: accessOwner, handler: (*Handler).listMembers,
		tag: "projects", summary: "List project members",
		req: new(projectPath), resp: new(membersResponse),
	},
	{
		method: http.MethodPost, path: "/api/v1/projects/{project}/members", access: accessOwner, handler: (*Handler).addMember,
		tag: "projects", summary: "Add or promote a member",
		req: new(addMemberRequest), resp: new(successResponse), extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/projects/{project}/members/{userID}", access: accessOwner, handler: (*Handler).removeMember,
		tag: "projects", summary: "Remove a member",
		description: "The last owner cannot be removed.",
		req:         new(memberPath), resp: new(successResponse),
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},

	{
		method: http.MethodGet, path: "/api/v1/notifications", access: accessAuthed, handler: (*Handler).listNotifications,
		tag: "notifications", summary: "List notifications across projects",
		description: "Newest first, from every project and task the caller reaches. To page, pass the id " +
			"of the last notification received as before.",
		req: new(notificationsPage), resp: new(notificationsResponse),
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodPost, path: "/api/v1/notifications/seen", access: accessAuthed, handler: (*Handler).markNotificationsSeen,
		tag: "notifications", summary: "Mark notifications seen",
		description: "Seen is tracked per user. Idempotent; ids outside the caller's visibility are skipped.",
		req:         new(markSeenRequest), resp: new(successResponse), extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/notifications", access: accessViewer, handler: (*Handler).listNotifications,
		tag: "projects", summary: "List a project's notifications",
		description: "The cross-project feed, limited to this project. A retried deploy callback " +
			"for the image a task already runs records nothing.",
		req: new(notificationsRequest), resp: new(notificationsResponse),
		extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodPost, path: "/api/v1/projects/{project}/notifications/{id}/seen", access: accessViewer, handler: (*Handler).markNotificationSeen,
		tag: "projects", summary: "Mark a notification seen",
		description: "Seen is tracked per user. Idempotent; an id outside the caller's visibility is a no-op.",
		req:         new(notificationSeenPath), resp: new(successResponse), extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/projects/{project}/notifications/{id}/seen", access: accessViewer, handler: (*Handler).markNotificationUnseen,
		tag: "projects", summary: "Mark a notification unseen",
		description: "Clears the caller's own seen mark. Idempotent.",
		req:         new(notificationSeenPath), resp: new(successResponse), extraErrors: []int{http.StatusBadRequest},
	},

	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/folders", access: accessViewer, handler: (*Handler).listFolders,
		tag: "projects", summary: "List the project's shared folders",
		description: "The project's folders in the boreas-appdata volume, which task volumes mount read-only.",
		req:         new(projectPath), resp: new(foldersResponse),
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks", access: accessViewer, handler: (*Handler).listTasks,
		tag: "tasks", summary: "List tasks",
		req: new(projectPath), resp: new(tasksResponse),
	},
	{
		method: http.MethodPost, path: "/api/v1/projects/{project}/tasks", access: accessMember, handler: (*Handler).createTask,
		tag: "tasks", summary: "Create a task",
		description: "Names are unique per project and must match ^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$. " +
			"volumes maps a container path to one of the project's folders, mounted read-only; the folder must exist.",
		req: new(createTaskRequest), resp: new(taskResponse), status: http.StatusCreated,
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}", access: accessViewer, handler: (*Handler).getTask,
		tag: "tasks", summary: "Get a task",
		req: new(taskPath), resp: new(taskResponse),
	},
	{
		method: http.MethodPatch, path: "/api/v1/projects/{project}/tasks/{name}", access: accessMember, handler: (*Handler).updateTask,
		tag: "tasks", summary: "Update a task",
		description: "Only the fields sent are changed. image, port, labels, env and volumes need a new " +
			"container: auto_restart applies it at once and defaults to true, otherwise the next " +
			"start or restart does. description and dev_status leave a running container untouched. " +
			"dev_status tracks the code, not the container: in_progress on create, blocked is not " +
			"fit for QA, ready is.",
		req: new(updateTaskRequest), resp: new(taskResponse),
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodDelete, path: "/api/v1/projects/{project}/tasks/{name}", access: accessMember, handler: (*Handler).deleteTask,
		tag: "tasks", summary: "Delete a task",
		description: "The container is removed too.",
		req:         new(taskPath), resp: new(taskDeletedResponse),
	},
	{
		method: http.MethodPost, path: "/api/v1/projects/{project}/tasks/{name}/deploy", access: accessOperator, handler: (*Handler).deployTask,
		tag: "tasks", summary: "Deploy an image built elsewhere",
		description: "For a build pipeline to call after pushing an image. The image must be immutable, " +
			"of the form repository@sha256:<64 hex digits>, so the artifact that was built is the one " +
			"that runs. Pulls it and recreates the container, leaving a stopped task stopped. " +
			"Deploying the image a task already runs changes nothing, so callbacks are safe to retry.",
		req: new(deployTaskRequest), resp: new(taskResponse),
		extraErrors: []int{http.StatusBadRequest, http.StatusConflict},
	},
	{
		method: http.MethodPut, path: "/api/v1/projects/{project}/tasks/{name}/state", access: accessOperator, handler: (*Handler).updateState,
		tag: "tasks", summary: "Start, stop, or restart a task",
		req: new(updateStateRequest), resp: new(taskStateResponse), extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodPut, path: "/api/v1/projects/{project}/tasks/{name}/build", access: accessOperator, handler: (*Handler).reportBuild,
		tag: "tasks", summary: "Report CI build progress",
		description: "For a pipeline to call as each stage starts and once more when it ends, with the token it deploys with. " +
			"Only the latest report is kept. A final report keeps the stage, progress and url of the running build it ends, " +
			"and a build that turns to failure records a build_failed notification.",
		req: new(reportBuildRequest), resp: new(successResponse), extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}/logs", access: accessViewer, handler: (*Handler).logs,
		tag: "tasks", summary: "Read task logs",
		req: new(logsRequest), resp: new(string),
		contentType: "text/plain", extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}/logs/stream", access: accessViewer, handler: (*Handler).streamLogs,
		tag: "tasks", summary: "Stream task logs",
		description: "Server-Sent Events, each carrying " +
			`{"timestamp": string, "stream": "stdout"|"stderr", "message": string}, ` +
			"plus a comment heartbeat while idle. To resume, pass the last timestamp received as since: every " +
			"later entry follows and tail is ignored. " + readSSE,
		req: new(streamLogsRequest), resp: new(string),
		contentType: "text/event-stream", extraErrors: []int{http.StatusBadRequest},
	},

	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/metrics/stream", access: accessViewer, handler: (*Handler).streamMetrics,
		tag: "projects", summary: "Stream resource metrics for every running task",
		description: "Server-Sent Events, roughly one per task per second, each carrying " +
			`{"task": string, "cpu_percent": number, "memory_bytes": integer, "memory_limit": integer, ` +
			`"network_rx_bytes": integer, "network_tx_bytes": integer, "observed_at": string}. ` +
			"Tasks that are not running are omitted, and a comment heartbeat fills idle time. " + readSSE,
		req: new(projectPath), resp: new(string), contentType: "text/event-stream",
	},
	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}/metrics/stream", access: accessViewer, handler: (*Handler).streamMetrics,
		tag: "tasks", summary: "Stream resource metrics for one task",
		description: "The project-wide stream's payload, limited to this task. 409 when the task " +
			"has no container yet. " + readSSE,
		req: new(taskPath), resp: new(string),
		contentType: "text/event-stream", extraErrors: []int{http.StatusConflict},
	},

	{
		method: http.MethodGet, path: "/api/v1/projects/{project}/tasks/{name}/grants", access: accessOwner, handler: (*Handler).listGrants,
		tag: "tasks", summary: "List task grants",
		req: new(taskPath), resp: new(grantsResponse),
	},
	{
		method: http.MethodPost, path: "/api/v1/projects/{project}/tasks/{name}/grants", access: accessOwner, handler: (*Handler).grantTask,
		tag: "tasks", summary: "Grant a user access to one task",
		description: "Raises the user's role on this task above what the project gives them, never " +
			"lowers it. A grant reaches the project without membership and dies with the task. " +
			"Owner cannot be granted here.",
		req: new(grantRequest), resp: new(successResponse), extraErrors: []int{http.StatusBadRequest},
	},
	{
		method: http.MethodDelete, path: "/api/v1/projects/{project}/tasks/{name}/grants/{userID}", access: accessOwner, handler: (*Handler).revokeGrant,
		tag: "tasks", summary: "Revoke a task grant",
		req: new(grantPath), resp: new(successResponse), extraErrors: []int{http.StatusBadRequest},
	},
}

func (r route) errorStatuses() []int {
	var statuses []int
	if r.access != accessPublic {
		statuses = append(statuses, http.StatusUnauthorized)
	}
	projectScoped := projectRoles[r.access] != ""
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

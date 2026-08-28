package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
	"github.com/zenkiet/boreas/internal/service"
)

type Handler struct {
	tasks    TaskService
	auth     AuthService
	projects ProjectService
	push     PushStore
	logger   *slog.Logger
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "healthy", Service: "boreas"})
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.tasks.SystemStats(r.Context())
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, systemStatsDTO{
		TotalTasks: stats.TotalTasks, RunningTasks: stats.RunningTasks,
		StoppedTasks: stats.StoppedTasks, TotalProjects: stats.TotalProjects,
		TotalMemoryMB: float64(stats.TotalMemoryBytes) / (1024 * 1024),
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	token, user, err := h.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, core.ErrUnauthorized) {
			writeUnauthorized(w)
			return
		}
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Token: token, User: userFromCore(user)})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.auth.Logout(r.Context(), bearerToken(r)); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, userResponse{User: userFromCore(userFrom(r.Context()))})
}

func (h *Handler) createAPIToken(w http.ResponseWriter, r *http.Request) {
	var req createAPITokenRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	raw, token, err := h.auth.CreateAPIToken(r.Context(), userFrom(r.Context()).ID, service.CreateAPITokenInput{
		Name: req.Name, ValidFrom: req.ValidFrom, ValidTo: req.ValidTo,
	})
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, createAPITokenResponse{
		Token: raw, APIToken: apiTokenFromCore(token, time.Now().UTC()),
	})
}

func (h *Handler) listAPITokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.auth.ListAPITokens(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	now := time.Now().UTC()
	result := make([]apiTokenDTO, len(tokens))
	for i := range tokens {
		result[i] = apiTokenFromCore(tokens[i], now)
	}
	writeJSON(w, http.StatusOK, apiTokensResponse{APITokens: result, Total: len(result)})
}

func (h *Handler) revokeAPIToken(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	if err := h.auth.RevokeAPIToken(r.Context(), userFrom(r.Context()).ID, id); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true})
}

func (h *Handler) subscribePush(w http.ResponseWriter, r *http.Request) {
	var req pushSubscriptionRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	if err := core.ValidatePushToken(req.Token); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	if err := h.push.Create(r.Context(), userFrom(r.Context()).ID, req.Token); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, successResponse{Success: true})
}

func (h *Handler) unsubscribePush(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if err := core.ValidatePushToken(token); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	if err := h.push.Delete(r.Context(), userFrom(r.Context()).ID, token); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true})
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.auth.ListUsers(r.Context())
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	result := make([]userDTO, len(users))
	for i := range users {
		result[i] = userFromCore(users[i])
	}
	writeJSON(w, http.StatusOK, usersResponse{Users: result, Total: len(result)})
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	user, err := h.auth.CreateUser(r.Context(), service.CreateUserInput{
		Username: req.Username, Email: req.Email, Password: req.Password, Role: req.Role,
	})
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, userResponse{User: userFromCore(user)})
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	var req updateUserRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	user, err := h.auth.UpdateUser(r.Context(), id, service.UpdateUserInput{
		Email: req.Email, Password: req.Password, Role: req.Role, Disabled: req.Disabled,
	})
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, userResponse{User: userFromCore(user)})
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	if id == userFrom(r.Context()).ID {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "cannot delete your own account"})
		return
	}
	if err := h.auth.DeleteUser(r.Context(), id); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true})
}

func (h *Handler) listCredentials(w http.ResponseWriter, r *http.Request) {
	credentials, err := h.projects.ListCredentials(r.Context())
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	result := make([]credentialDTO, len(credentials))
	for i := range credentials {
		result[i] = credentialFromCore(credentials[i])
	}
	writeJSON(w, http.StatusOK, credentialsResponse{Credentials: result, Total: len(result)})
}

func (h *Handler) createCredential(w http.ResponseWriter, r *http.Request) {
	var req createCredentialRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	credential, err := h.projects.CreateCredential(r.Context(), userFrom(r.Context()), service.CreateCredentialInput{
		Name: req.Name, Registry: req.Registry, Username: req.Username, Token: req.Token,
	})
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, credentialResponse{Credential: credentialFromCore(credential)})
}

func (h *Handler) deleteCredential(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	if err := h.projects.DeleteCredential(r.Context(), id); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true})
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.projects.List(r.Context(), userFrom(r.Context()))
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	result := make([]projectDTO, len(projects))
	for i := range projects {
		result[i] = projectFromCore(projects[i])
	}
	writeJSON(w, http.StatusOK, projectsResponse{Projects: result, Total: len(result)})
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	project, err := h.projects.Create(r.Context(), userFrom(r.Context()), service.CreateProjectInput{
		Slug: req.Slug, Name: req.Name, RegistryCredentialID: req.RegistryCredentialID,
		DefaultImage: req.DefaultImage, DefaultPort: req.DefaultPort, DefaultEnv: req.DefaultEnv,
	})
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, projectResponse{Project: projectFromCore(project)})
}

func (h *Handler) getProject(w http.ResponseWriter, r *http.Request) {
	acc := accessFrom(r.Context())
	dto := projectFromCore(acc.Project)
	if !acc.AllTasks {
		// Task form defaults may carry project secrets, so grantees do not receive them.
		dto.DefaultEnv = map[string]string{}
	}
	writeJSON(w, http.StatusOK, projectResponse{Project: dto})
}

func (h *Handler) updateProject(w http.ResponseWriter, r *http.Request) {
	var req updateProjectRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	in := service.UpdateProjectInput{
		Name: req.Name, DefaultImage: req.DefaultImage,
		DefaultPort: req.DefaultPort, DefaultEnv: req.DefaultEnv,
	}
	if req.RegistryCredentialID.Set {
		in.RegistryCredentialID = &req.RegistryCredentialID.Value
	}
	project, err := h.projects.Update(r.Context(), r.PathValue("project"), in)
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, projectResponse{Project: projectFromCore(project)})
}

func (h *Handler) deleteProject(w http.ResponseWriter, r *http.Request) {
	if err := h.projects.Delete(r.Context(), r.PathValue("project")); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true})
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	members, err := h.projects.ListMembers(r.Context(), r.PathValue("project"))
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	result := make([]memberDTO, len(members))
	for i := range members {
		member := members[i]
		result[i] = memberDTO{UserID: member.UserID, Username: member.Username, Role: member.Role, CreatedAt: member.CreatedAt}
	}
	writeJSON(w, http.StatusOK, membersResponse{Members: result, Total: len(result)})
}

func (h *Handler) listNotifications(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			writeBadRequest(w)
			return
		}
		limit = parsed
	}
	notifications, err := h.projects.Notifications(r.Context(), accessFrom(r.Context()), limit)
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	result := make([]notificationDTO, len(notifications))
	for i, n := range notifications {
		result[i] = notificationDTO{
			ID: n.ID, TaskName: n.TaskName, Status: n.Status,
			Title: n.Title, Body: n.Body, Seen: n.Seen, CreatedAt: n.CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, notificationsResponse{Notifications: result, Total: len(result)})
}

func (h *Handler) markNotificationSeen(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	if err := h.projects.MarkNotificationSeen(r.Context(), accessFrom(r.Context()), id); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true})
}

func (h *Handler) markNotificationUnseen(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	if err := h.projects.MarkNotificationUnseen(r.Context(), accessFrom(r.Context()), id); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true})
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	var req addMemberRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	role := req.Role
	if role == "" {
		role = core.ProjectRoleMember
	}
	if err := h.projects.AddMember(r.Context(), r.PathValue("project"), req.UserID, role); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true})
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(r.PathValue("userID"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	if err := h.projects.RemoveMember(r.Context(), r.PathValue("project"), userID); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true})
}

func (h *Handler) listGrants(w http.ResponseWriter, r *http.Request) {
	grants, err := h.projects.ListGrants(r.Context(), r.PathValue("project"), r.PathValue("name"))
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	result := make([]grantDTO, len(grants))
	for i := range grants {
		grant := grants[i]
		result[i] = grantDTO{
			UserID: grant.UserID, Username: grant.Username,
			Role: grant.Role, CreatedAt: grant.CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, grantsResponse{Grants: result, Total: len(result)})
}

func (h *Handler) grantTask(w http.ResponseWriter, r *http.Request) {
	var req grantRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	role := req.Role
	if role == "" {
		role = core.ProjectRoleViewer
	}
	err := h.projects.Grant(r.Context(), r.PathValue("project"), r.PathValue("name"), req.UserID, role)
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true})
}

func (h *Handler) revokeGrant(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(r.PathValue("userID"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	if err := h.projects.Revoke(r.Context(), r.PathValue("project"), r.PathValue("name"), userID); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, successResponse{Success: true})
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.tasks.List(r.Context(), accessFrom(r.Context()))
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	result := make([]taskDTO, len(tasks))
	for i := range tasks {
		result[i] = taskFromCore(tasks[i])
	}
	writeJSON(w, http.StatusOK, tasksResponse{Tasks: result, Total: len(result)})
}

func (h *Handler) getTask(w http.ResponseWriter, r *http.Request) {
	task, err := h.tasks.Get(r.Context(), r.PathValue("project"), r.PathValue("name"))
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, taskResponse{Task: taskFromCore(task)})
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	task, err := h.tasks.Create(r.Context(), r.PathValue("project"), service.CreateTaskInput{
		Name: req.Name, Description: req.Description, Note: req.Note, Image: req.Image,
		Port: req.Port, Labels: req.Labels, Env: req.Env,
	})
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, taskResponse{Task: taskFromCore(task)})
}

func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request) {
	var req updateTaskRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	autoRestart := req.AutoRestart == nil || *req.AutoRestart
	task, err := h.tasks.Update(r.Context(), r.PathValue("project"), r.PathValue("name"),
		service.UpdateTaskInput{
			Description: req.Description, Note: req.Note, DevStatus: req.DevStatus, Image: req.Image, Port: req.Port,
			Labels: req.Labels, Env: req.Env,
		}, autoRestart)
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, taskResponse{Task: taskFromCore(task)})
}

func (h *Handler) deployTask(w http.ResponseWriter, r *http.Request) {
	var req deployTaskRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	task, err := h.tasks.Deploy(r.Context(), r.PathValue("project"), r.PathValue("name"), req.Image)
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, taskResponse{Task: taskFromCore(task)})
}

func (h *Handler) updateState(w http.ResponseWriter, r *http.Request) {
	var req updateStateRequest
	if err := decodeJSON(w, r, maxRequestBytes, &req); err != nil {
		writeBadRequest(w)
		return
	}
	project, name := r.PathValue("project"), r.PathValue("name")
	var task core.Task
	var err error
	switch strings.ToLower(req.Action) {
	case "start":
		task, err = h.tasks.Start(r.Context(), project, name)
	case "stop":
		task, err = h.tasks.Stop(r.Context(), project, name)
	case "restart":
		task, err = h.tasks.Restart(r.Context(), project, name)
	default:
		err = errors.Join(core.ErrInvalidInput, errors.New("unknown state action"))
	}
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, taskStateResponse{Success: true, Task: taskFromCore(task)})
}

func (h *Handler) deleteTask(w http.ResponseWriter, r *http.Request) {
	if err := h.tasks.Delete(r.Context(), r.PathValue("project"), r.PathValue("name")); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, taskDeletedResponse{Success: true, Message: "task deleted"})
}

func parseTail(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("tail")
	if raw == "" {
		return 100, nil
	}
	tail, err := strconv.Atoi(raw)
	if err != nil || tail < 0 {
		return 0, errors.Join(core.ErrInvalidInput, errors.New("tail must be a non-negative integer"))
	}
	return tail, nil
}

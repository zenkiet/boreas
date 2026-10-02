package httptransport

import (
	"cmp"
	"context"
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
	version  string
	logger   *slog.Logger
}

var succeeded = successResponse{Success: true}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "healthy", Service: "boreas", Version: h.version})
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.tasks.SystemStats(r.Context())
	h.reply(w, http.StatusOK, systemStatsDTO{
		TotalTasks: stats.TotalTasks, RunningTasks: stats.RunningTasks,
		StoppedTasks: stats.StoppedTasks, TotalProjects: stats.TotalProjects,
		TotalMemoryMB: float64(stats.TotalMemoryBytes) / (1024 * 1024),
	}, err)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decode(w, r, &req) {
		return
	}
	token, user, err := h.auth.Login(r.Context(), req.Username, req.Password)
	h.reply(w, http.StatusOK, loginResponse{Token: token, User: userFromCore(user)}, err)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	h.reply(w, http.StatusOK, succeeded, h.auth.Logout(r.Context(), bearerToken(r)))
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, userResponse{User: userFromCore(userFrom(r.Context()))})
}

func (h *Handler) createAPIToken(w http.ResponseWriter, r *http.Request) {
	var req createAPITokenRequest
	if !decode(w, r, &req) {
		return
	}
	raw, token, err := h.auth.CreateAPIToken(r.Context(), userFrom(r.Context()).ID, service.CreateAPITokenInput{
		Name: req.Name, ValidFrom: req.ValidFrom, ValidTo: req.ValidTo,
	})
	h.reply(w, http.StatusCreated, createAPITokenResponse{Token: raw, APIToken: apiTokenFromCore(token, time.Now().UTC())}, err)
}

func (h *Handler) listAPITokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.auth.ListAPITokens(r.Context(), userFrom(r.Context()).ID)
	now := time.Now().UTC()
	result := convert(tokens, func(t core.AuthToken) apiTokenDTO { return apiTokenFromCore(t, now) })
	h.reply(w, http.StatusOK, apiTokensResponse{APITokens: result, Total: len(result)}, err)
}

func (h *Handler) revokeAPIToken(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	h.reply(w, http.StatusOK, succeeded, h.auth.RevokeAPIToken(r.Context(), userFrom(r.Context()).ID, id))
}

func (h *Handler) subscribePush(w http.ResponseWriter, r *http.Request) {
	var req pushSubscriptionRequest
	if !decode(w, r, &req) {
		return
	}
	if err := core.ValidatePushToken(req.Token); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	h.reply(w, http.StatusCreated, succeeded, h.push.Create(r.Context(), userFrom(r.Context()).ID, req.Token))
}

func (h *Handler) unsubscribePush(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if err := core.ValidatePushToken(token); err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	h.reply(w, http.StatusOK, succeeded, h.push.Delete(r.Context(), userFrom(r.Context()).ID, token))
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.auth.ListUsers(r.Context())
	result := convert(users, userFromCore)
	h.reply(w, http.StatusOK, usersResponse{Users: result, Total: len(result)}, err)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if !decode(w, r, &req) {
		return
	}
	user, err := h.auth.CreateUser(r.Context(), service.CreateUserInput{
		Username: req.Username, Email: req.Email, Password: req.Password, Role: req.Role,
	})
	h.reply(w, http.StatusCreated, userResponse{User: userFromCore(user)}, err)
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	var req updateUserRequest
	if !decode(w, r, &req) {
		return
	}
	user, err := h.auth.UpdateUser(r.Context(), id, service.UpdateUserInput{
		Email: req.Email, Password: req.Password, Role: req.Role, Disabled: req.Disabled,
	})
	h.reply(w, http.StatusOK, userResponse{User: userFromCore(user)}, err)
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
	h.reply(w, http.StatusOK, succeeded, h.auth.DeleteUser(r.Context(), id))
}

func (h *Handler) listCredentials(w http.ResponseWriter, r *http.Request) {
	credentials, err := h.projects.ListCredentials(r.Context())
	result := convert(credentials, credentialFromCore)
	h.reply(w, http.StatusOK, credentialsResponse{Credentials: result, Total: len(result)}, err)
}

func (h *Handler) createCredential(w http.ResponseWriter, r *http.Request) {
	var req createCredentialRequest
	if !decode(w, r, &req) {
		return
	}
	credential, err := h.projects.CreateCredential(r.Context(), userFrom(r.Context()), service.CreateCredentialInput{
		Name: req.Name, Registry: req.Registry, Username: req.Username, Token: req.Token,
	})
	h.reply(w, http.StatusCreated, credentialResponse{Credential: credentialFromCore(credential)}, err)
}

func (h *Handler) deleteCredential(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	h.reply(w, http.StatusOK, succeeded, h.projects.DeleteCredential(r.Context(), id))
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	accesses, fleet, err := h.projects.Fleet(r.Context(), userFrom(r.Context()))
	result := make([]fleetProjectDTO, len(accesses))
	for i, acc := range accesses {
		tasks := make([]fleetTaskDTO, len(fleet[acc.Project.ID]))
		for j, task := range fleet[acc.Project.ID] {
			tasks[j] = fleetTaskDTO{
				Name: task.Name, Description: task.Description, Image: task.Image,
				Status: task.Status, DevStatus: task.DevStatus, MyRole: task.Role, Build: task.Build,
			}
			if task.LastDeploy != nil {
				tasks[j].LastDeploy = &lastDeployDTO{Status: task.LastDeploy.Status, At: task.LastDeploy.CreatedAt}
			}
		}
		result[i] = fleetProjectDTO{projectDTO: projectFromCore(acc.Project, acc.Role), Tasks: tasks}
	}
	h.reply(w, http.StatusOK, projectsResponse{Projects: result, Total: len(result)}, err)
}

func (h *Handler) publicProjects(w http.ResponseWriter, r *http.Request) {
	projects, tasks, err := h.projects.Directory(r.Context())
	grouped := make(map[uuid.UUID][]publicTaskDTO, len(projects))
	for _, task := range tasks {
		grouped[task.ProjectID] = append(grouped[task.ProjectID], publicTaskDTO{
			Name: task.Name, Description: task.Description, Note: task.Note,
			Status: task.Status, DevStatus: task.DevStatus, UpdatedAt: task.UpdatedAt,
		})
	}
	result := make([]publicProjectDTO, len(projects))
	for i, project := range projects {
		// Appending to an empty literal keeps a project without tasks at [] rather than null.
		result[i] = publicProjectDTO{Slug: project.Slug, Name: project.Name, Tasks: append([]publicTaskDTO{}, grouped[project.ID]...)}
	}
	h.reply(w, http.StatusOK, publicProjectsResponse{Projects: result, Total: len(result)}, err)
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if !decode(w, r, &req) {
		return
	}
	project, err := h.projects.Create(r.Context(), userFrom(r.Context()), service.CreateProjectInput{
		Slug: req.Slug, Name: req.Name, RegistryCredentialID: req.RegistryCredentialID,
		DefaultImage: req.DefaultImage, DefaultPort: req.DefaultPort, DefaultEnv: req.DefaultEnv,
	})
	h.reply(w, http.StatusCreated, projectResponse{Project: projectFromCore(project, core.ProjectRoleOwner)}, err)
}

func (h *Handler) getProject(w http.ResponseWriter, r *http.Request) {
	acc := accessFrom(r.Context())
	dto := projectFromCore(acc.Project, acc.Role)
	if !acc.AllTasks {
		// Task form defaults may carry project secrets, so grantees do not receive them.
		dto.DefaultEnv = map[string]string{}
	}
	writeJSON(w, http.StatusOK, projectResponse{Project: dto})
}

func (h *Handler) updateProject(w http.ResponseWriter, r *http.Request) {
	var req updateProjectRequest
	if !decode(w, r, &req) {
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
	h.reply(w, http.StatusOK, projectResponse{Project: projectFromCore(project, accessFrom(r.Context()).Role)}, err)
}

func (h *Handler) deleteProject(w http.ResponseWriter, r *http.Request) {
	h.reply(w, http.StatusOK, succeeded, h.projects.Delete(r.Context(), r.PathValue("project")))
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	members, err := h.projects.ListMembers(r.Context(), r.PathValue("project"))
	result := convert(members, func(m core.ProjectMember) memberDTO {
		return memberDTO{UserID: m.UserID, Username: m.Username, Role: m.Role, CreatedAt: m.CreatedAt}
	})
	h.reply(w, http.StatusOK, membersResponse{Members: result, Total: len(result)}, err)
}

// Serves both feeds: only the project route scopes the list to its project.
func (h *Handler) listNotifications(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit := 50
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			writeBadRequest(w)
			return
		}
		limit = parsed
	}
	var projectID, before *uuid.UUID
	if raw := query.Get("before"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeBadRequest(w)
			return
		}
		before = &id
	}
	if r.PathValue("project") != "" {
		id := accessFrom(r.Context()).Project.ID
		projectID = &id
	}
	notifications, err := h.projects.Notifications(r.Context(), userFrom(r.Context()), projectID, before, limit)
	result := convert(notifications, func(n core.Notification) notificationDTO {
		return notificationDTO{
			ID: n.ID, Project: n.Project, TaskName: n.TaskName, Type: n.Type, Status: n.Status,
			Title: n.Title, Body: n.Body, Seen: n.Seen, CreatedAt: n.CreatedAt,
		}
	})
	h.reply(w, http.StatusOK, notificationsResponse{Notifications: result, Total: len(result)}, err)
}

func (h *Handler) markNotificationsSeen(w http.ResponseWriter, r *http.Request) {
	var req markSeenRequest
	if !decode(w, r, &req) {
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > 200 {
		writeBadRequest(w)
		return
	}
	h.reply(w, http.StatusOK, succeeded, h.projects.MarkNotificationsSeen(r.Context(), userFrom(r.Context()), req.IDs))
}

func (h *Handler) markNotificationSeen(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	h.reply(w, http.StatusOK, succeeded,
		h.projects.MarkNotificationsSeen(r.Context(), userFrom(r.Context()), []uuid.UUID{id}))
}

func (h *Handler) markNotificationUnseen(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	h.reply(w, http.StatusOK, succeeded, h.projects.MarkNotificationUnseen(r.Context(), accessFrom(r.Context()), id))
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	var req addMemberRequest
	if !decode(w, r, &req) {
		return
	}
	h.reply(w, http.StatusOK, succeeded, h.projects.AddMember(r.Context(), r.PathValue("project"),
		req.UserID, cmp.Or(req.Role, core.ProjectRoleMember)))
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(r.PathValue("userID"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	h.reply(w, http.StatusOK, succeeded, h.projects.RemoveMember(r.Context(), r.PathValue("project"), userID))
}

func (h *Handler) listGrants(w http.ResponseWriter, r *http.Request) {
	grants, err := h.projects.ListGrants(r.Context(), r.PathValue("project"), r.PathValue("name"))
	result := convert(grants, func(g core.TaskGrant) grantDTO {
		return grantDTO{UserID: g.UserID, Username: g.Username, Role: g.Role, CreatedAt: g.CreatedAt}
	})
	h.reply(w, http.StatusOK, grantsResponse{Grants: result, Total: len(result)}, err)
}

func (h *Handler) grantTask(w http.ResponseWriter, r *http.Request) {
	var req grantRequest
	if !decode(w, r, &req) {
		return
	}
	h.reply(w, http.StatusOK, succeeded, h.projects.Grant(r.Context(), r.PathValue("project"), r.PathValue("name"),
		req.UserID, cmp.Or(req.Role, core.ProjectRoleViewer)))
}

func (h *Handler) revokeGrant(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(r.PathValue("userID"))
	if err != nil {
		writeBadRequest(w)
		return
	}
	h.reply(w, http.StatusOK, succeeded,
		h.projects.Revoke(r.Context(), r.PathValue("project"), r.PathValue("name"), userID))
}

func (h *Handler) listFolders(w http.ResponseWriter, r *http.Request) {
	folders, err := h.tasks.Folders(r.Context(), r.PathValue("project"))
	h.reply(w, http.StatusOK, foldersResponse{Folders: folders, Total: len(folders)}, err)
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	acc := accessFrom(r.Context())
	tasks, err := h.tasks.List(r.Context(), acc)
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	grants, err := h.projects.GrantedRoles(r.Context(), acc.UserID)
	result := convert(tasks, func(t core.Task) taskDTO { return taskFromCore(t, acc.Role.Max(grants[t.ID])) })
	h.reply(w, http.StatusOK, tasksResponse{Tasks: result, Total: len(result)}, err)
}

func (h *Handler) getTask(w http.ResponseWriter, r *http.Request) {
	task, err := h.tasks.Get(r.Context(), r.PathValue("project"), r.PathValue("name"))
	h.reply(w, http.StatusOK, taskResponse{Task: taskFromCore(task, accessFrom(r.Context()).Role)}, err)
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if !decode(w, r, &req) {
		return
	}
	task, err := h.tasks.Create(r.Context(), r.PathValue("project"), service.CreateTaskInput{
		Name: req.Name, Description: req.Description, Note: req.Note, Image: req.Image,
		Port: req.Port, Labels: req.Labels, Env: req.Env, Volumes: req.Volumes,
	})
	h.reply(w, http.StatusCreated, taskResponse{Task: taskFromCore(task, accessFrom(r.Context()).Role)}, err)
}

func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request) {
	var req updateTaskRequest
	if !decode(w, r, &req) {
		return
	}
	task, err := h.tasks.Update(r.Context(), r.PathValue("project"), r.PathValue("name"),
		service.UpdateTaskInput{
			Description: req.Description, Note: req.Note, DevStatus: req.DevStatus, Image: req.Image, Port: req.Port,
			Labels: req.Labels, Env: req.Env, Volumes: req.Volumes,
		}, req.AutoRestart == nil || *req.AutoRestart)
	h.reply(w, http.StatusOK, taskResponse{Task: taskFromCore(task, accessFrom(r.Context()).Role)}, err)
}

func (h *Handler) deployTask(w http.ResponseWriter, r *http.Request) {
	var req deployTaskRequest
	if !decode(w, r, &req) {
		return
	}
	task, err := h.tasks.Deploy(r.Context(), r.PathValue("project"), r.PathValue("name"), req.Image)
	h.reply(w, http.StatusOK, taskResponse{Task: taskFromCore(task, accessFrom(r.Context()).Role)}, err)
}

func (h *Handler) reportBuild(w http.ResponseWriter, r *http.Request) {
	var req reportBuildRequest
	if !decode(w, r, &req) {
		return
	}
	h.reply(w, http.StatusOK, succeeded, h.tasks.ReportBuild(r.Context(), r.PathValue("project"), r.PathValue("name"),
		core.Build{State: req.State, Stage: req.Stage, Progress: req.Progress, URL: req.URL}))
}

func (h *Handler) updateState(w http.ResponseWriter, r *http.Request) {
	var req updateStateRequest
	if !decode(w, r, &req) {
		return
	}
	act, ok := map[string]func(context.Context, string, string) (core.Task, error){
		"start": h.tasks.Start, "stop": h.tasks.Stop, "restart": h.tasks.Restart,
	}[strings.ToLower(req.Action)]
	if !ok {
		writeServiceError(w, h.logger, core.ErrInvalidInput)
		return
	}
	task, err := act(r.Context(), r.PathValue("project"), r.PathValue("name"))
	h.reply(w, http.StatusOK, taskStateResponse{Success: true, Task: taskFromCore(task, accessFrom(r.Context()).Role)}, err)
}

func (h *Handler) deleteTask(w http.ResponseWriter, r *http.Request) {
	h.reply(w, http.StatusOK, taskDeletedResponse{Success: true, Message: "task deleted"},
		h.tasks.Delete(r.Context(), r.PathValue("project"), r.PathValue("name")))
}

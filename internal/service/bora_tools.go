package service

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/zenkiet/boreas/internal/core"
)

const (
	maxActions    = 10
	logTail       = 16 << 10
	activityLimit = 30
	// Docker reports CPU from the second sample of a stream, about a second in.
	metricsWindow = 2500 * time.Millisecond
)

var envKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// tool decodes the model's arguments and tells it an error as text; some models send "" for no arguments.
func tool[T any](name, description string, params map[string]any, run func(context.Context, T) (string, error)) core.Tool {
	return core.Tool{Name: name, Description: description, Params: params, Run: func(ctx context.Context, raw string) string {
		var in T
		if err := json.Unmarshal([]byte(cmp.Or(raw, "{}")), &in); err != nil {
			return "error: arguments are not valid JSON"
		}
		out, err := run(ctx, in)
		if err != nil {
			return err.Error()
		}
		return out
	}}
}

// object requires every property it lists.
func object(properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": append([]string{}, slices.Sorted(maps.Keys(properties))...)}
}

func text(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integer(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

type args struct {
	Project, Task, Since, Query, Repo, Path, Title string
	Tail, Limit                                    int
}

var proposeParams = map[string]any{
	"type": "object", "required": []string{"op", "project", "reason"},
	"properties": map[string]any{
		"op":                  map[string]any{"type": "string", "enum": core.BoraOp("").Enum()},
		"project":             text("project slug"),
		"task":                text("task.* and grant.*: the task's name; for task.create, the new task's name"),
		"reason":              text("one short sentence shown by the button, in the user's language"),
		"username":            text("member.* and grant.*: who"),
		"role":                map[string]any{"type": "string", "enum": core.ProjectRole("").Enum(), "description": "member.add, member.role and grant.add; a grant's role stops at member"},
		"name":                text("project.update: the project's new name"),
		"description":         text("task.create or task.update: the task's description"),
		"note":                text("task.update: the task's note, in Markdown"),
		"dev_status":          map[string]any{"type": "string", "enum": core.DevStatus("").Enum(), "description": "task.update"},
		"image":               text("task.create or task.update: the image to run; project.update: the default image"),
		"port":                integer("task.create or task.update: the container port; project.update: the default port"),
		"auto_restart":        map[string]any{"type": "boolean", "description": "task.update: false applies image, port or volume changes at the next start instead of now"},
		"volumes":             map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "task.update: every mount, from container path to project folder; replaces them all"},
		"set":                 map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "task.env, or project.update's default env: variables to set"},
		"unset":               map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "task.env, or project.update's default env: variable names to remove"},
		"registry_credential": text("project.update: the registry credential to pull images with, by name"),
	},
}

// tools are what Bora may use for one answer; only a first answer may name its conversation.
func (b *bora) tools(first bool) []core.Tool {
	every := text("a project slug, or empty for every project")
	project, task := text("project slug"), text("task name")
	tools := []core.Tool{
		tool("list_projects", "List your projects: your role, how many tasks run, stop or fail, and the last deploy.",
			object(map[string]any{}), func(context.Context, args) (string, error) {
				b.step("Looking at your projects…")
				return b.listProjects(), nil
			}),
		tool("list_tasks", "List the tasks of one project, or of every project: status, dev status, image, port, description, error, latest build and last deploy.",
			object(map[string]any{"project": every}), b.listTasks),
		tool("task_details", "Show one task in full: its list_tasks line plus its note, env variable names (never values), volumes and whether it waits to be recreated.",
			object(map[string]any{"project": project, "task": task}), b.taskDetails),
		tool("activity", "List the newest events (deploys, failures, status changes, builds) of one project, or of every project.",
			object(map[string]any{"project": every, "limit": integer("how many, 1 to 50; 0 for 30")}), b.activity),
		tool("project_settings", "Show a project's name, default image and port, default env variable names (never values) and whether it pulls with a registry credential.",
			object(map[string]any{"project": project}), b.projectSettings),
		tool("propose_action", "Offer the user a button for one change: start, stop, restart, create, delete or update a task or set its env variables; "+
			"add a member, change their role or remove them; grant someone a role on one task or revoke it; update or delete a project. Nothing runs unless the user confirms.",
			proposeParams, b.propose),
	}
	if b.s.Logs != nil {
		tools = append(tools, tool("task_logs", "Read one task's newest log lines with their times, only those from a time on when since is set.",
			object(map[string]any{"project": project, "task": task, "tail": integer("how many lines, 1 to 200; 0 for 200"), "since": text("an RFC 3339 time, or empty")}),
			b.taskLogs))
	}
	if b.s.Metrics != nil {
		tools = append(tools, tool("metrics", "Measure CPU, memory and network now, for one task or every running task of a project.",
			object(map[string]any{"project": project, "task": text("task name, or empty for every running task")}), b.metrics))
	}
	if slices.ContainsFunc(b.members, func(acc core.ProjectAccess) bool { return acc.Role == core.ProjectRoleOwner }) {
		tools = append(tools,
			tool("members", "List who is a member of a project you own, with their role.", object(map[string]any{"project": project}), b.listMembers),
			tool("grants", "List who holds a grant on one task of a project you own, with its role.",
				object(map[string]any{"project": project, "task": task}), b.listGrants))
	}
	if b.readsCode() {
		code := text("project slug, or empty for the project asked about")
		tools = append(tools,
			tool("search_code", "Search a project's code for words that must all appear; quote a phrase, and file:<regex> narrows paths. "+
				"Returns snippets with repository, file and line.", object(map[string]any{"project": code, "query": text("the words")}), b.searchCode),
			tool("read_file", "Read one file of a project's code.", object(map[string]any{
				"project": code, "repo": text("repository as search_code returns it"), "path": text("file path as search_code returns it"),
			}), b.readFile))
	}
	if first {
		tools = append(tools, tool("set_title", "Name this new conversation in the user's list, in their language, in at most 60 characters.",
			object(map[string]any{"title": text("the name")}), b.setTitle))
	}
	return tools
}

func (b *bora) readsCode() bool { return slices.ContainsFunc(b.members, b.readable) }

func (b *bora) readable(acc core.ProjectAccess) bool {
	return b.s.Code != nil && len(acc.Project.Repositories) > 0
}

// step shows what Bora does, naming only what was checked, or the file it is about to read.
func (b *bora) step(text string) {
	if b.steps != nil {
		b.steps(text)
	}
}

// scope is the named project, or every one for an empty slug; either counts as read.
func (b *bora) scope(slug string) ([]core.ProjectAccess, error) {
	if slug != "" {
		acc, ok := b.project(slug)
		if !ok {
			return nil, notMember(slug)
		}
		return []core.ProjectAccess{acc}, nil
	}
	for _, acc := range b.members {
		b.read = append(b.read, acc.Project.ID)
	}
	return b.members, nil
}

func named(scope []core.ProjectAccess) string {
	if len(scope) == 1 {
		return scope[0].Project.Slug
	}
	return "your projects"
}

func notMember(slug string) error {
	return errors.New("error: you are not a member of a project named " + slug)
}

func noTask(slug, name string) error {
	return errors.New("error: there is no task " + slug + "/" + name + " in your projects")
}

func taskLine(t core.FleetTask) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s: %s", t.Name, t.Status)
	if t.DevStatus != "" {
		fmt.Fprintf(&out, ", dev %s", t.DevStatus)
	}
	fmt.Fprintf(&out, ", image %s, port %d", t.Image, t.Port)
	if t.Description != "" {
		fmt.Fprintf(&out, ", about: %.200s", oneLine(t.Description))
	}
	if t.Error != "" {
		fmt.Fprintf(&out, ", error: %s", oneLine(t.Error))
	}
	if t.Build.State != "" {
		fmt.Fprintf(&out, ", build %s", strings.TrimSpace(string(t.Build.State)+" "+t.Build.Stage))
	}
	if t.LastDeploy != nil {
		fmt.Fprintf(&out, ", last deploy %s at %s", t.LastDeploy.Status, t.LastDeploy.CreatedAt.Format(time.RFC3339))
	}
	return out.String()
}

func oneLine(text string) string { return strings.Join(strings.Fields(text), " ") }

func truncate(text string, n int) string {
	runes := []rune(text)
	return string(runes[:min(len(runes), n)])
}

func (b *bora) listProjects() string {
	var out strings.Builder
	for _, acc := range b.members {
		tasks := b.tasks[acc.Project.ID]
		fmt.Fprintf(&out, "%s (%s): you are %s, %d tasks", acc.Project.Slug, acc.Project.Name, acc.Role, len(tasks))
		counts := map[core.TaskStatus]int{}
		var last core.FleetTask
		for _, t := range tasks {
			counts[t.Status]++
			if t.LastDeploy != nil && (last.LastDeploy == nil || t.LastDeploy.CreatedAt.After(last.LastDeploy.CreatedAt)) {
				last = t
			}
		}
		for _, status := range slices.Sorted(maps.Keys(counts)) {
			fmt.Fprintf(&out, ", %d %s", counts[status], status)
		}
		if last.LastDeploy != nil {
			fmt.Fprintf(&out, ", last deploy %s of %s at %s", last.LastDeploy.Status, last.Name, last.LastDeploy.CreatedAt.Format(time.RFC3339))
		}
		if b.readable(acc) {
			out.WriteString(", its code can be read")
		}
		out.WriteByte('\n')
	}
	return out.String()
}

func (b *bora) listTasks(_ context.Context, in args) (string, error) {
	scope, err := b.scope(in.Project)
	if err != nil {
		return "", err
	}
	b.step("Looking at the tasks of " + named(scope) + "…")
	var out strings.Builder
	for _, acc := range scope {
		fmt.Fprintf(&out, "%s (%s), you are %s:\n", acc.Project.Slug, acc.Project.Name, acc.Role)
		for _, t := range b.tasks[acc.Project.ID] {
			out.WriteString("- " + taskLine(t) + "\n")
		}
	}
	return out.String(), nil
}

func (b *bora) taskDetails(_ context.Context, in args) (string, error) {
	t, ok := b.find(in.Project, in.Task)
	if !ok {
		return "", noTask(in.Project, in.Task)
	}
	b.step("Reading " + in.Project + "/" + in.Task + "…")
	var out strings.Builder
	out.WriteString(taskLine(t))
	if t.Note != "" {
		fmt.Fprintf(&out, "\nnote: %s", t.Note)
	}
	fmt.Fprintf(&out, "\nenv names: %s", strings.Join(slices.Sorted(maps.Keys(t.Env)), ", "))
	for _, path := range slices.Sorted(maps.Keys(t.Volumes)) {
		fmt.Fprintf(&out, "\nfolder %s mounted at %s", t.Volumes[path], path)
	}
	if t.PendingRecreate {
		out.WriteString("\nwaits to be recreated to apply its last change")
	}
	return out.String(), nil
}

func (b *bora) taskLogs(ctx context.Context, in args) (string, error) {
	if _, ok := b.find(in.Project, in.Task); !ok {
		return "", noTask(in.Project, in.Task)
	}
	opts := core.LogOptions{Tail: cmp.Or(in.Tail, 200)}
	if opts.Tail < 1 || opts.Tail > 200 {
		return "", errors.New("error: tail must be 1 to 200")
	}
	if in.Since != "" {
		since, err := time.Parse(time.RFC3339, in.Since)
		if err != nil {
			return "", errors.New("error: since must be an RFC 3339 time")
		}
		opts.Since = since
	}
	b.step("Reading " + in.Task + "'s logs…")
	logs, err := b.s.Logs(ctx, in.Project, in.Task, opts)
	switch {
	case errors.Is(err, core.ErrConflict):
		return "no logs: the task has no container yet", nil
	case err != nil:
		return "", errors.New("error: could not read the logs")
	case logs == "":
		return "no log lines", nil
	}
	return cmp.Or(cut(logs), "error: the lines are too long to read"), nil
}

// cut keeps the newest 16 KiB of whole lines, so redaction still sees the key of every secret.
func cut(logs string) string {
	if len(logs) <= logTail {
		return logs
	}
	_, newest, _ := strings.Cut(logs[len(logs)-logTail:], "\n")
	return newest
}

func (b *bora) activity(ctx context.Context, in args) (string, error) {
	limit := cmp.Or(in.Limit, activityLimit)
	if limit < 1 || limit > 50 {
		return "", errors.New("error: limit must be 1 to 50")
	}
	scope, err := b.scope(in.Project)
	if err != nil {
		return "", err
	}
	b.step("Reading the activity of " + named(scope) + "…")
	// ponytail: one query per project keeps grant-only projects out of the limit; one SQL query if it shows.
	var events []core.Notification
	for _, acc := range scope {
		list, err := b.s.Notifications(ctx, b.actor, &acc.Project.ID, nil, limit)
		if err != nil {
			return "", errors.New("error: could not read the activity")
		}
		events = append(events, list...)
	}
	slices.SortFunc(events, func(x, y core.Notification) int { return y.CreatedAt.Compare(x.CreatedAt) })
	var out strings.Builder
	for _, n := range events[:min(limit, len(events))] {
		fmt.Fprintf(&out, "%s %s/%s %s: %s", n.CreatedAt.Format(time.RFC3339), n.Project, n.TaskName, n.Type, n.Title)
		if n.Body != "" {
			fmt.Fprintf(&out, ": %s", oneLine(n.Body))
		}
		out.WriteByte('\n')
	}
	return cmp.Or(out.String(), "no activity"), nil
}

func (b *bora) metrics(ctx context.Context, in args) (string, error) {
	acc, ok := b.project(in.Project)
	if !ok {
		return "", notMember(in.Project)
	}
	want := 1
	if in.Task == "" {
		want = b.running(acc)
	} else if _, ok := b.task(acc, in.Task); !ok {
		return "", noTask(in.Project, in.Task)
	}
	b.step("Measuring " + cmp.Or(in.Task, in.Project) + "…")
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, metricsWindow)
	defer cancel()
	samples, err := b.s.Metrics(ctx, acc, in.Task)
	switch {
	case errors.Is(err, core.ErrConflict):
		return "no metrics: the task has no container yet", nil
	case err != nil:
		return "", errors.New("error: could not read the metrics")
	}
	last, measured := map[string]core.TaskMetric{}, map[string]bool{}
	for m := range samples {
		_, again := last[m.TaskName]
		last[m.TaskName] = m
		// A stream's first sample has no CPU, and replayed ones are up to a minute old.
		if again && m.ObservedAt.After(started) && !measured[m.TaskName] {
			measured[m.TaskName] = true
			if len(measured) == want {
				cancel()
			}
		}
	}
	mib := func(n int64) string { return fmt.Sprintf("%.0f MiB", float64(n)/(1<<20)) }
	var out strings.Builder
	for _, task := range slices.Sorted(maps.Keys(last)) {
		m := last[task]
		fmt.Fprintf(&out, "%s: CPU %.1f%%, memory %s of %s, network %s in and %s out since start\n",
			task, m.CPUPercent, mib(m.MemoryBytes), mib(m.MemoryLimit), mib(m.NetworkRXBytes), mib(m.NetworkTXBytes))
	}
	return cmp.Or(out.String(), "no metrics: nothing is running"), nil
}

// running counts the tasks of a project that TaskService.Metrics measures.
func (b *bora) running(acc core.ProjectAccess) int {
	n := 0
	for _, t := range b.tasks[acc.Project.ID] {
		if t.Status == core.StatusRunning && t.ContainerID != "" {
			n++
		}
	}
	return n
}

// owned is a project the actor owns, as the member and grant routes require.
func (b *bora) owned(slug string) (core.ProjectAccess, error) {
	acc, ok := b.project(slug)
	if !ok {
		return acc, notMember(slug)
	}
	if acc.Role != core.ProjectRoleOwner {
		return acc, errors.New("refused: only owners see who has access to " + slug)
	}
	return acc, nil
}

func (b *bora) listMembers(ctx context.Context, in args) (string, error) {
	acc, err := b.owned(in.Project)
	if err != nil {
		return "", err
	}
	b.step("Reading who is in " + in.Project + "…")
	members, err := b.s.projects.ListMembers(ctx, acc.Project.ID)
	if err != nil {
		return "", errors.New("error: could not read the members")
	}
	var out strings.Builder
	for _, m := range members {
		fmt.Fprintf(&out, "%s: %s\n", m.Username, m.Role)
	}
	return out.String(), nil
}

func (b *bora) listGrants(ctx context.Context, in args) (string, error) {
	acc, err := b.owned(in.Project)
	if err != nil {
		return "", err
	}
	task, ok := b.task(acc, in.Task)
	if !ok {
		return "", noTask(in.Project, in.Task)
	}
	b.step("Reading who holds " + in.Task + "…")
	grants, err := b.s.grants.ListForTask(ctx, task.ID)
	if err != nil {
		return "", errors.New("error: could not read the grants")
	}
	var out strings.Builder
	for _, g := range grants {
		fmt.Fprintf(&out, "%s: %s\n", g.Username, g.Role)
	}
	return cmp.Or(out.String(), "no one holds a grant on "+in.Task), nil
}

func (b *bora) projectSettings(ctx context.Context, in args) (string, error) {
	acc, ok := b.project(in.Project)
	if !ok {
		return "", notMember(in.Project)
	}
	b.step("Reading " + in.Project + "'s settings…")
	p := acc.Project
	credential := "none"
	if p.RegistryCredentialID != nil {
		credential = "set, and only admins see which"
	}
	// Administrators alone may list credentials, so only they learn the name.
	if p.RegistryCredentialID != nil && b.actor.IsAdmin() {
		if c, err := b.s.credentials.Get(ctx, *p.RegistryCredentialID); err == nil {
			credential = c.Name
		}
	}
	return fmt.Sprintf("%s (%s): default image %s, default port %d, default env names: %s, registry credential: %s",
		p.Slug, p.Name, cmp.Or(p.DefaultImage, "none"), p.DefaultPort, cmp.Or(strings.Join(slices.Sorted(maps.Keys(p.DefaultEnv)), ", "), "none"), credential), nil
}

func (b *bora) setTitle(_ context.Context, in args) (string, error) {
	if title := truncate(oneLine(core.Redact(in.Title)), 60); b.title == "" && title != "" {
		b.title = title
	}
	return "done", nil
}

func (b *bora) searchCode(ctx context.Context, in args) (string, error) {
	b.tainted = true
	acc, err := b.repos(in.Project)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(in.Query) == "" {
		return "", errors.New("error: query is empty")
	}
	b.step("Searching " + acc.Project.Slug + "'s code…")
	matches, err := b.s.Code.Search(ctx, acc.Project.Repositories, in.Query)
	if err != nil {
		return "", errors.New("error: code search failed")
	}
	var out strings.Builder
	for _, m := range matches {
		fmt.Fprintf(&out, "%s %s:%d\n%s\n\n", m.Repo, m.Path, m.Line, m.Snippet)
	}
	return cmp.Or(out.String(), "no matches"), nil
}

func (b *bora) readFile(ctx context.Context, in args) (string, error) {
	b.tainted = true
	acc, err := b.repos(in.Project)
	if err != nil {
		return "", err
	}
	b.step(fmt.Sprintf("Reading %.60s…", path.Base(in.Path)))
	file, err := b.s.Code.Read(ctx, acc.Project.Repositories, in.Repo, in.Path)
	if err != nil {
		return "", errors.New("error: file not found or not readable")
	}
	if source := (core.CodeSource{Repo: file.Repo, Path: file.Path, URL: file.URL}); !slices.Contains(b.sources, source) {
		b.sources = append(b.sources, source)
	}
	return file.Content, nil
}

// repos is the project whose code a tool reads: the one named, or the only one Bora sees.
func (b *bora) repos(slug string) (core.ProjectAccess, error) {
	if slug == "" && len(b.members) == 1 {
		slug = b.members[0].Project.Slug
	}
	acc, ok := b.project(slug)
	switch {
	case slug == "":
		return acc, errors.New("error: name the project whose code to read")
	case !ok:
		return acc, notMember(slug)
	case len(acc.Project.Repositories) == 0:
		return acc, errors.New("error: you cannot see " + slug + "'s code: an admin adds its repositories in the project's About")
	}
	return acc, nil
}

type proposal struct {
	Op, Project, Task, Reason, Username, Name, Description, Note, Image string
	Role                                                                core.ProjectRole
	DevStatus                                                           core.DevStatus `json:"dev_status"`
	Port                                                                int
	AutoRestart                                                         *bool `json:"auto_restart"`
	Volumes, Set                                                        map[string]string
	Unset                                                               []string
	RegistryCredential                                                  string `json:"registry_credential"`
}

// propose checks what the route will check again and turns names into ids; nothing runs here.
func (b *bora) propose(ctx context.Context, in proposal) (string, error) {
	op := core.BoraOp(strings.ToLower(in.Op))
	need, ok := core.BoraOps[op]
	if !ok {
		return "", fmt.Errorf("refused: op must be one of %v", core.BoraOp("").Enum())
	}
	if b.tainted && !slices.Contains([]core.BoraOp{"task.start", "task.stop", "task.restart"}, op) {
		return "", errors.New("refused: after reading code or an image, only start, stop and restart can be offered")
	}
	acc, ok := b.project(in.Project)
	if !ok {
		return "", errors.New("refused: you are not a member of a project named " + in.Project)
	}
	if len(b.actions) >= maxActions {
		return "", fmt.Errorf("refused: at most %d actions per answer", maxActions)
	}
	action := core.BoraAction{Op: op, Project: in.Project, Reason: truncate(strings.TrimSpace(core.Redact(in.Reason)), 200)}
	role, where := acc.Role, in.Project
	if family, _, _ := strings.Cut(string(op), "."); family == "task" || family == "grant" {
		var err error
		if role, err = b.taskRole(acc, op, in); err != nil {
			return "", err
		}
		action.Task, where = in.Task, in.Project+"/"+in.Task
	}
	if role.Rank() < need.Rank() {
		return "", fmt.Errorf("refused: you are %s on %s; %s needs %s or higher", role, where, op, need)
	}
	if err := b.fields(ctx, acc, in, &action); err != nil {
		return "", err
	}
	if slices.ContainsFunc(b.actions, func(a core.BoraAction) bool {
		return a.Op == op && a.Project == action.Project && a.Task == action.Task && a.UserID == action.UserID
	}) {
		return "", errors.New("refused: the user already has a button for this")
	}
	b.actions = append(b.actions, action)
	return fmt.Sprintf("proposed: the user now sees a button for %s on %s; it has not run, and runs only if they confirm", op, where), nil
}

// taskRole checks the task a task or grant op names, and gives the actor's role on it.
func (b *bora) taskRole(acc core.ProjectAccess, op core.BoraOp, in proposal) (core.ProjectRole, error) {
	if core.ValidateTaskName(in.Task) != nil {
		return "", errors.New("refused: " + in.Task + " is not a task name")
	}
	task, exists := b.task(acc, in.Task)
	switch {
	case op == "task.create" && exists:
		return "", errors.New("refused: task " + in.Task + " already exists in " + in.Project)
	case op == "task.create" && in.Image == "" && acc.Project.DefaultImage == "":
		return "", errors.New("refused: " + in.Project + " has no default image, so name the image to run")
	case op != "task.create" && !exists:
		return "", errors.New("refused: there is no task " + in.Task + " in " + in.Project)
	case exists:
		return task.Role, nil
	}
	return acc.Role, nil
}

// fields copies what op carries onto action, checked as its route checks it.
func (b *bora) fields(ctx context.Context, acc core.ProjectAccess, in proposal, action *core.BoraAction) error {
	switch action.Op {
	case "task.create":
		action.Image, action.Port, action.Description = strings.TrimSpace(in.Image), in.Port, in.Description
	case "task.update":
		if err := taskUpdate(in, action); err != nil {
			return err
		}
	case "task.env":
		action.Set, action.Unset = in.Set, in.Unset
		return checkEnv(in.Set, in.Unset)
	case "member.add", "member.role", "member.remove", "grant.add", "grant.remove":
		if err := b.person(ctx, acc, in, action); err != nil {
			return err
		}
		return setRole(action, in.Role)
	case "project.update":
		if err := b.projectUpdate(ctx, in, action); err != nil {
			return err
		}
	}
	if action.Port < 0 || action.Port > 65535 || action.DefaultPort < 0 || action.DefaultPort > 65535 {
		return errors.New("refused: port must be 1 to 65535")
	}
	if action.DevStatus != "" && core.ValidateDevStatus(action.DevStatus) != nil {
		return errors.New("refused: dev_status must be in_progress, blocked or ready")
	}
	return nil
}

func taskUpdate(in proposal, action *core.BoraAction) error {
	action.Description, action.Note, action.DevStatus, action.Image, action.Port, action.AutoRestart, action.Volumes =
		in.Description, in.Note, in.DevStatus, strings.TrimSpace(in.Image), in.Port, in.AutoRestart, in.Volumes
	if action.Description == "" && action.Note == "" && action.DevStatus == "" && action.Image == "" && action.Port == 0 && len(action.Volumes) == 0 {
		return errors.New("refused: task.update needs a description, note, dev_status, image, port or volumes")
	}
	if core.ValidateVolumes(in.Volumes) != nil {
		return errors.New("refused: volumes map a clean absolute container path to a project folder name")
	}
	return nil
}

func (b *bora) projectUpdate(ctx context.Context, in proposal, action *core.BoraAction) error {
	action.Name, action.DefaultImage, action.DefaultPort = strings.TrimSpace(in.Name), strings.TrimSpace(in.Image), in.Port
	if len(in.Set)+len(in.Unset) > 0 {
		if err := checkEnv(in.Set, in.Unset); err != nil {
			return err
		}
		action.DefaultEnvSet, action.DefaultEnvUnset = in.Set, in.Unset
	}
	if in.RegistryCredential != "" {
		if err := b.credential(ctx, in.RegistryCredential, action); err != nil {
			return err
		}
	}
	if action.Name == "" && action.DefaultImage == "" && action.DefaultPort == 0 && action.DefaultEnvSet == nil &&
		action.DefaultEnvUnset == nil && action.RegistryCredentialID == nil {
		return errors.New("refused: project.update needs a name, image, port, env or registry_credential")
	}
	return nil
}

// person resolves whom an op names; to a non-admin, someone outside the project looks like no one.
func (b *bora) person(ctx context.Context, acc core.ProjectAccess, in proposal, action *core.BoraAction) error {
	nobody := errors.New("refused: no one named " + in.Username + " is in " + in.Project + "; an admin can name anyone")
	if b.actor.IsAdmin() {
		nobody = errors.New("refused: there is no active user named " + in.Username)
	}
	if b.s.Users == nil {
		return nobody
	}
	user, err := b.s.Users.GetByUsername(ctx, in.Username)
	switch {
	case errors.Is(err, core.ErrNotFound) || err == nil && user.Disabled():
		return nobody
	case err != nil:
		return errors.New("error: could not look up " + in.Username)
	}
	members, err := b.s.projects.ListMembers(ctx, acc.Project.ID)
	if err != nil {
		return errors.New("error: could not read the members")
	}
	member := slices.ContainsFunc(members, func(m core.ProjectMember) bool { return m.UserID == user.ID })
	granted, err := b.s.grants.AnyInProject(ctx, acc.Project.ID, user.ID)
	if err != nil {
		return errors.New("error: could not read the grants")
	}
	if !b.actor.IsAdmin() && !member && !granted {
		return nobody
	}
	action.UserID, action.Username = user.ID, user.Username
	switch action.Op {
	case "member.add":
		if member {
			return errors.New("refused: " + user.Username + " is already a member of " + in.Project + "; use member.role")
		}
	case "member.role", "member.remove":
		if !member {
			return errors.New("refused: " + user.Username + " is not a member of " + in.Project)
		}
	case "grant.remove":
		return b.holdsGrant(ctx, acc, in.Task, user)
	}
	return nil
}

func (b *bora) holdsGrant(ctx context.Context, acc core.ProjectAccess, name string, user core.User) error {
	task, _ := b.task(acc, name)
	grants, err := b.s.grants.ListForTask(ctx, task.ID)
	if err != nil {
		return errors.New("error: could not read the grants")
	}
	if !slices.ContainsFunc(grants, func(g core.TaskGrant) bool { return g.UserID == user.ID }) {
		return errors.New("refused: " + user.Username + " holds no grant on " + name)
	}
	return nil
}

// setRole gives a member or grant op its role; a grant's stops at member.
func setRole(action *core.BoraAction, role core.ProjectRole) error {
	switch action.Op {
	case "member.add", "member.role":
		if role.Rank() == 0 {
			return errors.New("refused: role must be viewer, operator, member or owner")
		}
	case "grant.add":
		if role.Rank() == 0 || role == core.ProjectRoleOwner {
			return errors.New("refused: a grant's role must be viewer, operator or member")
		}
	default:
		return nil
	}
	action.Role = role
	return nil
}

func (b *bora) credential(ctx context.Context, name string, action *core.BoraAction) error {
	if !b.actor.IsAdmin() {
		return errors.New("refused: only an admin can change the registry credential")
	}
	credentials, err := b.s.credentials.List(ctx)
	if err != nil {
		return errors.New("error: could not read the registry credentials")
	}
	i := slices.IndexFunc(credentials, func(c core.RegistryCredential) bool { return strings.EqualFold(c.Name, name) })
	if i < 0 {
		return errors.New("refused: there is no registry credential named " + name)
	}
	action.RegistryCredentialID = &credentials[i].ID
	return nil
}

func checkEnv(set map[string]string, unset []string) error {
	if len(set)+len(unset) == 0 {
		return errors.New("refused: name variables to set or unset")
	}
	for _, key := range append(slices.Collect(maps.Keys(set)), unset...) {
		if !envKey.MatchString(key) {
			return errors.New("refused: " + key + " is not a variable name")
		}
	}
	if core.ValidateEnv(set) != nil {
		return errors.New("refused: Boreas sets BOREAS_PROJECT, BOREAS_TASK, BOREAS_PORT and BASE_HREF itself")
	}
	if slices.ContainsFunc(unset, func(key string) bool { _, ok := set[key]; return ok }) {
		return errors.New("refused: a variable cannot be both set and unset")
	}
	return nil
}

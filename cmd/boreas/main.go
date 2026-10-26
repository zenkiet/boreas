package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/config"
	"github.com/zenkiet/boreas/internal/core"
	"github.com/zenkiet/boreas/internal/infra/apprise"
	dockerinfra "github.com/zenkiet/boreas/internal/infra/docker"
	"github.com/zenkiet/boreas/internal/infra/fcm"
	"github.com/zenkiet/boreas/internal/infra/llm"
	pginfra "github.com/zenkiet/boreas/internal/infra/postgres"
	proxyinfra "github.com/zenkiet/boreas/internal/infra/proxy"
	"github.com/zenkiet/boreas/internal/infra/s3"
	"github.com/zenkiet/boreas/internal/infra/sourcebot"
	"github.com/zenkiet/boreas/internal/pkg/database"
	"github.com/zenkiet/boreas/internal/pkg/logging"
	"github.com/zenkiet/boreas/internal/service"
	httptransport "github.com/zenkiet/boreas/internal/transport/http"
	"github.com/zenkiet/boreas/internal/web"
)

// These operational constants are intentionally not deployment configuration.
const (
	dockerNetwork    = "boreas-net"
	restartPolicy    = "unless-stopped" // a reboot brings running tasks back; one stopped in Boreas stays stopped
	readinessTimeout = 20 * time.Second
	readinessPoll    = 200 * time.Millisecond
	dialTimeout      = 5 * time.Second
	responseTimeout  = 30 * time.Second
	notifyTimeout    = 5 * time.Second
	startupTimeout   = 30 * time.Second
	shutdownTimeout  = 10 * time.Second
)

var version = "dev"

func main() {
	logger := logging.New(os.Stdout)
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("boreas stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	startupCtx, startupCancel := context.WithTimeout(context.Background(), startupTimeout)
	defer startupCancel()

	pool, err := database.NewPostgres(startupCtx, cfg.Postgres.DSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.RunMigrations(startupCtx, pool, logger); err != nil {
		return err
	}

	users := pginfra.NewUserStore(pool)
	tokens := pginfra.NewTokenStore(pool)
	projectStore := pginfra.NewProjectStore(pool)
	events := httptransport.NewHub()
	taskStore := publishingTasks{pginfra.NewTaskStore(pool), events}
	credentials := pginfra.NewCredentialStore(pool)
	notifications := pginfra.NewNotificationStore(pool)
	grants := pginfra.NewGrantStore(pool)
	push := pginfra.NewPushStore(pool)

	auth := service.NewAuthService(users, tokens)
	if err := seedAdmin(startupCtx, auth, users, cfg.Admin, logger); err != nil {
		return err
	}

	runtime, err := dockerinfra.New(dockerNetwork, restartPolicy)
	if err != nil {
		return err
	}
	defer runtime.Close()
	if err := runtime.EnsureNetwork(startupCtx); err != nil {
		return err
	}
	if err := runtime.EnsureRestartPolicy(startupCtx); err != nil {
		logger.Warn("restart policy update completed with warnings", "error", err)
	}

	routes := proxyinfra.New(dialTimeout, responseTimeout)
	routes.Fallback = web.Handler()
	defer routes.CloseIdleConnections()

	sender, err := fcm.New(cfg.FCM.Project, cfg.FCM.Keyfile, push.Tokens, push.DeleteTokens, logger)
	if err != nil {
		return err
	}
	teamSender := apprise.New(cfg.TeamNotifyURL(), notifyTimeout)
	notify := notifier(notifications, events, sender, teamSender, logger)

	projects := service.NewProjectService(projectStore, credentials, notifications, grants, taskStore, runtime.MakeFolder)
	projects.Notify = notify
	projects.Users = users
	projects.Conversations = pginfra.NewBoraStore(pool)
	if cfg.Sourcebot.Enabled() {
		projects.Code = sourcebot.New(cfg.Sourcebot.URL, cfg.Sourcebot.APIKey)
	}
	if cfg.AI.Enabled() {
		projects.Assist = assistant(llm.New(cfg.AI.BaseURL, cfg.AI.APIKey, cfg.AI.Model), logger)
	}
	if cfg.S3.Enabled() {
		projects.Images = s3.New(cfg.S3.Endpoint, cfg.S3.Bucket, cfg.S3.Region, cfg.S3.AccessKeyID, cfg.S3.SecretAccessKey)
	}
	tasks := service.NewTaskService(
		runtime, taskStore, projectStore, credentials, routes,
		dockerinfra.TCPReadyChecker{DialTimeout: time.Second}.Ready,
		service.Config{
			ReadinessTimeout: readinessTimeout,
			PollInterval:     readinessPoll,
			Notify:           notify,
		},
	)
	projects.Logs, projects.Metrics = taskLogs(tasks), tasks.Metrics
	if err := tasks.Reconcile(startupCtx); err != nil {
		logger.Warn("startup reconciliation completed with warnings", "error", err)
	}

	handler := httptransport.ApplicationHandler(
		httptransport.APIHandler(tasks, auth, projects, push, events, version, logger),
		routes,
		logger,
	)

	server := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	server.RegisterOnShutdown(events.Close)
	server.RegisterOnShutdown(projects.StopAllBora)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.ListenAddr())
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		err = <-serverErrors
	case err = <-serverErrors:
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTP server: %w", err)
	}
	return nil
}

// assistant logs each turn's cost, never its content.
func assistant(a *llm.Assistant, logger *slog.Logger) func(context.Context, uuid.UUID, core.Ask) (string, []string, error) {
	return func(ctx context.Context, user uuid.UUID, ask core.Ask) (string, []string, error) {
		started := time.Now()
		reply, err := a.Answer(ctx, ask)
		logger.Info("answer", "user", user, "tools", len(ask.Tools), "images", len(ask.Images), "clarified", reply.Choices != nil,
			"prompt_tokens", reply.PromptTokens, "completion_tokens", reply.CompletionTokens, "duration", time.Since(started), "error", err)
		return reply.Content, reply.Choices, err
	}
}

func taskLogs(tasks *service.TaskService) func(context.Context, string, string, core.LogOptions) (string, error) {
	return func(ctx context.Context, project, task string, opts core.LogOptions) (string, error) {
		logs, err := tasks.Logs(ctx, project, task, opts)
		if err != nil {
			return "", err
		}
		defer logs.Close()
		var out newest
		if _, err := stdcopy.StdCopy(&out, &out, logs); err != nil && len(out) == 0 {
			return "", fmt.Errorf("read logs: %w", err)
		}
		return string(out), nil
	}
}

// newest keeps the last 64 KiB written; Bora reads only a log's newest 16 KiB.
type newest []byte

func (b *newest) Write(p []byte) (int, error) {
	if *b = append(*b, p...); len(*b) > 64<<10 {
		*b = (*b)[len(*b)-64<<10:]
	}
	return len(p), nil
}

func notifier(store *pginfra.NotificationStore, events *httptransport.Hub, sender *fcm.Sender, team *apprise.Sender, logger *slog.Logger) func(context.Context, core.Notification) {
	return func(ctx context.Context, n core.Notification) {
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), notifyTimeout)
			defer cancel()
			if _, err := store.Create(ctx, n); err != nil {
				logger.Error("record notification", "error", err)
			}
			events.Publish() // the fleet's last deploy reads this row
			if err := sender.Send(ctx, n); err != nil {
				logger.Error("push notification", "error", err)
			}
			if err := team.Send(ctx, n); err != nil {
				logger.Error("team notification", "error", err)
			}
		}()
	}
}

// seedAdmin fails startup rather than leave an empty installation unreachable.
func seedAdmin(ctx context.Context, auth *service.AuthService, users *pginfra.UserStore, admin config.AdminConfig, logger *slog.Logger) error {
	if admin.Provided() {
		created, err := auth.EnsureAdmin(ctx, admin.Username, admin.Email, admin.Password)
		if err != nil {
			return err
		}
		if created {
			logger.Info("created initial admin user", "username", admin.Username)
		}
		return nil
	}
	count, err := users.Count(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("no users exist: set BOREAS_ADMIN_USERNAME, BOREAS_ADMIN_EMAIL, and BOREAS_ADMIN_PASSWORD to create the first administrator")
	}
	return nil
}

// publishingTasks signals open event streams after every task write, a deploy's interim states included;
// a write method added to core.TaskStore must be wrapped here too.
type publishingTasks struct {
	core.TaskStore
	events *httptransport.Hub
}

func (s publishingTasks) Create(ctx context.Context, task core.Task) (core.Task, error) {
	defer s.events.Publish()
	return s.TaskStore.Create(ctx, task)
}

func (s publishingTasks) Update(ctx context.Context, task core.Task) (core.Task, error) {
	defer s.events.Publish()
	return s.TaskStore.Update(ctx, task)
}

func (s publishingTasks) SetBuild(ctx context.Context, id uuid.UUID, build core.Build) error {
	defer s.events.Publish()
	return s.TaskStore.SetBuild(ctx, id, build)
}

func (s publishingTasks) Delete(ctx context.Context, id uuid.UUID) error {
	defer s.events.Publish()
	return s.TaskStore.Delete(ctx, id)
}

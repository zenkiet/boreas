package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zenkiet/boreas/internal/config"
	dockerinfra "github.com/zenkiet/boreas/internal/infra/docker"
	pginfra "github.com/zenkiet/boreas/internal/infra/postgres"
	proxyinfra "github.com/zenkiet/boreas/internal/infra/proxy"
	"github.com/zenkiet/boreas/internal/pkg/database"
	"github.com/zenkiet/boreas/internal/service"
	httptransport "github.com/zenkiet/boreas/internal/transport/http"
)

// These operational constants are intentionally not deployment configuration.
const (
	dockerNetwork    = "boreas-net"
	restartPolicy    = "on-failure"
	readinessTimeout = 20 * time.Second
	readinessPoll    = 200 * time.Millisecond
	dialTimeout      = 5 * time.Second
	responseTimeout  = 30 * time.Second
	startupTimeout   = 30 * time.Second
	shutdownTimeout  = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		log.Printf("boreas stopped with error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	logger := log.New(os.Stdout, "boreas ", log.LstdFlags|log.Lmicroseconds)
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
	taskStore := pginfra.NewTaskStore(pool)
	credentials := pginfra.NewCredentialStore(pool)

	auth, err := service.NewAuthService(users, tokens)
	if err != nil {
		return err
	}
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

	routes := proxyinfra.New(dialTimeout, responseTimeout)
	defer routes.CloseIdleConnections()

	projects, err := service.NewProjectService(projectStore, taskStore, credentials)
	if err != nil {
		return err
	}
	tasks, err := service.NewTaskService(
		runtime, taskStore, projectStore, credentials, routes,
		dockerinfra.TCPReadyChecker{DialTimeout: time.Second}.Ready,
		service.Config{
			DefaultPort:      80,
			ReadinessTimeout: readinessTimeout,
			PollInterval:     readinessPoll,
		},
	)
	if err != nil {
		return err
	}
	if err := tasks.Reconcile(startupCtx); err != nil {
		logger.Printf("startup reconciliation completed with warnings: %v", err)
	}

	httpOptions := httptransport.Options{
		Logger: logger,
		CORS:   httptransport.CORSConfig{AllowedOrigins: []string{"*"}},
		Docs:   true,
	}
	handler := httptransport.ApplicationHandler(
		httptransport.APIHandler(tasks, auth, projects, httpOptions),
		routes,
		logger,
	)

	server := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 1)
	go func() {
		logger.Printf("listening on http://%s", cfg.ListenAddr())
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
		stop()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTP server: %w", err)
	}
	return nil
}

// seedAdmin fails startup rather than leave an empty installation unreachable.
func seedAdmin(ctx context.Context, auth *service.AuthService, users *pginfra.UserStore, admin config.AdminConfig, logger *log.Logger) error {
	if admin.Provided() {
		created, err := auth.EnsureAdmin(ctx, admin.Username, admin.Email, admin.Password)
		if err != nil {
			return err
		}
		if created {
			logger.Printf("created initial admin user %q", admin.Username)
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

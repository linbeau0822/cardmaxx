// Command server runs the CardMaxx HTTP API.
//
// Start-up order is deliberate: configuration is validated, then the database
// is reached, then the HTTP server is started. Anything that would make the
// API unable to serve requests is a start-up failure, not a runtime surprise.
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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/linbeau0822/cardmaxx/backend/internal/ai"
	"github.com/linbeau0822/cardmaxx/backend/internal/api"
	"github.com/linbeau0822/cardmaxx/backend/internal/config"
	"github.com/linbeau0822/cardmaxx/backend/internal/database"
	"github.com/linbeau0822/cardmaxx/backend/internal/database/db"
	"github.com/linbeau0822/cardmaxx/backend/internal/plaid"
	"github.com/linbeau0822/cardmaxx/backend/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Signals are translated into context cancellation, so shutdown follows the
	// same path whether it is triggered by an operator or by a fatal error.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger); err != nil {
		logger.Error("server exited with error", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("server stopped")
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()
	logger.Info("connected to database")

	queries := db.New(pool)

	// Authentication does not exist yet (Phase 8), so the application runs as
	// a single implicit user. Failing to find or create it is a start-up
	// failure, same as an unreachable database.
	currentUserID, err := bootstrapUser(ctx, queries)
	if err != nil {
		return fmt.Errorf("bootstrap user: %w", err)
	}
	logger.Info("running as implicit user", slog.String("user_id", currentUserID.String()))

	plaidClient, err := plaid.NewClient(cfg.Plaid)
	if err != nil {
		return fmt.Errorf("create plaid client: %w", err)
	}
	logger.Info("plaid client configured",
		slog.String("env", string(cfg.Plaid.Env)),
		slog.String("base_url", plaidClient.BaseURL()),
	)
	plaidSvc := service.NewPlaidService(queries, plaidClient)

	// OpenAI is optional: Phase 7's AI endpoints return 503 instead of
	// failing start-up when no key is configured.
	var aiSvc *service.AIService
	if cfg.OpenAI.APIKey == "" {
		logger.Warn("OPENAI_API_KEY not set; AI assistant endpoints disabled")
	} else {
		aiSvc = service.NewAIService(plaidSvc, ai.NewClient(cfg.OpenAI.APIKey, cfg.OpenAI.Model))
		logger.Info("openai client configured", slog.String("model", cfg.OpenAI.Model))
	}

	srv := &http.Server{
		Addr:              cfg.Server.Addr(),
		Handler:           api.NewRouter(cfg.Server, logger, plaidSvc, aiSvc, currentUserID),
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	// ListenAndServe blocks, so it runs in its own goroutine and reports its
	// outcome back over a channel.
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections",
			slog.Duration("timeout", cfg.Server.ShutdownTimeout))
	}

	// Shutdown gets its own context: the signal context is already cancelled,
	// and in-flight requests need a bounded window to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	return <-serveErr
}

// bootstrapUser returns the single implicit user CardMaxx runs as until
// Phase 8 adds authentication, creating it on first run.
func bootstrapUser(ctx context.Context, queries *db.Queries) (uuid.UUID, error) {
	user, err := queries.GetFirstUser(ctx)
	if err == nil {
		return user.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, fmt.Errorf("look up user: %w", err)
	}
	user, err = queries.CreateUser(ctx)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("create user: %w", err)
	}
	return user.ID, nil
}

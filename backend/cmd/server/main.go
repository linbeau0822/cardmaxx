// Command server runs the CardWise HTTP API.
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

	"github.com/linbeau0822/cardmaxx/backend/internal/api"
	"github.com/linbeau0822/cardmaxx/backend/internal/config"
	"github.com/linbeau0822/cardmaxx/backend/internal/database"
	"github.com/linbeau0822/cardmaxx/backend/internal/plaid"
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

	// Nothing queries yet, but connecting here fails a misconfigured database
	// at start-up instead of on the first request that needs one.
	// TODO(phase-2): pass db.New(pool) to the handlers that need it.
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()
	logger.Info("connected to database")

	// TODO(phase-2): hand this to the Plaid handlers. Constructing it here
	// validates PLAID_ENV before the server accepts traffic.
	plaidClient, err := plaid.NewClient(cfg.Plaid)
	if err != nil {
		return fmt.Errorf("create plaid client: %w", err)
	}
	logger.Info("plaid client configured",
		slog.String("env", string(cfg.Plaid.Env)),
		slog.String("base_url", plaidClient.BaseURL()),
	)

	srv := &http.Server{
		Addr:              cfg.Server.Addr(),
		Handler:           api.NewRouter(cfg.Server, logger),
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

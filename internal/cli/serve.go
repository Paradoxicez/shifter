package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/logging"
	"github.com/spf13/cobra"
)

// serveCmd is the long-running production entry point: load config, build a
// pgxpool, auto-apply pending migrations (D-13), then start the HTTP server.
//
// This file is the skeleton — its handler tree is filled by Plans 09 (login),
// 13 (mqtt subscriber), 14 (install middleware), 15 (install handlers), 17
// (test-connection), 18 (router/health), and 19 (SPA embed). Today the
// listener exists end-to-end with a placeholder /health response so the binary
// is runnable for smoke tests and the Docker image's HEALTHCHECK has something
// to probe before Plan 18 ships.
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run migrations then start the HTTP server (D-13)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		log := logging.New(cfg.LogLevel)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		// 1. DB pool
		pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
		if err != nil {
			return fmt.Errorf("db: %w", err)
		}
		defer pool.Close()

		// 2. D-13: auto-migrate before listening. If the schema is dirty,
		// surface the error and refuse to start — operator runs `shifter
		// migrate force <prev>` to recover.
		if err := db.RunMigrations(ctx, pool, log); err != nil {
			return err
		}
		log.Info("startup", "http_port", cfg.HTTPPort, "env", cfg.Env)

		// TODO(plan-09 + plan-13 + plan-14 + plan-15 + plan-17 + plan-18 + plan-19):
		// build the production http.Handler with:
		//   - session manager (Plan 08)
		//   - login + account routes (Plan 09)
		//   - install wizard handlers (Plans 14, 15)
		//   - test-connection probes (Plan 17)
		//   - chirpstack v3 probe + refusal (Plan 12)
		//   - mqtt subscriber start (Plan 13)
		//   - canonical /health + /health/detailed (Plan 18)
		//   - SPA embed via go:embed (Plan 19)
		// For now we serve a 503 placeholder on / so the listener exists end-to-end
		// and Docker HEALTHCHECK has a /health to probe.
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "shifter starting up — full wiring lands in Plans 09-19", http.StatusServiceUnavailable)
		})
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			// Plan 18 replaces this with the canonical /health response.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","version":"placeholder"}`))
		})

		srv := &http.Server{
			Addr:              ":" + cfg.HTTPPort,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       2 * time.Minute,
		}
		go func() {
			log.Info("listening", "addr", srv.Addr)
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("listen failed", "err", err)
				cancel()
			}
		}()

		<-ctx.Done()
		log.Info("shutting down")
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutCancel()
		return srv.Shutdown(shutCtx)
	},
}

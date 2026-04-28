// Package cli — `shifter serve` long-running entry point.
//
// Plan 18 ships the full wiring: config + DB + auto-migrate (D-13) + boot-time
// ChirpStack v3 refusal (INST-05) + MQTT subscriber start (CHIRP-02) + chi
// router (Plan 18 NewRouter) + graceful shutdown on SIGTERM. Plan 19 fills in
// the SPA embed; Plan 23 fills in the login screen surface; Phase 2+ adds
// device handlers.
//
// INST-05 startup gate is extracted into probeChirpStackOrRefuse so a unit
// test (serve_test.go) can drive the v3-rejection path against the bufconn
// mock from Plan 12 without booting a real ChirpStack.
package cli

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

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/chirpstack"
	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/db"
	httpapi "github.com/shifter-io/shifter/internal/http"
	"github.com/shifter-io/shifter/internal/install"
	"github.com/shifter-io/shifter/internal/logging"
)

// secretsDir is the on-disk path under which file-by-REF secrets live.
// Production deploys (Plans 20/21) mount /run/secrets via Compose secrets;
// dev paths can override SHIFTER_SECRETS_DIR (future Plan 04 extension).
const secretsDir = "/run/secrets"

// serveCmd is the long-running production entry point.
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

		// 1. DB pool.
		pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
		if err != nil {
			return fmt.Errorf("db: %w", err)
		}
		defer pool.Close()

		// 2. D-13: auto-migrate before listening.
		if err := db.RunMigrations(ctx, pool, log); err != nil {
			return err
		}
		log.Info("startup", "http_port", cfg.HTTPPort, "env", cfg.Env)

		// 3. INST-05 boot-time gate. If ChirpStack is configured AND reachable,
		// reject v3. Unreachable ChirpStack at boot is degraded mode (the
		// install wizard / Settings → Edit can repair); a v3 server is a hard
		// stop because nothing in Phase 2+ will work against v3.
		if err := probeChirpStackOrRefuse(ctx, log, productionCSDial, cfg.ChirpStack); err != nil {
			return err
		}

		// 4. MQTT subscriber (CHIRP-02). Phase 1 default handler logs uplinks;
		// Phase 2 swaps in normalize+persist via the same Plan 13 hook point.
		var mqttSub *chirpstack.MQTTSubscriber
		if cfg.MQTT.URL != "" {
			sub, mqttErr := chirpstack.NewMQTTSubscriber(
				cfg.MQTT.URL, cfg.MQTT.User, cfg.MQTT.Password,
				"shifter-"+os.Getenv("HOSTNAME"), log, nil,
			)
			if mqttErr != nil {
				log.Warn("mqtt not reachable on boot — degraded mode", "err", mqttErr)
			} else {
				mqttSub = sub
			}
		}

		// 5. Auth wiring.
		sm := auth.NewSessionManager(pool, cfg.IsDev(), cfg.Session.IdleTimeout, cfg.Session.Lifetime)
		limiter := auth.NewLoginLimiter()
		defer limiter.Stop()
		userStore := auth.NewStore(pool)

		// 6. Install wiring. Both `installDeps.Dial` and the boot-time probe
		// dial use the SAME *csConnWrapper shape — install.CSConn (alias) and
		// the boot-probe csBootConn share the {Conn(), Close()} method set, so
		// one wrapper satisfies both interfaces. Go does not implicitly convert
		// between distinct interface types even when shapes match, so the dial
		// closure builds the wrapper and returns it under install.CSConn.
		installStore := install.NewStore(pool)
		installDeps := install.Deps{
			Pool:       pool,
			Store:      installStore,
			SecretsDir: secretsDir,
			Log:        log,
			Dial: func(c context.Context, csCfg config.CSConfig) (install.CSConn, error) {
				conn, err := chirpstack.Dial(c, csCfg)
				if err != nil {
					return nil, err
				}
				return &csConnWrapper{c: conn}, nil
			},
		}

		// 7. Test-Connection deps reuse Plan 17's ProductionDial (its private
		// chirpStackConn interface has the same shape but cannot be aliased
		// without exporting; passing the function directly is the cleanest path).
		tcDeps := httpapi.TestConnDeps{
			Pool:     pool,
			Log:      log,
			Dial:     httpapi.ProductionDial,
			PingMQTT: chirpstack.PingMQTT,
		}

		// 8. Router.
		router := httpapi.NewRouter(httpapi.Deps{
			Pool:         pool,
			SessionMgr:   sm,
			Log:          log,
			LoginLimiter: limiter,
			UserStore:    userStore,
			InstallStore: installStore,
			InstallDeps:  installDeps,
			TestConnDeps: tcDeps,
			SecretsDir:   secretsDir,
			SPA:          httpapi.SPAHandler(),
		})

		// 9. HTTP server.
		srv := &http.Server{
			Addr:              ":" + cfg.HTTPPort,
			Handler:           router,
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
		if mqttSub != nil {
			mqttSub.Shutdown(5 * time.Second)
		}
		return srv.Shutdown(shutCtx)
	},
}

// csBootConn is the minimum surface probeChirpStackOrRefuse needs from a
// *grpc.ClientConn-bound dial result. Same shape as install.CSConn so a
// single *csConnWrapper satisfies both — see csConnWrapper below.
type csBootConn interface {
	Conn() *grpc.ClientConn
	Close() error
}

// csConnDialFunc is the testable boot dial signature. Production passes
// productionCSDial; tests pass a bufconn-backed dial via dialMockBoot.
type csConnDialFunc func(ctx context.Context, cfg config.CSConfig) (csBootConn, error)

// productionCSDial is the real boot dial. Wraps chirpstack.Dial in the
// shared *csConnWrapper shape.
func productionCSDial(ctx context.Context, cfg config.CSConfig) (csBootConn, error) {
	conn, err := chirpstack.Dial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &csConnWrapper{c: conn}, nil
}

// probeChirpStackOrRefuse runs the boot-time v3 rejection per INST-05.
//
// Behavior:
//   - cfg.GRPCURL empty                          → no-op (pre-install state).
//   - dial fails                                 → log warn, return nil (degraded).
//   - probe returns ErrChirpStackV3OrUnknown     → return refusal error referencing
//     INST-05 + "ChirpStack v3" so the operator log spells the cause.
//   - probe returns any other error              → log warn, return nil.
//   - probe succeeds                             → return nil.
//
// serve only opens its listener AFTER this returns nil — a v3 environment
// never accepts connections. This is the second leg of INST-05 (the first
// leg is the install wizard step 2 refusal; Plan 15).
func probeChirpStackOrRefuse(ctx context.Context, log *slog.Logger, dial csConnDialFunc, cfg config.CSConfig) error {
	if cfg.GRPCURL == "" {
		return nil
	}
	conn, err := dial(ctx, cfg)
	if err != nil {
		log.Warn("chirpstack not reachable on boot — degraded mode", "err", err)
		return nil
	}
	defer conn.Close()
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	_, probeErr := chirpstack.ProbeVersion(probeCtx, conn.Conn())
	if errors.Is(probeErr, chirpstack.ErrChirpStackV3OrUnknown) {
		return fmt.Errorf("INST-05: refusing to start — ChirpStack v3 detected at %s", cfg.GRPCURL)
	}
	if probeErr != nil {
		log.Warn("chirpstack probe failed on boot — degraded mode", "err", probeErr)
	}
	return nil
}

// csConnWrapper bridges chirpstack.Dial's *grpc.ClientConn into both the
// install package's CSConn interface (Plan 15) and the boot-probe csBootConn
// (this file). Go does not implicitly convert between distinct interface types
// even when their method sets match, so a single concrete value implements
// both interfaces explicitly.
type csConnWrapper struct{ c *grpc.ClientConn }

func (w *csConnWrapper) Conn() *grpc.ClientConn { return w.c }
func (w *csConnWrapper) Close() error           { return w.c.Close() }

// Package cli — `shifter serve` long-running entry point.
//
// Plan 18 ships the Phase 1 wiring: config + DB + auto-migrate (D-13) +
// boot-time ChirpStack v3 refusal (INST-05) + MQTT subscriber start (CHIRP-02)
// + chi router + graceful shutdown on SIGTERM.
//
// Plan 02-12 adds: ChirpStack gRPC Client construction, EnsureTenantAnd
// Application bootstrap call, profile.RunSeedSync first-boot codec push,
// *resolver.Resolver construction + listener goroutine, ingest.UplinkHandler
// binding to MQTTSubscriber via SetUplinkHandler, and DeviceDeps + SwapDeps
// + ProfileDeps construction so every Phase 2 route surface mounts in
// production.
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
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/shifter-io/shifter/internal/alert"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/chirpstack"
	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/dashboard"
	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/device"
	"github.com/shifter-io/shifter/internal/events"
	"github.com/shifter-io/shifter/internal/floorplan"
	"github.com/shifter-io/shifter/internal/gateway"
	httpapi "github.com/shifter-io/shifter/internal/http"
	importpkg "github.com/shifter-io/shifter/internal/import"
	"github.com/shifter-io/shifter/internal/ingest"
	"github.com/shifter-io/shifter/internal/install"
	"github.com/shifter-io/shifter/internal/logging"
	mapapi "github.com/shifter-io/shifter/internal/map"
	"github.com/shifter-io/shifter/internal/profile"
	"github.com/shifter-io/shifter/internal/report"
	"github.com/shifter-io/shifter/internal/resolver"
	"github.com/shifter-io/shifter/internal/settings"
	"github.com/shifter-io/shifter/internal/swap"
	"github.com/shifter-io/shifter/internal/user"
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

		// 6a. ChirpStack ConnectionStore — singleton row adapter; satisfies
		//     BOTH chirpstack.ConnectionStore (bootstrap) and
		//     profile.ConnectionStore (seed sync + editor).
		csConnStore := NewConnectionStore(pool)

		// 6b. ChirpStack gRPC Client — only constructed if CS is configured AND
		//     reachable. Reused across DeviceDeps + ProfileDeps + RunSeedSync
		//     + EnsureTenantAndApplication. Nil tolerated downstream by the
		//     RegisterRoutes nil-guards (Plan 02-11) so degraded boot still
		//     serves the SPA + auth + install + settings.
		var (
			csClient   *chirpstack.Client
			csTenantID string
			csAppID    string
		)
		if cfg.ChirpStack.GRPCURL != "" {
			csConn, err := chirpstack.Dial(ctx, cfg.ChirpStack)
			if err != nil {
				log.Warn("chirpstack dial failed; CS-dependent routes will not mount", "err", err)
			} else {
				csClient = chirpstack.NewClient(csConn)
				// First-boot tenant + application bootstrap (D-28). Best-effort
				// — a CS-unreachable window does NOT block boot. RunSeedSync
				// also gracefully skips when csTenantID is empty.
				tID, aID, bootErr := chirpstack.EnsureTenantAndApplication(ctx, csClient, csConnStore, log)
				if bootErr != nil {
					log.Warn("chirpstack bootstrap failed; first add-device attempt will retry", "err", bootErr)
				} else {
					csTenantID = tID
					csAppID = aID
				}
			}
		}
		_ = csTenantID // referenced by RunSeedSync indirectly via csConnStore

		// 6c. Resolver — dev_eui → binding cache + LISTEN/NOTIFY listener
		//     (D-25). Loader wraps the existing sqlc query; Run starts the
		//     listener goroutine that drops cache entries on swap NOTIFY.
		//     *resolver.Resolver satisfies swap.Invalidator so swap commits
		//     invalidate defensively.
		res := resolver.New(&sqlcResolverLoader{pool: pool})
		go res.Run(ctx, pool, log)

		// 6c-sse. Events Hub — in-process fan-out of measurement_inserted
		//     NOTIFY payloads to SSE subscribers (Plan 04-03). Hub.Run
		//     mirrors resolver's listener shape: outer reconnect loop + inner
		//     WaitForNotification loop. Started here alongside resolver so
		//     the SSE endpoint is live from the first request after boot.
		eventsHub := events.NewHub(log.With("component", "events"))
		go eventsHub.Run(ctx, pool, log.With("component", "events.listener"))

		// 6d. Ingest pipeline binding — production MQTT messages route through
		//     the full decode → resolve → normalize → persist pipeline. Without
		//     this the binary would log uplinks via the Phase 1 default handler
		//     and silently drop every measurement (VERIFICATION.md Truth 2 root
		//     cause).
		if mqttSub != nil {
			ingestDeps := ingest.Deps{
				Pool:     pool,
				Resolver: res,
				Mappings: &ingest.SQLCMappingStore{Pool: pool},
				Log:      log,
			}
			mqttSub.SetUplinkHandler(ingest.UplinkHandler(ingestDeps))
		}

		// 6e. profile.RunSeedSync — first-boot codec push to ChirpStack per
		//     D-09. Best-effort; never returns error; never blocks boot. Only
		//     runs when CS gRPC client is non-nil; RunSeedSync does its own
		//     re-check on the tenant id (defensive).
		if csClient != nil {
			seedDeps := profile.Deps{
				Pool:      pool,
				CSClient:  csClient,
				ConnStore: csConnStore,
				Log:       log,
			}
			profile.RunSeedSync(ctx, seedDeps)
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

		// 8. Phase 2 + Phase 3 handler deps — only constructed when CS gRPC
		//    client is wired. Each pointer is nil-tolerated by Plan 02-11's
		//    RegisterRoutes nil-guards so a degraded boot (CS unreachable)
		//    still serves the Phase 1 surface.
		var (
			deviceDeps  *device.Deps
			swapDeps    *swap.HTTPDeps
			profileDeps *profile.HTTPDeps
			gatewayDeps *gateway.Deps
			importDeps  *importpkg.Deps
		)
		if csClient != nil {
			// Closure that adapts EnsureTenantAndApplication into the
			// device.CSBootstrapper 1-method interface. The SAME adapter
			// satisfies gateway.CSBootstrapper + importpkg.CommitBootstrap
			// (identical 1-method shape — see Phase 3 SUMMARY 03-04).
			bootstrapper := bootstrapperFunc(func(c context.Context) (string, string, error) {
				return chirpstack.EnsureTenantAndApplication(c, csClient, csConnStore, log)
			})
			pingMQTT := func(c context.Context) error {
				if cfg.MQTT.URL == "" {
					return errors.New("mqtt not configured")
				}
				return chirpstack.PingMQTT(c, cfg.MQTT.URL, cfg.MQTT.User, cfg.MQTT.Password)
			}
			capturedAppID := csAppID
			deviceDeps = &device.Deps{
				Pool:       pool,
				SessionMgr: sm,
				Log:        log,
				CS:         csClient,
				Bootstrap:  bootstrapper,
				PingGRPC: func(c context.Context, _ string) error {
					return csClient.PingDevices(c, capturedAppID)
				},
				PingMQTT: pingMQTT,
			}
			swapDeps = &swap.HTTPDeps{
				Pool:       pool,
				SessionMgr: sm,
				Log:        log,
				Resolver:   res,
			}
			profileDeps = &profile.HTTPDeps{
				Pool:       pool,
				SessionMgr: sm,
				Log:        log,
				CSClient:   csClient,
				ConnStore:  csConnStore,
			}

			// 8a. Phase 3 GatewayDeps. Constructs the 1-min TTL metrics cache
			//     (D-02), an async stats refresher that writes back into PG so
			//     the next list-page render reads the cached sparkline from
			//     gateway.stats_sparkline (Plan 03-04 Task 3), and the install
			//     state region adapter that resolves the create-gateway dialog
			//     default to chirpstack_connection.region_name (D-03).
			metricsCache := chirpstack.NewMetricsCache(csClient)
			cacheRefresher := &gateway.CacheRefresher{
				Cache:   metricsCache,
				Queries: sqlc.New(pool),
				Log:     log,
			}
			gatewayDeps = &gateway.Deps{
				Pool:         pool,
				SessionMgr:   sm,
				Log:          log,
				CS:           csClient,
				Bootstrap:    bootstrapper,
				MetricsCache: metricsCache,
				InstallState: &installStateRegionReader{pool: pool},
				Refresher:    cacheRefresher,
			}

			// 8b. Phase 3 ImportDeps. CommitDeps reuses the same CS client +
			//     bootstrapper as the device handler so a bulk import row that
			//     creates a device follows the identical CS+PG atomic pattern.
			importDeps = &importpkg.Deps{
				Pool:       pool,
				SessionMgr: sm,
				Log:        log,
				Commit: &importpkg.CommitDeps{
					Pool:      pool,
					CS:        csClient,
					Bootstrap: bootstrapper,
					Log:       log,
				},
			}
		}

		// 9a. River background-job client (Phase 5 REPT-06 + report cleanup).
		// Workers: PDFReportWorker (async PDF generation) +
		//          CleanupExpiredReportsWorker (hourly artifact purge D-07).
		// The EnqueuePDF closure is injected into reportDeps so GenerateHandler
		// can call riverClient.InsertTx inside the same pgx.Tx as the report
		// INSERT + audit row (D-06 + D-23 atomicity requirement).
		q := sqlc.New(pool)
		riverWorkers := river.NewWorkers()
		identityProvider := &sqlcIdentityProvider{pool: pool}
		river.AddWorker(riverWorkers, &report.PDFReportWorker{
			Pool:     pool,
			Queries:  q,
			Identity: identityProvider,
			Log:      log.With("component", "pdf_worker"),
		})
		river.AddWorker(riverWorkers, &report.CleanupExpiredReportsWorker{
			Pool:    pool,
			Queries: q,
			Log:     log.With("component", "report_cleanup"),
		})

		// Phase 6 — Plan 06-01 (D-38 + D-51): daily audit-log retention prune.
		// Calls admin_prune_audit_rows() at 03:00 install_tz via River cron.
		// Reads retention_config.audit_log_days each cycle so a Settings
		// change takes effect at the next scheduled run.
		river.AddWorker(riverWorkers, &alert.AuditPruneWorker{
			Pool:    pool,
			Queries: q,
			Log:     log.With("component", "audit_prune"),
		})

		// Phase 6 — Plan 06-02 (ALERT-01/02/03): threshold + offline evaluators.
		// EvaluateContext is the shared dependency bundle each worker holds;
		// constructed once and reused. RuleStore + AlertStore + WorkerStateStore
		// are also process-wide singletons (concurrent-safe by design).
		alertEng := alert.EvaluateContext{
			Pool:      pool,
			Queries:   q,
			Hub:       eventsHub,
			InstallTZ: time.UTC, // overwritten per-cycle if/when anomaly worker uses install_tz (Plan 06-03)
			Log:       log.With("component", "alert_engine"),
		}
		alertRules := alert.NewRuleStore(pool)
		alertStore := alert.NewAlertStore(pool)
		alertWorkerStat := alert.NewWorkerStateStore(pool)

		river.AddWorker(riverWorkers, &alert.ThresholdInstantaneousWorker{
			Eng: alertEng, Rules: alertRules, Alerts: alertStore, WorkerStat: alertWorkerStat,
		})
		river.AddWorker(riverWorkers, &alert.ThresholdHourlyWorker{
			Eng: alertEng, Rules: alertRules, Alerts: alertStore, WorkerStat: alertWorkerStat,
		})
		river.AddWorker(riverWorkers, &alert.ThresholdDailyWorker{
			Eng: alertEng, Rules: alertRules, Alerts: alertStore, WorkerStat: alertWorkerStat,
		})
		river.AddWorker(riverWorkers, &alert.OfflineWorker{
			Eng: alertEng, Rules: alertRules, Alerts: alertStore, WorkerStat: alertWorkerStat,
		})

		// Build cron schedule for the audit prune. Install timezone is
		// pulled from install_identity (singleton id=1) so the operator-
		// chosen tz at install time drives the schedule. Falls back to
		// UTC if the row hasn't been seeded yet (pre-FinishSetup boots).
		installTZName := "UTC"
		_ = pool.QueryRow(ctx, `SELECT timezone FROM install_identity WHERE id = 1`).Scan(&installTZName)
		// CRON_TZ prefix is the documented robfig/cron way to bind a
		// schedule to a non-UTC timezone (see robfig/cron docs §Time Zones).
		auditPruneSpec := "CRON_TZ=" + installTZName + " 0 3 * * *"
		auditPruneSchedule, err := cron.ParseStandard(auditPruneSpec)
		if err != nil {
			return fmt.Errorf("cron.ParseStandard(%q): %w", auditPruneSpec, err)
		}

		riverPeriodicJobs := []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(1*time.Hour),
				func() (river.JobArgs, *river.InsertOpts) {
					return report.CleanupExpiredReportsArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: false},
			),
			// Audit retention prune — daily at 03:00 install_tz.
			river.NewPeriodicJob(
				auditPruneSchedule,
				func() (river.JobArgs, *river.InsertOpts) {
					return alert.AuditPruneArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: false},
			),
			// Phase 6 Plan 06-02 (ALERT-01) — threshold subtype cadences from
			// 06-RESEARCH §Decision C. Each is independent so they can run
			// concurrently in the default queue (MaxWorkers bumped to 8 below).
			river.NewPeriodicJob(
				river.PeriodicInterval(1*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) {
					return alert.ThresholdInstantaneousArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: false},
			),
			river.NewPeriodicJob(
				river.PeriodicInterval(15*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) {
					return alert.ThresholdHourlyArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: false},
			),
			river.NewPeriodicJob(
				river.PeriodicInterval(1*time.Hour),
				func() (river.JobArgs, *river.InsertOpts) {
					return alert.ThresholdDailyArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: false},
			),
			// Phase 6 Plan 06-02 (ALERT-02/03) — offline evaluator cadence.
			river.NewPeriodicJob(
				river.PeriodicInterval(2*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) {
					return alert.OfflineArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: false},
			),
		}
		riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
			// MaxWorkers bumped from 4 → 8 in Plan 06-02 to absorb the new
			// per-minute / per-2-minute / per-15-minute / per-hour alert
			// periodic flux without queueing pressure on the existing PDF
			// + cleanup + audit_prune workers.
			Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 8}},
			Workers:      riverWorkers,
			PeriodicJobs: riverPeriodicJobs,
		})
		if err != nil {
			return fmt.Errorf("river NewClient: %w", err)
		}
		go func() {
			if riverErr := riverClient.Start(ctx); riverErr != nil {
				log.Error("river start failed", "err", riverErr)
			}
		}()
		defer func() {
			shutdownCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer shutCancel()
			_ = riverClient.Stop(shutdownCtx)
		}()

		// Phase 6 — Plan 06-01 (D-22): degraded-state subscriber. Flips
		// alert_worker_state.degraded=true when a job reaches
		// rivertype.JobStateDiscarded so the shell can render a banner.
		alert.StartDegradedSubscriber(
			ctx, riverClient, alert.NewWorkerStateStore(pool),
			log.With("component", "alert_degraded_subscriber"),
		)

		// Report handler deps — EnqueuePDF calls riverClient.InsertTx inside
		// the same tx as the report INSERT so both are atomic (D-23).
		reportsRoot := cfg.ReportsRoot
		reportDeps := &report.Deps{
			Pool:          pool,
			Queries:       q,
			SessionMgr:    sm,
			Identity:      report.InstallIdentity{Timezone: time.UTC}, // overwritten by identityProvider at worker time
			ArtifactsRoot: reportsRoot,
			EnqueuePDF: func(ctx context.Context, tx pgx.Tx, reportID uuid.UUID, artifactDir string) error {
				_, err := riverClient.InsertTx(ctx, tx, report.PDFReportArgs{ReportID: reportID}, nil)
				return err
			},
		}

		// 9. Router with full Phase 2 + Phase 3 + Phase 4 wiring.
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
			DeviceDeps:   deviceDeps,
			SwapDeps:     swapDeps,
			ProfileDeps:  profileDeps,
			GatewayDeps:  gatewayDeps,
			ImportDeps:   importDeps,
			EventsDeps: &events.Deps{
				Hub:    eventsHub,
				Logger: log.With("component", "events.handler"),
			},
			DashboardDeps: &dashboard.Deps{
				Pool:   pool,
				Logger: log.With("component", "dashboard"),
			},
			ReportDeps: reportDeps,
			SettingsDeps: &settings.Deps{
				Pool:    pool,
				Queries: q,
			},
			MapDeps: &mapapi.Deps{
				Pool:       pool,
				Logger:     log.With("component", "mapapi"),
				SessionMgr: sm,
			},
			FloorPlanDeps: &floorplan.Deps{
				Pool:       pool,
				Queries:    q,
				SessionMgr: sm,
				ImageRoot:  cfg.FloorPlanRoot,
			},
			UserDeps: &user.Deps{
				Pool:       pool,
				Store:      userStore,
				SessionMgr: sm,
				Log:        log.With("component", "user"),
			},
			SPA: httpapi.SPAHandler(),
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

// sqlcResolverLoader adapts the resolver.Loader interface to the production
// sqlc query GetActiveBindingByDevEUI. Plan 02-04 + 02-07 ship the query;
// this loader wraps it, normalizes pgx.ErrNoRows to resolver.ErrNoActive
// Binding, and builds a resolver.Binding with all fields populated.
//
// W5 reuse: the loader calls ingest.BigFloatFromNumeric +
// BigFloatFromNumericNullable directly — no inline duplication of the
// pgtype.Numeric → *big.Float conversion. Single source of truth lives in
// internal/ingest/numeric.go.
type sqlcResolverLoader struct{ pool *pgxpool.Pool }

func (l *sqlcResolverLoader) LoadActive(ctx context.Context, devEUI string, at time.Time) (resolver.Binding, error) {
	q := sqlc.New(l.pool)
	row, err := q.GetActiveBindingByDevEUI(ctx, sqlc.GetActiveBindingByDevEUIParams{
		DevEui:    devEUI,
		ValidFrom: pgtype.Timestamptz{Time: at, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return resolver.Binding{}, resolver.ErrNoActiveBinding
	}
	if err != nil {
		return resolver.Binding{}, fmt.Errorf("resolver loader: %w", err)
	}
	// reading_offset is NOT NULL in schema; fall through to 0 if some
	// edge case sneaks an invalid value.
	offset := ingest.BigFloatFromNumeric(row.ReadingOffset)
	if offset == nil {
		offset = big.NewFloat(0)
	}
	b := resolver.Binding{
		BindingID:       uuid.UUID(row.ID.Bytes),
		MeteringPointID: uuid.UUID(row.MeteringPointID.Bytes),
		DeviceID:        uuid.UUID(row.DeviceID.Bytes),
		DeviceProfileID: uuid.UUID(row.DeviceProfileID.Bytes),
		ReadingOffset:   offset,
		LastRawValue:    ingest.BigFloatFromNumericNullable(row.LastRawValue),
		CounterModulus:  row.CounterModulus,
	}
	if row.ValidFrom.Valid {
		b.ValidFrom = row.ValidFrom.Time
	}
	if row.ValidTo.Valid {
		b.ValidTo = row.ValidTo.Time
	}
	return b, nil
}

// bootstrapperFunc adapts a function to the device.CSBootstrapper interface.
// Used in serve.go's Phase 2 wiring to thread chirpstack.EnsureTenantAnd
// Application (free-function with extra args) into the 1-method interface
// device.Deps.Bootstrap expects.
//
// The SAME adapter also satisfies gateway.CSBootstrapper +
// importpkg.CommitBootstrap (identical 1-method shape) — Phase 3 SUMMARY 03-04
// documents this re-use.
type bootstrapperFunc func(ctx context.Context) (tenantID, appID string, err error)

func (f bootstrapperFunc) EnsureTenantAndApplication(ctx context.Context) (string, string, error) {
	return f(ctx)
}

// sqlcIdentityProvider satisfies report.InstallIdentityProvider by loading
// the install_identity row from Postgres at worker runtime. This ensures the
// PDF worker always uses the current identity (display_name, address, timezone)
// even if it was updated via the install wizard after the binary started.
type sqlcIdentityProvider struct{ pool *pgxpool.Pool }

func (p *sqlcIdentityProvider) Load(ctx context.Context) (report.InstallIdentity, error) {
	var displayName, address, tzName, capabilities string
	err := p.pool.QueryRow(ctx,
		`SELECT display_name, COALESCE(address, ''), timezone, capabilities FROM install_identity WHERE id = 1`,
	).Scan(&displayName, &address, &tzName, &capabilities)
	if err != nil {
		return report.InstallIdentity{}, fmt.Errorf("load identity: %w", err)
	}
	tz, tzErr := time.LoadLocation(tzName)
	if tzErr != nil {
		tz = time.UTC
	}
	return report.InstallIdentity{
		DisplayName:  displayName,
		Address:      address,
		Timezone:     tz,
		Capabilities: capabilities,
	}, nil
}

// installStateRegionReader satisfies gateway.InstallStateReader by querying
// chirpstack_connection.region_name (the post-install LoRaWAN region). The
// wizard's install_state row is deleted at finish (D-11) so the gateway
// create dialog default reads from the persisted chirpstack_connection
// singleton instead.
//
// Behavior:
//   - pgx.ErrNoRows (pre-install row missing)         → return "" + nil so
//     gateway.resolveInstallRegion falls back to AS923_2 (D-03 fallback).
//   - any other error                                  → propagate (handler
//     also tolerates errors via the fallback).
type installStateRegionReader struct{ pool *pgxpool.Pool }

func (r *installStateRegionReader) GetLoRaWANRegionDefault(ctx context.Context) (string, error) {
	var region string
	err := r.pool.QueryRow(ctx,
		`SELECT region_name FROM chirpstack_connection WHERE id = 1`,
	).Scan(&region)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("install_state region reader: %w", err)
	}
	return region, nil
}

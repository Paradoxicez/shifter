package cli

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// bootMockConn satisfies csBootConn against a bufconn-backed *grpc.ClientConn.
// Tests use this to plug Plan 12's NewChirpStackMockBuf into
// probeChirpStackOrRefuse without exercising real TCP.
type bootMockConn struct{ c *grpc.ClientConn }

func (b *bootMockConn) Conn() *grpc.ClientConn { return b.c }
func (b *bootMockConn) Close() error           { return b.c.Close() }

// dialMockBoot returns a csConnDialFunc that opens a bufconn ClientConn against
// the given mock mode ("v4", "v3", "down"). Used to unit-test the boot probe
// without spinning up a real ChirpStack server.
func dialMockBoot(t *testing.T, mode string) csConnDialFunc {
	t.Helper()
	dial, _ := testsupport.NewChirpStackMockBuf(t, mode)
	return func(_ context.Context, _ config.CSConfig) (csBootConn, error) {
		cc, err := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(dial),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, err
		}
		return &bootMockConn{c: cc}, nil
	}
}

// TestServe_RefusesV3 verifies INST-05: when ChirpStack returns
// codes.Unimplemented from InternalService.GetVersion (the v3 fingerprint),
// probeChirpStackOrRefuse returns an error mentioning INST-05 and "ChirpStack
// v3", and the caller (serve) MUST NOT proceed to open a listener.
//
// We cannot directly assert "no listener opened" without running the full
// serve.RunE — instead we assert via the error contract above + the
// structural property that probeChirpStackOrRefuse is called BEFORE
// srv.ListenAndServe() in serve.go (verified by acceptance criteria + by
// reading the serve.go source).
func TestServe_RefusesV3(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := config.CSConfig{GRPCURL: "passthrough:///bufnet", APIToken: "test", Insecure: true}

	err := probeChirpStackOrRefuse(ctx, log, dialMockBoot(t, "v3"), cfg)
	require.Error(t, err, "v3 mock must produce a refusal error")
	require.Contains(t, err.Error(), "INST-05",
		"INST-05: error message must reference the requirement ID for traceability")
	require.Contains(t, strings.ToLower(err.Error()), "chirpstack v3",
		"INST-05: error message must mention ChirpStack v3 so the operator knows to upgrade")
}

// TestServe_AcceptsV4 verifies the inverse: a v4 mock (GetVersion returns a
// non-empty version string) allows boot to proceed.
func TestServe_AcceptsV4(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := config.CSConfig{GRPCURL: "passthrough:///bufnet", APIToken: "test", Insecure: true}

	err := probeChirpStackOrRefuse(ctx, log, dialMockBoot(t, "v4"), cfg)
	require.NoError(t, err, "v4 must permit boot")
}

// TestServe_DegradedOnUnreachable verifies that an unreachable ChirpStack at
// boot logs a warning and returns nil (degraded mode — install wizard /
// Settings → Edit can repair). Refusing to start on transient network blips
// would create an outage cascade that the install gate already handles.
func TestServe_DegradedOnUnreachable(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cfg := config.CSConfig{GRPCURL: "passthrough:///bufnet", APIToken: "test", Insecure: true}

	failingDial := func(_ context.Context, _ config.CSConfig) (csBootConn, error) {
		return nil, context.DeadlineExceeded
	}
	err := probeChirpStackOrRefuse(ctx, log, failingDial, cfg)
	require.NoError(t, err, "unreachable ChirpStack at boot must NOT block startup (degraded mode)")
}

// TestServe_NoConfigSkipsProbe verifies that pre-install (GRPCURL empty) the
// probe is a no-op. This is the path serve takes the first time the operator
// boots Shifter before completing the install wizard.
func TestServe_NoConfigSkipsProbe(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := probeChirpStackOrRefuse(context.Background(), log, nil, config.CSConfig{GRPCURL: ""})
	require.NoError(t, err, "empty GRPCURL must skip probe (pre-install state)")
}

// TestServe_AutoMigrate is intentionally deferred — D-13 auto-migrate is
// covered end-to-end by Plan 20 (compose-smoke-bundled), which boots the full
// binary against a fresh Postgres and asserts schema_migrations rows. Unit-
// testing the same property here would require pulling RunE apart and
// duplicating the migration runner; the integration coverage is stronger.
func TestServe_AutoMigrate(t *testing.T) {
	t.Skip("Plan 20 (compose-smoke-bundled) covers D-13 auto-migrate end-to-end (full binary boot)")
}

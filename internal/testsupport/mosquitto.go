package testsupport

import (
	"context"
	"testing"
	"time"

	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartMosquitto returns a broker URL backed by an ephemeral
// eclipse-mosquitto:2.0.20 container. Auto-cleaned via t.Cleanup. Used by
// internal/testharness and integration tests in internal/ingest,
// internal/resolver, internal/profile.
//
// Pinned tag (T-02-01-01 / T-02-01): eclipse-mosquitto:2.0.20 — never
// `:latest`. The image ships with a /mosquitto-no-auth.conf preset that
// enables anonymous + listener on 1883; using it avoids bind-mounting a
// custom config from the test process.
//
// Phase 2 plan 02-01 bumped the pinned tag from 2.0.18 → 2.0.20 to track the
// upstream patch line; the no-auth preset path is unchanged.
func StartMosquitto(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	req := tc.ContainerRequest{
		Image:        "eclipse-mosquitto:2.0.20",
		ExposedPorts: []string{"1883/tcp"},
		Cmd:          []string{"mosquitto", "-c", "/mosquitto-no-auth.conf"},
		WaitingFor: wait.ForLog("mosquitto version").
			WithStartupTimeout(30 * time.Second),
	}
	cont, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("mosquitto container: %v", err)
	}
	t.Cleanup(func() {
		_ = cont.Terminate(context.Background())
	})

	host, err := cont.Host(ctx)
	if err != nil {
		t.Fatalf("mosquitto host: %v", err)
	}
	port, err := cont.MappedPort(ctx, "1883/tcp")
	if err != nil {
		t.Fatalf("mosquitto port: %v", err)
	}
	return "tcp://" + host + ":" + port.Port()
}

package testsupport

import (
	"context"
	"testing"
	"time"

	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartMosquitto starts eclipse-mosquitto:2.0.18 with anonymous access enabled
// and returns the broker URL like `tcp://127.0.0.1:32789`. The container is
// torn down via t.Cleanup.
//
// Pinned tag (T-02-01): eclipse-mosquitto:2.0.18. The image ships with a
// /mosquitto-no-auth.conf preset that enables anonymous + listener on 1883;
// using it avoids bind-mounting a custom config from the test process.
func StartMosquitto(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	req := tc.ContainerRequest{
		Image:        "eclipse-mosquitto:2.0.18",
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

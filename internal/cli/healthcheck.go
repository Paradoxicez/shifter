package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// healthcheckCmd implements D-15: a localhost HTTP probe used as the Docker
// HEALTHCHECK command. It must run inside the same container as the binary,
// so it talks to 127.0.0.1 rather than a service name. Exits 0 on a 2xx/3xx
// /health response, non-zero otherwise (Docker counts non-zero as unhealthy).
//
// PITFALL #11: a curl-based HEALTHCHECK adds a 7 MB curl dependency to the
// final image; using the binary itself keeps the image scratch-friendly.
var healthcheckCmd = &cobra.Command{
	Use:   "healthcheck",
	Short: "Localhost HTTP GET /health for use as a Docker HEALTHCHECK",
	RunE: func(cmd *cobra.Command, _ []string) error {
		port := os.Getenv("SHIFTER_HTTP_PORT")
		if port == "" {
			port = "8080"
		}
		url := fmt.Sprintf("http://127.0.0.1:%s/health", port)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("healthcheck failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("healthcheck status %d", resp.StatusCode)
		}
		return nil
	},
}

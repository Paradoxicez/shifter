package compose_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// composePaths are the two canonical Shifter compose files relative to the
// repo root. The test file lives at internal/compose/ so the root is ../../.
var composePaths = []string{
	"../../compose/bundled.yml",
	"../../compose/external.yml",
}

// readCompose reads a compose file and returns its contents as a string.
func readCompose(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err, "reading compose file %s", path)
	return string(b)
}

// nonCommentLines returns only the non-comment lines of a compose file
// (strips lines starting with optional whitespace then '#').
func nonCommentLines(content string) []string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			out = append(out, line)
		}
	}
	return out
}

// TestCompose_NoLatestTag — no non-comment line in either compose file may
// contain ":latest" (OPS-07 / T-20-02 / PITFALLS §15). Comments explaining
// the convention are permitted.
func TestCompose_NoLatestTag(t *testing.T) {
	for _, p := range composePaths {
		content := readCompose(t, p)
		for i, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				continue // skip comment lines
			}
			require.NotContains(t, line, ":latest",
				"%s line %d must not use :latest image tag: %q", p, i+1, trimmed)
		}
	}
}

// TestCompose_PinnedImageVersions — every "image:" line must have a version
// tag (image: name:tag). A bare "image: name" with no colon-tag is a
// convention violation.
func TestCompose_PinnedImageVersions(t *testing.T) {
	// Matches image lines that have a tag (image: something:something).
	taggedRe := regexp.MustCompile(`^\s+image:\s+\S+:\S+`)
	// Matches image lines with NO tag (image: something — no colon after name).
	bareRe := regexp.MustCompile(`^\s+image:\s+[^\s:]+\s*$`)

	for _, p := range composePaths {
		content := readCompose(t, p)
		for i, line := range strings.Split(content, "\n") {
			if bareRe.MatchString(line) && !taggedRe.MatchString(line) {
				t.Errorf("%s line %d has unpinned image (no tag): %q", p, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// serviceNames extracts the top-level service names from a compose YAML by
// finding the services: block and collecting lines with exactly 2 leading
// spaces followed by a name and colon (e.g. "  postgres:").
// This avoids matching nested keys like "  depends_on:", "  environment:", etc.
// by only collecting names found between the "services:" line and the next
// top-level key (like "volumes:" or "networks:").
func serviceNames(content string) []string {
	lines := strings.Split(content, "\n")
	// Find the "services:" section.
	inServices := false
	// Matches exactly "  word:" — two leading spaces, a lowercase-start name, colon.
	svcLineRe := regexp.MustCompile(`^  ([a-z][a-z0-9_-]+):\s*$`)
	// Top-level key (no leading spaces) that ends the services section.
	topLevelRe := regexp.MustCompile(`^[a-z][a-z0-9_-]+:\s*$`)

	var names []string
	for _, line := range lines {
		if line == "services:" {
			inServices = true
			continue
		}
		if inServices {
			if topLevelRe.MatchString(line) && line != "services:" {
				// Entered the next top-level block (volumes:, networks:, etc.)
				inServices = false
				continue
			}
			if m := svcLineRe.FindStringSubmatch(line); m != nil {
				names = append(names, m[1])
			}
		}
	}
	return names
}

// TestCompose_EveryServiceHasJsonLogging — every service block must contain
// "logging: *json-logging" (anchor reference) to ensure json-file caps apply
// to all containers (OPS-05 / T-20-04). This includes the backup-cron sidecar
// added in Plan 06-08.
func TestCompose_EveryServiceHasJsonLogging(t *testing.T) {
	for _, p := range composePaths {
		content := readCompose(t, p)
		names := serviceNames(content)
		require.Greater(t, len(names), 0, "%s must define at least one service", p)

		loggingRefCount := strings.Count(content, "logging: *json-logging")
		require.Equal(t, len(names), loggingRefCount,
			"%s: every service must have 'logging: *json-logging' (%d services: %v, %d logging refs)",
			p, len(names), names, loggingRefCount)
	}
}

// TestCompose_NoEnvCredentials — no service may expose secrets via environment
// variables with ${...} interpolation for credential-bearing keys (D-06 /
// OPS-06). Credentials must be mounted via Compose secrets: as _FILE vars.
func TestCompose_NoEnvCredentials(t *testing.T) {
	// Matches env vars whose name suggests a credential value (not a _FILE
	// pointer) being set to a ${...} interpolation.
	credentialLeakRe := regexp.MustCompile(
		`(?i)(PASSWORD|API_TOKEN|SECRET|SIGNING_KEY|API_KEY):\s+\$\{`)

	for _, p := range composePaths {
		content := readCompose(t, p)
		// Only check non-comment lines.
		nonComment := strings.Join(nonCommentLines(content), "\n")
		matches := credentialLeakRe.FindAllString(nonComment, -1)
		require.Empty(t, matches,
			"%s leaks secret via env var interpolation (use secrets: + _FILE pattern instead): %v",
			p, matches)
	}
}

// TestCompose_AllSecretsMountedViaFiles — every credential env var must use
// the _FILE: /run/secrets/<name> pattern, not a bare value. This test verifies
// the positive case: _FILE vars are present for each known credential.
func TestCompose_AllSecretsMountedViaFiles(t *testing.T) {
	for _, p := range composePaths {
		content := readCompose(t, p)
		// Shifter service must use _FILE variants for all four credentials.
		for _, fileVar := range []string{
			"SHIFTER_DB_PASSWORD_FILE",
			"SHIFTER_CHIRPSTACK_API_TOKEN_FILE",
			"SHIFTER_SESSION_KEY_FILE",
			"SHIFTER_MQTT_PASSWORD_FILE",
		} {
			require.Contains(t, content, fileVar,
				"%s must use %s (secrets via file, not env var)", p, fileVar)
		}
	}
}

// TestCompose_BackupsVolumeMounted — both compose files must have a "backups:"
// named volume and it must be mounted in the shifter service at
// /var/lib/shifter/backups (Plan 06-08 / OPS-02).
func TestCompose_BackupsVolumeMounted(t *testing.T) {
	for _, p := range composePaths {
		content := readCompose(t, p)
		require.Contains(t, content, "backups:",
			"%s must declare a 'backups:' named volume", p)
		require.Contains(t, content, "backups:/var/lib/shifter/backups",
			"%s must mount the backups volume at /var/lib/shifter/backups in the shifter service", p)
	}
}

// TestCompose_NoExposedInternalServices — postgres, mosquitto, redis must NOT
// have host port mappings. Only Caddy (80/443) and chirpstack-gateway-bridge
// (1700/udp) may bind host ports (T-20-03).
//
// Strategy: extract each internal service's block (lines between its header
// and the next service header), and assert the block contains no "ports:"
// key.
func TestCompose_NoExposedInternalServices(t *testing.T) {
	internalServices := []string{"postgres", "mosquitto", "redis"}

	for _, p := range composePaths {
		content := readCompose(t, p)
		lines := strings.Split(content, "\n")
		svcLineRe := regexp.MustCompile(`^  ([a-z][a-z0-9_-]+):\s*$`)

		// Build a map of service_name → block_text.
		blocks := map[string][]string{}
		var currentSvc string
		for _, line := range lines {
			if m := svcLineRe.FindStringSubmatch(line); m != nil {
				currentSvc = m[1]
				blocks[currentSvc] = nil
				continue
			}
			if currentSvc != "" {
				blocks[currentSvc] = append(blocks[currentSvc], line)
			}
		}

		for _, svc := range internalServices {
			block, found := blocks[svc]
			if !found {
				continue // service not in this compose flavor
			}
			// Filter comment lines before checking for ports: to avoid false
			// positives from documentation comments like "# No `ports:` block".
			var nonCommentBlock []string
			for _, bl := range block {
				trimmed := strings.TrimSpace(bl)
				if !strings.HasPrefix(trimmed, "#") {
					nonCommentBlock = append(nonCommentBlock, bl)
				}
			}
			blockText := strings.Join(nonCommentBlock, "\n")
			require.NotContains(t, blockText, "ports:",
				"%s: internal service %q must not expose host ports", p, svc)
		}
	}
}

// TestCompose_TopLevelSecretsDeclaration — both files must have a top-level
// secrets: block declaring the four Shifter secrets (D-06).
func TestCompose_TopLevelSecretsDeclaration(t *testing.T) {
	for _, p := range composePaths {
		content := readCompose(t, p)
		for _, secret := range []string{
			"postgres_password:",
			"chirpstack_api_token:",
			"session_signing_key:",
			"mqtt_password:",
		} {
			require.Contains(t, content, secret,
				"%s must declare secret %q in the top-level secrets: block", p, secret)
		}
	}
}

// TestRunbook_HasComposeConventions — docs/operator-runbook.md must contain
// the "## Compose conventions" heading (Plan 06-11).
func TestRunbook_HasComposeConventions(t *testing.T) {
	content := readRunbook(t)
	require.Contains(t, content, "## Compose conventions",
		"operator-runbook.md must contain '## Compose conventions' section")
}

// TestRunbook_HasUpgradingSection — docs/operator-runbook.md must contain the
// "## Upgrading Shifter" heading and 5 numbered steps (Plan 06-11).
func TestRunbook_HasUpgradingSection(t *testing.T) {
	content := readRunbook(t)
	require.Contains(t, content, "## Upgrading Shifter",
		"operator-runbook.md must contain '## Upgrading Shifter' section")

	// The section must have 5 numbered steps.
	for _, step := range []string{"1. ", "2. ", "3. ", "4. ", "5. "} {
		require.Contains(t, content, step,
			"operator-runbook.md Upgrading section must contain step %q", step)
	}
}

// TestRunbook_BackupRestoreSection — docs/operator-runbook.md must contain the
// "## Backup & Restore" section added by Plan 06-09 (preserved in Plan 06-11
// audit).
func TestRunbook_BackupRestoreSection(t *testing.T) {
	content := readRunbook(t)
	require.Contains(t, content, "## Backup & Restore",
		"operator-runbook.md must retain '## Backup & Restore' section from Plan 06-09")
}

// readRunbook reads docs/operator-runbook.md relative to the repo root.
func readRunbook(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../docs/operator-runbook.md")
	require.NoError(t, err, "reading docs/operator-runbook.md")
	return string(b)
}

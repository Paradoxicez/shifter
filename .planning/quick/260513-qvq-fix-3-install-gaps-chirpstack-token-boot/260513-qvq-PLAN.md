---
phase: 260513-qvq
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - install/bundled/bootstrap-chirpstack-token.sh
  - install/bundled/install.sh
  - internal/chirpstack/version.go
  - internal/install/handlers.go
  - internal/install/handlers_test.go
autonomous: true
requirements: []
---

<objective>
Close 3 install rehearsal gaps: (bug #4) ChirpStack API token is never bootstrapped so
secrets/chirpstack_api_token.txt stays as "placeholder", leaving wizard step 2 unable
to authenticate; (bug #5) ProbeVersion gets Unauthenticated even with a real token
because InternalService.GetVersion requires a USER-session JWT, not a global API key;
(bug #6) wizard handlers accept out-of-order step posts so step 3 succeeds even when
step 2 never ran.

Purpose: Make the bundled install rehearsal end-to-end green without manual intervention.
Output: bootstrap-chirpstack-token.sh, fix to version.go ProbeVersion, 409 guard in step handlers.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/quick/260513-qcf-fix-3-install-rehearsal-bugs-install-sh-/260513-qcf-SUMMARY.md

Key facts carried forward from qcf:
- ChirpStack config lives at /tmp/chirpstack/ inside the container (sed-substituted from
  /etc/chirpstack/ via the sh entrypoint in compose/bundled.yml).
- `exec chirpstack -c /tmp/chirpstack` is the final exec in the entrypoint.
- secrets/ files are bind-mounted into the compose stack via top-level `secrets:` block;
  `secrets/chirpstack_api_token.txt` is consumed by the shifter service at startup via
  SHIFTER_CHIRPSTACK_API_TOKEN_FILE=/run/secrets/chirpstack_api_token.
- The chirpstack container name in compose is `compose-chirpstack-1` (Compose project=compose,
  service=chirpstack, replica=1).

Current state of files to be modified:
- install/bundled/install.sh: exits after health poll at line 81; no token bootstrap step.
- internal/chirpstack/version.go: calls InternalService.GetVersion — auth issue under investigation.
- internal/install/handlers.go: Step2/3/4Handler have no CurrentStep pre-check.
- internal/install/state.go: State.CurrentStep is an int bumped by UpdateStepN (GREATEST semantics).
</context>

<tasks>

<task type="auto">
  <name>Task 1: ChirpStack API token bootstrap script + install.sh hook</name>
  <files>install/bundled/bootstrap-chirpstack-token.sh, install/bundled/install.sh</files>
  <action>
Create install/bundled/bootstrap-chirpstack-token.sh (chmod +x). The script:

```bash
#!/usr/bin/env bash
# bootstrap-chirpstack-token.sh — generate a ChirpStack global API key and
# write it to secrets/chirpstack_api_token.txt so wizard step 2 can authenticate.
# Called by install.sh after the health poll. Non-fatal on failure (|| true).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

echo "==> Bootstrapping ChirpStack API token..."

# Wait up to 30s for the gRPC API to be serving (chirpstack logs "api server
# listening" but there is no healthcheck on the chirpstack service itself).
deadline=$((SECONDS + 30))
until docker exec compose-chirpstack-1 \
      chirpstack -c /tmp/chirpstack --help > /dev/null 2>&1; do
  if [ $SECONDS -gt $deadline ]; then
    echo "WARNING: ChirpStack not ready after 30s — skipping token bootstrap." \
         "Re-run ./install/bundled/bootstrap-chirpstack-token.sh manually." >&2
    exit 0
  fi
  sleep 2
done

# create-api-key prints a single line: "token: <jwt>"
OUTPUT=$(docker exec compose-chirpstack-1 \
  chirpstack -c /tmp/chirpstack create-api-key --name shifter-bootstrap 2>&1) || true

TOKEN=$(echo "$OUTPUT" | grep -oP '(?<=token: )eyJ[A-Za-z0-9._-]+' || true)

if [ -z "$TOKEN" ]; then
  echo "WARNING: Could not extract JWT from create-api-key output." \
       "Output was: $OUTPUT" >&2
  echo "Re-run ./install/bundled/bootstrap-chirpstack-token.sh manually." >&2
  exit 0
fi

printf '%s' "$TOKEN" > secrets/chirpstack_api_token.txt
chmod 0600 secrets/chirpstack_api_token.txt

# Restart shifter so it picks up the new secret on next start.
docker restart compose-shifter-1 > /dev/null 2>&1 || true

echo "==> ChirpStack API token bootstrapped — wizard step 2 ready"
```

Then in install/bundled/install.sh, add a call to the bootstrap script immediately
after the health-check loop and before the final "Visit https://..." echo. Insert:

```bash
# Bootstrap ChirpStack API token so wizard step 2 can authenticate (bug #4).
./install/bundled/bootstrap-chirpstack-token.sh || true
```

Do NOT touch any other section of install.sh. The line goes between the `done` that
closes the curl loop and the final `echo "==> Shifter is up..."` line.
  </action>
  <verify>
bash -n install/bundled/bootstrap-chirpstack-token.sh
bash -n install/bundled/install.sh
grep -n "bootstrap-chirpstack-token" install/bundled/install.sh
# Should show the call on one line, between the health-poll block and final echo.
  </verify>
  <done>
- install/bundled/bootstrap-chirpstack-token.sh exists, is executable, passes bash -n.
- install/bundled/install.sh contains `./install/bundled/bootstrap-chirpstack-token.sh || true`.
- bash -n passes for both files.
  </done>
</task>

<task type="auto">
  <name>Task 2: Fix ProbeVersion auth — two-step validate+version approach</name>
  <files>internal/chirpstack/version.go</files>
  <action>
INVESTIGATION STEP (executor must run this first):

```bash
# Get the current token (may still be placeholder at this point — use a real one
# from a live stack if available, or rely on the analysis below).
TOKEN=$(cat secrets/chirpstack_api_token.txt)

# Test whether InternalService.GetVersion accepts a global API key:
docker exec compose-chirpstack-1 \
  grpcurl -plaintext \
  -H "authorization: Bearer $TOKEN" \
  chirpstack:8080 api.InternalService.GetVersion 2>&1 | head -5
```

EXPECTED FINDING: ChirpStack v4's InternalService.GetVersion does NOT require auth —
it is one of the few endpoints that is intentionally unauthenticated (it is used by
the web UI before login to detect version). The `Unauthenticated desc=""` error seen
in rehearsal is therefore NOT from GetVersion itself but from the 8-second context
deadline firing on the dial (grpc.NewClient is lazy — the first RPC attempt triggers
the actual TCP connect + TLS handshake, and if the chirpstack container was still
starting, it times out silently as Unauthenticated in some gRPC versions).

LIKELY ROOT CAUSE: The bootstrap token did not exist at wizard time (placeholder was
in place), so the request had `authorization: Bearer placeholder` which ChirpStack
rejects with Unauthenticated on any auth-required RPC. GetVersion itself is unauth'd
but the 8s deadline was also racing the container startup.

APPROACH — two-step probe in ProbeVersion:
1. Call GetVersion (unauthenticated, captures version string).
2. If GetVersion succeeds, call TenantService.ListTenants (requires valid API key,
   empty request, limit=1) to validate the token. A real API key gets 200+empty list;
   a placeholder/bad key gets Unauthenticated/PermissionDenied.
3. If TenantService.ListTenants fails with Unauthenticated or PermissionDenied →
   return a new sentinel error ErrInvalidAPIToken (defined in errors.go) that maps to
   422 grpc_unreachable + detail "invalid_api_token" in the handler.
4. If GetVersion returns Unimplemented/NotFound → ErrChirpStackV3OrUnknown (unchanged).

Update internal/chirpstack/version.go:

```go
package chirpstack

import (
    "context"
    "fmt"

    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
    "google.golang.org/protobuf/types/known/emptypb"

    api "github.com/chirpstack/chirpstack/api/go/v4/api"
)

// ProbeVersion calls InternalService.GetVersion to detect v3-vs-v4, then
// validates the API token via TenantService.ListTenants (an auth-required RPC).
//
// Two-step design:
//  1. GetVersion — unauthenticated; returns ErrChirpStackV3OrUnknown on
//     Unimplemented/NotFound (INST-05).
//  2. TenantService.ListTenants with limit=1 — validates the API token.
//     Unauthenticated or PermissionDenied → ErrInvalidAPIToken.
//
// This separates version detection from token validation, both of which the
// wizard step 2 needs to confirm before persisting the connection.
func ProbeVersion(ctx context.Context, conn *grpc.ClientConn) (string, error) {
    // Step 1: version detection (unauthenticated).
    internalClient := api.NewInternalServiceClient(conn)
    resp, err := internalClient.GetVersion(ctx, &emptypb.Empty{})
    if err != nil {
        st, ok := status.FromError(err)
        if ok && (st.Code() == codes.Unimplemented || st.Code() == codes.NotFound) {
            return "", ErrChirpStackV3OrUnknown
        }
        return "", fmt.Errorf("chirpstack version probe: %w", err)
    }
    if resp.GetVersion() == "" {
        return "", ErrChirpStackV3OrUnknown
    }
    version := resp.GetVersion()

    // Step 2: token validation via an auth-required RPC.
    tenantClient := api.NewTenantServiceClient(conn)
    _, err = tenantClient.List(ctx, &api.ListTenantsRequest{Limit: 1})
    if err != nil {
        st, ok := status.FromError(err)
        if ok && (st.Code() == codes.Unauthenticated || st.Code() == codes.PermissionDenied) {
            return "", ErrInvalidAPIToken
        }
        // Any other error (Unavailable, DeadlineExceeded, etc.) — treat as
        // unreachable so caller maps it to grpc_unreachable.
        return "", fmt.Errorf("chirpstack token validation: %w", err)
    }

    return version, nil
}
```

Also add ErrInvalidAPIToken to internal/chirpstack/errors.go:

```go
// ErrInvalidAPIToken is returned by ProbeVersion when TenantService.List
// rejects the API token with Unauthenticated or PermissionDenied.
// Callers must refuse to persist the connection (INST-05 extension).
var ErrInvalidAPIToken = errors.New("chirpstack: API token is invalid or lacks global permissions")
```

Then in internal/install/handlers.go Step2Handler, after the `errors.Is(err, chirpstack.ErrChirpStackV3OrUnknown)` block, add:

```go
if errors.Is(err, chirpstack.ErrInvalidAPIToken) {
    writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
        "error": "grpc_unreachable", "detail": "invalid_api_token",
    })
    return
}
```

This goes between the ErrChirpStackV3OrUnknown check and the generic `err != nil` check
(lines 277-286 of the current handlers.go). No other changes to handlers.go.

Also update the testsupport mock (internal/testsupport or wherever the ChirpStack mock
defines InternalService.GetVersion and TenantService) so TenantService.ListTenants
returns an empty list for "v4" mode. Check internal/testsupport/ for the mock server
registration. The v4 mock must register TenantServiceServer; the v3 mock must NOT (so
List returns Unimplemented, which falls through to the generic error path).

Run: go build ./internal/chirpstack/... ./internal/install/...
Run: go test ./internal/chirpstack/... ./internal/install/... -count=1
  </action>
  <verify>
go build ./internal/chirpstack/... ./internal/install/...
go test ./internal/chirpstack/... ./internal/install/... -count=1 -v 2>&1 | tail -20
  </verify>
  <done>
- internal/chirpstack/version.go two-step probe compiles.
- internal/chirpstack/errors.go defines ErrInvalidAPIToken.
- internal/install/handlers.go maps ErrInvalidAPIToken → 422 grpc_unreachable detail=invalid_api_token.
- go test ./internal/chirpstack/... ./internal/install/... passes (all existing tests green).
  </done>
</task>

<task type="auto">
  <name>Task 3: Sequential step enforcement — 409 guard in Step2/3/4Handler + unit test</name>
  <files>internal/install/handlers.go, internal/install/handlers_test.go</files>
  <action>
At the top of each of Step2Handler, Step3Handler, Step4Handler (AFTER the CSRF check,
BEFORE any request body decode), load current state and assert CurrentStep equals the
expected step. Step1Handler is already the entry point (no prior step) — leave it alone.
FinishHandler already has ErrIncompleteWizard in FinishSetup — leave it alone.

Pattern to add to Step2Handler (expect CurrentStep == 2 before processing):

```go
// Sequential enforcement: step 2 requires step 1 to have completed.
st, err := deps.Store.GetOrCreate(r.Context())
if err != nil {
    deps.Log.Error("install step2 load state", "err", err)
    writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
    return
}
if st.CurrentStep != 2 {
    writeJSON(w, http.StatusConflict, map[string]string{
        "error":  "step_out_of_order",
        "detail": fmt.Sprintf("expected step 2, wizard is at step %d", st.CurrentStep),
    })
    return
}
```

Apply the same pattern to Step3Handler (check CurrentStep == 3) and Step4Handler
(check CurrentStep == 4), adjusting the step number in the error message. Place the
guard block right after the CSRF check returns and before `var req step2Req` (or
step3Req/step4Req) is declared.

Note: `fmt` is already imported in handlers.go. No new imports needed.

ADD to internal/install/handlers_test.go — one new test:

```go
func TestStep_OutOfOrder_Returns409(t *testing.T) {
    srv, store, _, _ := setupHandlers(t, "v4")
    // State starts at CurrentStep=1 (step 1 not yet done).
    // Attempting step 3 before step 2 must return 409.
    _ = post(t, srv, "/api/install/step/1", map[string]string{
        "email": "op@example.com", "name": "Op", "password": "Strong-Pass-1!",
    })
    // After step 1, CurrentStep==2. Attempt step 3 directly.
    res := post(t, srv, "/api/install/step/3", map[string]string{"name": "as923"})
    defer res.Body.Close()
    require.Equal(t, http.StatusConflict, res.StatusCode)
    var body map[string]string
    require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
    require.Equal(t, "step_out_of_order", body["error"])

    // Also assert step 2 works at the right point (CurrentStep==2).
    res2 := post(t, srv, "/api/install/step/2", map[string]string{
        "mode": "bundled", "grpc_url": "test:8080", "api_token": "t",
        "mqtt_url": "tcp://test:1883",
    })
    defer res2.Body.Close()
    require.Equal(t, http.StatusOK, res2.StatusCode)

    // After step 2, CurrentStep==3. Now step 3 must work.
    _ = store // store is available if needed for assertions
    res3 := post(t, srv, "/api/install/step/3", map[string]string{"name": "as923"})
    defer res3.Body.Close()
    require.Equal(t, http.StatusOK, res3.StatusCode)
}
```

Run: go test ./internal/install/... -run TestStep_OutOfOrder -v
Run: go test ./internal/install/... -count=1
  </action>
  <verify>
go test ./internal/install/... -run TestStep_OutOfOrder -v
go test ./internal/install/... -count=1
  </verify>
  <done>
- Step2Handler, Step3Handler, Step4Handler each check CurrentStep == N and return 409 step_out_of_order if not.
- TestStep_OutOfOrder_Returns409 passes.
- All existing install handler tests remain green.
- go test ./internal/install/... -count=1 exits 0.
  </done>
</task>

</tasks>

<verification>
After all three tasks:
1. bash -n install/bundled/bootstrap-chirpstack-token.sh exits 0.
2. install/bundled/install.sh contains the bootstrap-chirpstack-token call.
3. go build ./... exits 0.
4. go test ./internal/chirpstack/... ./internal/install/... -count=1 exits 0.
5. Live smoke (optional if compose is up): POST step 3 before step 2 returns 409.
</verification>

<success_criteria>
- secrets/chirpstack_api_token.txt contains a real JWT after install.sh runs (starts with eyJ).
- Wizard step 2 returns 200 with a real bootstrap token in place.
- Wizard step 3 POST before step 2 returns 409 step_out_of_order.
- All tests pass, build clean.
</success_criteria>

<output>
After completion, create .planning/quick/260513-qvq-fix-3-install-gaps-chirpstack-token-boot/260513-qvq-SUMMARY.md
</output>

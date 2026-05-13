---
plan_id: 07-15-wire-phase-7-deps
phase: 07-multi-vendor-breadth-v1-x-differentiators
gap_closure: true
wave: 1
depends_on: []
files_modified:
  - internal/cli/serve.go
autonomous: false
requirements_addressed: [V2-VEND-01, V2-VEND-02, V2-VEND-03, ALERT-04, UX-POWER]
must_haves:
  truths:
    - All 6 Phase 7 HTTP endpoint families respond from the production binary (return 401/405 when unauthed/wrong-method, not 200+HTML SPA fallback)
  artifacts:
    - "internal/cli/serve.go contains CatalogDeps assignment in httpapi.Deps{} literal"
    - "internal/cli/serve.go contains CodecTestDeps assignment in httpapi.Deps{} literal"
    - "internal/cli/serve.go contains BacktestDeps assignment in httpapi.Deps{} literal"
    - "internal/cli/serve.go contains ReportTemplateDeps assignment in httpapi.Deps{} literal"
    - "internal/cli/serve.go contains CompareDeps assignment in httpapi.Deps{} literal"
    - "internal/cli/serve.go contains GatewayImportDeps assignment in httpapi.Deps{} literal"
  key_links:
    - "GET /api/catalog → expects 401 (admin/viewer auth required), NOT 200 HTML"
    - "POST /api/device-profiles/{id}/test-codec → expects 401, NOT 200 HTML"
    - "POST /api/alerts/backtest → expects 401, NOT 200 HTML"
    - "GET /api/reports/templates → expects 401, NOT 200 HTML"
    - "POST /api/reports/compare → expects 401, NOT 200 HTML"
    - "POST /api/gateways/bulk-import/validate → expects 401, NOT 200 HTML"
---

<objective>
Wire 6 Phase 7 deps that were declared in internal/http/router.go but never instantiated in internal/cli/serve.go, then rebuild + redeploy the docker image. Without this, every Phase 7 HTTP endpoint falls through to the SPA fallback returning HTML+200 instead of JSON.

**Root cause (from 07-HUMAN-UAT.md gap diagnosis):** The Deps struct at `internal/http/router.go` lines 197-236 declares `CatalogDeps`, `CodecTestDeps`, `BacktestDeps`, `ReportTemplateDeps`, `CompareDeps`, `GatewayImportDeps` with nil-guards. The production wiring in `internal/cli/serve.go` lines 578-657 omits all 6. Unit tests passed because they construct minimal Deps in `handler_test.go` files.
</objective>

<task id="1" name="Wire 6 Phase 7 deps into serve.go">
<read_first>
- internal/cli/serve.go (lines 575-660 — where httpapi.NewRouter(httpapi.Deps{...}) is called)
- internal/http/router.go (lines 197-260 — Deps struct fields + types)
- internal/api/catalog_handler.go (lines 38-50 — CatalogDeps struct)
- internal/api/codec_test_handler.go (lines 46-60 — CodecTestDeps struct)
- internal/api/backtest_handler.go (lines 19-30 — BacktestDeps struct)
- internal/api/report_templates_handler.go (lines 40-55 — ReportTemplateDeps struct)
- internal/api/compare_handler.go (lines 32-45 — CompareDeps struct)
- internal/api/gateway_import_handler.go (lines 54-70 — GatewayImportDeps struct)
- internal/gateway/import.go (lines 95-110 — NewImportService constructor)
</read_first>

<action>
Edit `internal/cli/serve.go`. Inside the `httpapi.NewRouter(httpapi.Deps{...})` literal (around line 578-657), add these 6 fields. Place them after the existing `AuditDeps:` block but before `SPA: httpapi.SPAHandler(),`. Add the import for `gateway` package at the top of the file if not already present (it should be, since `gatewayDeps` is already wired).

Exact deps to add (use existing `pool`, `sm`, `log`, `gatewayDeps` variables from the enclosing scope — do NOT re-construct them):

```go
CatalogDeps: &apipkg.CatalogDeps{
    Pool:       pool,
    SessionMgr: sm,
},
CodecTestDeps: &apipkg.CodecTestDeps{
    Pool:       pool,
    SessionMgr: sm,
    Log:        log.With("component", "codec_test"),
},
BacktestDeps: &apipkg.BacktestDeps{
    Pool: pool,
},
ReportTemplateDeps: &apipkg.ReportTemplateDeps{
    Pool:       pool,
    SessionMgr: sm,
},
CompareDeps: &apipkg.CompareDeps{
    Pool:       pool,
    SessionMgr: sm,
},
GatewayImportDeps: &apipkg.GatewayImportDeps{
    Pool:       pool,
    SessionMgr: sm,
    Log:        log.With("component", "gateway_import"),
    ImportSvc:  gateway.NewImportService(pool),
},
```

Note on imports:
- `apipkg` is already aliased — same alias as for `DeviceDeps`, `AlertDeps`, etc.
- `gateway.NewImportService` may require adding `"github.com/shifter-io/shifter/internal/gateway"` if the import isn't present (check `goimports` / `go build` output after the edit).

After editing, run:
```bash
go build ./...
go test ./internal/cli/... ./internal/api/... -short -count=1
```

Both must exit 0. Build catches missing imports; tests catch typos in struct field names.
</action>

<acceptance_criteria>
- `grep -q "CatalogDeps:" internal/cli/serve.go` exits 0
- `grep -q "CodecTestDeps:" internal/cli/serve.go` exits 0
- `grep -q "BacktestDeps:" internal/cli/serve.go` exits 0
- `grep -q "ReportTemplateDeps:" internal/cli/serve.go` exits 0
- `grep -q "CompareDeps:" internal/cli/serve.go` exits 0
- `grep -q "GatewayImportDeps:" internal/cli/serve.go` exits 0
- `grep -q "gateway.NewImportService" internal/cli/serve.go` exits 0
- `go build ./...` exits 0
- `go test ./internal/cli/... ./internal/api/... -short -count=1` exits 0
</acceptance_criteria>

<done>
Commit message: `fix(07-15): wire 6 Phase 7 deps into serve.go — restores /api/catalog, /api/alerts/backtest, /api/reports/{compare,templates}, /api/gateways/bulk-import, codec test runner`
</done>
</task>

<task id="2" name="Rebuild docker image + restart shifter container">
<read_first>
- justfile (recipes: `_compose-build-image`, `_compose-prep-secrets`)
- compose/bundled.yml (the running stack)
</read_first>

<action>
1. Rebuild the shifter docker image with the new binary:
   ```bash
   just _compose-prep-secrets
   just _compose-build-image
   ```

2. Restart the shifter container in the running bundled stack so it picks up the new image. ChirpStack is in restart-loop (separate compose bug) — DO NOT bring it down; only recreate the shifter service:
   ```bash
   cd compose && docker compose -f bundled.yml up -d --force-recreate --no-deps shifter
   ```

3. Wait for shifter to be healthy:
   ```bash
   for i in {1..30}; do
     if curl -fs http://localhost:8080/health > /dev/null 2>&1; then
       echo "shifter healthy"
       break
     fi
     sleep 2
   done
   curl -s http://localhost:8080/health | head -2
   ```

4. Confirm the running binary's commit hash matches the latest local commit (so we know the rebuild produced a fresh binary, not an old cached layer):
   ```bash
   curl -s http://localhost:8080/health | python3 -c "import json,sys;print(json.load(sys.stdin)['version']['commit'])"
   git rev-parse --short HEAD
   ```
   The two values should match (or running may be one ahead if commit just happened).
</action>

<acceptance_criteria>
- `docker images shifter:0.1.0 --format '{{.CreatedSince}}'` shows "Less than a minute ago" or similar fresh timestamp
- `curl -fs http://localhost:8080/health` returns `{"status":"ok",...}` within 60 seconds of restart
- The `version.commit` reported by `/health` matches the new HEAD commit hash from task 1
</acceptance_criteria>

<done>
No commit (image rebuild + restart only — no code change)
</done>
</task>

<task id="3" name="Verify all 6 Phase 7 endpoints respond properly">
<read_first>
- internal/http/router.go (lines 530-620 — to know the expected mount paths and methods)
</read_first>

<action>
Probe each Phase 7 endpoint without credentials. They should return JSON 401 (admin/viewer auth required) or 405 (wrong method) — NOT 200 with HTML body (which means SPA fallback caught the route, indicating the deps wiring failed).

```bash
echo "--- /api/catalog (expect 401) ---"
curl -s -o /tmp/r1 -w "%{http_code} %{content_type}\n" http://localhost:8080/api/catalog
head -1 /tmp/r1

echo "--- /api/device-profiles/00000000-0000-0000-0000-000000000000/test-codec (expect 401 or 405) ---"
curl -s -o /tmp/r2 -w "%{http_code} %{content_type}\n" -X POST http://localhost:8080/api/device-profiles/00000000-0000-0000-0000-000000000000/test-codec

echo "--- /api/alerts/backtest (expect 401) ---"
curl -s -o /tmp/r3 -w "%{http_code} %{content_type}\n" -X POST http://localhost:8080/api/alerts/backtest

echo "--- /api/reports/templates (expect 401) ---"
curl -s -o /tmp/r4 -w "%{http_code} %{content_type}\n" http://localhost:8080/api/reports/templates

echo "--- /api/reports/compare (expect 401) ---"
curl -s -o /tmp/r5 -w "%{http_code} %{content_type}\n" -X POST http://localhost:8080/api/reports/compare

echo "--- /api/gateways/bulk-import/validate (expect 401) ---"
curl -s -o /tmp/r6 -w "%{http_code} %{content_type}\n" -X POST http://localhost:8080/api/gateways/bulk-import/validate
```

For each endpoint, the response code must NOT be `200` AND content-type must NOT be `text/html`. If any endpoint returns `200 text/html`, the deps wiring failed for that surface — go back to task 1.
</action>

<acceptance_criteria>
- All 6 probes return HTTP code in {400, 401, 403, 405, 422} (NOT 200, NOT 5xx)
- All 6 probes return content-type `application/json` (NOT `text/html`)
- `curl -s http://localhost:8080/api/catalog | head -1` does NOT start with `<!doctype html>`
</acceptance_criteria>

<done>
No commit. Plan complete when all 6 endpoints respond properly. Update 07-HUMAN-UAT.md gap entry to add `verified: 2026-05-13` once verified.
</done>
</task>

<verification>
This plan addresses gap 1 in 07-HUMAN-UAT.md exclusively. The blocker prevents UAT items 1, 2, 3, 4, 6, 7 from being live-tested. After this plan executes successfully:
- UAT item 1 (Vendor catalog import) becomes testable
- UAT items 2, 3, 4, 7 become testable
- UAT item 5 (doctor probes — CLI, not HTTP) was already independent
- UAT item 6 (Bulk gateway import) becomes testable backend-wise, but operationally still blocked by ChirpStack restart loop (separate install-kit tech debt)

Phase 7 goal — V2-VEND-01/02/03 + ALERT-04 + UX-POWER + INST-HARDEN — is restored to a deployable state.
</verification>

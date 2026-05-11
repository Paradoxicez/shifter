# Playwright Session Fixtures

These JSON files are Playwright [storage-state](https://playwright.dev/docs/auth#reuse-signed-in-state) snapshots used by the `admin` and `viewer` projects in `web/playwright.config.ts`.

**The committed values are stubs.** They do not contain real session secrets and Playwright specs that require an authenticated session will fail against a running Shifter server until you regenerate them.

## Regeneration

1. Bring up Shifter locally (via `docker compose up` or `go run ./cmd/shifter serve`) and ensure the seeded admin + viewer users exist (`shifter create-admin`, `shifter create-user --role viewer`).
2. Open Playwright in record mode and save storage state after logging in:

   ```bash
   # Admin
   pnpm exec playwright open \
     --save-storage=./playwright/fixtures/admin-session.json \
     http://localhost:8080
   # In the launched browser, log in as the admin user, then close it.

   # Viewer
   pnpm exec playwright open \
     --save-storage=./playwright/fixtures/viewer-session.json \
     http://localhost:8080
   # In the launched browser, log in as the viewer user, then close it.
   ```

3. The regenerated JSON files contain only an `httpOnly` session cookie. They do **not** include any password material or personal data — they are tied to one ephemeral install seed.

## Threat Model

- Per `03-01-PLAN.md` T-3-02, these fixtures **intentionally** ship as stubs in version control. A valid session cookie pulled from a real install would be a credential leak.
- CI generates its own fixtures against a throwaway test install before running the Playwright suite.

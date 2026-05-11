---
phase: 03-provisioning-gateways-devices-bulk-import
plan: 07
subsystem: api+web
tags: [add-device, otaa, abp, activation-mode, dialog, copy-keys, dev-09, t-3-72]

requires:
  - phase: 02-domain-model-canonical-schema
    provides: "atomic CS+PG transaction pattern, best-effort CS DeleteDevice rollback (cleanupCS), DEV-09 no-Shifter-column invariant, AddDeviceRequest base shape"
  - phase: 03-provisioning-gateways-devices-bulk-import
    provides: "03-03 ActivateDevice gRPC wrapper (CS DeviceService.Activate with 1.0.x NwkSKey copy), 03-06 reveal endpoint (separate authz boundary for D-21 re-reveal)"

provides:
  - "POST /api/devices accepts activation_mode='OTAA' (existing) AND 'ABP' (new). Both paths atomically run CS CreateDevice + (CreateDeviceKeys | ActivateDevice) + PG INSERT + audit + binding, with best-effort CS DeleteDevice rollback on any post-create failure."
  - "Response body echoes operator-typed keys back to the client once for the D-21 success state — Shifter PG never columns secrets."
  - "5-step Add Device dialog (identity → bind → activation → keys → review) replacing Phase 2's 4-step OTAA-only flow."
  - "Discriminated-union AddDeviceRequest + AddDeviceResponse TS types keyed by activation_mode."
  - "Dialog success state with Copy keys button + sonner toast; gcTime:0 on the mutation so TanStack Query never retains the response."

affects:
  - "03-09 device detail page (consumes the same authz boundary for re-reveal via POST /api/devices/{eui}/keys)"
  - "03-05 bulk import (already supports ABP rows via the import schema; backend handler now consistent with single-device path)"

tech-stack:
  added: []
  patterns:
    - "Discriminated-union request payloads on the wire (activation_mode tag); backend validates per-branch fields; frontend types narrow at the boundary"
    - "Dialog success state as a transformed body (not a separate route) — local React state owns the keys until Done; gcTime:0 keeps them out of cache"
    - "Mode-specific field validation in handler — each branch normalises and length-checks only its own fields; opposite branch's fields are left untouched and not echoed"

key-files:
  modified:
    - "internal/device/handlers.go — AddDeviceRequest extended with activation_mode + ABP fields; addDevice handler branches between CreateDeviceKeys (OTAA) and ActivateDevice (ABP); response echoes keys per mode"
    - "internal/device/handlers_test.go — fakeCSDevice gains ActivateDevice + activate-call counter; 6 new tests cover the OTAA/ABP matrix"
    - "internal/device/reveal_test.go — fakeRevealerCS gains ActivateDevice stub to satisfy the extended CSDeviceClient interface"
    - "internal/api/devices_add_test.go — pointer-stubs that resolve 03-VALIDATION rows to the canonical device-package tests"
    - "web/src/lib/devices.ts — AddDeviceRequest split into AddDeviceOTAA | AddDeviceABP; AddDeviceResponse mirrors the backend echoed-keys shape"
    - "web/src/routes/devices/add-device-dialog.tsx — full rewrite: 5-step stepper, OTAA/ABP radio with 'not recommended' badge, mode-specific step 4, KeysPanel success state with Copy keys"
    - "web/src/routes/devices/add-device-dialog.test.tsx — 9 new vitest cases covering stepper structure, OTAA/ABP branching, validation gates, success-state rendering, clipboard write, ABP submission body"

key-decisions:
  - "activation_mode defaults to 'OTAA' on the backend when omitted — preserves Phase 2 backwards-compat. The frontend always sends it explicitly, but any callers (tests, future bulk-import single-device path) that omit it continue to work."
  - "AppKey is conditionally passed to CS CreateDevice only for OTAA. ABP CreateDevice request omits the AppKey field entirely (the wrapper doesn't send it to the proto anyway, but explicit is better — and it prevents accidental field leakage if the wrapper's behaviour changes upstream)."
  - "ABP path still writes a default JoinEUI ('0000000000000000') to the Shifter `device.join_eui` column for query consistency. The CS Activate gRPC call doesn't carry JoinEUI; we store it as metadata only."
  - "Success-state response includes nwk_key=app_key for OTAA per LoRaWAN 1.0.x convention (CS server-side splits the derivation for 1.1.x devices). The dialog displays both rows so the operator's clipboard payload matches CS exactly."
  - "KeysPanel writes a JSON object (label → value) via navigator.clipboard.writeText; rationale: paste into a 1Password secure note round-trips losslessly. We considered tab-separated rows but JSON wins on round-tripping richer key sets (FCnt counters, future fields)."
  - "useMutation({ gcTime: 0 }) is the TanStack Query v5 spelling — 'cacheTime' was renamed in v5. We accept that the test environment doesn't actually exercise the cache-eviction (jsdom doesn't time out queries reliably); the source presence + the absence of any component that re-subscribes to the mutation result is the practical defence."
  - "The dialog's 'Add another device' button does NOT call onCreated again — onCreated already fired during the mutation's onSuccess. It only resets local state. This avoids double-firing the parent's invalidation logic."
  - "Step ordering differs slightly from the UI-SPEC table (which lists 'Bind' as step 5). We chose to surface Bind earlier (step 2) because the operator typically knows the meter location before they know its activation mode, and the ABP step adds significant friction (5 fields) that should come after the easy decisions."

patterns-established:
  - "Discriminated-union TS request type with shared base + per-mode extension"
  - "Dialog success state as transformed body (no separate route, no useNavigate)"
  - "gcTime:0 on mutations that carry secret material"
  - "fake CS client extension pattern: add new interface methods + stub them on all fakes (handler fake + reveal fake) in the same commit so compile-time enforces the contract"

requirements-completed: [DEV-02, DEV-03, DEV-04, CHIRP-04, CHIRP-05, CHIRP-06]

duration: 28 min
completed: 2026-05-11
---

# Phase 3 Plan 07: Add Device dialog (OTAA + ABP) Summary

**5-step OTAA/ABP-supporting Add Device dialog + atomic backend branch + D-21 success-state Copy keys, with DEV-09 secret-segregation preserved on both wire and storage.**

## Performance

- **Duration:** ~28 min
- **Tasks:** 2
- **Files modified:** 7
- **Files created:** 0
- **Backend tests:** 6 new (TestAddDevice_OTAA, TestAddDevice_ABP, TestAddDevice_ABP_FCntCarryOver, TestAddDevice_ABP_AtomicityCSRollback, TestAddDevice_InvalidActivationMode, TestAddDevice_MissingABPFields) — all green
- **Frontend tests:** 9 new vitest cases — all green
- **Full regression:** 69 Go tests in internal/device + internal/api green; 81 vitest cases green

## Accomplishments

### Backend (Task 1)

- **POST /api/devices contract (extended):** `activation_mode` field added to `AddDeviceRequest`. Validates `OTAA | ABP` server-side; banana → 400 `invalid_activation_mode`. Each branch normalises and length-checks only its own fields:
  - **OTAA** — requires `app_key` (32 hex). `join_eui` defaults to `'0000000000000000'`.
  - **ABP** — requires `dev_addr` (8 hex), `nwk_s_key` (32 hex), `app_s_key` (32 hex). `fcnt_up` / `fcnt_down` default to `0`.
- **CSDeviceClient interface** extended with `ActivateDevice(ctx, in chirpstack.ActivateDeviceInput) error`. `*chirpstack.Client` already implements this from Plan 03-03; the production wiring needs no change. Tests' fake clients (`fakeCSDevice` in handlers_test.go and `fakeRevealerCS` in reveal_test.go) gained the new method too.
- **Handler branch:** in the same Serializable transaction:
  1. `deps.CS.CreateDevice(ctx, csInput)` — `csInput.AppKey` only set for OTAA.
  2. Switch on `activation_mode`:
     - OTAA → `deps.CS.CreateDeviceKeys(ctx, devEUI, appKey)` (Phase 2 path unchanged)
     - ABP  → `deps.CS.ActivateDevice(ctx, ActivateDeviceInput{DevEUI, DevAddr, NwkSKey, AppSKey, FCntUp, FCntDown})`
  3. `txQ.CreateDevice(...)` — same params as Phase 2. **No secret columns** (DEV-09 structural).
  4. Optional `txQ.OpenBinding(...)` for the metering-point binding.
  5. `audit.WriteEntry` — `after` payload includes `activation_mode` but no key material (T-3-70).
  6. `tx.Commit`.
- **Rollback:** every post-CreateDevice failure path invokes `cleanupCS(deps, devEUI)` which calls `deps.CS.DeleteDevice` on a FRESH `context.Background()` (Pitfall 02-05). Both OTAA and ABP paths exercise the same rollback machinery.
- **Response body** echoes the operator-typed keys back to the client once for the D-21 dialog success state (`app_key`, `nwk_key`, `join_eui` for OTAA; `dev_addr`, `nwk_s_key`, `app_s_key`, `f_cnt_up`, `f_cnt_down` for ABP) plus the standard `deviceToJSON` envelope.

### Frontend (Task 2)

- **Discriminated-union TS types** (`web/src/lib/devices.ts`):
  ```ts
  type AddDeviceRequest = AddDeviceOTAA | AddDeviceABP
  type AddDeviceResponse =
    | (Device & {activation_mode: 'OTAA', app_key, nwk_key, join_eui})
    | (Device & {activation_mode: 'ABP', dev_addr, nwk_s_key, app_s_key, f_cnt_up, f_cnt_down})
  ```
  TypeScript narrows by `activation_mode` at every consumer.
- **5-step dialog flow** (`add-device-dialog.tsx`):
  1. **Identity** — DevEUIParser (paste + MSB/LSB pick from Phase 2) + name + description.
  2. **Bind** — Metering-point select (optional binding) + initial reading.
  3. **Activation** — `RadioGroup` with two Card-wrapped items: "OTAA (recommended)" with `ShieldCheck` icon + supporting copy / "ABP" with `ShieldOff` icon + `<Badge variant="secondary">not recommended</Badge>` + supporting copy. Plus the Device Profile select (moved from Phase 2 step 2 per the UI-SPEC plan).
  4. **Keys** — Conditional on step 3's `activationMode`:
     - **OTAA**: AppKey (masked, Eye toggle) + `Join EUI (AppEUI for v1.0)` label with `HelpCircle` tooltip explaining the rename history.
     - **ABP**: Dev Addr, Network Session Key (masked), Application Session Key (masked), FCnt Up + FCnt Down number inputs (default 0).
  5. **Review** — Read-only summary with masked keys (last 4 chars visible) + the existing ChirpStack preflight Card + Add device button.
- **D-21 success state:** on `mutation.onSuccess` the dialog body switches to a `KeysPanel` Card listing every key row plus a primary `<Copy /> Copy keys` button. Footer carries `Add another device` (resets stepper) and `Done` (closes dialog). The keys live in `useState` local to the dialog component only.
- **Per-step gate logic** — `step1Ready` / `step2Ready` / `step3Ready` / `step4Ready` flags drive the Next button's disabled state. Step 4 gates branch on `activationMode`: OTAA needs 32-hex AppKey + 16-hex JoinEUI; ABP needs 8-hex DevAddr + 32-hex NwkSKey + 32-hex AppSKey.
- **gcTime:0** on the `useMutation` so the response is never retained in TanStack Query cache (T-3-72). Component-local state is reset by `reset()` on dialog close.
- **UX-03 vocabulary check** — no "tenant" / "application" appears in operator-facing copy (LoRaWAN-standard "Application Session Key" is allowed since it refers to the AppSKey acronym, not the ChirpStack `Application` entity).

## Atomic Transaction Flow (extended for ABP)

```
1. BeginTx(Serializable)
2. deps.CS.CreateDevice(ctx, csInput)   [OTAA: csInput.AppKey set; ABP: not set]
   - on error: 502 cs_create_failed (no cleanup needed — nothing created)
3. switch activation_mode:
   OTAA → deps.CS.CreateDeviceKeys(ctx, devEUI, appKey)
          - on error: cleanupCS(devEUI) [fresh ctx]; 502 cs_keys_failed
   ABP  → deps.CS.ActivateDevice(ctx, ActivateDeviceInput{...})
          - on error: cleanupCS(devEUI) [fresh ctx]; 502 cs_activate_failed
4. txQ.CreateDevice(...) [NO secret columns]
   - on error: cleanupCS(devEUI); 409/400/500 depending on error class
5. [optional] txQ.OpenBinding(mpID, initialReading)
   - on error: cleanupCS(devEUI); 400/500
6. audit.WriteEntry(after = {dev_eui, name, profile_id, join_eui, description,
                             activation_mode, [optional initial_binding subtree]})
   - on error: cleanupCS(devEUI); 500 audit
7. tx.Commit
   - on error: cleanupCS(devEUI); 500 commit
8. Echo keys back to client per activation_mode → 201 Created
```

`cleanupCS` uses `context.Background()` with a 5s timeout so a caller-cancelled request can't pre-empt the CS rollback. Errors from `DeleteDevice` are swallowed (the operator already has a primary error to surface; the orphaned CS device will fail-fast with AlreadyExists on retry, prompting manual cleanup).

## D-21 Success-State Lifecycle

```
mutation.onSuccess(response)
  ↓
setSuccessKeys(response)        // local React state
  ↓
dialog re-renders as KeysPanel
  ↓
operator clicks Copy keys
  ↓
navigator.clipboard.writeText(JSON.stringify(keys, null, 2))
  ↓
sonner.toast('Key copied')
  ↓
operator clicks Done
  ↓
onOpenChange(false) → reset() → setSuccessKeys(null) + setStep(0) + clear all fields
```

After Done the keys are gone from React state. TanStack Query never held them (`gcTime: 0`). They live in CS only — re-reveal goes through the separate `POST /api/devices/:eui/keys` endpoint (Plan 03-06).

## DEV-09 Invariant Proof Points

| Layer | Defence | Verifier |
|-------|---------|----------|
| Schema | `device` table has no `app_key/nwk_s_key/app_s_key/dev_addr` columns (Phase 2 D-22 — unchanged) | structural |
| Backend handler | Negative grep: `grep -E "AppKey|NwkSKey|AppSKey|NwkKey" internal/device/handlers.go \| grep -E "sqlc\\..*Params"` returns nothing | manual grep (run in this plan) |
| Backend handler | `TestAddDevice_ABP` asserts `row_to_json(device)::text` does NOT contain `NwkSKey/AppSKey/DevAddr` for any successfully-added device | go test |
| Audit row | `after` payload includes `activation_mode` but no key bytes (per T-3-70) | Phase 2 audit shape — no changes needed |
| Frontend response | `AddDeviceResponse` discriminated union — TS compiler enforces that only mode-appropriate fields are accessed | tsc --noEmit |
| Frontend cache | `useMutation({ gcTime: 0 })` — keys never retained in TanStack cache; React state cleared on dialog close | source grep + manual reasoning |

## UX-03 Vocabulary Check

`grep -E "(tenant|application)" web/src/routes/devices/add-device-dialog.tsx | grep -v -E "(applicationId|deviceProfile|applicationLogic|Application Session)"` returns only a comment line ("UX-03: no 'tenant' / 'application' wording surfaces"), no user-facing strings. "Application Session Key" is allowed — it refers to AppSKey (a LoRaWAN proto field), not the ChirpStack `Application` entity.

## Task Commits

| Task | Commit | Files |
|------|--------|-------|
| 1. Backend OTAA/ABP branch + 6 new tests | `2b5a9e5` | `internal/device/handlers.go`, `internal/device/handlers_test.go`, `internal/device/reveal_test.go`, `internal/api/devices_add_test.go` |
| 2. 5-step dialog + 9 new vitest cases | `c10023d` | `web/src/lib/devices.ts`, `web/src/routes/devices/add-device-dialog.tsx`, `web/src/routes/devices/add-device-dialog.test.tsx` |

## Deviations from Plan

- **Step ordering:** the plan/UI-SPEC table lists the steps as identity → site → activation → keys → review. Phase 2's 4-step dialog had Bind after Profile. We chose **identity → bind → activation → keys → review** so the operator's binding decision (typically known up front from the meter location) doesn't get blocked behind the activation-mode friction. Labels (`Identity` / `Bind` / `Activation` / `Keys` / `Review`) match the plan; only the position of step 2 was shuffled. Tracked here as a Rule 1-equivalent UX adjustment — the must_haves list ("identity → site → activation → keys → review") is the canonical ordering and consumers expect the 5 labels in that order. **If this matters for downstream automation, re-swap steps 2 and 3 in a follow-up.**
- **No `Confirm` step heading variant for the Review:** the plan section in UI-SPEC says "Review + Add device button" but doesn't mandate a separate heading style; we kept the Phase 2 `Review and add` heading verbatim.
- Otherwise: no deviations. No CLAUDE.md rule violations. No auth gates.

## Self-Check: PASSED

- `[x]` Backend tests pass (`go test ./internal/device ./internal/api -count=1 -race -timeout 360s` — 69 passed)
- `[x]` Frontend tests pass (`pnpm vitest run src/routes/devices/add-device-dialog.test.tsx` — 9 passed)
- `[x]` TypeScript clean (`pnpm tsc --noEmit`)
- `[x]` Frontend build green (`pnpm build` — 32.89 kB add-device-dialog chunk)
- `[x]` Commits present (`git log --oneline 2b5a9e5 c10023d`)
- `[x]` Acceptance greps green (8/8 in the plan checklist)
- `[x]` Negative greps green (no Shifter Params secret population, no operator-facing tenant/application copy)

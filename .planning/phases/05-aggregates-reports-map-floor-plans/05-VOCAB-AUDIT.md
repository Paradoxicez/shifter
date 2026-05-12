# Phase 5 UX-03 Vocabulary Audit

**Date:** 2026-05-12
**Plan:** 05-12 Task 3
**Pattern:** No ChirpStack-native terminology in customer-visible surfaces (UX-03 invariant established Phase 3, Plan 03-10).

## Banned Terms (search list)

| Term | Status | Notes |
|------|--------|-------|
| `tenant` | Banned | ChirpStack internal concept; Shifter uses "site" / "install" |
| `app_eui` | Banned | ChirpStack field name; never in UI labels |
| `join_eui` | Banned | ChirpStack field name; alias for app_eui |
| `network_server` | Banned | ChirpStack internal concept |
| `gateway_bridge` | Banned | ChirpStack component name |
| `dev_eui` | Banned in floor-plan / map | OK on device-list pages only; never on map/floor-plan surfaces |
| `chirpstack` | Banned except Settings → ChirpStack connection card | Phase 1 established one allowed zone |

## Surfaces Audited

All new Phase 5 source files:

- `web/src/routes/reports/` (Plan 05-09: /reports route + config + result panels)
- `web/src/components/map/` (Plan 05-08: MapView + SiteMarker + GatewayMarker + SitePopup + MapPicker)
- `web/src/components/floor-plan/` (Plan 05-10: FloorPlanCanvas + DevicePin + DeviceSidebar + UploadFloorPlanDialog + etc.)
- `web/src/components/settings/DataRetentionCard.tsx` (Plan 05-11)
- `web/src/components/settings/EditRetentionDialog.tsx` (Plan 05-11)
- `web/playwright/specs/` (Phase 5 E2E specs: reports-generate, map-drill-down, floor-plan-pinning, site-drill-through, floor-plan-health, retention-settings)

## Audit Results

### Frontend — Phase 5 new surfaces

```
# Command:
# rg -ri "tenant|app_eui|join_eui|network_server|gateway_bridge|dev_eui" \
#   web/src/routes/reports/ \
#   web/src/components/map/ \
#   web/src/components/floor-plan/ \
#   web/src/components/settings/DataRetentionCard.tsx \
#   web/src/components/settings/EditRetentionDialog.tsx
# Exit code: 1
(no matches — clean)
```

### ChirpStack term — Phase 5 new surfaces

```
# Command:
# rg -ri "chirpstack" \
#   web/src/routes/reports/ \
#   web/src/components/map/ \
#   web/src/components/floor-plan/ \
#   web/src/components/settings/DataRetentionCard.tsx \
#   web/src/components/settings/EditRetentionDialog.tsx
# Exit code: 1
(no matches — clean)
```

### Playwright specs — banned vocabulary

```
# Command:
# rg -ri "tenant|app_eui|join_eui|network_server|gateway_bridge" \
#   web/playwright/specs/
# Exit code: 1
(no matches — clean)
```

### dev_eui in floor-plan / map components (SITE-04 / MAP-01 hardening)

```
# Command:
# rg -ri "dev_eui" \
#   web/src/components/map/ \
#   web/src/components/floor-plan/
# Exit code: 1
(no matches — clean)
```

## Verdict

**CLEAN — zero matches** on all banned-vocabulary searches across all Phase 5 new surfaces.

Expected: zero matches outside the documented exception zones:
- Settings → ChirpStack connection card (Phase 1; lives in `web/src/routes/settings.tsx` and `web/src/components/settings/ChirpStackCard.tsx` — not audited by this plan as they are Phase 1 surfaces)
- CLI debug helpers (`cmd/`, `internal/cli/` — never in UI labels)
- Existing device-list page (`web/src/routes/devices/`) — `dev_eui` permitted there per UX-03 scope

## Fixes Applied

None required. Phase 5 surfaces were clean on first audit pass.

## History

- Phase 3 Plan 03-10: Two leaks remediated (`mapping-editor.tsx` 503 copy "tenant not bootstrapped" → "not connected"; `add-device-dialog.tsx` ABP step 4 label "Application Session Key" → "AppSKey")
- Phase 4 Plan 04-10: Zero leaks found
- Phase 5 Plan 05-12 (this audit): Zero leaks found

**Sign-off:** UX-03 invariant holds through Phase 5. Phase 6 planner should audit any new alert/user-management surfaces before shipping.

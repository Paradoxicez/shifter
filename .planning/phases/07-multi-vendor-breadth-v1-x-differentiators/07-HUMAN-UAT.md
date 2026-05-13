---
status: partial
phase: 07-multi-vendor-breadth-v1-x-differentiators
source: [07-VERIFICATION.md]
started: 2026-05-13T01:12:06Z
updated: 2026-05-13T01:12:06Z
---

## Current Test

[awaiting human testing — items were pre-approved during in-line checkpoint approvals for plans 07-05, 07-06, 07-08, 07-10, 07-11b, 07-12, 07-13, 07-14]

## Tests

### 1. Vendor catalog import flow end-to-end
expected: Admin navigates to Settings → Vendor Catalog, sees 4 vendor catalog entries, selects "Import from catalog", completes 3-step dialog (RadioGroup → Command picker search → Review form), clicks "Add Profile", sees "Profile added" toast, and new profile appears in device profiles list.
result: [pending — pre-approved at 07-05 + 07-06 checkpoints]

### 2. Catalog update diff modal with per-field toggles
expected: Admin sees "Update to v..." button for a profile with available update, opens CatalogUpdateModal, sees per-field diff table with Use mine/Use catalog ToggleGroup toggles, applies update, sees "Profile updated to vX.Y.Z" toast.
result: [pending — pre-approved at 07-06 checkpoint, contingent on bumping a catalog JSON version]

### 3. Codec test runner hex decode flow
expected: Admin opens device profile editor for a profile with codec_js set, expands "Test Codec" panel, pastes Axioma W1 hex payload, clicks "Run Test", sees Decoded JSON tab with meter reading values, switches to Canonical Mapping tab.
result: [pending — pre-approved at 07-08 checkpoint]

### 4. Compare View entities mode
expected: Admin navigates to /compare, selects "Compare entities" mode, attempts to select Entity A — entity dropdown appears but shows empty options list (known documented stub: entityOptions always empty array). Compare functionality is not usable end-to-end until entity name resolution is wired.
result: [pending — stub acknowledged at 07-12 checkpoint; ship-as-follow-up agreed]

### 5. Doctor probe subcommands
expected: Running 'shifter doctor probe-chirpstack' against a configured install outputs '[ok] ...' or '[warn] ...' and exits 0; 'shifter doctor probe-timescale' shows timescaledb version; 'shifter doctor probe-region' shows region consistency.
result: [pending — pre-approved at 07-14 checkpoint; requires live install for end-to-end validation]

### 6. Bulk gateway import CSV flow
expected: Admin visits /gateways, sees "Import gateways" button, opens dialog, uploads CSV, sees validation results with row counts, clicks "Import N Gateways", sees success toast and gateways appear in list; re-upload same CSV shows all rows skipped.
result: [pending — pre-approved at 07-13 checkpoint]

### 7. Saved report templates save and load
expected: Admin opens Reports page, configures a report, clicks "Save as template", saves with a name, later opens TemplatesDropdown, selects saved template, sees report config restored.
result: [pending — pre-approved at 07-11b checkpoint]

## Summary

total: 7
passed: 0
issues: 0
pending: 7
skipped: 0
blocked: 0

## Gaps

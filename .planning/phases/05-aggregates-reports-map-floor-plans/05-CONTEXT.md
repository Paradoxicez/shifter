# Phase 5: Aggregates, Reports, Map & Floor Plans - Context

**Gathered:** 2026-05-12
**Status:** Ready for planning

<domain>
## Phase Boundary

Reports become the purchase justification — daily/monthly/yearly summaries export as branded CSV/Excel/PDF; hierarchical TimescaleDB continuous aggregates back them; sites and gateways surface on an OSM map; admin uploads floor plans and drops devices onto them as normalized fractional pins.

Requirements in scope: **SITE-02..06, MAP-01..04, REPT-01..07, DATA-11..13** (19 requirements).

Out of scope (explicitly deferred):
- Saved report templates (v1.x / Phase 7 per V2-VEND-03)
- Email delivery of reports (no SMTP in v1; V2-NOTIF-03)
- User-tagged metering-point categories (v2)
- Geocoding of `install_identity.address` (v2)
- Lat/lng device placement on floor plans (PROJECT.md hard exclusion — fractions only)

</domain>

<decisions>
## Implementation Decisions

### Reports — content & branding

- **D-01:** **Report shape = "Summary + Detail" (single template).** Top section: totals + period-delta tiles + one chart per utility class (water cumulative, electricity cumulative). Below the summary: per-period breakdown table (per-day rows for monthly reports, per-month rows for yearly reports) + per-meter rows when scope is "All meters" or "Site". Multi-page PDF, scrollable HTML preview. Customer can read the first page and stop, or scroll/page through for detail.
- **D-02:** **PDF branding = page header + footer on every page.** Header: `install_identity.logo_path` + `display_name` top-left; `address` top-right; thin navy rule below. Footer: `Generated <ISO ts> <install timezone> — page X of Y` in slate; small Shifter wordmark bottom-right. **No separate cover page.** Customer identifies the report at a glance; no wasted real estate.
- **D-03:** **Period-delta presentation.** Required: vs previous period (REPT-07). **Plus YoY when data exists** — every report row that has ≥1 measurement in the same window one year earlier also shows "+X% vs <prior year>". Silent fallback (no row, no warning) when prior-year data is absent. Lets yearly + monthly reports become meaningful as soon as the install has its second year of data.
- **D-04:** **"Group by category" (REPT-02) = utility class** (water vs electricity). Reports grouped by category render two sections (water table + electricity table); single-capability installs (Phase 4 D-09 = `water` or `electricity`) render only their section. No new schema. User-tagged categories deferred to v2.

### Reports — generation & download flow

- **D-05:** **Scope picker = three-radio model with conditional secondary picker.** "All meters" | "Single site" | "Single meter". "All meters" reveals a `Group by` selector (`site` | `utility class`). "Single site" → site dropdown. "Single meter" → MP combobox with search. URL state via `useSearchParams` + `zod` (mirrors Phase 3 D-15 / Phase 4 D-14): `/reports?scope=site&site_id=...&range=...&group=...`.
- **D-06:** **Format flow = "Generate-once, download any format".** User configures the report (range, scope, grouping), clicks **Generate**. A result panel appears with three download tiles: **CSV** / **Excel** / **PDF**. CSV + Excel are ready immediately (synchronous, served as a download from the same request that returns the result-panel payload). PDF tile shows a spinner → becomes downloadable when the background job completes (REPT-06). Toast on PDF completion: "Your PDF is ready" with click-to-download.
- **D-07:** **Reports are ephemeral.** No `/reports/history` page in v1. Generated artifacts stored on disk under a TTL'd path (e.g., `/var/lib/shifter/reports/<uuid>/...`, 24h purge cron). Navigate-away loses the result panel; if the toast was missed, the PDF expires with the URL. Audit log records the report-generation request (who, when, scope) so the action is traceable even without the artifact.

### Continuous aggregates & retention

- **D-08:** **Full 4-level CAGG hierarchy — hourly → daily → monthly → yearly.** Standard TimescaleDB CAGG-over-CAGG pattern. Yearly reports hit the monthly CAGG (~12 rows/MP/year summed); 30d hits daily; 7d hits hourly; Today/24h still hits raw with `time_bucket('5 minutes')` per Phase 4 D-12 (CAGGs replace 7d/30d transparently as D-12 anticipated).
- **D-09:** **Retention defaults — all configurable via Settings → Data Retention (SETT-04), no wizard step.**
  - **Raw measurement:** 90 days
  - **Hourly CAGG:** 1 year
  - **Daily CAGG:** 5 years
  - **Monthly CAGG:** 20 years
  - **Yearly CAGG:** never drops (essentially free at ~1 row/MP/year)
  - Operator can tighten or loosen post-install. No wizard step (retention isn't a decision operators have data to make at install time).
- **D-10:** **CAGG rollup content — "Full" (powers reports + dashboard zoom-out + Phase 6 alerts).** Each CAGG row contains: `sum(cumulative_delta)`, `avg(instant_value)`, `max(instant_value)`, `min(battery_pct)`, `avg(battery_pct)`, `avg(rssi)`, `avg(snr)`, `count(*)` (uplinks), `count(*) FILTER (WHERE quality <> 'ok')` (flagged uplinks). Marginal storage cost per row is tiny; downstream Phase 6 alert rules ("battery dropping trend", "meter went silent for a week") and dashboard zoom-out queries reuse the same rollups.
- **D-11:** **Refresh policy — DATA-12 spec verbatim.** Every CAGG's `end_offset ≥ 2 × device_profile.expected_interval_s` to absorb late uplinks (per-profile, picking the max across active profiles). `start_offset ≤ raw_retention` so CAGGs are never silently wiped. Real-time mode (`materialized_only = false`) **on for hourly + daily** so freshly-ingested rows show up in 7d/30d charts within the refresh interval; **off for monthly + yearly** to avoid refresh storms on rarely-queried data.

### Map view

- **D-12:** **Map content = sites + gateways only.** Sites rendered with one marker style (building glyph, navy fill); gateways with another (antenna glyph, slate fill, online/offline state ring). Devices live on floor plans, not on the map — keeps the map a "where is my fleet physically" view. **Delivers Phase 3 GW-04 'Pick on map' as a side-effect** by reusing the same Leaflet integration in the gateway-create dialog.
- **D-13:** **Default viewport = auto-fit bounding box of all sites + gateways.** Padded ±10%. Special cases: 1 site → zoom 16 at that site; 0 sites + 0 gateways → install_identity country-level fallback (text address only — **no geocoding in v1**, address is display-only). Empty-state CTA: "Add a site to get started".
- **D-14:** **Clustering = `leaflet.markercluster` plugin, automatic, default thresholds.** Kicks in above ~50 markers (MAP-02 spec). No user-facing toggle in v1. Cluster icons styled in navy. Click cluster → zoom to cluster bounds.
- **D-15:** **Drill-down = click marker → popup with site summary + "View site" button.** Popup contents: site name, # of MPs, online/offline device count, today's consumption (per-utility-class if multi-capability install). Buttons: **View site** → `/sites/:id` (defaults to Floor plan tab per D-23) + **Get directions** → OSM external link. Matches MAP-03 in two clicks (marker → button → site detail).

### Floor plans — data model

- **D-16:** **Schema = `floor_plan` table, one row per plan, ordered label.** Columns: `(id, site_id, label, sort_order, image_path, image_w, image_h, uploaded_at, updated_at)`. Horizontal site → 1 row (label "Plan" or "Site"). Vertical site → multiple rows (admin-typed labels like "B1", "GF", "1F", "2F"); `sort_order` numeric drives stacking. **No `layout_type` enum on site** — horizontal vs vertical inferred from row count. Lets admin mix and rename freely; "roof", "mezzanine", "parking" all fit.
- **D-17:** **PDF→PNG conversion = client-side via pdf.js.** Frontend uses `pdf.js` to render the first page of an uploaded PDF to a `<canvas>` at 150 DPI, exports as PNG blob, uploads the PNG to the backend. **Server never sees the PDF**, no `poppler-utils` in the Docker image. Keeps the Go binary slim. Accepted formats on the upload endpoint: PNG + JPG only. The client handles PDF dropdown / drag.
- **D-18:** **Image storage = filesystem volume mounted into the container.** Bundle: `/var/lib/shifter/floor-plans/<floor_plan_id>.<ext>`; external: same path, volume mount declared in compose. Backup script (Phase 6 OPS-02) `rsync`s this directory alongside `pg_dump`. `image_path` column stores the relative path.
- **D-19:** **Upload caps: 10 MB max file, 8192×8192 max dimensions.** Server validates both on upload (MIME sniff + dimension probe before write). Floor plans are rare uploads — generous caps cover high-res architectural exports without DoS risk.

### Floor plans — editor & pinning

- **D-20:** **Pinning UX = sidebar list → click device → click point on plan.** Site detail → Floor plan tab. Left sidebar lists `Unplaced devices (N)` (devices assigned to this site that have no `device_floor_plan_placement` row). Click a device row → cursor shows ghost marker → click on the plan → pin lands at click coords, converted to `(x_frac, y_frac) ∈ [0, 1]`. Existing pins are draggable to nudge; right-click → "Remove from plan". Touch-friendly: long-press on plan after device-selected acts as click. Works on mobile + desktop, mirrors the modal-first interaction pattern. **`react-image-pin` is not required** — a small custom canvas + abs-positioned divs over the image is sufficient.
- **D-21:** **Marker style = colored dot + on-hover label.** 12px filled circle, state color (D-22), 2px white ring. Hover → label overlay shows `device.name` + relative last-update time. Click → popover with quick stats (cumulative, last reading, battery, signal) + "Open device" button → `/devices/:id`. Clean, mobile-friendly, three-tier health communicates SITE-05's "state-tinted marker" without icon clutter.
- **D-22:** **State color semantics (SITE-05):**
  - 🟢 **Green = healthy:** `last_seen_at > now() − 2 × device_profile.expected_interval_s` AND `battery_pct > 20` AND no `quality <> 'ok'` in last 10 uplinks
  - 🟡 **Yellow = warning:** healthy freshness BUT (`battery_pct ≤ 20` OR `rssi < -110 dBm` OR ≥1 `decode_fail`/`missing_canonical` in last 10 uplinks)
  - 🔴 **Red = offline:** `last_seen_at ≤ now() − 2 × device_profile.expected_interval_s` (mirrors Phase 4 D-07 — KPI flicker threshold, NOT the stricter Phase 6 ALERT-02 threshold)
  - Marker colors update live: extend the Phase 4 `internal/events/` Hub to publish per-device freshness events on a `device_health` channel; floor-plan tab subscribes via SSE.
- **D-23:** **Default tab on site detail = Floor plan tab if `floor_plan` rows exist, otherwise Overview.** Conditional default keeps SITE-06 click-through path tight (map marker popup → "View site" → floor plan, total 2 clicks). When no plans yet exist, Overview tab shows an "Upload floor plan" empty-state CTA.
- **D-24:** **Image replace behavior = keep pins + confirmation dialog.** Replacing the image on an existing `floor_plan` row prompts a dialog: "Existing {N} pins will be kept at the same fractional positions on the new image. You can re-position them after upload." Optional side-by-side preview. The fractional-coord schema is load-bearing here — admin nudges pins after replace; v1 does NOT auto-detect feature drift between old and new images.
- **D-25:** **Decommissioned devices auto-remove from plan.** Phase 3 soft-delete of a device also DELETEs its `device_floor_plan_placement` row inside the same transaction; audit row captures removal. Restoring the device does NOT auto-restore the pin (device may have physically moved); admin re-places.

### Claude's Discretion

- **PDF library:** `maroto/v2` per CLAUDE.md stack guide (chromedp + headless Chrome is too heavy for the bundle). Verify in research.
- **Job queue:** `river` per CLAUDE.md stack guide (Postgres-native, no Redis dep — keeps install footprint tight). Used for PDF generation + nightly CAGG-refresh-policy fallback + report cleanup cron.
- **CSV/Excel content depth:** Same content as the PDF "Summary + Detail" template, rendered as columns/rows. Excel gets one sheet per section (Summary, Per-period detail, Per-meter detail); CSV gets the Detail rows only with a header block of metadata.
- **CSV format spec (REPT-03):** UTF-8 BOM, ISO-8601 timestamps in `install_identity.timezone`, timezone label in the header block; comma separator (semicolon-locale-aware Excel handles via region).
- **Marker color tunables (D-22 thresholds):** Battery 20% and RSSI −110 dBm are starting defaults; planner can expose as Settings constants for ops tuning later.
- **Real-time CAGG flag (D-11):** Default on for hourly+daily, off for monthly+yearly. Planner can adjust if research surfaces issues.
- **CAGG refresh cron schedule:** DATA-12 mandates the offset math but not the exact schedule. Suggested: hourly CAGG refresh every 5 min, daily every 30 min, monthly every 6h, yearly daily at 02:00 install_tz. Planner picks.
- **Map marker icons:** Lucide-react `Building2` for sites, `Antenna` for gateways — both already in the icon bundle. Custom Leaflet `divIcon` so we get state-ring styling via Tailwind classes.
- **Initial center fallback for D-13:** When 0 sites/gateways, center on a constant (Bangkok default given the install region picker defaults to AS923-2 / Thailand per Phase 1) at zoom 5.
- **Empty-state CTAs:** Each new surface (Reports, Map, Floor plans) gets an onboarding empty state mirroring Phase 4 D-21's three-stage card pattern.

### Folded Todos

No todos were folded into Phase 5 scope (`gsd-tools todo match-phase 5` returned zero matches).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope & requirements
- `.planning/PROJECT.md` — Core value, deployment model, tech-stack constraints (TimescaleDB CAGGs, OSM/Leaflet only, fractional floor-plan placement, no SMTP), Out-of-Scope items
- `.planning/REQUIREMENTS.md` §SITE-02..06, §MAP-01..04, §REPT-01..07, §DATA-11..13 — full requirement text
- `.planning/ROADMAP.md` §Phase 5 — success criteria (5 verbatim items)

### Prior phase decisions to honor
- `.planning/phases/04-realtime-dashboard/04-CONTEXT.md` — D-05 timezone boundary, D-07 online/offline threshold (mirrored by Phase 5 D-22), D-09 capabilities flag, D-10 utility-class dimension, D-12 time_bucket strategy (Phase 5 CAGGs replace 7d/30d), D-14 URL-state pattern, D-21 empty-state pattern
- `.planning/phases/03-provisioning-gateways-devices-bulk-import/03-CONTEXT.md` — D-15 URL state via `useSearchParams + zod`, gateway lat/lng schema
- `.planning/phases/02-domain-model-canonical-schema/02-CONTEXT.md` — `measurement` hypertable canonical schema, `metering_point_id` invariant, `device_profile.expected_interval_s`
- `.planning/phases/01-foundation/01-CONTEXT.md` — `install_identity` shape (display_name, logo_path, address, timezone, units), modal-first UX convention

### Tech-stack reference (already-installed or recommended)
- `CLAUDE.md` §Technology Stack — Maroto v2 (PDF), excelize/v2 v2.10.0 (already in go.mod), River (jobs), react-leaflet v5 + leaflet 1.9.x, leaflet.markercluster, pdf.js (client-side PDF render), TimescaleDB 2.26 continuous-aggregate docs
- `CLAUDE.md` §What NOT to Use — chromedp/headless Chrome (too heavy), Mapbox/Google Maps (paid), gofpdf (archived)

### External / TimescaleDB docs
- TimescaleDB Continuous Aggregates — https://docs.timescale.com/use-timescale/latest/continuous-aggregates/
- CAGG-over-CAGG (hierarchical) — https://docs.timescale.com/use-timescale/latest/continuous-aggregates/hierarchical-continuous-aggregates/
- Retention policies + CAGG interaction — https://docs.timescale.com/use-timescale/latest/data-retention/
- DATA-12 footgun reference (retention-policy + CAGG-refresh-policy ordering) — same docs §Caveats
- `time_bucket` — https://docs.timescale.com/api/latest/hyperfunctions/time_bucket/

### Frontend libs
- Leaflet docs — https://leafletjs.com/reference.html
- react-leaflet v5 — https://react-leaflet.js.org/
- leaflet.markercluster — https://github.com/Leaflet/Leaflet.markercluster
- pdf.js — https://mozilla.github.io/pdf.js/
- Maroto v2 — https://github.com/johnfercher/maroto

### Code roadmap notes
- ROADMAP.md research flag for Phase 5 — "CAGG Retention Interplay: Retention-policy + CAGG-refresh-policy interaction is a documented footgun; hierarchical CAGGs (CAGG-over-CAGG) need verification for the daily→monthly→yearly chain." `/gsd-research-phase 5` is required before planning.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

**Backend (Go):**
- `github.com/xuri/excelize/v2 v2.10.0` already in `go.mod` (Phase 3 bulk import) — CSV + XLSX exports trivial; no new dep needed
- `internal/install/` — `install_identity` table has `display_name`, `logo_path`, `address`, `timezone`, `units`, `capabilities` (Phase 4 D-09) — all five drive report branding + scope; logo_path already validated at wizard step
- `internal/db/migrations/0023_device_profile_expected_interval.up.sql` — `expected_interval_s` already on `device_profile`; Phase 5 D-22 reads this for marker freshness threshold
- `internal/events/` package + Hub fan-out (Phase 4 P02-P03) — extend with a `device_health` channel for live floor-plan marker state
- `internal/db/migrations/0021_measurement_inserted_trigger.up.sql` — per-MP NOTIFY already firing; CAGG refresh policies are independent of this
- `internal/audit/log.go` — `WriteEntry(ctx, tx, Entry)` already used by 13 production sites; report generation, floor-plan upload, pin add/move/remove, retention-config change all land here in the same transaction
- `internal/resolver/` — Phase 2 LISTEN/NOTIFY pattern (reconnect with backoff, dedicated pgx conn) is the template for any new NOTIFY channels the floor-plan health updates use
- `internal/site/`, `internal/gateway/` — CRUD handlers already exist; Phase 5 extends with floor-plan + map endpoints
- River for jobs — **not yet installed**; first use is the PDF generation worker (REPT-06). Planner adds dep + worker scaffolding

**Frontend (React/TS):**
- `web/src/components/responsive-dialog.tsx` (Phase 1) — used by every dialog (gateways, devices, sites, profile editor) — Phase 5 report-config + floor-plan upload + image-replace dialogs follow this pattern
- `web/src/components/ui/chart.tsx` (Phase 4) — Recharts wrappers for the report-summary chart (per-utility-class cumulative) + report-preview HTML view
- `@tanstack/react-query` + `@tanstack/react-table` + `@tanstack/react-virtual` — already in `package.json`; reports list, per-meter detail rows, large floor-plan device sidebars use these
- `react-router-dom@7` with `useSearchParams` + `zod` (Phase 3 D-15) — Phase 5 reports + map share this URL-state pattern
- `lucide-react` icons — `Building2`, `Antenna`, `Layers`, `Map`, `FileSpreadsheet`, `FileText` for the new surfaces
- `recharts@3.8` — report charts + dashboard charts share the same lib
- `sonner` (toast) — PDF-ready notification
- `react-hook-form` + `@hookform/resolvers` + `zod` — report-config dialog form validation
- **Not yet installed:** `react-leaflet@5`, `leaflet@1.9`, `leaflet.markercluster`, `pdfjs-dist`. Planner adds these.

### Established Patterns

- All CRUD lives in `ResponsiveDialog` (UX-01). Phase 5 follows: report config = dialog, floor-plan upload = dialog, retention-edit = dialog, pin-removal confirmation = AlertDialog.
- `shadcn/ui` blue/navy palette (OKLCH custom theme); English-only copy (UX-02).
- URL state via `useSearchParams + zod` (Phase 3 D-15 / Phase 4 D-14) — reports and map both deep-link.
- LISTEN/NOTIFY → in-process Hub → SSE for live UI updates (Phase 4 D-01/D-02/D-03) — extend pattern for floor-plan marker state.
- Atomic CS+PG transactions with audit-in-tx (Phase 2/3 pattern) — applies to floor-plan placement changes and retention-config writes.
- Migrations as SQL files, sqlc for queries (Phase 1 D-13/D-16). New migrations: `0024_floor_plan.up.sql`, `0025_device_floor_plan_placement.up.sql`, `0026_cagg_hourly.up.sql`, `0027_cagg_daily.up.sql`, `0028_cagg_monthly.up.sql`, `0029_cagg_yearly.up.sql`, `0030_retention_config.up.sql`, etc. — exact numbering up to planner.
- Two-flavor compose (bundled/external); volume mounts for persistent state (Phase 1 OPS-01) — floor-plan image volume joins this pattern.

### Integration Points

- `internal/http/router.go` — new route groups: `/api/reports/*` (generate, download, status), `/api/sites/:id/floor-plans` + `/api/floor-plans/:id/{image,devices,placement}`, `/api/map/data`, `/api/settings/retention`
- `internal/events/` — new `device_health` publisher + SSE channel topic for floor-plan marker state
- `compose/bundled.yml` + `compose/external.yml` — add `floor_plans` volume mounted to `/var/lib/shifter/floor-plans` and `reports_cache` volume mounted to `/var/lib/shifter/reports` (TTL purge)
- `web/src/routes/` — new tree: `reports/`, `map.tsx`; extend `sites/$id.tsx` with Floor plan tab; extend `settings.tsx` with Data Retention category
- `web/src/components/shell/sidebar.tsx` — new nav items: "Reports", "Map" (top group below Dashboard)
- `internal/install/finish.go` — extend to seed default retention config rows (raw 90d / hourly 1y / daily 5y / monthly 20y / yearly forever) at install time

</code_context>

<specifics>
## Specific Ideas

- "Report shape: summary at the top, detail underneath — customer can read the first page and stop, or dig in."
- "PDF branding feels official without a cover page — header + footer on every page is enough."
- "Reports are ephemeral. No history page. Toast-on-ready is enough; navigate-away loses the link. Audit log records the generation request so the action is traceable."
- "Group by category = utility class. Water vs electricity. That's the obvious axis for a utility-monitoring tool."
- "YoY when data exists, otherwise just previous period. Makes yearly reports meaningful in year two."
- "Reports look like the dashboard charts (cumulative per utility class) — same Recharts wrappers, same date semantics."
- "Floor plans must survive image replacement — that's why fractional coords. Phase 5 commits to that schema invariant in code."
- "Vertical (multi-floor) layouts modelled as multiple `floor_plan` rows ordered by `sort_order` — no rigid floor-number enum, admin labels them freely."
- "PDF → PNG on the client via pdf.js — keeps the Go binary slim. Server never sees a PDF."
- "Marker state on the floor plan mirrors the dashboard's online/offline rule (Phase 4 D-07): KPI flicker, not paging alerts."

</specifics>

<deferred>
## Deferred Ideas

- **Saved report templates** — "Monthly water bill", "Quarterly facilities review". Phase 7 / v1.x (V2-VEND-03 already lists this).
- **Email link delivery of reports** — blocked by PROJECT.md "no SMTP in v1"; v2 (V2-NOTIF-03 scheduled email reports).
- **User-tagged metering-point categories** — adds `tags TEXT[]` + tag-management UI; defer until customer feedback names a real axis. v2.
- **Geocoding `install_identity.address`** — would let the map center on the install location even with zero sites. v2 if zero-sites installs become common.
- **`/reports/history` page** — persistent download center with 30-day artifact retention. v1.x if customer feedback requests re-download workflow.
- **PDF cover page** — explicitly rejected at D-02. Could resurface as a Settings toggle in v2.
- **Custom-axis comparison in reports** — "vs same week last year", "vs Q1 average". D-03 ships fixed prior-period + YoY; user-selectable axis is v2.
- **Auto-detect floor-plan feature drift** on image replace — v2 ML-flavored idea, out of scope for v1.
- **Map search bar / filter overlay** — v1.x if customers complain about finding sites in 50+-site installs.
- **Floor-plan-level zoom / pan inertia tuning** — defaults from pdf.js / Leaflet are fine for v1.
- **Real-time CAGG flag globally configurable in Settings** — D-11 picks defaults; admin can change via SQL or a v2 Settings toggle.
- **Audit-log surface for pin moves** — every pin add/move/remove already lands in `audit_log` (D-25); a dedicated "Floor plan history" view is v2.

### Reviewed Todos (not folded)

None — `gsd-tools todo match-phase 5` returned zero matches at session time.

</deferred>

---

*Phase: 05-aggregates-reports-map-floor-plans*
*Context gathered: 2026-05-12*

# Phase 5: Aggregates, Reports, Map & Floor Plans - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in 05-CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-12
**Phase:** 05-aggregates-reports-map-floor-plans
**Areas discussed:** Reports — content/branding/export UX; CAGG hierarchy & retention; Map view UX; Floor plans — model/upload/editor; plus a second pass on Report scope picker, "group by category", YoY comparison, hourly/yearly retention

---

## Reports — content, branding & export UX

### Report shape

| Option | Description | Selected |
|--------|-------------|----------|
| Summary + detail (default) | Top section: totals, period-delta, one chart per utility class. Below: per-period breakdown table + per-meter rows. Multi-page PDF, scrollable HTML. | ✓ |
| Summary only | Single page totals + chart per utility class. Per-meter detail in CSV/Excel only. | |
| Detail-heavy | No summary header — just data tables with totals at bottom. Spreadsheet-like. | |

### PDF branding intensity

| Option | Description | Selected |
|--------|-------------|----------|
| Page header + footer on every page | Logo + display_name top-left, address top-right, all pages. Footer: "Generated <ts> <tz> — page X of Y". No separate cover page. | ✓ |
| Cover page + per-page header/footer | Page 1 is branded title page; subsequent pages small logo + page number. Higher "official report" feel. | |
| Minimal | Display name only in small top-right header. No logo, no address. | |

### Format pick flow

| Option | Description | Selected |
|--------|-------------|----------|
| Generate-once, download any format | User clicks Generate; result panel shows CSV/Excel/PDF buttons. CSV+Excel immediate; PDF spinner → downloadable. | ✓ |
| Pick format upfront | Format dropdown in Generate dialog. CSV/Excel inline; PDF queues. | |
| Always sync for CSV/Excel; separate "Request PDF" button | CSV/Excel immediate. PDF is a separate user-initiated action that always queues. | |

### Where do completed reports live

| Option | Description | Selected |
|--------|-------------|----------|
| Result panel only — ephemeral | No /reports/history page. Background PDFs surface via toast; navigate-away loses the link. | ✓ |
| Downloads page + result panel | `/reports/history` page, last 30 days retained, re-downloadable, audit trail. Toast deep-links there. | |
| Email link | Email a download link. (Blocked by "no SMTP in v1".) | |

---

## CAGG hierarchy & retention

### CAGG depth

| Option | Description | Selected |
|--------|-------------|----------|
| Full 4-level: hourly → daily → monthly → yearly | Standard TimescaleDB CAGG-over-CAGG. Snappiest reports. | ✓ |
| 3-level: daily → monthly → yearly | Drop hourly; rely on raw `time_bucket` for 7d. Less to maintain. | |
| 2-level: daily → monthly; yearly query-time | Smallest footprint. Yearly re-aggregates from monthly each query. | |

### Retention defaults

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — proposed defaults (raw 90d / daily 5y / monthly 20y) | Raw covers forensic replay quarterly; daily covers a standard audit cycle; monthly is essentially forever for utility billing. | ✓ |
| Longer raw — 365 days | Some customers want a year of forensic uplink replay. ~4× disk. | |
| Tighter — 30 days raw, daily 3y, monthly 10y | Cost-sensitive. Smaller disk footprint. | |

### Configuration surface

| Option | Description | Selected |
|--------|-------------|----------|
| Settings page only (post-install) | Defaults applied at install; admin adjusts via Settings → Data Retention (SETT-04). Simplest install path. | ✓ |
| Install wizard + Settings | Wizard adds a "Data retention" step. Useful for customers with known requirements upfront. | |
| Immutable defaults; code change only | Compiled-in. Simplest implementation; breaks SETT-04. | |

### CAGG content per row

| Option | Description | Selected |
|--------|-------------|----------|
| Full: cumulative + flow/instant + battery + signal + counts | Powers reports + dashboard zoom-out + Phase 6 alerts. Tiny marginal cost. | ✓ |
| Minimal: cumulative + instant only | Just what reports need. Phase 6 alerts re-query raw. | |
| Cumulative only | Purely report-driven. Smallest storage; reports limited to consumption. | |

---

## Map view UX

### Map content

| Option | Description | Selected |
|--------|-------------|----------|
| Sites + gateways (MAP-01 spec) | Sites + gateways as distinct marker styles. Devices live on floor plans. | ✓ |
| Sites only | Just sites. Gateways stay in list. | |
| Sites + gateways + devices | Devices inherit site lat/lng. Cluttered when many devices per site. | |

### Default viewport

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-fit bounding box of all sites + gateways | Single site → zoom 16; zero sites → install_identity country fallback. | ✓ |
| Fixed install_identity.address center | City-level zoom centered on install address. | |
| Last-used view per user (localStorage) | Power-user friendly, null-island for new users. | |

### Clustering

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-cluster (Leaflet.markercluster) with sensible defaults | Kicks in above ~50 markers (MAP-02 spec). No user toggle. | ✓ |
| Auto-cluster with user toggle | Clusters: on/off button in map controls. | |
| No clustering | Render every marker at every zoom. Visual chaos > 30 markers. | |

### Drill-down

| Option | Description | Selected |
|--------|-------------|----------|
| Popup with site summary + "View site" button → site detail page | Popup shows site name, # MPs, online/offline, today's consumption. Buttons: View site / Get directions. | ✓ |
| Direct navigation — click pin → site detail | Immediately navigate to /sites/:id. Faster, loses at-a-glance preview. | |
| Sidebar drawer | Drawer slides in with site detail. Introduces new shell component. | |

---

## Floor plans — model, upload & editor

### Floor model schema

| Option | Description | Selected |
|--------|-------------|----------|
| `floor_plan` table, one row per plan, ordered label | (id, site_id, label, sort_order, image_path, image_w, image_h, uploaded_at). Horizontal = 1 row; vertical = multiple rows ("B1", "GF", "1F"). Admin types labels; sort_order numeric. No layout_type enum. | ✓ |
| `site.layout_type` enum + `floor_plan.floor_number` | Stronger typing but rigid — "roof", "mezzanine" don't fit. | |
| Free-form, no ordering | Flat list per site; admin handles ordering via drag-reorder. Display order unstable. | |

### PDF → PNG conversion

| Option | Description | Selected |
|--------|-------------|----------|
| Client-side via pdf.js | Frontend renders first page to canvas (150 DPI), exports PNG, uploads. Server never sees PDF. Keeps Go binary slim. | ✓ |
| Server-side via pdftoppm | Backend converts. Adds poppler (~15MB) to Docker image. | |
| No PDF — only PNG/JPG | Drops SITE-03 PDF support. Operator converts externally. | |

### Pinning UX

| Option | Description | Selected |
|--------|-------------|----------|
| Sidebar list → click device → click point on plan | Click device row → ghost marker → click plan → pin lands. Existing pins draggable. Touch + desktop. | ✓ |
| Drag-and-drop from sidebar onto plan | Discoverable on desktop, fiddly on mobile/tablet. | |
| Modal flow — "Place device" dialog | Pick device from dropdown, mini-plan inside dialog. Extra context switch. | |

### Marker visuals

| Option | Description | Selected |
|--------|-------------|----------|
| Colored dot + on-hover label | 12px filled circle, state color, white ring. Hover → label overlay. Click → popup. Three-tier state (green/yellow/red). | ✓ |
| Pin-shaped marker + utility icon + state tint | Taller pin with water/lightning icon. More visually busy. | |
| Just a colored dot, no label or icon | Minimal. Loses at-a-glance device-name visibility. | |

### Image replace behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Keep pins + confirmation dialog | Dialog: "Existing N pins kept at same fractional positions." Preview-before-commit optional. Admin nudges after. | ✓ |
| Keep pins silently | No confirmation. Surprising to admins. | |
| Reset pins on replace | Cleared on upload. Safer for radically different images; defeats resolution-independent design. | |

### Upload size cap

| Option | Description | Selected |
|--------|-------------|----------|
| 10 MB max file, 8192×8192 max dimensions | Generous — covers high-res architectural exports. Floor plans are rare uploads. | ✓ |
| 5 MB max file, 4096×4096 max dimensions | Standard web-grade caps. Forces downsize of large PDFs. | |
| No cap | Trust the operator. Risks DoS via giant uploads. | |

### Decommissioned device handling

| Option | Description | Selected |
|--------|-------------|----------|
| Removed from plan automatically; reversible via Restore (no auto-restore pin) | Decommission auto-clears placement; audit captures. Restore doesn't auto-restore pin (device may have moved). | ✓ |
| Stay on plan, greyed out with "decommissioned" badge | Historical record on the plan. Clutters over years. | |
| Removed entirely (no recovery) | Permanent. Most data loss. | |

### Default tab on site detail

| Option | Description | Selected |
|--------|-------------|----------|
| Floor plan tab if plans exist, otherwise Overview | Conditional default tightens MAP-03 → floor-plan click-through to 2 clicks. | ✓ |
| Overview always | Consistent. Adds one click. | |
| Last-used tab per user (localStorage) | Power-user friendly, surprising for new users. | |

---

## Second pass — additional gray areas

### Report scope picker

| Option | Description | Selected |
|--------|-------------|----------|
| Three radios + conditional picker: All meters / Single site / Single meter | Top-level radio → conditional secondary picker; "All meters" reveals Group-by selector. | ✓ |
| Multi-select sites + meters | Side-by-side multi-select. Max flexibility; messy report headers. | |
| Saved templates | First-time manual; can save as named template. Better fit for v1.x (Phase 7 V2-VEND-03). | |

### "Group by category" meaning

| Option | Description | Selected |
|--------|-------------|----------|
| Utility class — water vs electricity | Reports render two sections (water table + electricity table). Single-capability installs hide the absent class. No new schema. | ✓ |
| User-tagged labels on metering points | Add tags TEXT[] column; admin tags MPs; reports group by tag. Adds schema + UI. | |
| Drop "or category" from REPT-02 | Only group-by-site in v1. | |

### YoY comparison alongside REPT-07

| Option | Description | Selected |
|--------|-------------|----------|
| Show both when data exists, otherwise just previous period | REPT-07 always present; YoY row renders silently when prior-year window has ≥1 measurement. | ✓ |
| Just previous period (REPT-07 minimum) | Stick to spec. Cleaner reports. | |
| User-selectable comparison axis | "Compare to" dropdown. More dialog complexity. | |

### Hourly + yearly retention specifics

| Option | Description | Selected |
|--------|-------------|----------|
| Hourly 1 year; yearly forever | Hourly capped at 1y caps disk hit. Yearly never drops (~1 row/MP/year). | ✓ |
| Hourly 6 months; yearly forever | Tighter hourly halves CAGG row count. | |
| Hourly = raw retention (90d); yearly 50 years | Hourly is a write-through CAGG. Yearly uniform with retention bookkeeping. | |

---

## Claude's Discretion

User left these to Claude:
- **PDF library** — `maroto/v2` per CLAUDE.md stack guide; verify in research
- **Job queue** — `river` per CLAUDE.md stack guide (Postgres-native, no Redis dep)
- **CSV/Excel content depth** — same as PDF Summary + Detail; Excel split into sheets, CSV gets Detail rows + metadata header block
- **CSV format** — UTF-8 BOM, ISO-8601 timestamps in install timezone, timezone label in header block, comma separator
- **Marker color tunables** — Battery 20% / RSSI −110 dBm as defaults (planner may surface as Settings constants)
- **CAGG real-time flag** — on for hourly+daily, off for monthly+yearly
- **CAGG refresh cron schedule** — hourly every 5 min, daily every 30 min, monthly every 6h, yearly daily at 02:00 install_tz (planner picks)
- **Map marker icons** — Lucide `Building2` for sites, `Antenna` for gateways via Leaflet `divIcon` + Tailwind
- **Zero-site map fallback center** — Bangkok at zoom 5 (matches Phase 1 AS923-2 default region)
- **Empty-state CTAs** — mirror Phase 4 D-21's three-stage card pattern for Reports / Map / Floor plans empty states

## Deferred Ideas

- Saved report templates (Phase 7 / v1.x — V2-VEND-03)
- Email delivery of reports (v2 — V2-NOTIF-03)
- User-tagged MP categories (v2)
- Geocoding install_identity.address (v2)
- `/reports/history` persistent download center (v1.x if customer demand)
- PDF cover page (rejected; may resurface as Settings toggle in v2)
- Custom comparison axis in reports (v2)
- Auto-detect floor-plan feature drift on image replace (v2)
- Map search bar / filter overlay (v1.x)
- Settings toggle for CAGG real-time flag (v2)
- Dedicated "Floor plan history" audit view (v2 — pin events already in audit_log)

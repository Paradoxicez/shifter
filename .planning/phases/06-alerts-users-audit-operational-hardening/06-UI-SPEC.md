---
phase: 6
slug: alerts-users-audit-operational-hardening
status: draft
shadcn_initialized: true
preset: new-york / slate / custom navy OKLCH
created: 2026-05-12
---

# Phase 6 — UI Design Contract

> Visual and interaction contract for Phase 6: Alerts, Users, Audit & Operational Hardening.
> Eight surfaces: (1) header bell + drawer, (2) `/alerts` page, (3) Settings → Alerts (rule
> library + cold-start roster), (4) MP detail anomaly card, (5) `/settings/users`, (6) `/audit`,
> (7) Settings → Backup, (8) sidebar nav + shell degraded banner.

---

## Design System

| Property | Value |
|----------|-------|
| Tool | shadcn/ui (new-york style, cssVariables=true) — pre-existing |
| Preset | `new-york` + `slate` baseColor + custom navy OKLCH tokens in `web/src/theme.css` |
| Icon library | lucide-react |
| Font | Inter Variable (body + UI); JetBrains Mono 400/600 (timestamps, IDs, JSON, passwords) |

Source: `web/components.json` + `web/src/theme.css` (detected). Phase 6 introduces NO new design
tokens.

---

## Spacing Scale

Inherited unchanged from Phases 1–5. Tailwind v4 default 4px scale.

| Token | Value | Phase 6 usage |
|-------|-------|---------------|
| xs | 4px | Severity dot gap, badge padding, JsonTree row gap |
| sm | 8px | Filter chip gap, drawer row inner padding |
| md | 16px | Form field gap, dialog body padding, table cell padding |
| lg | 24px | Card content padding, Settings card vertical rhythm |
| xl | 32px | Page section gap (filter chips → table) |
| 2xl | 48px | Major separators between Settings sub-cards |

Phase-6 layout composed from the scale:
- Alert drawer (`Sheet side="right"`): `sm:max-w-[480px]`, header 56px, chip row 48px, row 72px
- Audit expanded row: stacked `<md`, `grid grid-cols-2 gap-6` at `md+`
- Header bell badge: 16px (`h-4 w-4`) severity-tinted dot, top-right of bell

---

## Typography

Inherited unchanged. Two weights only: 400, 600.

| Role | Size | Weight | Line Height | Phase 6 usage |
|------|------|--------|-------------|---------------|
| Body | 14px (`text-sm`) | 400 | 1.5 (`leading-6`) | All rows in alerts / audit / users tables |
| Label / UI | 12px (`text-xs`) | 600 | 1.5 | Filter chips, severity pill text, column headers (uppercase + tracking-wide) |
| Heading | 20px (`text-xl`) or 24px (`text-2xl`) | 600 | 1.2 (`leading-8`) | Drawer title (text-xl); page H1 (text-2xl); dialog title |
| Display / Numeric | 28px (`text-3xl`) | 600 | 1.2 | "Last backup: 14h ago" stat |

Monospace (JetBrains Mono) new to Phase 6:
- Random-password show-once panel
- `request_id`, `entity_id` cells in audit table (already mono inside JsonTree)
- Backup filename + sha256 in history list

---

## Color

All tokens from `web/src/theme.css` (Phase 1). Phase 6 adds NO new tokens; locks the severity
map onto existing semantic colors (D-07).

| Role | Token | Phase 6 mapping |
|------|-------|-----------------|
| Dominant 60% | `--background` | Page + drawer + dialog background |
| Secondary 30% | `--card` / `--secondary` | All Phase 6 cards, sidebar, default chip fill |
| Accent 10% | `--primary` (navy) | Primary CTAs, active nav, focus rings, severity-`info` pill border |
| Muted | `--muted-foreground` | Secondary text, relative timestamps, disabled actions |
| Destructive | `--destructive` | Severity `critical`, "Disable user" / "Logout everywhere" confirm |
| Success | `--success` | Backup status dot (fresh), Anomaly "active" chip, "Re-enable" confirm |
| Warning | `--warning` | Severity `warning`, Backup status dot (stale), shell degraded banner |

### Severity map (locked)

| Severity | Fill | Foreground | Dot |
|----------|------|------------|-----|
| `critical` | `bg-destructive/10` | `text-destructive` | `bg-destructive` |
| `warning` | `bg-warning/10` | `text-warning-foreground` | `bg-warning` |
| `info` | `bg-secondary` | `text-secondary-foreground` | `bg-slate-400` |

### Accent reservation list (`--primary` reserved for)

1. Primary buttons in dialogs ("Add rule", "Save rule", "Add user", "Run backup now", "Export CSV")
2. Active sidebar nav items (Alerts, Audit, Settings sub-tabs)
3. Bell icon hover/focus state
4. Severity `info` pill border (1px navy at 20% alpha)
5. Filter chip "active" state (`bg-primary text-primary-foreground`)
6. Anomaly card "Active" state border-left accent strip
7. Focus rings on all interactive elements (shadcn default)

### Role badges

| Role | Variant |
|------|---------|
| `admin` | `Badge variant="default"` (navy fill) |
| `viewer` | `Badge variant="secondary"` (slate) |
| `disabled` status | `Badge variant="outline"` + `text-muted-foreground` |

### Backup freshness dot

| State | Color | Threshold |
|-------|-------|-----------|
| Fresh | `bg-success` | age ≤ `warn_threshold_hours` (default 24h) |
| Stale | `bg-warning` | warn < age ≤ `crit_threshold_hours` (default 168h) |
| Critical | `bg-destructive` | age > crit OR no backup ever |

Source: CONTEXT.md D-07, D-46. All underlying tokens from `web/src/theme.css`.

---

## Copywriting Contract

English-only (UX-02). Sentence case; periods on full sentences, NOT on buttons/chips.

### Primary CTAs

| Surface | Label |
|---------|-------|
| Alert drawer footer | **See all alerts** |
| `/alerts` page header | **Export CSV** (secondary) |
| Alert detail dialog | **Acknowledge** + **Snooze** (split-button) |
| Settings → Alerts | **Add rule** |
| MP / Site detail anomaly CTA | **Add alert rule** |
| Add Rule dialog — review step | **Save rule** / **Save changes** (edit) |
| Add Rule dialog — secondary | **Test fire** |
| `/settings/users` | **Add user** |
| Add User dialog — step 1 | **Create user** |
| Add User dialog — step 2 | **I've shared this** |
| Edit User dialog | **Save user** |
| Reset Password dialog | **Reset password** |
| Logout-everywhere AlertDialog | **Sign user out everywhere** |
| Role-change AlertDialog | **Change role** |
| Disable-user AlertDialog | **Disable user** |
| Re-enable AlertDialog | **Re-enable** |
| `/audit` page header | **Refresh** + **Export CSV** |
| Settings → Backup | **Run backup now** |
| Settings → Backup secondary | **Configure schedule →** (link) |
| Shell degraded banner | **View details** (link) |
| Bell icon | (no label — `aria-label="Alerts ({unread} unread)"`) |

### Snooze preset labels (D-09)

`Snooze 1 hour` / `Snooze 8 hours` / `Snooze 24 hours` / `Snooze 7 days` / `Mute until I clear`.

### Severity pill text

`Critical` / `Warning` / `Info`.

### Empty States

All follow Phase 4 `EmptyStateOnboarding` three-stage card pattern: `bg-primary/10` icon circle,
`CardTitle`, `CardDescription`, optional CTA.

| Surface | Icon | Heading | Body | CTA |
|---------|------|---------|------|-----|
| `/alerts` — never fired | `BellOff` | "No alerts yet" | "When a rule fires, alerts will appear here. Open Settings → Alerts to create your first rule." | "Open alert settings" → `/settings/alerts` |
| `/alerts` — filtered out | `Filter` | "No alerts match these filters" | "Adjust the filters above or clear them to see more results." | "Clear filters" |
| Alert drawer — empty | `Check` | "All clear" | "No unread alerts right now." | — |
| Settings → Alerts — no rules | `BellRing` | "No alert rules yet" | "Create a rule to be notified when a meter reads outside expected range, a device goes offline, or a quiet-hour leak is detected." | "Add rule" |
| `/audit` — filtered out | `Search` | "No audit entries match these filters" | "Try widening the date range or removing a filter. Default view shows the last 7 days." | "Reset to last 7 days" |
| Settings → Backup — never run | `Database` | "No backups yet" | "Run a backup now or configure the nightly schedule in the operator runbook." | "Run backup now" |

### Anomaly cold-start states (D-16)

| State | Icon | Headline | Body | Action |
|-------|------|----------|------|--------|
| `warming_up` | `Hourglass` | "Warming up" | "{N} days of history remaining before anomaly alerts can fire. We need 21 days of measurements per meter to learn its normal pattern." + Progress bar | (no action; `?` tooltip explains the gate) |
| `eligible_inactive` | `Activity` | "Ready to enable" | "21 days of history collected. Turn on one or more rules below to start detecting anomalies." | Per-rule toggles |
| `active` | `CheckCircle2` (success-tint) | "Anomaly detection: active" | "{N of 3} rules enabled: {comma-list}." | Per-rule toggles + "Edit thresholds" link |

### Error States

| Scenario | Copy |
|----------|------|
| Alert ack fails | "Could not acknowledge this alert. Try again." |
| Snooze fails | "Could not snooze. Try again." |
| Rule save fails | "Could not save the rule." (banner) |
| Test fire fails | "Test fire failed. Check the rule configuration and try again." |
| Add user — email exists | "That email is already in use." (inline) |
| Add user — weak password | "Password does not meet the strength requirements." (inline) |
| Logout-everywhere fails | "Could not sign user out. Their sessions may still be active." (destructive toast) |
| Audit CSV export fails | "Export failed. Try again or generate a full export." (destructive toast) |
| Backup "Run now" fails | "Backup failed: {reason}." (destructive toast + sticky `Alert` until dismissed) |

### Toast Notifications (sonner)

| Event | Variant | Copy |
|-------|---------|------|
| Alert acknowledged | success | "Alert acknowledged." |
| Alert snoozed (preset) | success | "Snoozed for {duration}." |
| Alert muted indefinitely | success | "Muted until cleared." |
| Test alert fired | info | "Test alert fired. It will auto-clear in 60 seconds." |
| Rule created | success | "Rule \"{rule name}\" added." |
| Rule updated | success | "Rule saved." |
| Rule disabled / enabled | success | "Rule disabled." / "Rule enabled." |
| User created | success | "User created. Share the credentials before closing this window." |
| User updated | success | "User updated." |
| User disabled | success | "User disabled. Their sessions have been ended." |
| User re-enabled | success | "User re-enabled." |
| Password reset | success | "Password reset. Share the new credentials with the user." |
| Role changed | success | "Role changed. The user has been signed out and must sign back in." |
| Logout everywhere | success | "{N} sessions ended." |
| Audit CSV downloaded | success | "CSV downloaded." |
| Audit full export queued | info | "Full export queued. You'll be notified when it's ready." |
| Audit full export ready | success | "Full export ready — click to download." (with action) |
| Backup started | info | "Backup started." |
| Backup completed | success | "Backup completed — {size} written to {destination}." |
| Password / email copied | success | "Password copied." / "Email copied." |

### Destructive Confirmations (AlertDialog)

No timer auto-confirm. Cancel button is always `variant="outline"` reading **Cancel**.
Destructive confirm button is NEVER the initial focus.

| Action | Heading | Body | Confirm |
|--------|---------|------|---------|
| Sign user out everywhere | "Sign {name} out everywhere?" | "All active sessions for this user will end immediately. They can sign back in normally." | **Sign user out everywhere** (destructive) |
| Change user role | "Change role for {name}?" | "Changing the role will sign {name} out everywhere. They'll see the new role the next time they sign in." | **Change role** (destructive) |
| Disable user | "Disable {name}?" | "The user will be signed out and unable to sign in. You can re-enable them later." | **Disable user** (destructive) |
| Re-enable user | "Re-enable {name}?" | "The user will be able to sign in with their existing password." | **Re-enable** (default variant) |
| Reset password | "Reset password for {name}?" | "A new random password will be generated. The user will be signed out and forced to change it on next sign-in." | **Reset password** (destructive) |
| Disable alert rule | "Disable rule \"{name}\"?" | "The rule will stop firing. Its fired-alert history stays queryable. You can re-enable it later." | **Disable rule** (destructive) |

Phase 6 has NO hard-delete UX for rules or users — all "destructive" actions are soft-state
changes (D-04, D-27) with audit rows in same tx (D-30).

Source: CONTEXT.md D-09, D-23..D-27.

---

## Surface 1 — Header Bell + Alert Drawer

**Position:** Topbar (`web/src/components/shell/topbar.tsx`), left of `AccountMenu`.

**Bell icon:** lucide `Bell` `h-5 w-5`, `text-muted-foreground` rest / `text-foreground` hover.
Tab-stop button (`aria-label="Alerts (N unread)"`). Severity-tinted dot anchored top-right:
0 unread = no badge; ≥1 critical = `bg-destructive`; else warning = `bg-warning`; else info =
`bg-slate-400`. Pulses on new unread (CSS `animate-pulse` 2s, respects `motion-reduce`).

**Drawer (`Sheet side="right"`, `sm:max-w-[480px] flex flex-col`):**

```
┌───────────────────────────────────────────────┐
│  Alerts                                  [×]  │  56px header
│  ──────────────────────────────────────────── │
│  [ All ] [ Unread ]                           │  48px chip row
│  ──────────────────────────────────────────── │
│  ● Critical  High flow — Building A meter     │  72px row
│    2 min ago             [Ack] [Snooze ▾]    │
│  ──────────────────────────────────────────── │
│  ... (up to 10 most-recent rows)              │
│  ──────────────────────────────────────────── │
│                         [ See all alerts → ]  │  footer link
└───────────────────────────────────────────────┘
```

**Composition:** `Sheet` + `SheetHeader/SheetTitle` + `ToggleGroup type="single"` (filter chips)
+ row list (virtualized via `@tanstack/react-virtual`) + `SheetFooter` with `Button variant="link"`.
Row: severity dot + 2-line title/timestamp + inline `Button variant="ghost" size="sm"` Ack +
`DropdownMenu` Snooze ▾ with the 5 presets.

**States:** loading = 3 `Skeleton` rows; empty = "All clear" EmptyStateOnboarding; populated =
up to 10 rows sorted severity DESC, fired_at DESC; error = `Alert variant="destructive"` + Retry.

**Behavior:** Esc / outside-click / × closes. New alerts arrive via React Query polling (or
optional `events.alert` SSE topic per CONTEXT.md). Ack is optimistic (row dims). Click row body
(not action buttons) opens Alert Detail Dialog. Viewers (D-11) have Ack / Snooze hidden via
`auth.Can(user, "alert.ack" | "alert.snooze")`. Mobile: full-screen below `sm`. Keyboard: Tab
cycles chips → row Acks → row Snoozes → footer link.

**Reuse:** `web/src/components/ui/sheet.tsx`, `ui/toggle-group.tsx`, `ui/dropdown-menu.tsx`,
`dashboard/EmptyStateOnboarding.tsx`, `ui/skeleton.tsx`.

**Acceptance:**
- [ ] Bell visible on all authenticated routes; hidden on `/login` and install-wizard
- [ ] Badge dot uses correct severity (critical > warning > info)
- [ ] Drawer opens on click; `aria-expanded` reflects state
- [ ] Max 10 rows sorted severity DESC then fired_at DESC
- [ ] Ack/Snooze hidden for viewers; "See all" visible to both roles
- [ ] Empty + loading + error states render correctly
- [ ] Esc closes; focus returns to bell

---

## Surface 1b — Alert Detail Dialog

`ResponsiveDialog`. Opens on click of alert row title (drawer or page).

**Title:** `{rule name}` + severity pill aligned right. TEST badge inline if `is_test=true` (D-19).

**Body sections:**
1. **Target row** — entity icon (`MapPin`/`Cpu`/`Radio`) + label + router link
2. **Status row** — severity pill, status chip (Open / Acknowledged / Snoozed until {ts} / Cleared), fired_at relative + absolute on hover
3. **Reading row** — `value` vs `threshold` (e.g., "1.42 L/min · threshold ≥ 1.0 L/min")
4. **Notes timeline** — audit events on this alert (ack, snooze, auto-clear); each row actor + verb + time
5. **Payload preview** — collapsible `<details>` containing `JsonTree` (reused from `web/src/components/metering-point/JsonTree.tsx`) rendering `alert.payload` JSONB (D-12). Collapsed by default.

**Footer:** **Acknowledge** (primary) + **Snooze** (split-button menu with 5 presets) + **Cancel**.
If acknowledged: hide Ack. If cleared: hide both action buttons.

**Acceptance:**
- [ ] Title shows rule name + severity pill (+ TEST badge for test fires)
- [ ] Target link navigates to entity (MP/site/gateway)
- [ ] Snooze menu has exactly 5 items in locked order
- [ ] JsonTree renders payload identically to Phase 4 Advanced tab
- [ ] Viewer sees dialog but Ack/Snooze hidden

---

## Surface 2 — `/alerts` (Full List)

**Route:** `/alerts` (admin + viewer); `web/src/routes/alerts/index.tsx`. Breadcrumb: `Alerts`.

```
┌────────────────────────────────────────────────────────────┐
│  Alerts                                       [Export CSV] │
│  ────────────────────────────────────────────────────────  │
│  Filter:                                                   │
│   [ Severity: All ▾ ] [ Status: Open ▾ ] [ Category: All ▾]│
│   [ Target type: All ▾ ] [ Clear all ]                     │
│  ────────────────────────────────────────────────────────  │
│  ● Critical · High flow · Building A — Meter 042           │
│    1.42 L/min · threshold ≥ 1.0 · Open · 2 min ago         │
│                            [ Ack ] [ Snooze ▾ ]  [ → ]     │
│  ────────────────────────────────────────────────────────  │
│  ... (virtualized list)                                    │
│  ────────────────────────────────────────────────────────  │
│                           [ Load more ]                    │
└────────────────────────────────────────────────────────────┘
```

**URL-state (D-32 pattern via `useSearchParams + zod`):**
```
?severity=critical|warning|info|all   (default: all)
&status=open|acknowledged|snoozed|cleared|all   (default: open)
&category=threshold|offline|anomaly|all   (default: all)
&target_type=metering_point|site|gateway|all   (default: all)
&from={ISO}&to={ISO}   (optional)
```

**Composition:** `<h1 text-2xl font-semibold>Alerts ({total})</h1>` + Export button; filter chips
(shadcn `Popover` per chip, "Clear all" link); row list (virtualized, same row component as
drawer with extra detail line); cursor-pagination "Load more" (D-36-style).

**States:** loading = 10 skeletons; empty-never = `BellOff` empty state; empty-filtered = `Filter`
empty state + Clear filters CTA; populated = list + Load more.

**Behavior:**
- Filter chips deep-linkable (refresh restores state)
- "Export CSV" respects active filters; ≤ 5000 = immediate download; > 5000 = queued + toast
- Real-time: React Query poll OR optional `events.alert` SSE topic; list updates without refresh
- Keyboard: ↑/↓ row focus, Enter opens detail, `A` ack focused row (admin), `S` snooze menu

**Reuse:** `@tanstack/react-virtual`, `ui/popover.tsx`, drawer's `AlertRow` component.

**Acceptance:**
- [ ] Default cold-arrival = `?status=open`
- [ ] Filter chips URL-state persisted
- [ ] Row count badge in H1 reflects filtered total
- [ ] Load more appends 100; appears only when cursor present
- [ ] Export respects active filters
- [ ] Viewer sees page; row actions hidden

---

## Surface 3 — Settings → Alerts (Rule Library)

**Route:** `/settings/alerts` (admin edit, viewer read-only). Breadcrumb: `Settings → Alerts`.

```
┌──────────────────────────────────────────────────────────┐
│  Settings · Alerts                                       │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │ Anomaly detection                                  │  │
│  │ 4 meters eligible · 12 warming up                  │  │
│  │ [ Show roster ▾ ]                                  │  │
│  └────────────────────────────────────────────────────┘  │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │ Alert rules                          [ + Add rule ]│  │
│  │ ────────────────────────────────────────────────── │  │
│  │ [ Show disabled ☐ ]                                │  │
│  │ ────────────────────────────────────────────────── │  │
│  │ ☑  Name           Kind     Target  Sev  Last fired │  │
│  │ ────────────────────────────────────────────────── │  │
│  │ ☑  High flow…  Threshold  Bldg A  Crit  2m ago [⋮] │  │
│  │ ☑  Battery low Threshold  Global  Warn  15m   [⋮] │  │
│  │ ☐  Quiet leak  Anomaly    Bldg A  Crit  3d    [⋮] │  │
│  │ ────────────────────────────────────────────────── │  │
│  │                                  [ Load more ]     │  │
│  └────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────┘
```

**Composition:**
- Anomaly status: `Card` with `CardHeader/CardContent`; "Show roster ▾" expands a grouped list (Eligible / Warming up) — each row links to MP detail
- Rule library: `Card` containing `Table` (TanStack Table) with leftmost row toggle + overflow `DropdownMenu` (`Edit`, `Test fire`, `Disable`/`Enable`)
- "Add rule" button (top-right of card): opens stepped Add Rule `ResponsiveDialog`

**States:** loading = 5 skeletons; empty = `BellRing` empty state + "Add rule" CTA; populated =
table + Load more.

**URL-state:** `?show_disabled=0|1`.

### Add Rule dialog — 5 steps

Reuses `web/src/components/responsive-dialog.tsx` + `web/src/components/stepper.tsx` (Phase 1).
Same component for the three D-18 entry points (MP detail / Site detail / Settings → Alerts);
when invoked with prefilled scope, **Step 1 (Scope) is skipped**.

| Step | Fields |
|------|--------|
| 1. Scope | Radio: All meters / Site (Combobox) / Metering point (Combobox) |
| 2. Kind | Radio: Threshold {sub: Instantaneous / Hourly / Daily} (D-02) / Offline / Anomaly {sub: P95 / IQR / Quiet-hour} (D-17). Inline warning if target MP is `warming_up`: "This meter is warming up — the rule will only fire once it becomes eligible in {N} days." |
| 3. Conditions | Threshold: comparison `> \| >= \| < \| <=` + value + unit (from MP `utility_class`). Offline: read-only computed window "Will fire when no uplink for {3 × expected} = {duration}". Anomaly P95/IQR: no params for v1 (Phase 7 tunes). Anomaly Quiet-hour: time-range picker + days-of-week multi-select (default all 7). |
| 4. Severity + Cooldown + Notes | Severity radio (defaults: critical for threshold/offline/anomaly, warning for battery/cold-start, info for first-uplink) + Cooldown input (default 900s, D-05) + Name (auto-default override) + Notes textarea |
| 5. Review | Summary card + secondary **Test fire** button (D-19) + footer **Back** / **Save rule** |

**Behavior:**
- Row toggle is optimistic; revert on error
- "Show disabled" toggle URL-state persisted
- Bulk select via leftmost checkbox; action bar slides in below header on selection (TanStack pattern)
- Test fire creates synthetic info-severity alert with TEST badge, auto-clears 60s

**Reuse:** `ui/card.tsx`, `ui/table.tsx` + `@tanstack/react-table`, `responsive-dialog.tsx`,
`stepper.tsx`, `ui/form.tsx`, `ui/select.tsx`, `ui/radio-group.tsx`, `ui/textarea.tsx`.

**Acceptance:**
- [ ] Add Rule has 5 steps; Scope skipped when prefilled
- [ ] Severity defaults match D-07 (critical/warning/info as per rule kind)
- [ ] Cooldown defaults to 900s; min 0, max 86400
- [ ] Test fire raises synthetic alert visible in drawer ≤ 3s
- [ ] Row toggles optimistic + persistent
- [ ] Show disabled URL-state persisted
- [ ] Viewer sees table read-only (no toggles / overflow / Add rule)

---

## Surface 4 — MP Detail Anomaly Cold-Start Card

**Location:** `web/src/routes/metering-points/$id.tsx` (or equivalent), ABOVE the Phase 4 tabs
(Normal / Advanced / Uplinks log). Full-width `Card` with `lg` horizontal padding.

```
WARMING UP  (slate-tinted bg-secondary/40)
┌─────────────────────────────────────────────────────────────┐
│  ⏳  Anomaly detection — Warming up                         │
│      16 days of history remaining before anomaly alerts     │
│      can fire.                                              │
│      ▓▓▓▓▓▓▓░░░░░░░░░░░░░░░░░░░░░  5 / 21 days              │
│      [?] Why is this gated?                                 │
└─────────────────────────────────────────────────────────────┘

ELIGIBLE — INACTIVE  (white bg-card, navy icon)
┌─────────────────────────────────────────────────────────────┐
│  ⚡  Anomaly detection — Ready to enable                    │
│      21 days collected. Turn on one or more rules below.    │
│      [ ☐ P95 spike ]  [ ☐ IQR outlier ]  [ ☐ Quiet-hour leak]│
└─────────────────────────────────────────────────────────────┘

ACTIVE  (white bg-card + border-l-4 border-l-success)
┌─────────────────────────────────────────────────────────────┐
│  ✓  Anomaly detection — Active                              │
│      2 of 3 rules enabled: P95 spike, Quiet-hour leak.      │
│      [ ☑ P95 spike ] [ ☐ IQR outlier ] [ ☑ Quiet-hour leak ]│
│                          [ Edit thresholds → Settings ]     │
└─────────────────────────────────────────────────────────────┘
```

**Composition:** `Card` + `CardHeader/CardContent`; icon (`Hourglass` / `Activity` / `CheckCircle2`);
shadcn `Progress` (warming_up only); per-rule `Toggle` (controlled checkbox); `Tooltip` on `?`
(copy: "Anomaly detection learns the normal pattern of each meter from its first 21 days. This
avoids false alarms during install ramp-up."); active state's link `Button variant="link"` →
`/settings/alerts`.

**Behavior:** Card admin+viewer visible; toggles admin-only (hidden for viewers via `auth.Can`).
Toggle writes PATCH `/api/metering-points/{id}/anomaly-rules/{kind}`; optimistic + revert on
error; no per-toggle toast.

**Reuse:** `ui/card.tsx`, `ui/progress.tsx`, `ui/toggle.tsx`, `ui/tooltip.tsx`. State pattern
mirrors Phase 4 D-21 progressive `EmptyStateOnboarding`.

**Acceptance:**
- [ ] Card renders above tabs in MP detail; full content width
- [ ] State transitions warming_up → eligible_inactive → active correctly
- [ ] Progress bar accurate (elapsed_days / 21)
- [ ] Tooltip explains 21-day gate
- [ ] Per-rule toggles persist; admin-only
- [ ] Viewer sees card; toggles disabled with tooltip "Read-only — viewer role"

---

## Surface 5 — `/settings/users`

**Route:** `/settings/users` (admin edit). Breadcrumb: `Settings → Users`. CONTEXT.md D-29
keeps this under Settings (no top-level nav).

```
┌────────────────────────────────────────────────────────────┐
│  Settings · Users                                          │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ Users (3)                              [ + Add user ]│  │
│  │ ──────────────────────────────────────────────────── │  │
│  │ [ Show disabled ☐ ]                                  │  │
│  │ ──────────────────────────────────────────────────── │  │
│  │ Name        Email             Role    Last login [⋮] │  │
│  │ ──────────────────────────────────────────────────── │  │
│  │ Anita K.    anita@acme.io     Admin   2m ago     [⋮] │  │
│  │ Ben S.      ben@acme.io       Viewer  3h ago     [⋮] │  │
│  │ You (Sura)  sura@acme.io      Admin   now        [⋮] │  │
│  └──────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────┘
```

**Columns:** Name (prefixed "You" if `row.id === current_user.id`), Email, Role (`Badge` per
role map above), Last login (relative + `Tooltip` absolute), Status (disabled-tab only — outline
Badge "Disabled at {date}"), Actions (`DropdownMenu`).

**Row action menu (admin only):**

For active users:
- **Edit** → Edit User dialog
- **Reset password** → Reset Password dialog
- **Sign out everywhere** → AlertDialog (D-24)
- **Disable** → Disable User AlertDialog

(D-26 blocks: hide "Sign out everywhere" + "Disable" on `current_user.id` row; hide "Disable"
+ role-radio in Edit on last-admin row.)

For disabled users (Show disabled toggle on):
- **Re-enable** → Re-enable AlertDialog (D-27)
- **View history** → `/audit?user_id={id}`

### Add User dialog — 2 steps (D-23)

**Step 1: Details** (ResponsiveDialog)
- Name (Input) / Email (Input, zod email) / Role (RadioGroup, default Viewer)
- Footer: Cancel / **Create user**
- Submit calls POST `/api/users`; backend generates random password

**Step 2: Share credentials** (replaces step 1 body on success)
```
┌─────────────────────────────────────────────────┐
│  User created                              [×]  │
│  ─────────────────────────────────────────────  │
│  Share these credentials with {name}. We won't  │
│  show this password again.                      │
│                                                 │
│  Email      ben@acme.io                  [📋]   │
│  Password   K9!mQ-bn7L@xWvZr            [📋]   │
│                                                 │
│  ⚠  This password is shown once. After you      │
│     close this dialog, only the user can see    │
│     it (and they'll be forced to change it on   │
│     first sign-in).                             │
│  ─────────────────────────────────────────────  │
│                          [ I've shared this ]   │
└─────────────────────────────────────────────────┘
```
- Password in `font-mono text-base` block with `Copy` button (toast "Password copied")
- Email row has Copy button (toast "Email copied")
- Warning row: `Alert variant="warning"` (or `bg-warning/10`)
- "I've shared this" closes + `queryClient.removeQueries` purges the password from cache

### Edit User dialog

Same Step 1 shape; no Step 2. If role changes, on Save the Role-change AlertDialog (D-25) gates
submission. Self-row: role radio disabled with helper "You can't change your own role." Last-admin
row: role radio disabled with helper "At least one admin must remain."

### Reset Password dialog

AlertDialog confirm → on confirm POST `/api/users/{id}/reset-password` → flips to step-2-shaped
panel with new password + Copy + "I've shared this".

**Behavior:**
- "Show disabled" URL-state `?show_disabled=1`
- Current user row tinted `bg-primary/5`
- Last-admin row tinted `bg-secondary/40` with tooltip on disabled actions
- Sort: created_at DESC; current user pinned top
- Mobile: collapses to card-per-row at `<md` (Name + Role badge in title, fields stacked, menu top-right)

**Reuse:** `responsive-dialog.tsx`, `ui/alert-dialog.tsx`, `ui/dropdown-menu.tsx`,
`ui/badge.tsx`, `ui/form.tsx` + `react-hook-form` + `zod`.

**Acceptance:**
- [ ] Table renders all users; sort created_at DESC; current user pinned top
- [ ] Add User dialog 2 steps; password panel only after successful create
- [ ] Password monospace + Copy with toast
- [ ] "I've shared this" closes + clears cache
- [ ] Edit dialog disables role radio for self + last-admin (D-26)
- [ ] Role change triggers Role-change AlertDialog before save
- [ ] Disable / Logout-everywhere / Reset open AlertDialog with verbatim copy
- [ ] Re-enable visible only with "Show disabled" on
- [ ] Viewer accessing route gets 403 (or self-only) — planner finalizes
- [ ] All mutations write audit row in same tx (D-30; verified via `/audit`)

---

## Surface 6 — `/audit` (Browse + Export)

**Route:** `/audit` (admin-only sidebar item, D-31); `web/src/routes/audit/index.tsx`.
Breadcrumb: `Audit log`. Hidden from viewers entirely (`Can(user, "audit.read")`).

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Audit log                                  [Refresh] [Export CSV]      │
│  ───────────────────────────────────────────────────────────────────── │
│  Filter:                                                                │
│   [ Date range: Last 7 days ▾ ] [ User: All ▾ ] [ Entity type: All ▾ ] │
│   [ Action: All ▾ ] [ Request ID: __________ ] [ Clear all ]            │
│  ───────────────────────────────────────────────────────────────────── │
│  Time      User       Action          Entity       Request ID          │
│  ───────────────────────────────────────────────────────────────────── │
│  2m ago    anita@…    device.swap     device:abc…  r/a9b…       [▸]    │
│  15m ago   ben@…      auth.login_…    -            r/3f1…       [▸]    │
│  1h ago    anita@…    alert_rule.cr…  rule:fa9…    r/77c…       [▾]    │
│  ╭─ Expanded ───────────────────────────────────────────────────────╮  │
│  │ before { null }            │  after { name: "High flow…",      } │  │
│  │                            │         severity: "critical",       │  │
│  │                            │         cooldown_s: 900   …         │  │
│  ╰──────────────────────────────────────────────────────────────────╯  │
│  ───────────────────────────────────────────────────────────────────── │
│                              [ Load more ]                              │
└─────────────────────────────────────────────────────────────────────────┘
```

**URL-state (D-32, zod-validated with `.catch()`):**
```
?from={ISO}          (default: now − 7d, D-33)
&to={ISO}            (default: now)
&user_id={uuid}
&entity_type=[]      (multi-select)
&action=[]           (multi-select)
&request_id={string} (free-text, debounce 400ms)
```

**Columns:**

| Column | Width | Render |
|--------|-------|--------|
| Time | 120px | Relative + `Tooltip` absolute install_tz |
| User | 200px | Email (or "system") |
| Action | 200px | Vocabulary string (e.g., `device.swap`); monospace |
| Entity | 240px | `{entity_type}:{short-id}`; monospace |
| Request ID | 120px | `r/{first-6}` truncated; mono; click copies |
| Expand | 32px | Chevron `▸`/`▾` |

**Expanded row (D-34):** Panel below the row spanning all columns; grid `grid-cols-1
md:grid-cols-2 gap-6` of two `JsonTree` instances (`before`, `after`) reused from
`web/src/components/metering-point/JsonTree.tsx`. Planner adds a `highlightKeys: string[]` prop
(or HOC) to highlight changed keys with `bg-warning/20`. `before=null` → left renders "(created)"
in `text-muted-foreground`; `after=null` → right renders "(removed)".

**Composition:**
- Header: `<h1>Audit log</h1>` + right-aligned **Refresh** (`RotateCw` icon, ghost) + **Export CSV** (primary)
- Filter chips: same `Popover`-per-chip pattern as `/alerts`; Date range chip reuses
  `web/src/components/dashboard/DateRangePicker.tsx` (Phase 4/5); Request ID is a debounced text-input chip
- Table: TanStack Table + `@tanstack/react-virtual` virtualization
- Cursor pagination "Load more" (D-36; mirrors Phase 4 uplinks log)

**States:** loading = header + 10 skeletons; empty = `Search` "No audit entries match these
filters" + "Reset to last 7 days" CTA; populated = table + Load more; error = destructive Alert
above chips; export-queued (>50k) = inline Alert below header + dismissible; export-ready =
sonner toast with download.

**Behavior:**
- No live tail (D-37); Refresh manually refetches; relative timestamps re-format via
  `refetchOnWindowFocus`
- "Export CSV" ≤ 50k = immediate browser download (REPT-03-shaped per D-35); > 50k = River job
  kicked + inline Alert + sonner on completion
- Row body click (not chevron, not cell links) toggles expand
- Click User cell → `?user_id=...` filter applied
- Click Entity cell → navigates to entity route if known (MP/site/device/gateway)
- Click Request ID → copies + toast "Request ID copied"
- Keyboard: ↑/↓ row focus, Enter toggles expand, `E` triggers Export

**Reuse:** `JsonTree.tsx` (with new `highlightKeys` prop), `ui/table.tsx` + `@tanstack/react-table`,
`@tanstack/react-virtual`, `ui/popover.tsx`, `dashboard/DateRangePicker.tsx`,
`dashboard/EmptyStateOnboarding.tsx`.

**Acceptance:**
- [ ] Cold-arrival = last 7 days, all types, all users (D-33)
- [ ] Filter chips URL-state deep-linkable
- [ ] Date range chip uses Phase 4 DateRangePicker
- [ ] Table virtualized; 5000 rows scroll smoothly
- [ ] Expand reveals side-by-side JsonTree on desktop, stacked on mobile
- [ ] Changed keys highlighted yellow in both columns
- [ ] CSV export ≤ 50k = immediate download
- [ ] CSV export > 50k = queued River job + toast on ready
- [ ] Refresh re-fetches; no SSE on this page
- [ ] Viewer route access blocked (redirect to `/`)

---

## Surface 7 — Settings → Backup

**Location:** New section in `web/src/routes/settings.tsx` below Data Retention. Two sub-cards:
**Backup status** + **Restore guidance** (read-only).

```
┌──────────────────────────────────────────────────────────────────┐
│  Backup                                                          │
│  ──────────────────────────────────────────────────────────────  │
│  ● Last backup: 14 hours ago                                     │
│    Destination:  /var/lib/shifter/backups/                       │
│                                                                  │
│  Warn threshold     [  24  ] hours                               │
│  Critical threshold [ 168  ] hours                               │
│                                  [ Save thresholds ]             │
│                                                                  │
│                                  [ Run backup now ]              │
│  ──────────────────────────────────────────────────────────────  │
│  Recent backups ▾                                                │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │ shifter-backup-acme-20260512-0200-0036.tar.gz  2.4 GB  ✓ │  │
│  │ Started 02:00:01  Finished 02:14:33                       │  │
│  │ sha256: 7a3f...c92b   [Show full]                         │  │
│  │ (4 more rows, most recent first)                          │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  [ Configure schedule → operator runbook ]                       │
└──────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│  Restore                                                         │
│  ──────────────────────────────────────────────────────────────  │
│  Restoring a backup requires Shifter to be stopped.              │
│  See the operator runbook for the `shifter restore` procedure.   │
│                                                                  │
│  [ Open operator runbook ↗ ]                                     │
└──────────────────────────────────────────────────────────────────┘
```

**Composition:**
- Two `Card`s
- Last-backup row: freshness dot (per map above) + relative + absolute on hover
- Destination text monospace; click-to-copy with toast
- Threshold inputs: `Input type="number"` + unit suffix; validates `warn < crit`, both `> 0`, integers; Save button enables on change
- **Run backup now**: primary; disabled with spinner while running; React Query polls GET `/api/backup/jobs/{id}` every 5s
- Recent backups: shadcn `Collapsible` (or `<details>`); up to 5 rows; expandable sha256 inline
- "Configure schedule": `Button variant="link"` + `ExternalLink` icon → runbook section in new tab
- Restore card: read-only doc; "Open operator runbook ↗" link button

**States:** loading = skeletons; never-run = "Last backup: Never" + red dot + "Run a backup now
to get started."; running = spinner replaces dot, button disabled + "Backing up…"; completed =
status updates + success toast; failed = red dot + destructive toast + sticky `Alert
variant="destructive"` in card until Dismiss.

**Behavior:** Card admin-only edit; viewer sees status + history but thresholds + Save + Run-now
hidden. Recent backups list is read-only (rotation is operator-managed per D-42).

**Reuse:** `ui/card.tsx`, `ui/input.tsx`, `ui/button.tsx`, `ui/alert.tsx`.

**Acceptance:**
- [ ] Status dot color matches D-46 thresholds
- [ ] Thresholds validate `warn < crit`, both > 0, integers
- [ ] "Run backup now" disables + spinner while job runs
- [ ] Success toast on completion; destructive toast + sticky Alert on failure
- [ ] Recent backups max 5; expand reveals sha256
- [ ] Schedule + runbook links open in new tab
- [ ] Restore card has no destructive controls
- [ ] Viewer sees status + history only

---

## Surface 8 — Sidebar Nav + Shell Degraded Banner

### Sidebar nav (final order)

Edits `web/src/components/shell/sidebar.tsx`:

```
Dashboard      (LayoutDashboard)
Map            (Map)
Sites          (MapPin)
Devices        (Cpu)
Gateways       (Radio)
Reports        (FileText)
Alerts         (Bell)        ← NEW (admin + viewer); unread severity-tinted badge inline-right
Audit          (Shield)      ← NEW (admin-only, Can(user, "audit.read"))
Settings       (Settings)
  ├─ Identity
  ├─ ChirpStack
  ├─ Units
  ├─ Timezone
  ├─ Alerts        ← NEW
  ├─ Users         ← NEW
  ├─ Data retention
  └─ Backup        ← NEW
─────────────
Admin
  ├─ Imports
```

**Alerts badge:** small severity-tinted circle + count text to the right of the label. Cap at
"99+" for layout stability. Hidden when count = 0.
```
Bell  Alerts                                 ● 3
```
- `text-[10px] font-semibold`, circle min 16px
- Color: critical > warning > info (locked map)

### Shell degraded banner (D-22)

Rendered above the topbar when `/health/detailed` returns `alert_worker.degraded = true`.
Visible to admin + viewer. New component: `web/src/components/shell/AlertWorkerBanner.tsx`,
mounted in `_root.tsx`.

```
┌──────────────────────────────────────────────────────────────────┐
│ ⚠  Alert evaluation degraded — last successful run 2h ago.       │
│                                              [ View details → ]  │
└──────────────────────────────────────────────────────────────────┘
[ ... rest of shell ... ]
```

- Background `bg-warning/20` (fallback `bg-yellow-100 dark:bg-yellow-950/40`)
- Border-bottom 1px `border-warning` (fallback `border-yellow-500/40`)
- Icon `AlertTriangle h-4 w-4 text-warning`
- "View details →" → `/health/detailed` view (admin-only); link hidden for viewers
- Auto-dismisses when next poll returns `degraded = false`
- No close button (real fault signal — not user-dismissible)
- Sticky (`sticky top-0 z-50`); topbar slides below it

**Acceptance:**
- [ ] Sidebar items render in the locked order
- [ ] Audit hidden for viewers
- [ ] Alerts unread badge severity matches highest unread (locked map)
- [ ] Banner renders when `alert_worker.degraded = true`; auto-dismisses on recovery
- [ ] Banner sticky; topbar slides below
- [ ] "View details" admin-only; viewer sees text-only banner

---

## Component Inventory

### New components (Phase 6)

| Component | Purpose | Reuses |
|-----------|---------|--------|
| `shell/AlertBell` | Topbar bell with severity-tinted badge | `Bell` icon + `Badge` |
| `shell/AlertWorkerBanner` | Top-of-shell degraded warning | `Alert` (warning variant) |
| `alerts/AlertDrawer` | Slide-over drawer (Surface 1) | `Sheet`, `ToggleGroup`, `Skeleton`, `DropdownMenu` |
| `alerts/AlertRow` | Single row (used in drawer + page) | `Badge`, `Button`, `DropdownMenu` |
| `alerts/AlertDetailDialog` | Per-alert detail with JsonTree payload (Surface 1b) | `ResponsiveDialog`, `JsonTree` |
| `alerts/SeverityPill` | Inline pill (info/warning/critical) | `Badge` |
| `alerts/SnoozeMenu` | Split-button with 5 presets | `DropdownMenu` |
| `routes/alerts/index.tsx` (`AlertsPage`) | Surface 2 | `@tanstack/react-virtual`, `Popover`, virtualized AlertRow |
| `alert-rules/AlertRulesPage` | Surface 3 content | `Card`, `Table`, `Skeleton`, `Collapsible` |
| `alert-rules/AnomalyRosterSection` | Cold-start status section | `Card`, `Collapsible` |
| `alert-rules/AlertRuleTable` | TanStack Table of rules | `@tanstack/react-table`, `Switch`, `DropdownMenu` |
| `alert-rules/AddRuleDialog` | 5-step ResponsiveDialog | `ResponsiveDialog`, `Stepper`, `Form`, `RadioGroup`, `Select`, `Textarea` |
| `alert-rules/TestFireButton` | Secondary button in rule dialog | `Button`, sonner |
| `alert-rules/AnomalyStateCard` | MP detail anomaly card (Surface 4) | `Card`, `Progress`, `Toggle`, `Tooltip` |
| `users/UsersPage` | Surface 5 | `Card`, `Table` |
| `users/UsersTable` | TanStack Table | `@tanstack/react-table`, `Badge`, `DropdownMenu` |
| `users/AddUserDialog` | 2-step add (form → share-credentials) | `ResponsiveDialog`, `Form`, `Input`, `RadioGroup`, `Alert` |
| `users/ShareCredentialsPanel` | Show-once email + password block | `Button`, `Alert`, sonner |
| `users/EditUserDialog` | Edit name + role | `ResponsiveDialog`, `Form` |
| `users/ResetPasswordDialog` | Confirm + step-2-shaped panel | `AlertDialog` then `ResponsiveDialog` |
| `users/LogoutEverywhereDialog` | Destructive confirm | `AlertDialog` |
| `users/DisableUserDialog` | Destructive confirm | `AlertDialog` |
| `users/ReEnableUserDialog` | Confirm (default variant) | `AlertDialog` |
| `users/RoleChangeDialog` | Destructive confirm gating Edit submit | `AlertDialog` |
| `routes/audit/index.tsx` (`AuditPage`) | Surface 6 | `@tanstack/react-virtual`, `Card`, `Table` |
| `audit/AuditFilterChips` | Date / User / Entity / Action / Request ID | `Popover`, `DateRangePicker`, `Combobox`, `Input` |
| `audit/AuditTable` | TanStack Table with expandable rows | `@tanstack/react-table`, `Button` |
| `audit/AuditRowExpand` | Side-by-side before/after JsonTree | `JsonTree` (with `highlightKeys`) |
| `audit/AuditExportButton` | Header CSV export with state | `Button`, sonner |
| `backup/BackupStatusCard` | Last-backup + thresholds + Run-now + history (Surface 7) | `Card`, `Input`, `Button`, `Collapsible` |
| `backup/BackupHistoryList` | Most-recent-5 expandable | `Collapsible` |
| `backup/BackupFreshnessDot` | Color-coded dot with tooltip | `Tooltip` |
| `backup/RestoreGuidanceCard` | Read-only doc card | `Card`, `Alert` |

### Reused components (no changes needed)

| Component | Reuse |
|-----------|-------|
| `web/src/components/responsive-dialog.tsx` (Phase 1) | Add Rule, Edit Rule, Add User, Edit User, Reset Password |
| `web/src/components/stepper.tsx` (Phase 1) | Add Rule 5-step dialog |
| `web/src/components/ui/alert-dialog.tsx` (shadcn) | All destructive confirms |
| `web/src/components/ui/sheet.tsx` (shadcn) | Alert drawer |
| `web/src/components/metering-point/JsonTree.tsx` (Phase 4 D-19) | Alert detail payload, audit row diff |
| `web/src/components/dashboard/EmptyStateOnboarding.tsx` (Phase 4 D-21) | All empty states |
| `web/src/components/dashboard/DateRangePicker.tsx` (Phase 4/5) | Audit date-range chip |
| `web/src/components/shell/sidebar.tsx` | Add nav items + unread badge |
| `web/src/components/shell/topbar.tsx` | Mount AlertBell |
| `@tanstack/react-table`, `@tanstack/react-virtual`, `@tanstack/react-query` | All tables / virtualized lists |
| `react-hook-form` + `zod` | All forms |
| `sonner` | All toasts |
| `lucide-react` | All icons |

---

## Lucide Icon Inventory

| Icon | Surface |
|------|---------|
| `Bell` / `BellOff` / `BellRing` | Alert bell / Alerts empty / Rules empty |
| `Shield` | Audit nav, Audit page |
| `Users` | Users nav, Users page |
| `Database` | Backup card, Backup empty state |
| `Activity` / `Hourglass` / `CheckCircle2` | Anomaly states (eligible / warming / active) |
| `AlertTriangle` / `XCircle` / `Info` | Severity (warning / critical / info) |
| `Filter` / `Search` / `Check` | Empty states (filtered / audit empty / drawer empty) |
| `RotateCw` | Audit Refresh button |
| `Copy` | Email/password copy buttons |
| `ExternalLink` | Schedule / runbook links |
| `MapPin` / `Cpu` / `Radio` | Entity icon in alert target row |
| `ChevronDown` / `ChevronRight` | Expand/collapse rows |

No new icon dependencies — all already in `lucide-react`.

---

## Route Architecture

| Route | Component | Default URL-state |
|-------|-----------|-------------------|
| `/alerts` | `AlertsPage` | `?status=open` |
| `/audit` | `AuditPage` | `?from={now-7d}&to={now}` (D-33) |
| `/settings/users` | `UsersPage` (under settings) | `?show_disabled=0` |
| `/settings/alerts` | `AlertRulesPage` (under settings) | `?show_disabled=0` |
| `/settings/backup` | section inside `settings.tsx` | n/a |

URL-state schemas (zod with `.catch()` fallbacks; Phase 3 D-15 pattern):

```ts
const alertsParams = z.object({
  severity: z.enum(["critical","warning","info","all"]).catch("all"),
  status: z.enum(["open","acknowledged","snoozed","cleared","all"]).catch("open"),
  category: z.enum(["threshold","offline","anomaly","all"]).catch("all"),
  target_type: z.enum(["metering_point","site","gateway","all"]).catch("all"),
  from: z.string().datetime().optional(),
  to: z.string().datetime().optional(),
});

const auditParams = z.object({
  from: z.string().datetime().catch(() => sub7days(new Date()).toISOString()),
  to: z.string().datetime().catch(() => new Date().toISOString()),
  user_id: z.string().uuid().optional(),
  entity_type: z.array(z.string()).catch([]),
  action: z.array(z.string()).catch([]),
  request_id: z.string().optional(),
});
```

---

## Interaction Flows (Key Sequences)

### Alert acknowledgment from drawer

```
[Bell click] → AlertDrawer opens → GET /api/alerts?status=open&limit=10
[Ack click on row]
  → optimistic dim → POST /api/alerts/{id}/ack
  → audit row same tx (D-30)
  → toast "Alert acknowledged."
  → next poll removes from Unread filter
```

### Add User (show-once password)

```
[Add user] → Step 1 form → POST /api/users
  → backend generates random password (D-23) + audit row user.create
  → response { user, plaintext_password }
  → dialog flips to Step 2 ShareCredentialsPanel
  → user clicks Copy → clipboard + toast "Password copied"
  → "I've shared this" → close + queryClient.removeQueries → unrecoverable
  → users table refetches
```

### Audit row expand

```
[Click chevron] → row.expanded = true
  → render AuditRowExpand below row
  → compute changedKeys from before vs after
  → JsonTree.left + JsonTree.right with highlightKeys={changedKeys}
  → highlighted keys get bg-warning/20
[Click chevron again] → collapse
```

### Backup "Run now"

```
[Run backup now] → POST /api/backup/run → audit row backup.run.start
  → button disabled + spinner + "Backing up…"
[Poll GET /api/backup/jobs/{id} every 5s]
  → pending → running → completed | failed
[completed] → green dot + success toast + history prepend + audit row
[failed]    → red dot + destructive toast + sticky Alert + audit row
```

### Anomaly toggle

```
[Toggle P95 spike] → optimistic flip
  → PATCH /api/metering-points/{id}/anomaly-rules/p95
  → audit row alert_rule.update same tx
  → may transition card eligible_inactive → active on first enable
  → silent (no toast); revert + inline error on failure
```

---

## Accessibility Notes

- AlertDialog uses shadcn focus-trap + initial-focus on Cancel (destructive never gets initial focus)
- Severity pills have `aria-label="severity: critical"` etc. (not relying on color alone)
- Backup freshness dot: `aria-label="Backup status: fresh (14 hours)"`
- Audit row chevron: `aria-expanded` synced, `aria-controls` points to panel ID
- Shell degraded banner: `role="alert"` (announced on appearance)
- JsonTree highlightKeys: bg color + visually-hidden "▸ changed" text for screen readers
- Form fields use `aria-describedby` for zod errors
- Bell aria-label: `"Alerts (3 unread, 1 critical)"` (count + severity context)

---

## Registry Safety

| Registry | Blocks Used | Safety Gate |
|----------|-------------|-------------|
| shadcn official | `alert-dialog`, `sheet`, `toggle-group`, `popover`, `dropdown-menu`, `card`, `badge`, `button`, `table`, `dialog`, `form`, `input`, `label`, `radio-group`, `progress`, `toggle`, `tooltip`, `select`, `textarea`, `separator`, `tabs`, `skeleton`, `command`, `scroll-area` (all already installed Phase 1–5) | not required |
| npm official | `@tanstack/react-table`, `@tanstack/react-virtual`, `@tanstack/react-query`, `react-hook-form`, `zod`, `sonner`, `lucide-react` — all already in `web/package.json` | not required |

No third-party shadcn registries declared. Registry vetting gate: not applicable.

---

## Decision Source Traceability

| Section | Source |
|---------|--------|
| Severity color map (locked) | CONTEXT.md D-07 |
| Snooze presets (1h/8h/24h/7d/Mute until cleared) | CONTEXT.md D-09 |
| Shared inbox snooze | CONTEXT.md D-10 |
| Viewers see alerts read-only | CONTEXT.md D-11 |
| Structured alert payload (JsonTree-renderable) | CONTEXT.md D-12 |
| Bell + drawer + page (Surfaces 1, 1b, 2) | CONTEXT.md D-20 |
| Alert rule library + Show disabled + soft-delete | CONTEXT.md D-04 |
| Three entry points / one Add Rule dialog | CONTEXT.md D-18 |
| Cool-down default 900s | CONTEXT.md D-05 |
| Test fire button | CONTEXT.md D-19 |
| Cold-start anomaly chip (Surface 4) | CONTEXT.md D-16 |
| Three statistical anomaly rules | CONTEXT.md D-17 |
| Anomaly card three-state pattern | Phase 4 D-21 |
| Users at Settings → Users | CONTEXT.md D-29 |
| Random show-once password | CONTEXT.md D-23 |
| Logout-everywhere | CONTEXT.md D-24 |
| Role-change auto-revoke | CONTEXT.md D-25 |
| Block self-demote / self-disable / last-admin | CONTEXT.md D-26 |
| Re-enable preserves credentials | CONTEXT.md D-27 |
| Password strength reuse | CONTEXT.md D-28 |
| Audit-event audit retrofit | CONTEXT.md D-30 |
| Audit at top-level `/audit` admin-only | CONTEXT.md D-31 |
| Filter chips URL-state | CONTEXT.md D-32 |
| Default last 7 days | CONTEXT.md D-33 |
| JsonTree before/after diff | CONTEXT.md D-34 |
| CSV export ≤50k inline, >50k River job | CONTEXT.md D-35 |
| Cursor pagination | CONTEXT.md D-36 |
| No live tail | CONTEXT.md D-37 |
| Backup card thresholds + dot + history | CONTEXT.md D-46 |
| Three backup triggers (Run now button) | CONTEXT.md D-43 |
| Restore card (doc-only) | CONTEXT.md D-44 |
| Sidebar reorder | CONTEXT.md §Claude's Discretion + §Integration Points |
| Alert worker degraded banner | CONTEXT.md D-21, D-22 |
| ResponsiveDialog (UX-01) | Phase 1 + `responsive-dialog.tsx` |
| JsonTree reuse | Phase 4 D-19 + `metering-point/JsonTree.tsx` |
| URL-state (useSearchParams + zod) | Phase 3 D-15 / Phase 4 D-14 / Phase 5 D-05 |
| EmptyStateOnboarding three-stage card | Phase 4 D-21 |
| English-only / modal-first | UX-01, UX-02 |

---

## Open Questions

Non-blocking — planner routes back only if material.

1. **Audit "View details" link target.** `/health/detailed` is currently JSON-only (Phase 1 D-19).
   Recommendation: Phase 6 ships a minimal `/settings/diagnostics` page that pretty-prints
   `/health/detailed` JSON (read-only, admin-only). If not, banner link points to runbook.

2. **Viewers on `/settings/users`.** Show only own row, or 403? Recommendation: 403 with redirect
   to `/` for clean role separation.

3. **Bulk actions in rule library.** Bulk disable/enable via selection bar — recommend shipping;
   selection bar is a standard TanStack Table composition and rule libraries grow quickly.

4. **Anomaly Site-rollup.** Per-Site "4 of 6 meters eligible" chip on Site detail? Recommendation:
   ship MP-level only in Phase 6; per-Site rollup is v1.x polish.

5. **Mobile breakpoint for Users table card-collapse.** Recommendation: `<md` (640px), matching
   the existing dashboard pattern.

6. **Sidebar Alerts badge max display.** Cap at "99+" for layout stability. Recommendation: yes.

7. **Audit hover-preview vs click-to-expand.** Hover preview would surprise keyboard users and
   strain rendering. Recommendation: click-only.

---

*Phase: 06-alerts-users-audit-operational-hardening*
*UI contract drafted: 2026-05-12*

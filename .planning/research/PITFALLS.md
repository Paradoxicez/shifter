# Pitfalls Research

**Domain:** LoRaWAN-based water/electricity utility monitoring (ChirpStack-backed, self-hosted, single-tenant per install)
**Researched:** 2026-04-27
**Confidence:** HIGH for ChirpStack/LoRaWAN protocol behavior (Context7-style official docs verified); MEDIUM for utility-billing edge cases and operational concerns (synthesized from community forums + standards); LOW for some scale thresholds (estimated from analogous IoT systems).

> Read this before writing the roadmap. Every pitfall below has wrecked a real LoRaWAN/utility-monitoring project. Many are not obvious from a "build a CRUD app on top of an MQTT firehose" framing. Phase mapping at the bottom is load-bearing for ordering decisions.

---

## Critical Pitfalls

### Pitfall 1: Treating the Physical Device as the Source of Truth (Loses History on Meter Swap)

**What goes wrong:**
The schema models `device` as the primary entity. Telemetry rows reference `device_id`. When the operator replaces a failing meter, they get a new DevEUI, so they create a new device row — and the customer's consumption history for that location now lives split across two devices. Reports show a sudden drop to zero, charts have a discontinuity, year-over-year comparisons break. Operators "fix" this by manually editing rows or copying old data forward, corrupting it further.

**Why it happens:**
ChirpStack's data model is device-centric (DevEUI is the natural key). It is intuitive — and wrong — to mirror that 1:1. The metering domain's stable identity is the *metering point* (a tap, a panel breaker, a riser), not the physical box on the wall.

**How to avoid:**
- First-class `metering_point` entity with a stable internal ID, independent of any DevEUI.
- A device-binding join table (`metering_point_id`, `device_id`, `valid_from`, `valid_to`, `cumulative_offset_at_swap`) recording the history of which physical device served the metering point in each interval.
- All reports, charts, and alerts query by `metering_point_id`, never by `device_id`.
- The "replace meter" workflow is a single dialog: select metering point → enter old reading → enter new reading → record offset. Persist as a closed binding row + a new open binding row in one transaction.

**Warning signs:**
- Code path `getConsumption(deviceId)` exists in business logic.
- Foreign key from telemetry table directly to `device.id` with no `metering_point_id`.
- A "merge devices" admin tool gets requested in week 2.

**Phase to address:** Foundation / Domain Model phase. This must be designed before the first telemetry row is written. Retrofitting after launch is a multi-week migration.

---

### Pitfall 2: Cumulative-Reading Math That Breaks at Swap and at Rollover

**What goes wrong:**
Two related failure modes with the same root:
1. **Replacement:** New meter starts at 0 (or whatever the factory ships). The displayed cumulative consumption now shows "today's reading went from 14,832 m to 3 m" — leak alerts fire, customer panics, billing complains.
2. **Hardware rollover:** Some meters use a 32-bit (or smaller) counter. After enough years they wrap to 0. Naive `delta = current - previous` math returns a huge negative number. Daily aggregates show negative consumption, charts blow up, "anomaly" alerts fire.

**Why it happens:**
The naive model is "displayed cumulative = device-reported cumulative." It conflates two different quantities: what the device counter says (a register value) and what the customer should see (lifetime consumption at this metering point).

**How to avoid:**
- Store telemetry as the *raw* device-reported register value (never overwrite, never normalize on insert).
- Compute "displayed cumulative" as a function: `display = raw + offset_for_active_binding`, where the offset is the value that makes display continuous across binding boundaries.
- For deltas: always compute on a single binding window. Crossing a binding boundary uses `(end_of_old_binding_offsetted) → (start_of_new_binding_offsetted)` which is by construction continuous.
- For rollover detection: when `raw_t < raw_t-1`, treat it as either (a) a swap (if a binding boundary exists in the window — should not happen because swap is operator-driven) or (b) a counter wrap (advance the offset by the counter's modulus). Make this explicit, logged, and surfaced in the UI ("Counter rollover detected at 2027-03-14, offset adjusted by 2^32").
- Unit tests with synthetic series covering: clean swap, swap with concurrent in-flight uplink, rollover, swap-then-rollover, swap-with-overlapping-old-and-new-uplinks (yes, this happens).

**Warning signs:**
- Telemetry table has a column called `cumulative_consumption` that was computed at insert time.
- Any code that does `if (current < previous) { current = previous }` (silent clamp).
- Operator support tickets containing "the chart looks weird after we changed the meter."

**Phase to address:** Domain Model + Telemetry Ingestion phase. Get the offset math right before any reporting feature ships.

---

### Pitfall 3: Hard-Coding a Per-Vendor Decoder in Application Code (Codec Hell)

**What goes wrong:**
First customer has Vendor A water meters. Backend gets a switch on `device_profile_name` calling `decodeVendorA(bytes)`. Customer 2 has Vendor B *and* Vendor C, each in three firmware revisions. The switch grows. Each new vendor requires a backend deploy. Eventually a vendor ships a firmware update mid-fleet that changes byte 7 from "battery %" to "battery mV" and the decoder crashes silently.

**Why it happens:**
Decoding feels like backend logic, so it ends up there. ChirpStack's own codec system (per device profile, JS) gets bypassed because "we'll just call the raw payload endpoint."

**How to avoid:**
- **Decode in ChirpStack, not in Shifter.** ChirpStack v4 runs a QuickJS sandbox per device profile. The decoded `object` is delivered in the uplink event — Shifter consumes it.
- Shifter only ever speaks in *canonical fields* (e.g. `cumulative_volume_m3`, `flow_rate_lpm`, `battery_mv`, `rssi_dbm`). A `device_profile` row in Shifter maps each canonical field to where it lives in the decoded object (JSON-pointer or a simple key map).
- Maintain a Shifter-side **device profile catalog** (curated codecs + canonical mappings) seeded from the [TheThingsNetwork lorawan-devices repo](https://github.com/TheThingsNetwork/lorawan-devices) where possible. This is a ~1MB git submodule, not a backend dependency.
- The UI's "advanced" view renders the *full decoded object* generically (key/value tree from the JSON). The "normal" view renders the canonical fields the dashboard knows. New vendor → add a profile + mapping, no code change.
- Reject uplinks whose decoded object lacks the canonical fields the bound metering point requires, and surface this as a per-device health flag, not a silent drop.

**Warning signs:**
- A file named `decoders.ts` (or similar) with vendor-specific switch statements grows past 200 lines.
- "Add support for Vendor X" is a backend task instead of an admin-UI task.
- Decoded payloads are stored as opaque blobs in TimescaleDB ("we'll figure it out later").

**Phase to address:** Telemetry Ingestion phase. The boundary between "ChirpStack decodes" and "Shifter maps to canonical" must be set in stone before vendor #2 lands.

---

### Pitfall 4: False "Device Offline" Alerts From Normal LoRaWAN Behavior

**What goes wrong:**
A class A device on a 1-hour uplink interval misses two uplinks (gateway power blip, RF collision, duty-cycle backoff). Shifter fires "device offline" at 2x the interval. Customer gets paged at 3am for a meter that is fine. Trust in alerts collapses; real outages get ignored.

**Why it happens:**
LoRaWAN is *expected* to lose ~10% of uplinks under normal conditions (collisions, duty-cycle pacing, weather, gateway hand-off). Treating any missed uplink as offline is wrong by design. The alert designer thinks in TCP terms; the network is a lossy radio broadcast.

**How to avoid:**
- Define "offline" as **N consecutive missed expected uplinks**, where N ≥ 3 (per [TTN/MachineQ best practice](https://www.machineq.com/post/how-to-detect-connection-loss-on-a-lorawan-device-and-why-it-matters)).
- The "expected interval" is per-device-profile (or overrideable per device), not a global constant.
- Add a hysteresis grace period (e.g. `expected_interval × 1.5 × N`) — recovers cleanly without flapping.
- Distinguish "no uplink in window" (degraded) from "definitely offline" (3+ missed) in the UI with two visual states.
- Never use confirmed-uplink ack timeout as the offline signal — confirmed uplinks have their own retry semantics that mask connectivity.
- Suppress device-offline alerts globally when the *gateway* serving them is itself offline — a single alert per gateway is correct, not 200 alerts for the devices behind it.

**Warning signs:**
- The first deployment generates >20 offline alerts on day 1 for a fleet of <100 devices.
- Customer asks "can we just turn off the offline alerts?"
- Code path `if (now - lastUplink > expectedInterval) markOffline()` exists.

**Phase to address:** Alerting phase. Before alerting ships, the offline-detection rule must be parameterized per profile and gateway-aware.

---

### Pitfall 5: Choosing the Wrong Timestamp for Time-Series Storage

**What goes wrong:**
Telemetry is stored timestamped by `gateway_rx_time` (the gateway's clock when it received the packet). One gateway has bad NTP and is 90 seconds off. Another gateway buffers offline for 6 hours then floods. Continuous aggregates that already refreshed for "yesterday" don't see the late data — daily totals are silently wrong. Worse, a device heard by two gateways generates two rows in the hot path before deduplication.

**Why it happens:**
ChirpStack's uplink event includes several timestamps: `gateway_rx_time` (per RX info, gateway clock), `time` (server-received time), and optionally device-side time. Picking the wrong one is easy because they all *look* like "when the data happened."

**How to avoid:**
- **Persist the server-side ingest time as the authoritative `time` column on the hypertable.** This is monotonic, deduplicated by ChirpStack, and never goes backward. (See [LoRa Basics Station clock semantics](https://doc.sm.tc/station/time.html) — gateway clocks drift; do not trust them as primary.)
- Persist `gateway_rx_time` and any device-side time as separate columns for diagnostics and gateway-clock-skew monitoring, but never as the time-series key.
- When a gateway's `gateway_rx_time` deviates from server time by more than a configurable threshold (e.g. 60 s), surface a gateway-health warning. Do not silently accept it.
- For deduplication, rely on ChirpStack's built-in dedupe window — Shifter consumes one event per uplink. Verify by logging dedup-related fields and asserting no double-write per `(dev_eui, fcnt_up)`.
- Set TimescaleDB continuous-aggregate `end_offset` to ≥ a couple of expected-interval windows behind "now" so very-late uplinks (offline buffered gateway dump) still land before the bucket is finalized. Document the latency tradeoff.

**Warning signs:**
- Daily totals change retroactively when you re-run an aggregate.
- Two adjacent rows have the same `(dev_eui, fcnt_up)` with different timestamps.
- A buggy gateway's data shows up "in the future" relative to other gateways.

**Phase to address:** Telemetry Ingestion + TimescaleDB schema phase. Pick the canonical time column on day 1; changing it later is a hypertable rebuild.

---

### Pitfall 6: TimescaleDB Retention That Silently Wipes Continuous Aggregates

**What goes wrong:**
Engineer adds `add_retention_policy('telemetry_raw', INTERVAL '90 days')` to keep storage bounded. They also have a `daily_consumption` continuous aggregate with a 7-day refresh window. Six months in, a backfill runs, the aggregate refreshes a 6-month-old bucket, sees no source data (it was retained-out), and *deletes the materialized aggregate row*. Year-over-year reports vanish.

**Why it happens:**
The interaction between retention and continuous aggregates is non-obvious. Per [Tiger Data docs](https://www.tigerdata.com/docs/use-timescale/latest/continuous-aggregates/refresh-policies): "If the continuous aggregate policy window covers data that is removed by the data retention policy, the data will be removed when the aggregates for those buckets are refreshed." — i.e. refreshing the aggregate after retention drops the source data overwrites the materialization with empty.

**How to avoid:**
- The continuous aggregate `start_offset` MUST be ≤ the retention policy window. If retention is 90 days, the daily aggregate's `start_offset` must be ≤ 90 days.
- Or, use the "preserve aggregates" pattern: drop raw chunks older than the aggregate window, but never refresh the aggregate over already-dropped buckets.
- Have a separate, *longer*, retention policy on the aggregate hypertable itself (e.g. raw: 90 days, daily aggregate: 5 years, monthly aggregate: 20 years).
- Test this in the install-validation phase by simulating the retention boundary: ingest old data, advance time, verify aggregates survive.

**Warning signs:**
- A monthly chart that worked yesterday shows holes today.
- `SELECT count(*) FROM daily_consumption WHERE day < now() - '90 days'` returns 0 unexpectedly.
- Engineer says "I'll just refresh that bucket manually."

**Phase to address:** TimescaleDB schema phase. Retention + aggregates must be designed together, with explicit tests.

---

### Pitfall 7: ChirpStack v3 vs v4 API Mismatch (Picking the Wrong Target)

**What goes wrong:**
Backend integrates against ChirpStack v3 gRPC types (numeric IDs, `as_external/api/organization.proto`, REST shim). Customer #2 ships with ChirpStack v4 — UUIDs, no REST, refactored integration events. Half the codebase doesn't compile against the new client. API tokens are non-portable. The "support both modes" requirement turns into a fork.

**Why it happens:**
v3 is still widely deployed, but v4 has been the supported version for years. Tutorials, blog posts, and stale Stack Overflow answers reference v3. Per [v4 breaking changes](https://www.chirpstack.io/docs/v4-breaking-changes.html): gRPC structure mostly identical but client code is **not compatible**, IDs changed from numeric to UUID, REST API removed (gRPC-Web only), integration event payloads refactored, NetworkControllerService removed.

**How to avoid:**
- **Target ChirpStack v4 only.** Document this as a hard prerequisite for "external ChirpStack" mode (operator must run v4+).
- The bundled deployment ships v4 — never v3.
- Pin the ChirpStack version in compose to a known-tested minor version; bump deliberately as a tracked migration.
- Consume integration events via MQTT topic `application/{app_id}/device/+/event/up` (decoded `object` is now a struct, not a JSON string) — verify on first integration test.
- Use UUIDs end-to-end. Do not assume numeric IDs anywhere.
- For "external ChirpStack" mode, the install wizard must verify the version on connect and refuse to proceed against v3.

**Warning signs:**
- Code references `as_external/api/...` proto paths.
- Anywhere in the schema treats ChirpStack tenant/application/device IDs as integers.
- A blog post from 2021 is the "source" for an integration choice.

**Phase to address:** Foundation / ChirpStack Integration phase. Lock the version target before the gRPC client is generated.

---

### Pitfall 8: AS923 Frequency Sub-Plan Confusion (Thailand Specific)

**What goes wrong:**
Customer in Thailand. Operator selects "AS923" as the region in ChirpStack. Devices are in fact tuned for AS923-2 (different channel plan). Devices never join, or join intermittently from one frequency. Operator concludes "the gateway is broken." Day 3 of a deployment is spent on RF debugging that was a 30-second config issue.

**Why it happens:**
[AS923 has four sub-plans](https://www.chirpstack.io/network-server/features/regions/) (AS923-1, AS923-2, AS923-3, AS923-4) with overlapping but distinct channel frequencies. Thailand uses a specific allocation; not every AS923 device matches the regulator-mandated plan. ChirpStack supports all four but the operator must pick the right one.

**How to avoid:**
- Shifter's "create gateway" / "create device profile" flow surfaces region as an explicit, regulator-aware choice (e.g. "Thailand (AS923-x)" with the correct sub-plan defaulted), not a free-form string.
- Default region should be configurable per install (the customer's country) and act as the form default.
- Document AS923 sub-plan compatibility in the device-profile catalog: each profile in the catalog declares which sub-plans it supports.
- Surface a warning when a device is assigned to a profile whose sub-plan does not match the gateway's region.

**Warning signs:**
- Devices "join intermittently" or "join then go silent."
- Different device vendors require different region settings on the same gateway.
- Support ticket: "do I pick AS923 or AS923_2?"

**Phase to address:** ChirpStack Integration / Provisioning phase. The region picker and per-install default must exist before any device-profile flow ships.

---

### Pitfall 9: Realtime WebSocket Architecture That Reconnect-Storms

**What goes wrong:**
Backend pod restarts (deploy, OOM kill, scaling event). All connected dashboards reconnect at the same instant. TLS handshakes saturate CPU. Each client subscribes to "all metering points," fans out to a per-customer query, creates a thundering herd against the database. Connections fail. Clients retry immediately with no jitter. The system is unreachable for 60–120 seconds even though the underlying load is small.

**Why it happens:**
WebSocket implementations default to "subscribe to everything I might care about." Reconnect logic defaults to "retry immediately." Both are correct for a single-user laptop test and catastrophic at fleet scale. (See [WebSocket.org scaling guide](https://websocket.org/guides/websockets-at-scale/).)

**How to avoid:**
- **Subscribe per visible widget, not per user.** A dashboard open to a single site subscribes to that site's metering points only. Closing a widget unsubscribes.
- Server-side: subscriptions are filtered at the broadcast layer (or by topic in a pub/sub like NATS/Redis). Never broadcast all uplinks to all clients and let the client filter.
- Client-side reconnect: exponential backoff with jitter (e.g. `min(30s, 0.5s * 2^n) + rand(0, 1s)`). Mandatory. No exceptions.
- Server-side: TLS session resumption enabled to halve reconnect cost.
- Heartbeat / ping-pong to detect zombie clients within a defined window — stale subscribers must be reaped, not accumulated.
- Backpressure: if a client cannot keep up with the broadcast rate, drop oldest deltas and send a "snapshot" message; do not buffer unboundedly.
- Single-tenant scale is forgiving (rarely >50 concurrent dashboards), but the patterns above cost nothing extra to do correctly from day 1 and prevent cascade failures.

**Warning signs:**
- Restart causes >10s downtime even when the actual restart is sub-second.
- Memory creeps up over days and is freed by a restart.
- WebSocket connection count > 5x the number of logged-in operators.

**Phase to address:** Realtime Delivery phase. Reconnect strategy + topic-scoped subscription must be the default, not an optimization.

---

### Pitfall 10: Map View That Dies on a Site With Thousands of Devices

**What goes wrong:**
Customer has 4 sites, 2,500 metering points total, mostly clustered in two industrial zones. Dashboard map renders all 2,500 markers as DOM elements. Initial paint takes 6 seconds; pan/zoom are unusable on mobile; iPhone Safari crashes. "It works on my machine" with the demo's 12 devices.

**Why it happens:**
Leaflet's default marker is a DOM element. The DOM bottleneck appears around 1k–10k markers ([Leaflet performance discussion](https://medium.com/@silvajohnny777/optimizing-leaflet-performance-with-a-large-number-of-markers-0dea18c2ec99)). Real customers cross this threshold quickly when each panel breaker is a metering point.

**How to avoid:**
- Use clustering from the start: [`leaflet.markercluster`](https://github.com/Leaflet/Leaflet.markercluster) for Leaflet, or native source-clustering for MapLibre. Don't wait for a "performance issue" — design with it.
- Above ~5k markers, prefer MapLibre GL (WebGL canvas) over Leaflet (SVG/DOM). Shifter's stack decision should bias to MapLibre if device counts >1k are expected.
- Viewport culling: only fetch markers in the current map bounds + a small buffer. Server endpoint accepts a bounding box.
- For very dense sites: server-side clustering (return cluster summaries with counts, not individual markers) at low zoom levels.
- Mobile: limit initial zoom to a level where cluster count is <100 visible glyphs.

**Warning signs:**
- Map "freezes" on first load.
- DevTools shows >1000 DOM nodes under the map container.
- Mobile Safari memory warnings.

**Phase to address:** Map View / Site Visualization phase. The clustering + viewport-culling decision happens before the map ships, not after a perf complaint.

---

### Pitfall 11: Floor-Plan Pixel Coordinates That Drift With Image Replacement & Retina

**What goes wrong:**
Operator uploads a floor plan, places 30 devices. Three months later they upload an updated PDF (now exported at 300 DPI instead of 150). The pixel coordinates are now wrong by 2x — every device is in the upper-left quadrant. They re-place all 30. Six months later the same operator tests on an iPhone and the dots are 2x off again because of `devicePixelRatio` confusion.

**Why it happens:**
"Pixel coordinates" is ambiguous between (a) image-native pixels (the file's resolution), (b) CSS pixels (after browser scaling), and (c) physical pixels (after `devicePixelRatio`). [Per MDN](https://developer.mozilla.org/en-US/docs/Web/API/Window/devicePixelRatio), Retina screens have DPR 2.0+; mobile fractional DPRs introduce sub-pixel drift.

**How to avoid:**
- **Store coordinates as normalized fractions of the image** (`x_frac`, `y_frac` ∈ [0, 1]), not as pixel integers. Resolution-independent by construction.
- On image replacement, treat it as a new floor-plan version. Either (a) reuse the same device positions if normalized coords are still valid (image extent unchanged), or (b) require the operator to re-place / confirm with a side-by-side comparison.
- Render: `screen_x = x_frac × image_displayed_width`, `screen_y = y_frac × image_displayed_height`. Always derive from the *displayed* image dimensions, not the file's intrinsic pixel size.
- For multi-floor buildings: a `floor` entity with an image + an ordering, not a single multi-image blob.
- Image rotation is not a feature — require the operator to upload a correctly-oriented image. Optional: surface a one-time "rotate 90°" tool that re-saves the canonical image, then ALL placements are remapped (`x' = 1 - y`, `y' = x` for 90° CW).
- Image storage: never inline in the database. Use a known volume mount path; document backup separately.

**Warning signs:**
- Coordinates stored as `INT pixel_x, INT pixel_y` columns.
- Displayed dot positions move when window is resized.
- Mobile users report "the dots are wrong."

**Phase to address:** Floor-Plan Placement phase. Schema decision (`x_frac` not `pixel_x`) is permanent.

---

### Pitfall 12: Bulk Import That Partially Succeeds and Leaves the System Inconsistent

**What goes wrong:**
Operator uploads CSV with 200 devices. Row 47 has a duplicate DevEUI. Rows 1–46 commit, row 47 errors, rows 48–200 are skipped. Operator deletes the bad row from CSV, re-uploads — now rows 1–46 are duplicate-DevEUI errors, the rest commit. The operator has manually reconciled half the import and lost confidence in the bulk flow.

**Why it happens:**
Streaming row-by-row insertion with no transactional boundary. ChirpStack's per-device API call is the natural unit; wrapping the whole import in a backend transaction is harder when the side effect is in another system (ChirpStack).

**How to avoid:**
- **Two-phase: validate-all, then commit-all.** Phase 1: dry-run that reports every error (duplicate DevEUI in CSV, duplicate against existing devices, missing required fields, malformed AppKey, unknown device profile, invalid metering-point binding). Phase 2: only proceeds if dry-run is clean.
- Validation report is downloadable / inline in the UI — operator fixes the CSV and re-runs dry-run as many times as needed.
- Commit phase: best-effort with strict ordering (devices first, then bindings), with a per-row outcome log. If the ChirpStack API call for row 47 fails mid-commit, log it and continue, then surface a per-row report showing exactly which rows committed and which didn't.
- Idempotency: re-running an identical CSV is a no-op (matched on DevEUI). The operator can safely re-upload after fixing one row.
- DevEUI normalization: case-insensitive, hex-only, 16 chars. Reject "12-34-56-AB-CD-EF-12-34" with a clear message; better, accept and normalize.
- Provide a downloadable CSV template with required columns and example rows.

**Warning signs:**
- Bulk-import support tickets contain phrases like "I had to delete some devices and re-import."
- The bulk-import endpoint is a single `POST` with no preview step.
- Errors are shown only after partial commit.

**Phase to address:** Provisioning / Bulk Import phase. Dry-run is a first-class feature, not an MVP cut.

---

### Pitfall 13: OTAA / ABP Confusion and Endianness Footguns

**What goes wrong:**
Operator enters DevEUI from the device sticker. ChirpStack rejects the join. Operator types it backward (LSB instead of MSB or vice-versa). Now the device joins on one stack and not another. Operator switches to ABP "to skip the join hassle," device works for a week, then power-cycles and the frame counter resets to 0 — ChirpStack rejects all further uplinks because counters must be monotonic.

**Why it happens:**
- DevEUI/JoinEUI/AppKey have inconsistent endianness conventions across vendors (some print MSB-first on the sticker, some LSB-first; ChirpStack's web UI expects MSB-first).
- ABP is "easier" in tutorials and "harder" in production: frame counters must persist across power cycles, otherwise OTAA-equivalent reliability is impossible.
- Per [TTN device activation](https://www.thethingsnetwork.org/docs/lorawan/end-device-activation/): "Do not implement ABP, but if you must, also support OTAA."

**How to avoid:**
- **OTAA-first as the default UX.** ABP is available but flagged as "not recommended unless required by the device" with an explanatory tooltip.
- Provisioning UI explicitly labels DevEUI / JoinEUI / AppKey fields with their expected format and provides a "paste sticker text" parser that handles both endiannesses (with a "is this correct?" preview before save).
- Validation: DevEUI is exactly 16 hex chars after normalization; AppKey is 32; reject with a clear error otherwise.
- For ABP: if the operator chooses it, surface a warning that frame-counter resets will require manual intervention.
- ChirpStack-side: ensure the device profile is configured with `Disable frame-counter validation` only as a last-resort manual flag, never as a default.
- Document that some vendors ship the DevEUI on the sticker in big-endian and others in little-endian — the parser must handle both, with operator confirmation.

**Warning signs:**
- Support tickets containing "the DevEUI on the sticker doesn't work."
- Anyone reaches for "Disable frame-counter validation" to "fix" a device.
- ABP is the default in any sample/template.

**Phase to address:** Device Provisioning phase. The sticker-parsing UX and OTAA-default policy go in early.

---

### Pitfall 14: Two-Role Permission Model That Fails Mid-Customer

**What goes wrong:**
Customer org has an electrician who needs to provision new devices but should not see consumption data. Or a finance person who needs reports but must not change device config. The "admin / viewer" split forces a binary choice: either give them admin and accept the risk, or give them viewer and watch them ask the operator to do every provisioning task. Three months in the customer asks for "just one more role."

**Why it happens:**
Two-role models are cleanly enough for small/single-purpose tools. Multi-stakeholder utility monitoring (operator, technician, finance, contractor, auditor) has natural role overlap that a binary admin/viewer cannot express.

**How to avoid:**
- Accept the v1 constraint (admin/viewer) but design the schema for forward extension: `user_role` is a column, not a boolean. Permission checks call a single function `can(user, action, resource)`, not `user.is_admin`.
- Predefine action constants (`device.create`, `device.update`, `report.view`, `user.manage`, `gateway.configure`). The two v1 roles are simply two preset bundles of these constants. Adding a "technician" role later is a config change, not a code refactor.
- Document explicitly in PROJECT.md that this schema design is forward-compatible — protects against the "we'll just add a third boolean column" anti-pattern when the request lands.
- Audit log every state-changing action (who did what, when) — this is independent of role granularity and a customer expectation regardless of how many roles exist.

**Warning signs:**
- Permission checks scattered as `if (user.role === 'admin')` throughout the codebase.
- The first customer asks for a third role during onboarding.
- "Just give them admin" is the workaround for a feature gap.

**Phase to address:** Auth & Identity phase. The shape of `can(user, action, resource)` is set early; the role bundles can be expanded later without refactoring.

---

### Pitfall 15: Self-Hosted Operations Gaps (Backups, Upgrades, Image Volumes, Logs)

**What goes wrong:**
Six months after deployment, customer's disk fills up (image uploads, raw uplink logs, no rotation). Or: the host crashes, customer asks for a restore, operator has no documented backup policy. Or: ChirpStack v4.X+1 ships, security advisory, no upgrade path documented. Or: the operator who installed it left, the new operator can't find the secrets file. Each of these turns into an emergency.

**Why it happens:**
Self-hosting moves operational responsibility onto the developer. SaaS conveniences (managed backups, log rotation, vault-stored secrets, blue/green deploys) are not free in a Compose stack.

**How to avoid:**
- **Backups:** A scripted nightly `pg_dump` (TimescaleDB-aware — see [Tiger Data logical backup docs](https://www.tigerdata.com/docs/self-hosted/latest/backup-and-restore/logical-backup), do not use `pg_restore -j`) plus an `rsync` of the floor-plan image volume. Backup destination is configurable (local path, S3-compatible URL). Restore script is part of the install kit and tested in CI.
- **Image storage volume:** Sized in the install kit (default 50 GB), with a soft warning at 70% full. Document max image size per upload (e.g. 10 MB) to bound growth.
- **Log rotation:** All container logs go to a host-rotated logging driver (`json-file` with `max-size` + `max-file`, or journald). No "logs filling up disk" tickets.
- **Secrets:** File-based via Compose `secrets` (per [Docker docs](https://docs.docker.com/compose/how-tos/use-secrets/)), not in the compose `.env`. Document the location, ownership, and recovery path. Never commit `.env` or secret files.
- **Upgrade path:** A documented upgrade procedure: backup → pull new images → run migrations → restart. Tested per release. Include rollback (restore from backup, pin previous image tags). The compose file pins specific image tags, never `:latest`.
- **Telemetry retention default:** Conservative (e.g. raw 90d, daily-agg 5y, monthly-agg 20y) and parameterized in install config — different customers will want different windows.
- **Health endpoint:** A single `/health` that reports DB, ChirpStack, MQTT, disk free, last uplink age — operator can monitor with a $5/month uptime checker.

**Warning signs:**
- "Backup procedure" is a wiki page no one has tested.
- `docker compose logs` is the only way to find out what happened yesterday.
- The version of every customer's install is "whatever they happened to install."

**Phase to address:** Operational Hardening phase (cross-cutting; must ship before first paying customer). Each install is a tiny ops surface; the install kit is the deliverable.

---

### Pitfall 16: Codebase Drift Across Customer Installs

**What goes wrong:**
Customer A asks for a custom report tweak. Engineer ships it on the `customer-a` branch. Customer B finds a bug; fix lands on `main`. Customer A's branch never gets the fix. Six months and three customers in, no two installs are running the same code; security patches require N independent deploys; bugs reported by customer C "have already been fixed" but only on a branch they don't have. (Per [Qrvey multi-tenant deployment guide](https://qrvey.com/blog/multi-tenant-deployment/): single-tenant deploys "creep into" version drift.)

**Why it happens:**
Single-tenant per install + single codebase is achievable, but only with discipline. The natural pull is to fork "just for this customer."

**How to avoid:**
- **One main branch. No customer branches.** Customer-specific behavior is a config flag, a feature toggle, or a database setting — never a code fork.
- Ship the same image to every customer. Differences are in the `compose.yaml`, the secrets file, and the seed data — not in the binary.
- Versioning: every install reports its current version (visible in UI footer + `/health`). A central register (even a spreadsheet) tracks "customer X is on version Y."
- Customer-requested features: either go into the product (everyone gets them, off by default if needed) or are explicitly declined. "We'll just add it for you" is the failure mode.
- Upgrade cadence: regular cadence (monthly?) with a documented changelog. Customers know when they're getting upgrades.
- Migrations are forward-only and idempotent. Restoring a backup from version N onto version N+1 must work.

**Warning signs:**
- A `customer-x` branch exists.
- A feature flag is named after a customer.
- `git log` on main has gaps because work happens on side branches.

**Phase to address:** Foundation / Release Engineering. The "one branch, feature-flagged differences" rule is set at project start.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Decode payloads in Shifter backend (skip ChirpStack codecs) | Faster initial integration; backend devs avoid learning ChirpStack | Codec hell at vendor #2; backend deploys for new vendors; firmware-revision regressions | Never |
| Use `device_id` directly in telemetry FK (skip metering-point indirection) | Simpler initial schema | Lost history on first meter swap; manual data fixup; rewriting the schema mid-life | Never |
| Store cumulative consumption (delta-based) at insert time | Simpler reads | Cannot fix offset retroactively; rollover bugs are permanent | Never |
| Single global "expected interval" for offline detection | One config knob | Pages on day 1; trust in alerts collapses | Spike/demo only |
| Render all map markers as DOM elements | Trivial implementation | Mobile crash at first real customer | Demo with <100 devices |
| `pixel_x INT, pixel_y INT` for floor-plan placement | Easy to reason about | Resolution change requires re-placement; mobile drift | Never |
| Bulk import as a single streaming POST | Smaller PR | Partial-commit incidents on every customer onboarding | Never |
| Customer-branch for one-off tweaks | "Just for this one customer" | Permanent drift, security patch fan-out, support nightmare | Never |
| `:latest` image tags in compose | Simpler updates | Surprise upgrades; non-reproducible installs; rollback impossible | Never |
| Skip the install validation step for ChirpStack v3 vs v4 | Faster onboarding | First v3-only customer breaks the whole flow | Never |
| Two roles hardcoded as `if (is_admin)` checks | Fast first auth | Refactor on first "we need a third role" request | MVP only if `can()` is the public API |
| Defer backup script to "after launch" | Ship faster | First disk failure = data loss = customer churn | Never |

---

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| ChirpStack gRPC | Generating client from v3 protos | Generate from v4 protos; pin commit; regenerate as part of the upgrade procedure |
| ChirpStack uplink stream | Polling REST or gRPC for new uplinks | Subscribe to the MQTT topic `application/{app_id}/device/+/event/up`; consume the JSON event with the decoded `object` field |
| ChirpStack auth | Reusing v3 API tokens after upgrade | v3 tokens are not v4-compatible; regenerate; never store tokens in code |
| ChirpStack downlink | Expecting confirmed-downlink retry on the network server | ChirpStack does not auto-retry confirmed downlinks; on `ack: false`, your application is responsible for re-queueing |
| ChirpStack downlink (Class A) | Queueing downlink *after* the uplink arrives | Class A devices only have a downlink window immediately after their uplink; queue the downlink before the next expected uplink, not after |
| ChirpStack device profile | Free-text region selection | Drive from the regulator-aware region picker (e.g. AS923-x for Thailand specifically) |
| TimescaleDB | Using `pg_restore -j` on a hypertable backup | Single-threaded restore only; `-j` corrupts the TimescaleDB catalog |
| TimescaleDB | Creating a continuous aggregate before defining retention | Define retention + aggregate together; aggregate's `start_offset` ≤ retention window |
| MQTT | Subscribing to `#` (firehose) per dashboard client | Server is the only MQTT subscriber; dashboards subscribe to scoped WebSocket topics fed from the server |
| OpenStreetMap tiles | Hammering the OSM public tile server in production | Self-host tiles or use a permissive provider; document the tile-server URL in install config |
| Image upload | Storing in the database | Filesystem volume + path reference in DB; backup volume separately |
| Floor-plan image | Trusting client-uploaded EXIF orientation | Re-encode on upload to a canonical orientation; or refuse anything that's not landscape JPEG/PNG |

---

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| All-markers-in-DOM map rendering | Map freezes on load; mobile crashes | Cluster + viewport cull from day 1 | ~1k devices on a single map |
| Full-fleet WebSocket fan-out | Memory creep; reconnect storms; CPU spikes on deploy | Topic-scoped subscriptions; jittered reconnect | ~100 concurrent dashboards or 10k metering points |
| `SELECT * FROM telemetry WHERE device_id = X ORDER BY time DESC LIMIT 1` per device per page-render | Dashboard load time grows linearly with device count | Materialized "latest reading per metering point" table; refresh on uplink | ~500 metering points |
| Continuous aggregate refresh on the hot bucket | Ingest slows; aggregates are inaccurate near "now" | `end_offset` ≥ a couple of expected intervals behind now | Continuous, but most visible at >100 uplinks/minute |
| Synchronous decode of every uplink in the ingest path | Backpressure when MQTT volume spikes | Decode is in ChirpStack; Shifter's job is map+persist; queue if needed | Burst uplink (e.g. fleet reboot) at any scale |
| Unbounded raw telemetry retention | TimescaleDB disk grows unbounded; backups exceed window | Retention policy on raw, longer retention on aggregates, both tested | Year 1 for active fleets |
| Loading every site's devices on map zoom-out | Network spike + render spike | Server-side bbox query; cluster summaries below threshold zoom | 5k+ devices |
| PDF report generation in the request thread | UI hangs on "Export" click; timeouts | Background job + email/download link (or in-UI polling) | Reports >1 page or >1k rows |
| Unindexed `WHERE time > X` on a non-hypertable | Sequential scan; minutes-long queries | All telemetry tables are hypertables; queries time-bounded | Any table > 1M rows |

---

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| AppKey / NwkSKey / AppSKey embedded in QR codes or floor-plan exports | Key leak → device impersonation → false consumption data | Never include root keys in any export; redact in UI for non-admin |
| ChirpStack API token in `.env` committed to git | Full ChirpStack control by anyone with repo access | File-based Compose secrets; `.env` for non-secret config only; `.gitignore` enforced |
| MQTT broker exposed without auth | Anyone on the network can publish forged uplinks | MQTT requires user/pass + TLS; bind to `127.0.0.1` if both ChirpStack and Shifter share the host |
| Plain HTTP for the dashboard | Session cookie / password sniffable on customer LAN | TLS in front (Caddy/Traefik in compose), with auto-renewing certs or a self-signed CA documented |
| Letting viewers see device AppKey/credentials | Even read-only roles can leak keys | Admin-only fields server-side; do not even send to viewer clients |
| Default admin password in install kit | First customer never changes it | Install script forces a password set on first boot; no default credentials |
| Frame-counter validation disabled to "fix" a device | Replay attacks possible; stale joiner can re-inject old uplinks | Surface as an explicit, audit-logged, expiring override; never a permanent setting |
| No audit log for device/user changes | Cannot answer "who changed this and when" — first thing customer asks after an incident | Append-only audit log table for all CRUD on devices, users, gateways, alerts |
| Running ChirpStack and Shifter as root in containers | Container escape → host compromise | `USER` directive in Dockerfiles; document non-root in install kit |
| No rate-limit on login endpoint | Brute force against local accounts | Rate-limit by IP + by username; log failures; lockout after N attempts |
| Allowing arbitrary image upload formats / sizes | DoS via huge uploads; SVG XSS; malformed image parser RCE | Whitelist JPEG/PNG; max size; re-encode through a known-safe path; serve from a domain that does not run JS |

---

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Showing ChirpStack-native terminology (DevEUI, AppEUI, fPort) on every screen | Operator feels like they're using ChirpStack; no value-add | Use domain terminology (Meter ID, Site, Reading); expose ChirpStack identifiers only in an "advanced" expandable section |
| Multi-step ChirpStack flow exposed verbatim ("create device profile → create application → register device → activate") | Operator gives up; supports tickets multiply | Single dialog: pick a vendor profile from catalog → enter sticker info → done. Backend orchestrates the multi-step calls |
| All metrics in one chart | Visual noise; the one number that matters (today's consumption) is buried | Default view: today's consumption. "Advanced" tab for the firehose |
| "Device offline" alerts indistinguishable from "consumption anomaly" alerts in feed | Operator desensitized; misses real alerts | Categories with distinct colors + "muted" / "snoozed" states; gateway-down suppression |
| Replacing a meter requires deleting the old device and creating a new one | Loses history; operator avoids the workflow | One-step "Replace meter" dialog that records the binding swap + offset transparently |
| Mobile dashboard that's "just the desktop, smaller" | Charts unreadable; clicks miss; forms unusable | Mobile-first card layout for the common views (latest readings, alerts, site quick-glance) |
| No empty / first-run state | New customer sees a blank dashboard, doesn't know what to do | Install kit seeds a "Create your first site" wizard; per-section empty states with primary CTAs |
| Confusing "Site lat/lng" (real-world) with floor-plan position (pixel) | Operator places a device on the floor plan but it doesn't show on the map (or vice versa) | Two distinct, clearly-labeled fields; the UI explains the difference inline |
| Reports as a giant table | Useful for export, useless for understanding | Default to charts; table is the "show details" expand |
| Ambiguous timezone | Daily report for "April 26" includes midnight-to-midnight in *which* timezone? | Operator-selectable timezone per install (defaults to system); displayed prominently on every report; stored in PROJECT.md as a Key Decision |

---

## "Looks Done But Isn't" Checklist

- [ ] **Device provisioning:** Often missing duplicate-DevEUI detection across customers' fleets — verify a re-import of the same CSV is idempotent and an EUI collision is a clear error.
- [ ] **Meter replacement:** Often missing the offset-at-swap math — verify cumulative-consumption chart is continuous across a synthetic swap event in a test.
- [ ] **Bulk import:** Often missing the dry-run preview — verify the operator can see all errors before committing.
- [ ] **Offline detection:** Often missing the gateway-down suppression — verify killing a gateway in test does not produce per-device alerts.
- [ ] **Continuous aggregates:** Often missing retention-aware refresh — verify a 6-month-old daily aggregate survives a hypertable retention drop.
- [ ] **Floor plan placement:** Often missing the resolution independence — verify dot positions are correct on a Retina mobile after replacing the image with a 2x-resolution version.
- [ ] **WebSocket reconnect:** Often missing jitter — verify a backend restart with 50 connected clients does not produce a thundering-herd reconnect.
- [ ] **PDF/CSV export:** Often missing timezone in headers — verify exported reports show the timezone explicitly.
- [ ] **Map clustering:** Often missing the perf budget — verify map renders <2s on mobile with 2,500 markers.
- [ ] **Backup script:** Often missing actual restore test — verify a restore of last night's backup boots a clean instance with all data intact.
- [ ] **Install validation:** Often missing the ChirpStack version check — verify install refuses to proceed against ChirpStack v3 or unknown versions.
- [ ] **Anomaly alerts:** Often missing the "first 24h is silent" warmup — verify a fresh device does not fire anomaly alerts on its first day before a baseline exists.
- [ ] **Region picker:** Often missing AS923 sub-plan disambiguation — verify a Thailand operator can pick the regulator-correct sub-plan without consulting external docs.
- [ ] **Audit log:** Often missing actor on automated changes — verify system-driven changes (e.g. continuous aggregate refresh) are tagged "system" not blank.
- [ ] **Secrets:** Often missing rotation procedure — verify the install kit has a documented "rotate ChirpStack API token" runbook.

---

## Recovery Strategies

When pitfalls occur despite prevention, how to recover.

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Pitfall 1 (device-as-source-of-truth) | HIGH | Backfill: create metering_point rows from device rows; create binding rows with `valid_from = device.created_at`; rewrite all reads to query by metering_point_id; keep device_id columns for compatibility during transition. Multi-week migration. |
| Pitfall 2 (rollover/swap math) | MEDIUM (if raw data preserved) / HIGH (if normalized) | If raw register values are stored: recompute offsets retroactively per binding; rebuild aggregates. If only normalized cumulative was stored: data is partially lost; restore from backup pre-bug; reconstruct best-effort. |
| Pitfall 3 (codec hell) | MEDIUM | Move decoders to ChirpStack profiles per device; build a one-time backfill that re-decodes archived raw bytes (which is why archived raw bytes should always be kept). |
| Pitfall 4 (false offline alerts) | LOW | Tune N consecutive misses + grace period in config; deploy. No data loss. |
| Pitfall 5 (wrong timestamp) | HIGH | Hypertable rebuild keyed on the new time column; rebuild aggregates; data is preserved but migration is non-trivial. |
| Pitfall 6 (retention wiped aggregates) | HIGH (data loss) | Restore from backup if available; otherwise the affected aggregate buckets are gone. Adjust retention policy and add explicit aggregate-side retention immediately. |
| Pitfall 7 (v3/v4 mismatch) | MEDIUM | Lock target to v4; refuse v3 customers in install validation; do not fork the codebase. |
| Pitfall 8 (AS923 sub-plan) | LOW | Reconfigure region in ChirpStack + device profile; redeploy; devices re-join. |
| Pitfall 9 (WebSocket storms) | LOW (operational) | Add jitter to client reconnect; deploy. Server-side rate limit added separately. |
| Pitfall 10 (map performance) | LOW–MEDIUM | Add clustering + bbox filtering; if Leaflet, evaluate MapLibre swap. |
| Pitfall 11 (floor-plan drift) | MEDIUM | Migrate stored pixel coords to fractions using known image dimensions at upload time; if not stored, operator re-places once. |
| Pitfall 12 (partial bulk import) | LOW | Idempotent re-run + dry-run validator; clean up partial commits via the audit log. |
| Pitfall 13 (OTAA endianness) | LOW | Add the bidirectional parser + preview; existing devices unaffected. |
| Pitfall 14 (two roles too coarse) | LOW (if `can()` API exists) / MEDIUM (if `is_admin` scattered) | Add a third role bundle; refactor scattered checks if needed. |
| Pitfall 15 (ops gaps) | HIGH (post-incident) / LOW (pre-incident) | Pre: ship the install kit with backups, log rotation, secrets, upgrade docs. Post-incident: triage the specific gap (e.g. recover from disk-full by adding rotation + freeing space). |
| Pitfall 16 (codebase drift) | HIGH | Reconverge: rebase customer branches onto main; bring all installs to current version; institute the no-fork rule going forward. |

---

## Pitfall-to-Phase Mapping

How roadmap phases should address these pitfalls. (Phase names are suggestions; concrete naming is the roadmap author's call.)

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1. Metering-point as source of truth | Foundation / Domain Model | Schema review: no FK from telemetry to device; binding history queryable |
| 2. Rollover & swap math | Domain Model + Telemetry Ingestion | Synthetic-data test covering swap, rollover, swap+rollover; charts continuous |
| 3. Codec hell | ChirpStack Integration / Telemetry Ingestion | Decoders live in ChirpStack profiles; canonical-field mapping is admin-UI driven; new vendor onboarded with zero backend changes |
| 4. False offline alerts | Alerting | Day-1 alert noise on a 100-device fleet < 5 alerts; gateway-down suppresses descendant alerts |
| 5. Wrong timestamp | Telemetry Ingestion / TimescaleDB schema | Time column choice documented in PROJECT.md; idempotent dedup on `(dev_eui, fcnt_up)` |
| 6. Retention wipes aggregates | TimescaleDB schema / Operations | Test: retention boundary + aggregate refresh; aggregates survive |
| 7. ChirpStack v3/v4 | Foundation / ChirpStack Integration | Install validation rejects v3; client generated from v4 protos pinned to a tested commit |
| 8. AS923 sub-plan | ChirpStack Integration / Provisioning | Thailand-default install picks AS923-correct sub-plan; region picker is regulator-aware |
| 9. WebSocket reconnect storm | Realtime Delivery | Backend-restart load test with 50 clients; server CPU recovers <10s |
| 10. Map performance | Map View | 2,500-marker map renders <2s on mid-tier mobile |
| 11. Floor-plan resolution drift | Floor-Plan Placement | Coords stored as fractions; resolution-swap test verifies dots stay correct |
| 12. Partial bulk import | Provisioning / Bulk Import | Two-phase dry-run+commit; idempotent re-run on identical CSV |
| 13. OTAA endianness / ABP | Device Provisioning | Sticker parser handles both endiannesses with operator preview; OTAA is default |
| 14. Two-role coarseness | Auth & Identity | `can(user, action, resource)` is the only call site; roles are config bundles |
| 15. Ops gaps (backup/upgrade/secrets/logs) | Operational Hardening | Install kit includes scripted backup+restore (CI-tested), log rotation, file-based secrets, pinned image tags, upgrade runbook |
| 16. Codebase drift | Foundation / Release Engineering | "No customer branches" rule documented; all installs report version in `/health`; central version register exists |

---

## Sources

### ChirpStack
- [ChirpStack v4 breaking changes](https://www.chirpstack.io/docs/v4-breaking-changes.html)
- [ChirpStack v3 to v4 migration](https://www.chirpstack.io/docs/v3-v4-migration.html)
- [ChirpStack gRPC API](https://www.chirpstack.io/docs/chirpstack/api/grpc.html)
- [ChirpStack devices documentation](https://www.chirpstack.io/docs/chirpstack/use/devices.html)
- [ChirpStack device profiles](https://www.chirpstack.io/docs/chirpstack/use/device-profiles.html)
- [ChirpStack MQTT integration](https://www.chirpstack.io/docs/chirpstack/integrations/mqtt.html)
- [ChirpStack regions](https://www.chirpstack.io/network-server/features/regions/)
- [ChirpStack channel reconfiguration](https://www.chirpstack.io/docs/chirpstack/features/channel-configuration.html)
- [ChirpStack device classes (downlink semantics)](https://www.chirpstack.io/docs/chirpstack/features/device-classes.html)
- [ChirpStack confirmed downlink retry discussion (forum)](https://forum.chirpstack.io/t/confirmed-downlinks-and-a-retry-policy/7867)
- [ChirpStack downlink scheduler / device lock issue](https://github.com/chirpstack/chirpstack/issues/324)

### LoRaWAN Protocol
- [TTN End Device Activation (OTAA / ABP)](https://www.thethingsnetwork.org/docs/lorawan/end-device-activation/)
- [TTN Best Practices for LoRaWAN devices](https://www.thethingsindustries.com/docs/hardware/devices/concepts/best-practices/)
- [TTN Regional Parameters](https://www.thethingsnetwork.org/docs/lorawan/regional-parameters/)
- [LoRa Alliance Regional Parameters v1.0.3](https://lora-alliance.org/wp-content/uploads/2020/11/lorawan_regional_parameters_v1.0.3reva_0.pdf)
- [LoRa Alliance Payload Codec API press release](https://lora-alliance.org/lora-alliance-press-release/lora-alliance-announces-new-lorawan-payload-codec-api-feature-that-accelerates-device-onboarding-to-enable-massive-iot/)
- [TheThingsNetwork lorawan-devices repository](https://github.com/TheThingsNetwork/lorawan-devices)
- [LoRa Basics Station clock synchronization & timestamps](https://doc.sm.tc/station/time.html)
- [Semtech LoRaWAN device activation guide](https://lora-developers.semtech.com/documentation/tech-papers-and-guides/lorawan-device-activation/device-activation/)
- [Centennial Software — DevEUI/AppEUI/JoinEUI/AppKey](https://www.centennialsoftwaresolutions.com/help/deveui-appeui-joineui-and-appkey/)
- [MachineQ — detecting LoRaWAN connection loss](https://www.machineq.com/post/how-to-detect-connection-loss-on-a-lorawan-device-and-why-it-matters)
- [MachineQ — LoRaWAN packet loss & delays playbook](https://www.machineq.com/post/a-playbook-to-ensure-lorawan-r-network-reliability-6-strategies-to-mitigate-packet-loss-delays)

### TimescaleDB / Operations
- [Tiger Data — refresh policies for continuous aggregates](https://www.tigerdata.com/docs/use-timescale/latest/continuous-aggregates/refresh-policies)
- [Tiger Data — continuous aggregates + data retention interaction](https://github.com/timescale/docs/blob/latest/use-timescale/data-retention/data-retention-with-continuous-aggregates.md)
- [Tiger Data — logical backup with pg_dump and pg_restore](https://www.tigerdata.com/docs/self-hosted/latest/backup-and-restore/logical-backup)
- [Docker Compose secrets](https://docs.docker.com/compose/how-tos/use-secrets/)

### Realtime, Maps, Misc
- [WebSocket.org — WebSockets at scale](https://websocket.org/guides/websockets-at-scale/)
- [WebSocket.org — connection limits and bottlenecks](https://websocket.org/guides/connection-limits/)
- [Leaflet.markercluster GitHub](https://github.com/Leaflet/Leaflet.markercluster)
- [Optimizing Leaflet performance with many markers](https://medium.com/@silvajohnny777/optimizing-leaflet-performance-with-a-large-number-of-markers-0dea18c2ec99)
- [MDN — Window.devicePixelRatio](https://developer.mozilla.org/en-US/docs/Web/API/Window/devicePixelRatio)
- [Qrvey — multi-tenant deployment guide (single-tenant drift discussion)](https://qrvey.com/blog/multi-tenant-deployment/)

---
*Pitfalls research for: LoRaWAN water/electricity utility monitoring, ChirpStack-backed, self-hosted, single-tenant per install (Shifter)*
*Researched: 2026-04-27*

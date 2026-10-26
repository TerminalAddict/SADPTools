# SADPTools — Advanced Installer Features (v0.5 Roadmap)

**Implementation specification for Codex CLI**  
**Repository:** <https://github.com/TerminalAddict/SADPTools>  
**Working directory:** `/home/paul/git-repos/SADPTools` on Sparx  
**Primary release target:** Windows 10/11 AMD64  
**Secondary release target:** Linux  
**Status:** Proposed functionality; implementation and hardware support must be verified

---

## 0. Instructions to Codex — read this first

You are extending **the existing SADPTools project**, a Go + Wails v3 + Bootstrap 5 + vanilla JavaScript desktop application and Go CLI. **Do not recreate or replace the application.**

1. Read `AGENTS.md`, `README.md`, the existing `docs/` architecture/validation materials, the `Makefile`, and the actual source tree **before writing code**. The remote repository may have changed since this roadmap was authored; the local working tree is authoritative. Preserve uncommitted changes.
2. Inventory already-present capabilities. In particular, **reuse and extend** existing site inventory, encrypted credential vault, ONVIF, live-view window and playback, health audit, reporting, SNMP/switch mapping, diagnostics, CLI, and device identity mechanisms. Do not build competing versions of those services.
3. Produce a short gap analysis mapping each feature below to existing services/files, missing capabilities, external dependencies, device compatibility risks, and test strategy. Place it in `docs/ADVANCED_FEATURES_GAP_ANALYSIS.md`.
4. Implement in **small, independently testable milestones**, beginning with the recommended Phase A below. The list is a roadmap, **not** an instruction to implement all ten in one unreviewable commit. After the gap analysis, continue with the first feasible milestone, making genuine code changes rather than stopping at a plan.
5. Keep protocol clients, business logic, site persistence and tests independent of Wails; expose minimal service methods to the existing GUI. Keep Windows/Linux parity. Reuse the existing CLI wherever it benefits field diagnostics.
6. Prefer read-only operations first. Any operation that modifies a camera/NVR must be separately authorised by the user, include a preflight preview and safety checks, and never blindly retry an operation with an uncertain outcome.
7. **Never perform write operations on actual customer cameras or NVRs during automated testing.** Use fixtures, fake clients or explicitly authorised test devices. No unsolicited network scans or remote connections outside user-selected networks/sites.
8. Keep changes compatible with existing installation, Windows cross-build, Linux packaging, updater, credential management, tests and third-party licensing. Do not install privileged system packages or push to GitHub without explicit user instruction.
9. Separate **implemented**, **unit/integration tested**, **build verified**, and **physically validated** in progress reports. Do not claim hardware interoperability solely from successful builds or mocked tests.

### Product goal

Make SADPTools the offline-first toolkit an installer uses from discovery and commissioning, through troubleshooting and customer handover. A technician should be able to select a saved site, inspect devices/NVRs, see live video and events, validate configuration, generate a handover report, and return months later to troubleshoot changes.

### Existing capabilities — verify rather than assume

The public README at the time this roadmap was written describes saved sites and an encrypted vault, visual IPv4 scanning, ONVIF discovery, health audits, switch/PoE inspection, reporting infrastructure, batch provisioning, and a **single-stream Live View beta**. It also documents Wails v3 and a headless Go CLI. These are integration points, not tasks to start again from scratch.

**Non-goals for this roadmap:** Cloud accounts, fleet-wide unsolicited scans, arbitrary vendor backdoors, bypassing authentication, covert recording, and a full VMS/NVR replacement.

---

## 1. Feature summary and priority

| ID | Feature | Priority | Relative effort | Primary integration |
|---|---|---|---|---|
| F01 | NVR channel discovery and mapping | P0 | High | Sites, discovery, ONVIF/ISAPI |
| F02 | RTSP stream analyser | P0 | Medium–High | Existing Live View |
| F03 | Focus and alignment assistant | P1 | Medium | Live View, snapshots |
| F04 | PTZ controls and presets | P1 | Medium | Live View, ONVIF/ISAPI |
| F05 | Alarm and event walk-test | P0 | Medium–High | ONVIF events, site tests |
| F06 | Configuration backup, diff and cloning | P2 | High | Vault, vendor APIs, history |
| F07 | Storage and recording retention calculator | P1 | Low–Medium | NVR, bitrate statistics |
| F08 | Interactive site plans | P2 | High | Saved sites, live view |
| F09 | Commissioning and handover reports | P1 | Medium | Existing reports, site tests |
| F10 | Remote discovery over SSH | P2 | High | CLI, inventory, trusted SSH |

Priorities describe implementation order, not promises of universal device/firmware support.

---

## 2. Cross-cutting architecture and UX

### 2.1 Core design

- Extend the existing **Device** and **Site** records with stable relationships; avoid relying exclusively on mutable IP addresses. Use serial number, MAC, channel identifiers and device-scoped identifiers, with source/provenance and confidence where necessary.
- Model each vendor action as **capability-driven**. An ONVIF camera may support video streaming but not PTZ; a Hikvision NVR may expose channel configuration without per-channel recording state. Unsupported and unknown are distinct from failed.
- Backend calls must use context cancellation and bounded timeouts, with progress streamed through existing app events. UI must remain responsive.
- Treat credentials and recording/video URLs as sensitive. Leverage the existing vault. Never expose passwords in JavaScript-visible URLs, logs, generated reports, screenshots or CLI output.
- Avoid opening unintended network listeners; if a local proxy/server is needed, bind to loopback, generate strong session tokens, restrict origins, and close it with the viewer. Avoid exposing RTSP credentials to the WebView.
- Protect against command/URL injection, SSRF, unsafe file paths, malicious XML/SOAP and oversized media or device responses. Limit connections and file sizes.
- Store new site data using versioned, reversible **SQLite migrations** through existing persistence services. Large snapshots/floor plans may live in a managed per-site data directory with database references; use safe paths and atomic writes.
- Use existing visual conventions and navigation: device detail actions, site tabs, Live View panels and reports. Prefer context-aware buttons and disabled controls with explanations rather than unconditionally showing unsupported actions.
- Display clear provenance for results: observed live, read from device, inferred, manually entered, last-known/cached.

### 2.2 Device API abstractions (illustrative; adapt to existing code)

```text
DeviceCapabilities
  VideoProfiles / LiveView
  NVRChannels / RecordingStatus
  PTZ / Presets / FocusControl
  Events / Subscriptions
  ConfigExport / ConfigImport / SettingsRead
  StreamStatistics
  StorageInformation
```

A provider adapter may implement only part of the interface. Prefer composable capability interfaces over one giant implementation. Keep manufacturer-specific adapters behind the same service models, with explicit limitations and **no silent fallback to write commands**.

### 2.3 Common UI patterns

- Site-aware selectors for devices, channels and credentials; resolve inherited credentials according to existing vault rules.
- Show operation status, selected network interface, endpoint/transport (without secrets), capability support and last refresh.
- Long-running read-only operations should have Cancel, useful progress and a bounded concurrency limit.
- Never interpret a timeout after a write as proof it did not execute. Show **Outcome unconfirmed — verify before retry**.
- Any recorded test output should have timestamp, target, result and evidence/provenance.

---

## 3. F01 — NVR channel discovery and mapping

### User story

As a technician, I select an NVR and see its channels, connected camera details, status, and relationships to saved site devices without logging into multiple web pages.

### Functional requirements

1. Identify likely recorders from discovered/saved devices; do not classify by model-name guess alone where product capabilities can be queried.
2. Authenticate using site/device vault credentials or a secure one-session prompt.
3. Query **read-only** channel information using supported vendor APIs (e.g. Hikvision ISAPI on compatible NVRs) and/or ONVIF media/services where available. Detect capabilities and firmware variation; do not assume all endpoints exist.
4. Show physical/logical channel number, configured name, enabled/disabled state, associated IP/hostname (when exposed), model, MAC/serial (when available), connection status and recording status (only when verifiable).
5. Distinguish *empty*, *disabled*, *connected*, *offline*, and *unknown* channels. Do not equate failure to retrieve status with offline.
6. Match channels to existing site devices by stable identifiers before IP; show match confidence and allow manual linking and unlinking.
7. Support recorders with internal PoE camera networks. Only show IPs/metadata obtained from the NVR or genuinely reachable through authorised paths; don't assume the host can reach those private camera addresses.
8. Select a channel to open its available Live View stream with the right channel mapping; support NVR channel numbering variations and main/substream selection.
9. Cache the last observed channel map with observation timestamp, and optionally compare against earlier snapshots; never silently overwrite manual links.
10. Support CSV/JSON export of non-secret channel inventory and inclusion in commissioning reports.

### UI

**NVR → Channels** tab with a sortable table, online/offline/unknown indicators and a relationship view: `NVR → Channel → Saved camera`. Show obvious empty/offline slots and mismatched saved-camera identity.

### Acceptance criteria

- A fake NVR fixture with 0/4/16/32 channels renders correctly and handles missing values.
- Duplicate IPs or renumbered channels do not silently merge unrelated cameras.
- Wrong credentials, unsupported endpoints and unreachable cameras produce distinguishable errors.
- Channel links persist across restart and rescan; cached data includes a last-seen timestamp.
- Live View opens the chosen channel without leaking RTSP credentials.
- Real Hikvision NVR validation is separately documented with model/firmware and granted features.

---

## 4. F02 — RTSP stream analyser

### User story

When a stream fails or stutters, I want to know whether it is an authentication, network, codec, transport, bitrate or decoding problem.

### Functional requirements

1. Add a **Diagnostics / Stream Info** drawer to the existing Live View, not a separate duplicate player.
2. Show verified stream endpoint type, protocol, RTSP transport (TCP/UDP, when known), codec (H.264/H.265/MJPEG etc.), resolution, nominal and/or measured frame rate, measured throughput/bitrate, startup latency and reconnect/error history.
3. Label every metric by source: SDP metadata, player telemetry, packet observations, or estimate. Do not claim frame drops or decoded FPS unless the playback engine actually exposes those counters.
4. Support main/substream comparisons; show meaningful differences and measured results with a short timed diagnostic session.
5. Allow explicit transport selection or diagnostic retry over TCP/UDP **when supported**. Normal viewing should retain existing defaults.
6. Capture a bounded timeline of state changes (resolving/authenticating/connecting/receiving/playing/reconnecting/stopped) with elapsed durations.
7. Distinguish network loss, 401/403, unsupported codec, no video packets, invalid SDP, incompatible smart encoding and timeout where the stack exposes enough detail.
8. Treat H.264+/H.265+ as manufacturer-specific encoding modes and be explicit about decoder limitations. Never promise browser native H.265 playback.
9. Avoid persistent video recording and raw credential-bearing network captures by default. Diagnostic export is **sanitised JSON/CSV/text** only.
10. If existing Wails playback cannot expose stats accurately, add a narrow backend probe/provider rather than inventing values. Assess FFprobe/FFmpeg or LibVLC telemetry for an optional packaged backend with explicit licence and build implications.

### UI

Live View toolbar: **Stream Info**, **Run 15 s Test**, **TCP/UDP**, **Main/Sub**, **Copy Sanitised Diagnostics**. Show a compact live chart only for metrics actually available.

### Acceptance criteria

- Known H.264 fixture reports correct codec/resolution and real measured values where instrumented.
- An unavailable metric displays `Not available`, never fake `0`.
- Incorrect login, unreachable host and unsupported codec yield actionable distinct diagnostics.
- Cancel/close frees playback and probe resources.
- Credentials and credential-bearing RTSP URLs are absent from logs/exports.
- Windows test includes a real camera and notes codec/WebView or external decoder dependencies.

---

## 5. F03 — Focus and alignment assistant

### User story

While mounting a camera, I want quick visual tools to set aim and focus and retain evidence of the installation quality.

### Functional requirements

1. Overlay optional crosshair, thirds grid, horizon/level reference and adjustable guides over Live View (overlays never alter the underlying camera stream).
2. Zoom the preview digitally (pan + reset), independent of the camera's optical zoom or PTZ.
3. Capture snapshots locally with timestamp, camera/channel identity and optional site association; require an explicit user action.
4. Compute a *relative* sharpness measure (e.g. variance of Laplacian) from image frames if a suitable decoded frame is available; label it as a heuristic, not an objective focus score. Provide a focus trend while adjusting the lens.
5. Provide side-by-side before/after snapshots with a zoomable split comparison.
6. Detect camera capabilities before enabling optional motorised focus, iris, zoom or autofocus controls via supported APIs.
7. Make motorised movements small, explicit, cancellable when supported, with feedback; do not autofocus whole fleets or change camera settings without confirmation.
8. Save annotations and selected snapshots to the site as commissioning evidence.

### Acceptance criteria

- Overlays work at different window sizes and aspect ratios, including fullscreen.
- Snapshot does not expose credentials in filename or metadata.
- A fixed-focus camera has no active motorised control buttons.
- Sharpness trend is stable enough to compare adjacent frames and clearly marked approximate.
- UI returns to normal Live View without leaving orphaned video processes.

---

## 6. F04 — PTZ controls and preset manager

### User story

I want to test PTZ movement, zoom and presets without opening the manufacturer's web UI.

### Functional requirements

1. Detect supported PTZ capabilities using ONVIF and vendor-specific APIs where justified.
2. Add a virtual joystick or directional pad, optical zoom and speed slider, with touch/mouse usability.
3. Prefer bounded relative moves or continuous movement with **explicit stop** on mouse/key release, focus loss, navigation, connection loss or cancellation. Add a failsafe timeout so a stuck control cannot leave a camera moving indefinitely.
4. Show available presets, recall a preset on explicit request and allow supported add/rename/delete operations with confirmation.
5. Implement preset tour **preview/test** only on capable devices, clearly showing potential camera movement; avoid automatic patrol configuration as an initial feature.
6. Separate digital zoom in the viewer from optical PTZ movement and clearly identify which is being used.
7. Report unsupported actions rather than showing controls that appear to work but do nothing.
8. Apply permission checks, target identity verification and rate limits to outgoing commands.

### Acceptance criteria

- Continuous movement always attempts Stop on mouse-up, focus loss and UI close; failsafe is tested with fake provider.
- PTZ-disabled or fixed cameras do not show active PTZ controls.
- Preset list refresh handles stale or missing IDs.
- Commands target the selected camera/profile and never another site device.
- Real-camera movement is **never** run by CI or unattended tests.

---

## 7. F05 — Alarm and event walk-test

### User story

At the camera, I start a short test, walk through its field of view, and verify that the expected camera events are actually generated.

### Functional requirements

1. Subscribe to supported ONVIF events (e.g. PullPoint where implemented), supported Hikvision ISAPI event streams or other vendor adapters as appropriate.
2. Discover event capabilities and show supported event names; do not treat every vendor as supporting line crossing or intrusion.
3. Support an explicit **Start walk-test** / **Stop walk-test** session for one camera or selected channel, with duration and count of received events.
4. Show event timestamp, receipt timestamp, source device/channel, vendor-defined type, state (start/stop/active where exposed) and raw-to-normalised mapping.
5. Separate device-reported timestamps from local receipt timestamps; flag suspicious clock offsets.
6. Allow a technician to mark events **Pass**, **Fail**, **Not tested**, with notes and optionally a manually captured snapshot.
7. Handle long-running event feeds with bounded memory, reconnect policy for read-only subscriptions, and reliable shutdown/unsubscribe. Avoid an unlimited event history.
8. Allow optional audio/visual notification during an active walk-test, with user-controlled volume/mute.
9. Persist only explicit test summaries and evidence to a site; live raw event payloads remain temporary unless requested, sanitised and size-limited.
10. Do not automatically enable, disable or reconfigure camera analytics. This feature validates existing settings.

### Acceptance criteria

- Synthetic motion-start/motion-stop/line-crossing events are presented with correct device/channel and source time.
- No event received yields `No event observed`, not `Camera detection failed` without further evidence.
- Subscription cleanup works after timeout, cancel and window close.
- Event types not recognised retain a safe original label rather than being dropped.
- Report generator can consume pass/fail evidence.

---

## 8. F06 — Configuration backup, comparison and cloning

### User story

Before changing or replacing a camera, I want a backed-up configuration, an understandable diff, and an optional guided restore to a compatible replacement.

### Functional requirements

1. Research **documented, supported and authorised** device configuration export/import mechanisms (ONVIF, Hikvision ISAPI and model/firmware-specific methods). Implement read-only export first.
2. Store encrypted configuration backups in a site-specific, versioned format with metadata: device identity, model, firmware, date, adapter/provider, supported sections and checksum.
3. Do **not** assume all vendors permit full config export. Mark sections `Unsupported` or `Unreadable` without fabricating a complete backup.
4. Compare two backups or current-versus-backup settings with a structured diff: added/removed/changed/unknown; redact secrets, tokens, private certificates, recovery material and device-unique values.
5. Build a compatibility matrix by manufacturer, model family, firmware and supported settings. The default restore mode should select only portable parameters such as NTP/timezone, video profiles or OSD **when safely supported**.
6. Exclude IPs, MACs, serials, security keys, passwords and device-specific networking by default. Any network change is a separate, clearly previewed action.
7. Restoration/cloning requires an explicit device selection, full preview, confirmation, per-setting result, and post-write verification. No automatic retries of uncertain writes.
8. If the API exposes only opaque backup blobs, restrict to compatible same-model firmware recovery and do not attempt blind cross-model restore.
9. Provide backups for export/import in an encrypted package with a user-supplied backup passphrase and no credentials in plaintext.
10. Retain a human-readable, non-secret audit trail of user-confirmed changes and outcomes.

### Acceptance criteria

- Redaction works in previews, export and logs.
- Cross-model incompatibility blocks restore or clearly excludes incompatible settings.
- A simulated uncertain network/write response halts further writes and requires manual verification.
- Restoring one device cannot accidentally target another device with the same old IP.
- Encrypted backup round-trip, wrong passphrase, damaged file, and migration are tested.
- No production write path enabled before approved physical-device testing.

---

## 9. F07 — NVR storage and recording retention calculator

### User story

I want to estimate or verify how many recording days a site can retain and how storage changes when cameras, resolutions or frame rates change.

### Functional requirements

1. Offer a standalone calculator and prefill fields from selected site/NVR where verified data can be read.
2. Inputs: number of channels, per-channel average bitrate (or low/typical/high estimate), schedule hours/day, continuous/event recording fraction, retention target, actual usable storage and storage overhead/reserve.
3. Support scenarios (e.g. current, recommended, high quality) and side-by-side comparisons.
4. Distinguish **measured bitrate**, configured target bitrate and user estimate; never infer a fixed bitrate solely from resolution.
5. Show units clearly (Mb/s vs MB/s, TB decimal vs TiB binary), storage per day, expected capacity and approximate retention days.
6. Support VBR ranges, recording calendar, motion-triggered duty cycle, audio overhead and an optional safety margin.
7. Display warnings about RAID/redundancy, filesystem reserve, NVR retention rules, real activity patterns, pre/post event buffers and mixed main/substreams.
8. Save planning scenarios to the site and allow report inclusion.

### Calculation baseline

For each channel:

```text
bytes_per_day = average_megabits_per_second * 1_000_000 / 8
                * seconds_recorded_per_day
site_bytes_per_day = sum(channel_bytes_per_day) * (1 + overhead_fraction)
estimated_days = usable_recording_bytes / site_bytes_per_day
```

Always make duty-cycle and overhead assumptions explicit. Treat a zero/unavailable bitrate as **unknown**, not infinite retention.

### Acceptance criteria

- Unit tests cover a single channel, many channels, mixed bitrates, partial-day schedule, VBR estimates, invalid input and unit conversions.
- Calculator is deterministic for fixed inputs and labels estimates clearly.
- Manual and device-derived values are visibly different.
- Saving a scenario does not mutate NVR settings.

---

## 10. F08 — Interactive site plans and camera locations

### User story

I want a floor plan or aerial image for a site and to place cameras on it so a new technician can locate them quickly.

### Functional requirements

1. Allow a user to attach a local image (PNG/JPEG/WebP) as a plan. Consider PDF first-page rendering only if a safe, reliably packaged renderer exists; do not require a web service.
2. Support multiple plans per site (e.g. building, yard, level 1, level 2), with title, optional annotations and offline storage.
3. Drag site devices onto the plan; store **normalised coordinates** (0–1 range) so markers remain aligned at different resolutions and window sizes.
4. Marker shows camera name, identity, status based on last observation and direction/coverage cone if manually supplied. Coverage geometry must be labelled approximate, not calculated optical coverage unless lens/FOV info supports it.
5. Clicking a marker opens the corresponding device details and existing Live View, using normal vault access controls.
6. Allow quick search, show/hide layers, zoom and pan, move/rename device marker, and undo/redo for plan edits.
7. Handle devices with no plan location and markers pointing to deleted/archived devices gracefully.
8. Import/export a site plan with the encrypted site backup; exclude images from normal inventory CSV unless separately requested.
9. Ensure no automatic GPS/geolocation embedding and no remote map provider requirement; site plans should work completely offline.

### Acceptance criteria

- Marker positions survive resizing, save/load, zoom and image replacement workflows.
- Different sites cannot reference each other's assets accidentally.
- Removing an image shows a warning before deleting associated marker placements.
- Corrupt/oversized files are safely rejected and never executed as scripts.
- Viewer can open Live View for a marker without duplicating a player implementation.

---

## 11. F09 — Professional commissioning and customer handover reports

### User story

At job completion, I generate a polished customer-ready PDF (and optionally HTML) that documents all cameras, checks, screenshots and outstanding issues.

### Functional requirements

1. **Extend the existing report infrastructure** rather than introducing a second export system.
2. Build a site commissioning workflow with technician, date, site/customer identification, project reference, optional company branding and editable report title.
3. Per-camera checklist: reachable, focus/aim verified, timestamp/NTP, live video, recording verified, night/IR test, motion/alarm walk-test, optional PTZ/preset check, credentials handover, notes.
4. Values: `Pass`, `Fail`, `Not tested`, `Not applicable`; require evidence/source for automatically populated `Pass` where feasible.
5. Pull available NVR channels, stream diagnostics, site plan, storage projection, validated audit results and selected snapshots into report sections.
6. Include a concise executive summary, equipment inventory, commissioning matrix, issues/action items, optional acceptance/signoff section, and appendices.
7. Add reusable branding templates and optional Central Communications logo/business metadata as **user-configurable**, not hardcoded requirements.
8. Export PDF and HTML; retain existing CSV/JSON where appropriate. All exports must work offline and avoid credential or authenticated-stream URL leakage.
9. Sensitive site/IP details should have optional client-facing redaction; credentials are **never** in standard reports. Full secrets handover, if ever supported, must be a separate explicitly encrypted export.
10. Preserve a report snapshot with timestamp and schema version so a generated report describes the evidence at generation time, not an endlessly changing live inventory.

### Acceptance criteria

- Produces a readable multi-page report for a site with many devices, images and notes; no clipped tables or blank pages.
- Every checklist item has a provenance or manual-entry marker.
- Correctly distinguishes not tested from failed.
- PDF/HTML output contains no saved passwords, RTSP authentication tokens, recovery codes or hidden JSON secrets.
- Repeated generation from unchanged snapshot is stable apart from documented generated-at metadata.
- Renderer builds and packages on Windows/Linux without launching an external online service.

---

## 12. F10 — Remote site discovery over SSH

### User story

From my Windows laptop, I want to choose a remote customer site, run network discovery *on a trusted Linux host at that site*, and import results without opening public network ports.

### Functional requirements

1. Reuse the existing **Go CLI** on the remote host. Do not create an always-on unauthenticated daemon.
2. Support an explicitly configured SSH target for a saved site: hostname, port, username, optional identity key / agent integration, and allowed remote executable path.
3. Prefer system OpenSSH or a carefully reviewed SSH implementation with strict host-key verification (`known_hosts` or explicit fingerprint approval). **Do not disable host-key verification.**
4. Probe remote CLI version and capabilities, then execute an approved, fixed argument set, e.g. `sadptools discover --json --timeout 10s` and optional authorised interface/protocol selectors. **Never** interpolate unchecked UI text into a shell command.
5. Scope the discovery to the selected remote adapters/site and require an explicit user action before starting. Do not permit arbitrary internet-wide scans.
6. Parse versioned, bounded JSON with timeouts, cancel support, stderr diagnostics and partial-failure handling. Remote command must not print secrets to stdout.
7. Show results as **remote observations**, with source host, adapter, time and latency; reconcile them with saved site identities without confusing local and remote broadcast domains.
8. Store SSH settings in existing site configuration; private keys remain under user control, and passwords/passphrases should use OS agent/vault rather than plaintext DB fields.
9. Start with **read-only discovery and inventory sync**. Future remote writes, if ever implemented, require an extra authorisation model, target verification and explicit device-level approvals.
10. Support connecting through an established VPN/Tailscale/WireGuard route where available, but do not implement VPN provisioning in this feature.

### Acceptance criteria

- Works with a mocked SSH process/client and a test SSH server using a verified host key.
- Wrong key, disconnected host, missing CLI, incompatible CLI JSON schema and timeout show actionable errors.
- Cancellation terminates child processes or SSH channels and does not leave scans running.
- Command args are escaped safely; no command injection via adapter or hostname fields.
- Remote observations can be imported into the selected site but not silently merged into a different customer.
- A real remote-site test is clearly marked separate from unit tests.

---

## 13. Recommended development phases

### Phase A — Diagnostics and NVR understanding (start here)

**A1: RTSP Stream Analyser (F02)**
- Inspect current Live View backends and actual telemetry before choosing a probe engine.
- Implement the diagnostic state machine, safe data model, UI and unit/integration tests.
- Establish codec/transport test fixtures. Record gaps in Windows and real-camera validation.

**A2: NVR Channel Inventory (F01)**
- Implement read-only channel providers (start with one verified Hikvision/ONVIF path, not every NVR).
- Show NVR → channels → matched saved devices, with provenance and last refresh.
- Integrate channel selection into the existing viewer and site inventory.

**A3: Alarm Walk-Test (F05)**
- Add capability-driven event subscription, session-scoped event display and test results.
- Feed results into the site's future commissioning evidence.

**Phase A exit criteria:** these three features are usable at least with fixtures; tests pass; no secret exposure; Windows build succeeds; physical-camera validations are explicitly listed.

### Phase B — Installation assistance and handover

**B1:** PTZ and preset management (F04), initially conservative movement and presets.  
**B2:** Focus/alignment overlay and snapshots (F03), motor focus behind capability checks.  
**B3:** Retention calculator and saved scenarios (F07).  
**B4:** Commissioning checklists and PDF/HTML handover report (F09), using existing reporting modules.

**Phase B exit criteria:** a technician can verify camera operation and produce an offline, redacted customer-facing report.

### Phase C — Advanced site intelligence

**C1:** Read-only config backup and diff, then tightly gated restore for verified devices (F06).  
**C2:** Interactive offline site plans (F08).  
**C3:** Read-only SSH-based remote discovery and reconciliation (F10).

**Phase C exit criteria:** tested site-scoped persistence and migration, strict security gates and resilient cross-platform packaging.

### Task size guidance

Break each feature into small slices: domain schema/provider → fixture-based tests → service/CLI (if applicable) → Wails binding → frontend UX → package validation → hardware checklist. Prefer minimal changes that can be reviewed and reverted independently. Don't start the next slice with a failing test suite.

---

## 14. Database and migration guidance

First inspect current `sites.sqlite` migrations and avoid duplicating existing tables. Possible **conceptual** entities (names are illustrative, not mandated):

```text
site_nvr_channels        site_id, nvr_id, channel_id, identity link,
                         status, source, confidence, observed_at
stream_diagnostic_runs   site_id, device_id, channel_id, source, metrics,
                         error_summary, started_at, ended_at
commissioning_tests     site_id, device_id, test_type, result, evidence_ref,
                         technician_note, observed_at
config_backups          site_id, device_id, encrypted_artifact_ref,
                         metadata, created_at
site_plans              site_id, title, asset_ref, width, height, created_at
site_plan_markers       plan_id, device_id, normalized_x, normalized_y,
                         bearing, label, manual_notes
recording_scenarios     site_id, input_json, result_json, created_at
ssh_site_targets        site_id, host, port, user, host_key_reference,
                         remote_cli_path, allowed_interface
report_snapshots        site_id, schema_version, nonsecret_data_ref,
                         generated_at
```

- Avoid storing repeated plaintext credentials or complete authenticated stream URLs.
- Add foreign keys, indexes and schema-version migrations as appropriate.
- Define deletion/archive behaviour (especially deleting a site with attachments and report snapshots).
- Test migration forward, rollback/backup strategy, site export/import and failure recovery.

---

## 15. Test and release requirements

### Required automated checks

Use the repo's existing commands and pinned toolchain:

```bash
go test ./...
go vet ./...
go test -race ./...
make frontend
make windows
make linux-gui
```

Also run `make check` if supported and run desktop-tagged tests following existing repo guidance. Record skipped checks and why. Generate Wails bindings with `make bindings` **when Go desktop service methods change**.

### Mandatory fixtures

- NVR: zero channels, offline channels, duplicate names, missing serial/MAC, changed channel identifiers, invalid credentials.
- Streams: H.264 SDP, H.265 SDP, transport negotiation failure, authentication error, slow startup, unexposed frame-drop metrics.
- PTZ: unsupported profile, movement start/stop/failsafe timeout, disconnected device.
- Events: motion start/end, line crossing, unknown event, malformed stream, disconnected event subscription.
- Storage: mixed bitrate units, VBR assumptions, zero data, decimal/binary conversions.
- Configuration: differences with secret keys, encrypted restore package, incompatible firmware and uncertain write result.
- Plans: resizing, out-of-range coordinates, corrupted image, deleted camera.
- Reports: large site, many pages, unicode, images, absent evidence, secret-redaction assertions.
- SSH: valid/wrong host key, hostile arguments, cancellation, schema mismatch and site-scoped import.

### Physical validation checklist (human-operated)

- Hikvision camera: model/firmware, H.264 live stream, H.265/smart codec limitations, PTZ (if available), events.
- Hikvision NVR: model/firmware, internal PoE channels, channel mapping, stream access, available recording status.
- ONVIF camera from another vendor: capability discovery, stream information, optional PTZ/event interoperability.
- Windows 10/11 installer on a customer-like LAN: GUI playback, firewall behaviour, package dependencies, graceful close.
- Linux GUI: packaged runtime with selected stream/network interface.
- Remote SSH host: verified key, CLI version negotiation, timeout and reconciliation.

No device writes or camera movement in unattended automated tests.

---

## 16. Completion criteria / definition of done

A feature is **done** only when:

- It integrates with the existing SADPTools UI and site/device model; there is no parallel duplicated subsystem.
- Feature availability is capability-driven, and unknown/unsupported/errors are truthfully displayed.
- Its significant logic has unit tests and safe integration fixtures.
- Any new database data is migratable and covered by backup/export policy.
- Secrets and personal/customer data are excluded from logs and routine exports.
- Cancellation, timeouts, resource cleanup and restart behaviours are tested.
- Relevant Windows and Linux builds work (or limitations are clearly documented).
- User-facing README/docs, CLI help (where applicable) and manual hardware checklist are updated.
- Code licences and third-party notices are preserved.
- A completion report states what was implemented, verified, unsupported, and awaiting physical validation.

---

## 17. Codex kickoff prompt

Paste this into Codex after placing this document in `docs/ADVANCED_FEATURES_ROADMAP.md`:

> Read `AGENTS.md`, `README.md`, and `docs/ADVANCED_FEATURES_ROADMAP.md` in full. Inspect the current implementation and existing Wails v3 Live View, site inventory, ONVIF, reports, tests and build infrastructure. Do not replace working components. Produce `docs/ADVANCED_FEATURES_GAP_ANALYSIS.md` mapping the ten requested features to existing code and identifying technical risks. Then begin **Phase A1 — RTSP Stream Analyser**, implementing the smallest useful end-to-end increment with Go tests, Wails integration and a working Windows build. Keep credentials protected, avoid all automated write operations on real devices, and do not push code without approval. Show actual test/build results and document anything requiring physical camera validation. Continue through the first milestone rather than stopping at an architecture plan.

---

## 18. Useful reference links

- SADPTools project: <https://github.com/TerminalAddict/SADPTools>
- Wails v3: <https://v3.wails.io/>
- ONVIF specifications and profiles: <https://www.onvif.org/profiles/>
- Hikvision ISAPI documentation / supported interfaces: obtain the documentation appropriate for actual device models and firmware; do not assume universal endpoint availability.
- VideoLAN LibVLC documentation: <https://videolan.videolan.me/vlc/group__libvlc.html>
- FFmpeg / ffprobe: <https://ffmpeg.org/ffprobe.html>

**Principle:** hardware claims must be backed by device/firmware testing; integration must preserve the existing product's reliability and security.

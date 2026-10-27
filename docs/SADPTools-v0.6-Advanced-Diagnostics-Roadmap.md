# SADPTools v0.6 — Advanced Diagnostics & Field Service

**Status:** Proposed feature roadmap for Codex CLI  
**Repository:** https://github.com/TerminalAddict/SADPTools  
**Working tree on Sparx:** `/home/paul/git-repos/SADPTools`  
**Targets:** Windows 10/11 AMD64 first; Linux desktop and headless CLI  
**Stack:** Existing Go, Wails v3, Bootstrap 5, vanilla JavaScript, SQLite and credential vault

> **Implementation instruction:** Extend the current application. Do not replace its architecture or rebuild features that already exist. Read `AGENTS.md`, `README.md`, the existing `docs/` plans, existing migrations, and the relevant source files before starting. Inspect the working tree and protect uncommitted changes. Treat the scope described below as a proposal: reconcile it with actual implemented functionality, then implement one milestone at a time.

## Goal and boundaries

Make SADPTools indispensable when a technician is commissioning or troubleshooting a real CCTV installation. This roadmap adds **ten distinct tools** to the existing discovery, sites, credential vault, IP scanner, switch mapping, ONVIF, Live View, NVR mapping, stream analyser, PTZ, events, config backups, site plans and commissioning reports.

**Do not duplicate or regress** existing functionality. Reuse the current device/site IDs, auth/session management, transport clients, task lifecycle, logs, audit/report infrastructure, and existing Wails bindings where appropriate.

### Global engineering principles

- Perform network and device operations **only** against explicitly selected devices, sites or technician-entered targets. Don't run open-ended Internet scans.
- Discover device capabilities and permissions before showing controls. Display **unsupported**, **unknown**, **not tested**, **failed**, and **verified** as distinct states.
- Read-only discovery and inspection by default. Any configuration write, reboot, firmware transfer or relay/action requires a user-approved target, action preview, and device-specific confirmation. No automatic retry of writes after an uncertain outcome.
- Reuse Windows DPAPI and Linux vault protection. Keep credentials in backend memory for the shortest practical time. Never write credentials, challenges, authentication headers, full RTSP URLs with passwords, secrets or private keys into logs, exported bundles or browser-accessible URLs.
- HTTPS certificate trust must use the application's existing explicit trust model. Never silently disable verification.
- Treat device content and user-imported files as untrusted. Bound request/response sizes and durations. Avoid SSRF through arbitrary URLs or redirects; restrict operations to authorised targets.
- Preserve existing data with additive SQLite migrations and backup compatibility; test migrations from previous schema versions.
- Package cross-platform dependencies deliberately; retain offline operation and existing Windows/Linux release workflows. Optional privileged or native capabilities must fail gracefully.
- Prefer Go for device/protocol/business logic; the Wails frontend presents results and launches actions. Keep headless CLI support where genuinely useful.
- Fixtures and mocks are **not** hardware validation. Explicitly separate synthetic unit tests, Linux runtime tests, Windows GUI tests, and physical-camera/NVR tests.
- Follow `AGENTS.md` for build tags, dependency pinning, provenance, licensing, release rules and no-push/no-privileged-install restrictions.

## Features at a glance

| ID | Feature | Priority | Risk/effort | Primary value |
|---|---|---|---|---|
| AD-01 | Network Connectivity Troubleshooter | P0 | Medium | Find exactly where reachability fails |
| AD-02 | Recording Verification & Gap Finder | P1 | High | Prove cameras are recording, not merely streaming |
| AD-03 | Camera Replacement Wizard | P1 | High | Replace faulty hardware without losing site context |
| AD-04 | Firmware Compatibility Centre | P1 | High | Prevent wrong-firmware upgrades |
| AD-05 | Camera Security Inspector | P1 | Medium | Identify insecure services/configuration |
| AD-06 | Image Tuning Studio | P2 | High | Improve night image/exposure and privacy masks |
| AD-07 | ONVIF / ISAPI Protocol Explorer | P1 | Medium | Diagnose firmware/API incompatibilities |
| AD-08 | ANPR / LPR Commissioning Assistant | P1 | Medium–High | Validate plate events and external integration |
| AD-09 | Equipment QR & Label Generator | P0 | Low–Medium | Fast equipment identification on site |
| AD-10 | Diagnostic Support Bundle & Trace Capture | P0 | Medium | Portable, redacted evidence for support |

---

## AD-01 — Network Connectivity Troubleshooter

### User story

I select a saved or discovered camera and choose **Troubleshoot Connection**. SADPTools diagnoses the path from this workstation or approved remote SSH probe to the camera and tells me what is reachable, what failed, and what remains uncertain.

### Functional requirements

- Read the current network interface, bound IP, netmask, selected route, gateway and route-to-destination from the OS.
- Probe in sensible stages: local neighbour/ARP evidence where applicable, ICMP reachability, selected TCP ports (HTTP, HTTPS, RTSP, ONVIF/custom), TLS handshake/certificate, authenticated endpoint capability checks and optional RTSP DESCRIBE.
- Detect and explain: no matching route; host unreachable; absent ARP evidence; TCP refused; TCP timed out; TLS trust/hostname failure; HTTP 401/403; supported/unsupported ONVIF or ISAPI; RTSP authentication failure; RTSP transport failure.
- Compare expected site network settings with observed results. Distinguish failed DNS lookups from IP transport failures.
- Offer selectable interface, timeouts and port profile. Allow adding a limited number of custom ports.
- Present a step-by-step diagnostic timeline with elapsed times, raw status codes where safe, recommended next checks and **Copy safe summary**.
- Optional comparative test from an explicitly registered and trusted SSH remote probe, using the already implemented versioned CLI envelope.
- Probe status does not claim an address is unused solely because ping failed.

### Implementation guidance

A reusable Go diagnostics service should call existing networking, ONVIF, ISAPI and stream components rather than create competing clients. CLI suggestion: `sadptools troubleshoot --ip 192.168.1.64 --json` (final syntax decided from existing CLI conventions).

### Acceptance criteria

- A fixture for each distinct failure mode yields accurate, nonmisleading explanation.
- A selected diagnostic can be cancelled; all sockets/requests close promptly.
- The tool never changes device settings or sends credentials to unrelated destinations.
- Results include target, interface, protocol, test, status, duration, timestamp and evidence without secrets.
- Windows and Linux have tests for route-selection and process permissions; fallback works when ICMP requires privileges.

---

## AD-02 — Recording Verification & Gap Finder

### User story

A camera shows live video, but I need to know that it is *actually recording*. I select an NVR channel or camera with local storage, choose a time range, and SADPTools verifies recordings and reports gaps.

### Functional requirements

- Reuse existing NVR-channel and saved-camera mapping. Query recording-capable devices through supported ONVIF Profile G operations or explicitly supported vendor adapters.
- Show available recordings by date/time, channel, recorded intervals and known gap intervals.
- Offer a configurable sampling strategy (for example: check the latest completed interval and a user-selected period), with a clear distinction between **verified replay**, **metadata says recording exists**, and **not verified**.
- When a vendor allows it, fetch a *short approved replay sample* and verify it decodes; playback/download is opt-in and respects storage/privacy limits.
- Provide diagnostics for missing or unmounted disks, disabled schedules, disk full and camera disconnected when the device exposes those indicators.
- Report device clock, UTC offset/time zone and daylight-saving ambiguity. Normalize timestamps carefully; do not fabricate precise gaps when the API returns coarse or partial intervals.
- Add a result to commissioning reports, linked to site/device/NVR channel.

### Implementation guidance

Use an adapter interface for recording search/replay/capabilities. ONVIF Profile G is relevant for conformant devices; many NVRs and cameras will require conditional vendor-specific behaviour. Do not assume Profile G exists just because ONVIF discovery succeeded.

### Acceptance criteria

- Fixtures cover overlapping, missing, unsorted and timezone-crossing recording ranges.
- UI never reports “recording verified” from live video alone.
- When retrieval is unsupported, the UI states this and gives an actionable manual next step.
- Any downloaded clips have explicit user-selected storage location and cleanup controls; no default cloud upload.

---

## AD-03 — Camera Replacement Wizard

### User story

A failed camera is replaced with a new one. I want SADPTools to reuse the old camera's site identity, compare compatible settings, propose an address, and help update the NVR association without losing audit history.

### Functional requirements

1. **Choose old camera:** selected saved site device, its last known network/firmware/configuration and NVR/channel mapping.
2. **Discover replacement:** explicitly select a newly found device and verify MAC/serial/model identity. Do not infer a replacement solely from a reused IP.
3. **Plan:** display old vs new model, capabilities, firmware, expected address, credential profile, stream channel, settings that can and cannot be transferred.
4. **Preflight:** IP conflict checks; connectivity; recording/NVR dependencies; compatible configuration backups; credentials/activation requirements.
5. **Apply, separately approved:** activation if needed, safe network provisioning, *compatible and allowlisted* settings only, and NVR channel reassociation only through a supported, verified API.
6. **Verify:** re-discover by stable identity; confirm connectivity, stream, NVR channel and optionally recording (when supported).
7. **Reconcile:** preserve old device history as retired/replaced and add a replacement link with timestamps, operator notes and result states.
8. **Recovery:** produce a safe manual recovery checklist when a step's outcome is unconfirmed. Never auto-repeat ambiguous writes.

### Implementation guidance

Reuse existing sites, provisioning, configuration backup/diff, NVR mapping and auditing. Implement a persistent wizard state machine that can resume after an app restart without persisting plaintext passwords.

### Acceptance criteria

- Dry run includes every proposed change and warns about incompatible source/target devices.
- Wrong serial/MAC or IP conflicts block destructive actions.
- Simulated timeouts produce **unconfirmed** and require verification before further writes.
- Completed replacement preserves references and history for both devices.
- No live camera write is performed in automated tests.

---

## AD-04 — Firmware Compatibility Centre

### User story

I need to know exactly which firmware belongs on a specific camera/NVR and whether an upgrade path is safe, *before* I risk bricking a customer device.

### Functional requirements

- Read and display current model, hardware revision, platform, firmware build, language/region identifiers if exposed, and source confidence.
- Let a technician import a firmware package and attach an **official source URL, vendor, model, region/platform compatibility and release notes**.
- Show hash/checksum, digital signature information where available, and provenance. Never claim signature verification when the file has no verifiable signature.
- Match exact known identifiers; block or warn clearly about cross-region, hardware variant, platform and unsupported downgrade mismatches.
- Show ordered intermediate steps when documented; **do not infer a safe upgrade sequence from version strings alone**.
- Start as a **read-only compatibility and planning tool**. Firmware upload/reboot becomes a separate hardware-validated feature gate with explicit maintenance window, backups, power/network warnings, confirmation and post-upgrade verification.
- Provide a manual firmware library indexed by model and source, without silently scraping/downloading arbitrary third-party packages.

### Acceptance criteria

- Unknown model or region yields “Unable to verify compatibility” rather than an approved upgrade.
- Modified firmware packages are flagged by checksum differences.
- No automatic upload, reboot or retry occurs in initial release.
- UI and logs show source provenance, validated vs unvalidated data, and any documented required intermediate build.

### Safety background

Hikvision warns that firmware can be hardware-platform and region-specific, and updates may require intermediate versions. Treat this as a hard engineering constraint, not a cosmetic warning.

---

## AD-05 — Camera Security Inspector

### User story

I want a one-screen read-only review of a camera's exposed services and security configuration, with practical recommendations that will not accidentally break an NVR/VMS connection.

### Functional requirements

- Test selected endpoints and configured ports; identify HTTP vs HTTPS, TLS certificate validity/expiry/trust, supported authentication mechanisms and known exposed management/stream services.
- When authenticated and supported, query account roles, service enablement, ONVIF state, insecure legacy protocols and relevant device hardening settings.
- Produce **observed / risky / unsupported / unknown** assessments with evidence, no fake numerical security score based solely on open ports.
- Distinguish intentionally enabled compatibility services required by NVR/VMS integrations.
- Suggest remediation but require explicit compatibility review before any disablement or access-control change.
- Never perform password spraying or default-password login attempts automatically; no penetration testing without a separate explicit project scope.
- Show a **what could break** section for proposed hardening changes (e.g. disabling RTSP, ONVIF, HTTP, CGI, digest/basic support).

### Acceptance criteria

- No credentials or secrets are emitted in reports.
- TLS certificate error and expired certificate are differentiated; user trust decisions follow existing UI conventions.
- Unsupported read APIs are not mistaken for compliant settings.
- Initial implementation is strictly read-only.

---

## AD-06 — Image Tuning Studio & Privacy Mask Verification

### User story

I can see that a camera picture is poor at night, too bright against headlights, or exposes an area that should be masked. I want to compare settings and verify the result before I leave the site.

### Functional requirements

- Build on the existing Live View and focus/alignment tool; don't implement another competing player.
- Read **capabilities first** for exposure, WDR, BLC/HLC, white balance, gain, image enhancement, day/night, IR intensity and privacy masks, using ONVIF imaging or verified manufacturer APIs.
- Capture side-by-side before/after snapshots with timestamp, codec/resolution and setting metadata.
- Offer manual comparison only when settings cannot be read or changed programmatically.
- Show a preview of supported proposed changes; request confirmation before writing and support restore from the captured previous state **where verifiably supported**.
- Offer a visual privacy mask region editor only on devices with compatible mask APIs. Coordinate mapping must account for actual image resolution, rotation and crop. Do not assume a client-side overlay enforces privacy on the device or recording.
- Require the technician to verify the *device output* and, where applicable, NVR recording includes the mask.

### Acceptance criteria

- Unsupported imaging settings appear disabled with a useful reason.
- Simulated write timeout is unconfirmed, never auto-retried.
- A visual overlay is clearly labelled **preview**, not a real privacy mask, until device-side verification.
- Snapshot comparisons can be stored locally with site records but are opt-in and removable.

---

## AD-07 — ONVIF / ISAPI Protocol Explorer

### User story

A device reports strange or incomplete capabilities. I want a technical view of its actual ONVIF services and supported ISAPI endpoints without switching among several separate utilities.

### Functional requirements

- Display discovered ONVIF service endpoints, advertised capabilities, profiles, media profile tokens and relevant namespaces when available.
- Provide an **allowlisted read-only probe catalogue** covering safe ONVIF Get* requests and known ISAPI GET endpoints. An optional expert mode can inspect a user-provided read-only endpoint after strict target and method validation.
- Show HTTP status, latency, content type, redacted XML/JSON response, parser errors and a human-readable interpretation.
- Implement bounded, cancellable requests with explicit target host allowlisting, redirect prevention and response-size limits.
- Export a scrubbed protocol report, with a preview of exactly what leaves the app.
- Unsafe methods such as PUT, POST for configuration, DELETE and reboot are **not part of this inspector**. ONVIF reads that use HTTP POST are allowed only through fixed, explicitly validated read-only SOAP operations.
- Build an extensible capability registry so features can ask “is this supported?” rather than hardcoding model names.

### Acceptance criteria

- The inspector cannot be used as an arbitrary open HTTP proxy or credential relay.
- It refuses cross-host redirects and non-allowlisted operations.
- Errors distinguish authorization, unsupported operation, invalid payload and unreachable device.
- Export removes credentials, authenticated URLs, tokens and protected material.

---

## AD-08 — ANPR / LPR Commissioning Assistant

### User story

I'm installing a number-plate camera and need to confirm that the plate is read accurately and that the intended NVR/VMS integration receives the event and text, not merely the video.

### Functional requirements

- Reuse Live View plus the existing event walk-test but provide a specialised **ANPR session**.
- Subscribe to recognised vendor-supported LPR/ANPR events or ONVIF metadata where available; identify source, event type, time, plate text, confidence when provided, direction/lane when provided and optional authorised snapshot.
- Show an **event pipeline**: camera detection → local receipt → optional validated NVR/VMS integration observation. Never claim integration success from a camera event alone.
- Add field-test notes for lighting, plate size/angle, IR/white-light conditions, lane and plate capture quality.
- Include measured match rate from technician-approved test cases; distinguish recognition failure from event transport failure.
- Optionally test NX Witness generic-event integration **only with a documented and authorised target endpoint**. Begin with read-only diagnostics and an explicit consent gate before sending a synthetic event.
- Support event deduplication, UTC/time-zone handling, event ordering and configurable redaction/retention of plates and images.
- Multi-vendor adapters should expose capability/unsupported statuses; never assume every ONVIF camera emits plates.

### Acceptance criteria

- Missing plate text or missing confidence is represented as unknown; no invented values.
- A local camera event does not automatically mark the VMS event as verified.
- Plate number and pictures are excluded from ordinary diagnostic bundles by default.
- No event is posted to a customer VMS without explicit approval and an exact destination.

---

## AD-09 — Equipment QR & Label Generator

### User story

After installing a camera, I want a durable label on its cable, junction box or NVR channel so the next technician can identify it quickly.

### Functional requirements

- Generate printable labels for saved cameras, NVRs, switches and relevant site assets.
- Allow configurable template fields: site name/short code, camera number, descriptive location, asset ID, IP, MAC and NVR/channel designation.
- QR data should default to an **opaque local asset identifier**, not a plaintext password or live authentication link. Optional IP/serial encoding must be clearly disclosed and user-approved.
- Provide PDF or SVG label sheets, single label, common label sizes and a print-safe preview. Keep printer-vendor support optional; generic OS printing is sufficient initially.
- Generate unique asset IDs with site scoping; encode references stably across IP changes.
- Support scanning a QR within SADPTools where the desktop has a compatible camera/scanner, or copying/pasting its code to locate the asset. Gracefully support scanners that act as a keyboard.
- Printable data is stored with site records; no cloud service required.

### Acceptance criteria

- QR codes round-trip to the correct local saved item after a simulated IP change.
- Passwords, vault keys and tokens are never encoded or printed.
- PDF/SVG output fits supported label dimensions without clipping and has testable content.
- Labels work without network connectivity.

---

## AD-10 — Diagnostic Support Bundle & Optional Trace Capture

### User story

When a camera refuses to cooperate, I want one button to package the evidence needed for a support case or development issue, without accidentally emailing a customer's passwords or personal footage.

### Functional requirements

- Generate a **preview-first support bundle** containing app version, OS/build, selected adapter details, chosen device identity with optional anonymisation, bounded sanitized application logs, relevant test results and explicit capability data.
- Allow selective inclusion of network diagnostics, RTSP analysis summaries, ONVIF/ISAPI probe results, event-test summary and screenshots if the technician approves them.
- Optional time-bounded pcap/pcapng packet capture on **one selected adapter with a narrow capture filter**, only if supported and the operator grants required OS permission. The app must not silently install packet-capture drivers or start privileged capture.
- Clearly warn that **raw packet capture can contain credentials, plate data, session tokens, private IP addresses or video**. Default to metadata-only summaries; leave raw traces off by default.
- Add a scrubbing/redaction pass and a user-visible manifest indicating exactly what remains. Do not pretend packet capture can be fully anonymised automatically.
- Export a ZIP with checksums, manifest, timestamp, application version and an optional local encryption passphrase.
- Never send bundles to a remote service automatically. The user chooses whether/how to share the file.

### Acceptance criteria

- Default bundles contain no passwords, secret headers, private keys, recovery tokens or raw captures.
- Every included file appears in a preview manifest before creation.
- Cancellation and retention cleanup remove temporary files; sensitive intermediate files are protected by OS permissions.
- On a platform lacking capture support, the support bundle works without pcap.

---

## Suggested delivery phases

### Phase 1 — Highest-impact, least-destructive features

**AD-01 Connectivity Troubleshooter**, **AD-09 Equipment Labels**, **AD-10 Support Bundle**.

- Start with inspecting current diagnostics, reporting, site DB and Wails binding APIs.
- Implement one shared diagnostic result model and add read-only troubleshoot actions.
- Ship labels and bundle generation with simple UI and tests.
- No firmware uploads, device settings writes or VMS event injection.

### Phase 2 — Protocol confidence & site protection

**AD-07 ONVIF/ISAPI Explorer**, **AD-05 Security Inspector**, **AD-02 Recording Verification**.

- Implement safe protocol adapters and capability declarations.
- Record unsupported vs unknown states.
- Recording verification must use actual recording evidence, not Live View.

### Phase 3 — Specialist commissioning

**AD-08 ANPR/LPR Commissioning**, **AD-06 Image Tuning Studio**.

- Integrate current event-test and Live View components; avoid competing stream managers.
- Retain safety controls around plate information and device writes.

### Phase 4 — Guided maintenance

**AD-03 Camera Replacement Wizard**, **AD-04 Firmware Compatibility Centre**.

- Begin both as planning and read-only validation tools.
- Defer write/upload paths until documented model and firmware compatibility has been validated on real hardware.

> Priority may change after repository gap analysis; keep each phase independently usable and releasable.

## Shared data, APIs and UX

### Suggested shared types (adapt to existing conventions)

- `DiagnosticRun`: ID, target stable ID, site, timestamp, interface, scope, duration, state.
- `DiagnosticStep`: category, method, status, observed facts, timing, confidence and redacted details.
- `DeviceCapability`: protocol/service/feature, advertised vs verified, permitted actions, evidence.
- `RecordingEvidence`: source, channel, queried window, intervals, sample verification status, timestamps.
- `MaintenancePlan`: target(s), checked preconditions, proposed changes, approvals, per-step outcomes.
- `AssetLabel`: stable local asset ID, site ID, template, fields, last printed time.
- `SupportBundleManifest`: included files, sensitivity classification, redaction summary and hashes.

Avoid duplicating site metadata and credentials. Reuse SQLite migrations and current vault keys/profiles; add migrations only where essential.

### UI integration

Use the existing device selection and Sites context actions. Candidate entry points:

- **Troubleshoot** on a device's context menu.
- **Recording** on the Commissioning/NVR channel screen.
- **Replace device** on an existing saved-site item.
- **Firmware** and **Security** tabs in selected-device details.
- **Image Studio** within the existing Live View window.
- **Protocol Explorer** under an Advanced/Diagnostics tab.
- **ANPR Session** inside existing event commissioning.
- **Print label** in site/device inventory actions.
- **Export Support Bundle** in diagnostics/help.

Do not add ten unrelated top-level navigation pages. Use consistent busy/cancel/status patterns and keep the existing desktop responsive.

### CLI integration

Add CLI commands only where they improve field automation and fit current command conventions. Candidate commands are `troubleshoot`, `protocols`, `recordings`, `security-check`, `label-export` and `support-bundle`. Confirm existing flags and JSON output rules before choosing syntax. Preserve current CLI commands, JSON contracts and exit codes.

## Validation requirements

For every implemented feature:

1. Unit tests with synthetic or recorded **redacted** fixtures for success, unsupported, partial, permission denied, timeout, cancellation and malformed response.
2. Integration tests proving cross-module reuse rather than duplicated state or conflicting sockets.
3. Migration/backward compatibility and safe-storage tests for new persistent data.
4. Explicit security tests for no credential leakage to logs, exported files, QR codes, URLs, JavaScript and exception messages.
5. Negative tests: bad target, wrong device identity, unsupported capability, failed trust check, uncertain write outcome.
6. `go test ./...`, `go vet ./...` and `go test -race ./...` (as supported by project), plus desktop-tag equivalents when appropriate.
7. `make frontend`, `make bindings` when service APIs change, and `make windows`/`make windows-installer` for UI modifications.
8. Report which checks are **compiled/tested**, **run on Sparx**, **run on Windows**, and **verified on named physical equipment**. Never conflate those.

## Reference material to consult, not hardcode

- SADPTools repository and current docs: https://github.com/TerminalAddict/SADPTools
- Hikvision device integration capabilities: https://tpp.hikvision.com/tpp/OpenCapabilities
- ONVIF Profile T (imaging/audio/etc.): https://www.onvif.org/profiles/profile-t/
- ONVIF Profile G (recording/replay): https://www.onvif.org/profiles/profile-g/
- ONVIF profile support depends on the actual device: https://www.onvif.org/profiles/
- Hikvision firmware platform mismatch advice: https://supportusa.hikvision.com/support/solutions/articles/17000129608-i-cannot-update-camera-firmware-what-are-some-reasons-why-
- Hikvision firmware region warning: https://supportusa.hikvision.com/support/solutions/articles/17000129086-i-found-firmware-newer-than-the-firmware-on-your-website-can-i-use-it-

The protocol list and exact endpoints need verification against current official documentation and actual tested models. Device feature support can vary by firmware, region and account permissions.

---

## Paste into Codex CLI

```text
Read AGENTS.md, README.md, existing docs/ roadmaps and the current working tree. Then read docs/ADVANCED_DIAGNOSTICS_ROADMAP.md in full.

SADPTools is an existing Go + Wails v3 application with saved sites, credential vault, live view, ONVIF, diagnostics and commissioning features. Do not rewrite it and do not regress current workflows.

Start by mapping the ten AD-01 through AD-10 proposals against actual existing source code. Produce a concise gap analysis with proposed files/components and an incremental implementation sequence.

Proceed directly with Phase 1, starting with AD-01 Network Connectivity Troubleshooter, including Go implementation, a useful Wails UI, cancellation, tests and safe logs. Integrate AD-09 and AD-10 as the next independent deliverables when feasible. Reuse existing services and security conventions. Do not stop after planning.

Keep all operations read-only by default. Don't perform live device writes, privileged package installation, public Git pushes or firmware upgrades without my explicit permission. Never leak passwords, tokens, plate data or unredacted captures.

Run and report test/build results, attempt the Windows target build, and clearly distinguish simulated tests from real Windows/device validation. Make incremental local commits only when appropriate; do not push.
```


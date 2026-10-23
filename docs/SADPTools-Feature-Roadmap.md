# SADPTools — Feature Roadmap & Implementation Specification

**Status:** Proposed features; intended as a development brief for Codex CLI  
**Repository:** https://github.com/TerminalAddict/sadptools  
**Local checkout:** `/home/paul/git-repos/SADPTools` on Sparx  
**Product:** Cross-platform CCTV discovery, commissioning and maintenance utility  
**Primary target:** Windows 10/11 AMD64; Linux supported  
**Existing stack:** Go, Wails v3, Bootstrap 5, vanilla JavaScript  
**Date:** 2026-10-23

> **Important for Codex:** This document proposes additions to an **existing, working** SADPTools codebase. It is **not** permission to recreate or replace the application. Read `AGENTS.md`, `README.md`, the current implementation, and the relevant docs before designing changes. Preserve existing discovery, activation, IP configuration, recovery, export, update, build, and installer behaviour. The README reports that some device-write operations still need physical hardware validation; don't quietly change their guarantees.

## 1. Goal

Turn SADPTools from a useful Hikvision SADP replacement into a practical field technician's CCTV installation and maintenance toolkit.

A technician should be able to:

1. Arrive at a customer's site and discover devices.
2. Save the installation as a **named site** with a persistent device inventory and protected credentials.
3. Visually scan a subnet to identify responding hosts and investigate potentially unused IP addresses.
4. Plan and perform careful batch provisioning of cameras.
5. Discover compatible cameras from other brands via ONVIF.
6. Identify the physical switch/PoE port associated with a camera where infrastructure supports it.
7. Audit camera/network health and generate useful reports.
8. Return months later, rescan the site, and clearly see what has changed.

These seven feature groups are detailed below. They are a **roadmap**, not one giant implementation task. Implement incrementally and keep the application usable throughout.

## 2. Existing application: integration requirements

The public README currently describes a Go/Wails v3 desktop app and CLI with SADP discovery, single-device activation, IPv4/DHCP/SDK-port configuration, password recovery, desktop search/sort/export, settings, diagnostics and an installer/update mechanism. Confirm against the **local checkout** because the local branch is authoritative.

- Follow the architecture already in `internal/`, `cmd/sadptools/`, `frontend/`, `desktop.go`, `Makefile` and `Taskfile.yml`.
- Keep discovery/protocol/business logic independent of Wails; expose services through the existing controller and bindings.
- Preserve build tags, pinned dependencies, generated Wails bindings, licensing and third-party notices.
- Respect the existing rule: **no unapproved real-device write operations; no automatic retries of commands whose outcome is unconfirmed.**
- Maintain Windows and Linux compatibility. Do not require Sparx to have a graphical desktop for tests and Windows cross-builds.
- Prefer small, migration-safe changes over an architectural rewrite.
- No cloud service, user account, tracking or telemetry is required.

## 3. Priority and dependencies

| Phase | Feature | Priority | Depends on | Relative effort |
|---|---|---|---|---|
| 1A | Named site inventories and encrypted credential vault | P0 | Persistence foundation | Medium–high |
| 1B | Visual subnet/IP scanner | P0 | Existing adapter enumeration, background task lifecycle | Medium |
| 2 | Site rescan, reconciliation and history | P1 | Site inventory, discovery, scanner | Medium |
| 3 | Bulk camera provisioning | P1 | Device config engine, site inventory, IP conflict checks | High |
| 4 | ONVIF discovery and multi-vendor inventory | P1 | Unified device model, network interfaces | Medium–high |
| 5 | Camera health and configuration audit | P2 | Sites, authenticated device services | High |
| 6 | Managed switch/PoE mapping | P2 | Device identities, site network assets, SNMP/LLDP | High |

**First execution target for Codex:** Audit the local project, design the persistence/security boundary, then implement **Phase 1A** completely and test it. Once that is stable, take **Phase 1B** as its own task. Do not attempt every phase in one coding session.

---

# FEATURE 1 — Named Site Inventories & Secure Credential Vault

**Priority:** P0  
**User story:** "I arrive at a site, discover all the cameras and recorders, save the complete inventory under that customer's site name, and keep the connection details so I can manage them on my next visit."

## 1.1 Functional requirements

- Create, rename, archive, delete and select any number of named **Sites** (examples: Customer A — Main Building; Customer B — Warehouse).
- Each site can include an optional customer name, site address/label, notes, network/subnets, date created, last visited and tags.
- Allow **Save all discovered devices to site**, **Save selected devices to site**, and **Add to site** from device detail/context menu. Prevent accidental duplicate entries.
- Devices persist across application restarts and scan sessions. A site must be usable completely offline.
- Store, at minimum, the following device metadata *when known*: device UUID/local record ID, vendor/protocol, IP address(es), MAC address, serial, model, name, firmware, activation state, HTTP/HTTPS/SDK/RTSP ports, DHCP/netmask/gateway, last seen, first saved, optional physical location, installer notes and tags.
- Support **site-level default credentials** (username/password) and **device-specific credentials** that override them. Allow 'none' for devices with no credentials. Show which credential source is active without revealing its secret.
- For shared credentials, store a reference rather than duplicating the password for every device. A device may reference a reusable credential profile.
- Edit or remove credentials, change credential assignment and opt out of inherited credentials.
- Provide copy-to-clipboard for non-secret fields; for passwords, only offer an explicit reveal/copy action after vault unlock or platform authentication. Clear transient UI password state after use. Warn that the OS clipboard may retain copied secrets.
- Track who/what updated an inventory field in local history (e.g. manual entry vs discovery); do not log secret values.
- Support searching and filtering within a site and across saved sites, by IP/MAC/serial/model/name/notes.
- Provide sensible device merging by stable identity. MAC/serial are signals, not infallible global unique IDs; treat ambiguous/conflicting identity matches as user-review cases.
- Changes discovered on the network should **not silently overwrite** user-edited fields, saved credentials or notes.
- Support encrypted site backup/restore and separate ordinary data export (CSV/JSON) with **credentials excluded by default**.
- A site can have multiple network ranges and may include a mix of cameras, NVRs, switches, and future ONVIF devices.

## 1.2 Persistence and security

- Use a versioned, migratable local SQLite database in the OS user data/config directory; do not put operational data inside the Git checkout, executable installation folder or a fixed absolute path.
- Recommend explicit entities for: `sites`, `site_networks`, `devices`, `site_devices`/assignments, `credential_profiles`, `device_observations`, `scan_sessions`, `change_events`, `audit_results` and `switch_assets`. Adapt to the existing schema once inspected. Do not prebuild unused tables without reason.
- **Never store password plaintext** in SQLite, preferences, browser/frontend localStorage, logs, crash reports, command histories, CSV/JSON exports, debug traces or Git.
- Design a cross-platform secret-storage abstraction. On Windows use DPAPI/OS-backed protection for local credentials; on Linux use an available desktop secret service/keyring where reliable, or a user-unlocked password-derived vault (Argon2id + authenticated encryption). Do not assume a keyring exists on headless Linux. Never generate an encryption key and save it unprotected next to ciphertext.
- If protection is unavailable, do **not** downgrade to plaintext. Offer an explicit secure alternative or block saving secrets and explain why.
- For **portable encrypted backups**, use a user-supplied backup passphrase and a vetted password-based KDF (e.g. Argon2id), authenticated encryption (e.g. AES-256-GCM), random per-backup salt/nonces and a versioned format. A machine-bound DPAPI blob alone is not a usable backup for another PC.
- Avoid writing private data to stdout or to CLI JSON except by a deliberate, authenticated/explicit export action.
- Keep sensitive Go structures out of ordinary JSON serialization and Wails event payloads; use limited DTOs. Avoid retaining plaintext secrets longer than needed.
- Use atomic writes/transactions and SQLite migrations. Back up before potentially destructive migrations, and test restore paths.
- Treat site files as private customer information even when passwords are omitted.

## 1.3 User interface

Add a **Sites** navigation area:

- Left-side site selector/sidebar with site names and quick search.
- Overview: customer/site name, counts, recorded networks, date last visited, last successful scan.
- Main device table reuses the application's existing sortable/searchable inventory UI.
- Actions: `New Site`, `Save Selected`, `Save All Discovered`, `Edit Site`, `Rescan Site`, `Export`, `Backup`, `Restore`.
- Device detail adds a **Saved Site** section: credentials source, location/description, notes, timestamps and history.
- Provide a clear distinction between **Live Discovery** (transient observations) and **Saved Site** (persistent records).
- Context menu from a discovered device: `Add to Site`, `Move to Site`, `Apply Site Credentials`, `View History` (where applicable).

## 1.4 Acceptance criteria

- [ ] Create two sites, discover devices and save different inventories under each.
- [ ] Close and reopen SADPTools: both sites and their devices still exist.
- [ ] Save all or selected devices without creating duplicates when discovery repeats.
- [ ] A site credential is inherited where configured; a device override works and can be removed.
- [ ] Password is protected at rest, never present in DB plaintext, logs or ordinary exports.
- [ ] Data export omits credentials unless a separate **encrypted** backup is explicitly requested.
- [ ] Backup can be restored on a different machine using the supplied passphrase.
- [ ] Missing keyring/master password fails safely without destroying saved data.
- [ ] Changes in live discovery do not overwrite manual notes or credentials.
- [ ] Unit/integration tests cover database migrations, deduplication, credential overrides and encryption/restore.

---

# FEATURE 2 — Visual Subnet Scanner / IP Address Investigation

**Priority:** P0  
**User story:** "I select my Ethernet adapter or enter a subnet, then get a visual map of which IPs respond, which are known to be assigned, and which are candidates for a new camera."

## 2.1 Screenshot-inspired interface

Model the interaction on a compact **IP Scanner** panel:

- Network adapter/NIC dropdown.
- Read-only adapter details: local IP address, subnet mask, gateway (when known), interface name.
- Editable scan range: **Start IP**, **End IP**, and preferably a CIDR input (e.g. `192.168.10.0/24`) with calculated range.
- `Start Scan`, `Stop/Cancel`, `Rescan`, `Reset`, `Export`.
- Progress bar, scanned/total count, duration and result legend.
- A densely packed grid of individual addresses (similar to the supplied screenshot), with the host portion/last octet clearly shown for simple /24 scans.
- A second sortable **table view** for networks where a grid becomes unwieldy.
- Click a grid cell to open an IP details panel; right-click or context action to copy IP, ping again, open web interface, inspect services or associate a saved device.

**Screenshot behaviour to preserve:** Network interface selection on the left, IP start/end fields, 10-column grid, status colours, totals and export. Do not replicate misleading "Available" logic based only on failed ping. For /25, /23 and other CIDRs, clearly label full addresses and network boundaries where necessary.

## 2.2 Scan engine

- Use bounded, concurrent **ICMP echo** probes where supported and report latency/RTT.
- For targets on the same Layer 2 segment, supplement ICMP with ARP neighbour information or controlled ARP probes where supported. Windows/Linux permissions and adapter limitations must be handled gracefully.
- Optional, configurable **TCP connect probes** for common camera/admin ports (for example: 80, 443, 554, 8000), disabled or conservatively configured by default.
- Allow on-demand reverse DNS/hostname resolution and optional locally observed vendor/OUI identification. Don't block scanning on slow lookups.
- Cross-correlate with **existing live SADP discoveries** and **saved site inventory**.
- Prefer non-intrusive checks, rate limits, cancellation, timeouts and bounded worker pools; avoid overwhelming camera networks.
- Record probe types and timestamps so users know why a status was assigned.
- Warn or require explicit scope confirmation for scanning public/routed IP ranges or unusually large networks. Default to the selected adapter's subnet, with an explicit override.
- A failed ICMP echo does **not** prove an address is unused: firewalls and devices that block ping are common.
- Detect and warn about possible duplicate IPs and contradictory MAC/host evidence; never infer a conflict based solely on a timeout.
- Network address, broadcast address (for applicable IPv4 subnet), local interface address, gateway and user-marked reserved addresses must be labelled distinctly and must not be recommended for camera allocation. Respect prefix semantics rather than assuming `.0` and `.255` are always unavailable.
- Be clear when a scan crosses routing/VLAN boundaries; SADP discovery has different reachability from ICMP/TCP probing.

## 2.3 Status model and colours

Use text/icons as well as colour; support accessible contrast:

| Status | Suggested appearance | Meaning |
|---|---|---|
| Responding / occupied | Red | Host responded to ICMP, ARP or a supported TCP probe |
| Known assigned / saved device | Blue | IP is allocated in a saved site, even if it does not respond now |
| No response / candidate | Neutral/light | No response from configured probes; **not guaranteed free** |
| Reserved / network / gateway | Amber | Excluded or specially handled address |
| Unscanned / unknown | Grey | No conclusive scan result yet |
| Conflict / review | Distinct warning style | Evidence of duplicate or contradictory ownership |

The underlying data model may contain multiple observations: **Saved + Responding**, **Saved + No response**, **SADP-discovered but ICMP silent**, etc. Do not collapse these into misleading mutually exclusive states.

## 2.4 Useful integrations

- **Save scan to Site** with timestamp and selected interface/range.
- Mark IP addresses as **reserved** with a note (e.g. gateway, NVR, future device).
- Filter for `Responding`, `No response`, `Known`, `Conflicts`, `Unscanned`.
- Export IP, status, source probes, latency, hostname, MAC (when known), device match and scan time to CSV/JSON.
- From a candidate IP, launch `Assign to Device` in the future bulk provisioning workflow, but validate again before committing any change.
- CLI should expose something conceptually like `sadptools scan --cidr 192.168.10.0/24 --json`, with actual syntax matched to the current CLI conventions.

## 2.5 Acceptance criteria

- [ ] Defaults to the selected adapter's valid subnet and supports custom CIDR/start-end ranges.
- [ ] Grid and table represent every requested address without silently skipping or truncating.
- [ ] Handles /24 and /25 correctly; large ranges switch to an efficient virtualised/paginated presentation.
- [ ] Responding, no-response, reserved, known assigned and conflict states are distinguishable.
- [ ] Scanner may correctly label a ping-silent device as occupied when another reliable probe responds.
- [ ] A nonresponding address is **not** labelled definitively available.
- [ ] Scanning can be cancelled promptly; no leaked goroutines/sockets.
- [ ] Range/adapter validation prevents obviously erroneous or accidental broad scans.
- [ ] Scanner works on Windows and Linux within platform capability limits.
- [ ] Export and site-save preserve probe evidence and timestamps.

---

# FEATURE 3 — Bulk Camera Provisioning

**Priority:** P1  
**User story:** "I have a batch of new cameras. I want to activate and number them predictably without opening ten individual dialogs."

## 3.1 Functional requirements

- Select multiple **compatible Hikvision** devices from discovery or a saved site.
- Configure common intended settings: activation (where supported), static IPv4/DHCP, subnet mask, gateway, SDK port and other settings *only when already supported by the underlying engine*.
- Assign sequential IP addresses across a chosen range and preview one proposed IP per device.
- Respect manually reserved addresses, current occupied addresses, network/broadcast, gateway, selected interface and subnet boundaries.
- Validate conflicts both against live scanner results and the site inventory; warn that absence of a probe response is not proof of availability.
- Provide a **dry-run/preview** that shows old -> proposed configuration for each camera, validation findings and clear per-device action order.
- User must explicitly approve the complete write plan and provide required credentials; never perform mass writes as a side effect of discovery.
- Use a bounded, preferably serial/low-concurrency queue with per-device result state: `Pending`, `In progress`, `Confirmed`, `Rejected`, `Unconfirmed — manual verification required`, `Skipped`.
- Preserve the current no-automatic-retry rule for unconfirmed device changes; on network changes, verify by rediscovery and stable identifier rather than by the old IP only.
- Log non-secret operation metadata and results; never log passwords, recovery values or raw sensitive XML.
- Offer to add successfully verified devices to the selected site and record before/after snapshots.
- Do not imply a transaction/rollback across devices where devices themselves do not support one.
- Limit to devices and firmware where commands are verified. Do not implement guessed protocol messages.

## 3.2 Acceptance criteria

- [ ] Selecting multiple devices produces a preview with explicit per-device assignments.
- [ ] Duplicate IPs or protected/reserved IPs block confirmation or require documented corrective action.
- [ ] A rejected device doesn't silently abort or misreport outcomes for other devices.
- [ ] Timeout after a write becomes **Unconfirmed**, not automatically retried.
- [ ] Only verified successful changes update the persisted current state; uncertain results remain flagged.
- [ ] Synthetic tests cover mixed success/failure/timeout and stable-ID rediscovery; hardware tests require deliberate user authorisation.

---

# FEATURE 4 — ONVIF Discovery & Multi-Manufacturer Support

**Priority:** P1  
**User story:** "I want to see the other cameras on a CCTV network, not only Hikvision."

## 4.1 Functional requirements

- Add ONVIF WS-Discovery alongside SADP, with per-interface selection and timeouts.
- Discover devices from manufacturers such as Milesight, Dahua and Uniview **when the individual device implements and enables compatible ONVIF services**.
- Build a **vendor-neutral device model** with protocol-specific capability flags. One device may be discovered through multiple protocols; merge only with strong identity evidence and show sources.
- Show vendor, model, IP, MAC where reliably known, firmware, services/XAddrs, serial where available, discovery protocol(s) and reachability.
- Offer authenticated ONVIF device-information queries where supported. Optional later steps: media profiles, stream URIs and snapshot retrieval with explicit credentials.
- Do not treat discovery as authorisation to change configuration. Show action buttons only when protocol/device capability actually supports them.
- Support digest/WS-Security authentication according to device capabilities; do not log or persist secrets outside the credential vault.
- Handle multicast restrictions, unroutable XAddrs, HTTPS certificate issues and nonstandard vendor behaviour gracefully.
- User may enable/disable protocols for each scan; show which discovery methods were attempted.

## 4.2 Acceptance criteria

- [ ] Existing Hikvision SADP discovery still works unchanged.
- [ ] ONVIF-capable test devices appear with correct discovery method and partial metadata when authentication is unavailable.
- [ ] The same physical device seen via SADP and ONVIF does not create obvious duplicate UI entries when identity is confidently matched.
- [ ] Device actions are gated by supported capabilities and credentials.
- [ ] Malformed WS-Discovery responses and untrusted device-provided URLs cannot crash the application or trigger unrestricted requests to arbitrary endpoints; validate destination/scope.

---

# FEATURE 5 — Managed Switch / PoE Port Mapping

**Priority:** P2  
**User story:** "When I'm troubleshooting a camera, tell me which switch and port it is connected to and, if possible, its PoE draw."

## 5.1 Functional requirements

- Allow managed switches to be registered as site assets, with hostname/IP, vendor/model, SNMP settings, location and notes.
- Prefer read-only SNMPv3. Support read-only SNMPv2c where explicitly enabled, with its community stored as a protected secret and a clear warning about weaker security.
- Retrieve supported interface descriptions, administrative/operational state, link speed, VLAN context, PoE status/power, forwarding database (FDB)/MAC tables and LLDP neighbour information.
- Correlate a camera MAC with the switch forwarding table to identify likely interface/port. Account for uplink/trunk ports, VLANs, MAC moves, cascaded switches and stale FDB entries.
- Provide **confidence and evidence**: `Direct/strong`, `Likely`, `Ambiguous`, `Unknown`; never claim a precise camera port when the evidence only locates an uplink.
- Display `Camera -> Switch -> Port`, port label, VLAN, link state, negotiated speed, PoE power (if exposed), last polled and query source.
- Make this diagnostic-only by default: no SNMP writes, disabling ports or changing PoE power.
- Integrate mapped port into saved device details and network exports.

## 5.2 Acceptance criteria

- [ ] Read-only switch polling works with supported mock SNMP responses.
- [ ] Correlation includes MAC/FDB/VLAN evidence and timestamps.
- [ ] Uplink/ambiguous cases do not display an invented direct port.
- [ ] Missing PoE MIBs display `Unknown/unsupported`, not 0 W.
- [ ] SNMP credentials are securely stored and excluded from logs and exports.

---

# FEATURE 6 — Camera Health & Configuration Audit

**Priority:** P2  
**User story:** "Before leaving a site, run checks and give me a punch list of things to fix."

## 6.1 Functional requirements

- Run a read-only audit of a selected site or subset of devices.
- Use supported, authenticated ISAPI/ONVIF/other device-specific APIs as appropriate; never infer unsupported configuration state.
- Candidate checks: IP/subnet/gateway consistency; duplicate addresses; DNS and NTP settings; clock skew where obtainable; web/HTTPS/SDK/RTSP reachability; firmware version; authentication failures; device uptime/storage/health if supported.
- Distinguish `Pass`, `Warning`, `Fail`, `Unknown/Unsupported`, `Not checked` with evidence, timestamps and per-check explanations.
- Firmware version comparison must come from a reliable, compatible vendor/model/region source; don't call firmware obsolete merely because a larger version string exists.
- Explicitly note which checks cannot run without credentials, network routing or device support.
- Export a customer-friendly HTML or PDF report **only if the project adds the required generation support**, plus CSV/JSON for technician data. No secrets in reports.
- Optional comparison against a saved baseline; mark changed settings, missing cameras and inconsistent NTP/DNS/gateway configuration.
- Keep audit operations read-only. Any remediation must launch an explicit, separately confirmed configuration action.

## 6.2 Acceptance criteria

- [ ] Site audit can be cancelled and runs without blocking the GUI.
- [ ] Results preserve status, evidence/source and timestamp for each check.
- [ ] Unreachable devices or unsupported APIs report `Unknown`, not a false clean pass.
- [ ] Reports contain useful site/device identification and actionable findings without secrets.
- [ ] Tests cover success, partial data, authentication denied and timeout outcomes.

---

# FEATURE 7 — Site Rescan, Reconciliation & Change History

**Priority:** P1; implement after site persistence  
**User story:** "I return to a customer site months later. Show me which cameras changed, appeared, disappeared or moved IP address since my last visit."

## 7.1 Functional requirements

- `Rescan Site` uses the site's saved networks and selected adapters/protocols; avoid surprise scans outside the currently connected/approved scope.
- Save timestamped scan sessions and structured observations without overwriting the approved inventory immediately.
- Match observations using a confidence-graded identity strategy: reliable serial + vendor, MAC + vendor/site, alternate identifiers and manual resolution for collisions. **Never match by IP alone** as a permanent device identity.
- Detect and present: `New device`, `Previously saved but not seen`, `IP changed`, `Model/firmware changed`, `MAC/serial identity mismatch`, `Activation/config changed` where information is available.
- **Not seen in one scan is not proof that a device is offline or removed.** Present last-seen and scan-scope context; optionally require repeated observations before raising a persistent missing alert.
- Show a side-by-side before/after diff. Allow `Accept change`, `Ignore`, `Mark intentionally removed`, `Merge records` and `Keep manual value` as appropriate.
- Keep an append-only or auditable change history with timestamp, source and previous/new non-secret values.
- Update saved records only after user confirmation (or an explicitly configured, narrowly scoped auto-update policy for harmless fields).
- Export a site change report for customer records.
- Future integration: health audit and switch/port map can attach observations to the same history model.

## 7.2 Acceptance criteria

- [ ] A camera changing IP retains its saved identity and appears as an IP change, not a new camera plus deleted camera.
- [ ] A new device is shown as a proposed addition, not saved automatically.
- [ ] A missing response is shown with last-seen/scope and isn't falsely declared removed after one scan.
- [ ] User-edited fields and credentials survive scans.
- [ ] Ambiguous or contradictory identities require human review.
- [ ] The change history is persisted across app restarts and can be exported without secrets.

---

## 4. Shared technical design

### 4.1 Suggested domain concepts

Adapt to current naming and avoid unnecessary layers:

- **Site:** Named customer/site and its approved networks.
- **Device:** Stable local identity and manually curated inventory fields.
- **Observation:** What a scan/probe actually reported, with source, confidence and timestamp.
- **Credential profile:** Protected secret reference plus non-secret username/label and scope.
- **Scan session:** Interface, requested range, protocols/probes, time, warnings and outcomes.
- **Reservation:** An IP with an intended assignment/reason.
- **Change event:** A proposed or accepted inventory transition.
- **Audit finding:** An evidence-backed check and status.
- **Switch asset/port observation:** Data used for physical connectivity correlation.

**Important separation:** Persistent *intent* (site inventory/IP reservations) is not the same as observed *network reality*. Keep both; otherwise subsequent scans will accidentally erase site knowledge or mislabel addresses as free.

### 4.2 Backend/service boundaries

- `SiteStore` / repositories: SQLite CRUD, transactions and migrations.
- `CredentialVault`: platform-backed secret operations, unlock/reveal policies and portable encrypted backup.
- `IPScanner`: bounded ICMP/ARP/TCP probing, status evidence and cancellation.
- `InventoryReconciler`: identity matching and proposed diffs.
- `ProvisioningPlanner` and `ProvisioningRunner`: validation, dry run and explicit writes.
- `ONVIFDiscovery`: WS-Discovery and optional authenticated information retrieval.
- `SwitchInspector`: read-only SNMP/LLDP and correlation.
- `AuditService`: read-only health checks and reporting.

These are **logical boundaries**, not mandatory package names. Reuse `internal/app`, `internal/devices`, `internal/platform`, `internal/discovery`, `internal/configuration` where appropriate. Expose narrowly scoped Wails functions/events and optional matching CLI subcommands.

### 4.3 UX and operational principles

- Maintain the existing SADPTools visual identity and working discovery table; extend it with **Sites**, **IP Scanner**, **Provision**, **Audit**, and **Switches** views when each feature is ready.
- Use a clear site context indicator, so a technician never silently writes data into the wrong customer site.
- Show progress, partial results, warnings and `Cancel` for longer-running scans.
- Persist user-selected interface and non-secret preferences; don't silently scan every adapter when a site has a selected/approved scope.
- Disclose required OS privileges/capabilities if raw ICMP/ARP is not available, and continue with reduced methods where possible.
- Keep app responsive at hundreds/thousands of addresses; virtualise large lists.
- Use a11y-friendly status colours and accompanying labels/icons.
- All destructive actions (deleting a site, replacing inventory, provisioning) require deliberate user confirmation and sensible recovery/backup options.

### 4.4 Security and quality guardrails

1. No secret plaintext in source files, repo, terminal output, logs, exports or browser storage.
2. Explicitly review security when adding vault, credential inheritance, ONVIF or SNMP code. Prefer established crypto/credential libraries; never invent cryptography.
3. Treat device-reported XML, URIs and strings as untrusted input. Limit sizes and validate requests/destinations to avoid unexpected network access.
4. Limit network scanner concurrency, timeouts and target scope. Only operate against sites/networks the technician is authorised to manage.
5. Password reset/activation/configuration actions always stay subject to current safety rules in `AGENTS.md`.
6. Do not include secrets in diagnostic bug reports, screen previews, crash telemetry or test fixtures.
7. Preserve existing JSON/CSV export safeguards (including formula-injection protection).
8. Do not claim physical hardware or Windows GUI tests passed without actually performing them.
9. Prefer regression tests that leave the current working SADP discovery/configuration/update mechanisms intact.
10. Respect licence and attribution obligations of reused code.

## 5. Definition of done (for each phase)

A phase is complete only when:

- [ ] Functional requirements and scope are reflected in code, not just mocked frontend buttons.
- [ ] All new persistence changes include forward migrations and appropriate tests.
- [ ] New code has unit tests, and network/protocol code has synthetic fixture/integration coverage.
- [ ] Existing discovery/configuration/update/installer tests remain green.
- [ ] `go test ./...`, `go vet ./...`, `go test -race ./...` are run where supported (document any failures).
- [ ] The frontend production build is run; Wails-generated bindings are regenerated and committed when APIs change.
- [ ] The Windows cross-build is attempted using existing project build targets, and any build limits are documented.
- [ ] A minimal hands-on validation checklist is written for real Windows and Hikvision/ONVIF/switch hardware.
- [ ] Docs and README are updated, and risks/known limitations clearly identified.
- [ ] Codex reports exact files changed, commands run, actual results and what remains untested.

## 6. Recommended working sequence for Codex CLI

### Session 1 — Site inventories and vault (Phase 1A)

1. Read `AGENTS.md`, `README.md`, `docs/`, relevant Go packages, frontend state and app service APIs.
2. Confirm the existing UI and inventory data flow; document the current state briefly.
3. Propose a minimal site/credential schema, threat model and portable-backup design.
4. Implement the database/migrations and site/device save/load APIs.
5. Implement password protection and credential inheritance without leaking secrets to the frontend.
6. Add Sites UI and `Save Selected`/`Save All` actions integrated with the live discovery table.
7. Implement safe non-secret exports plus portable encrypted backup/restore.
8. Add tests, docs, build validation and a real Windows verification checklist.
9. Stop at a coherent, working milestone and report clearly. Do **not** start subnet scanner or bulk provisioning during this session unless specifically instructed.

### Session 2 — IP scanner (Phase 1B)

1. Implement a cancellable, bounded scanner that reuses selected NIC metadata.
2. Add a screenshot-inspired grid, list view, legends, statuses, details and export.
3. Integrate results with saved site IP assignments and reservations.
4. Distinguish `No response` from `Available`; do not claim addresses are free based on ping alone.
5. Add platform-aware tests and validation notes.

### Later sessions

- Phase 2: Site rescan/change history.
- Phase 3: Bulk provisioning preview and safe queue.
- Phase 4: ONVIF vendor-neutral discovery.
- Phase 5: Health audits and reporting.
- Phase 6: Switch/PoE mapping.

Ordering of phases 3–6 may change after repository research; document dependencies and tradeoffs instead of quietly implementing them out of order.

## 7. Starter instruction to paste into Codex

After placing this file at `docs/FEATURE_ROADMAP.md` inside the repository, start Codex from the repository root and use:

```text
Read AGENTS.md, README.md and docs/FEATURE_ROADMAP.md thoroughly.
This is an existing Wails v3 + Go application; do not rebuild or break current features.

Implement only Phase 1A (Named Site Inventories & Secure Credential Vault)
from docs/FEATURE_ROADMAP.md in this session.

Inspect the current architecture and identify the smallest compatible data model,
secret-storage abstraction and UI integration. Create a brief plan, then proceed
to actual code, migrations, frontend bindings, tests and documentation.

Never store a password in plaintext or leak secrets through logs, exports, JSON,
CLI or Wails events. If a secure cross-platform implementation cannot be
completed, leave credential saving disabled rather than silently falling back
to plaintext. Respect all existing AGENTS.md device-write safety rules.

Run the repository's documented checks and Windows cross-build where feasible.
Report exactly what works, what was tested, any blocked requirements and the
next steps. Do not push to GitHub without my instruction.
```

---

### Reference links

- SADPTools repository and current README: https://github.com/TerminalAddict/sadptools
- Repository contributor rules: https://github.com/TerminalAddict/sadptools/blob/main/AGENTS.md
- Wails: https://v3.wails.io/
- ONVIF: https://www.onvif.org/

**End of feature specification.**

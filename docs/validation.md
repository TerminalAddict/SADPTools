# Milestone-one validation

Performed on Sparx, Ubuntu 26.04.1 AMD64, 21 October 2026.

## Environment

- Go 1.26.8; Node 24.21.0; npm 11.16.0.
- Wails CLI/module/runtime pinned to `v3.0.0-beta.34` / `3.0.0-beta.34`.
- GTK4 4.22.4 and WebKitGTK 6.0 pkg-config version 2.52.6 already installed.
- Wails CLI was installed in the user's Go bin directory. No privileged system installation was performed.
- Docker exists but the user cannot access its daemon. Windows cross-compilation does not need it.

## Automated checks

| Check | Result |
| --- | --- |
| `go test ./...` | Passed |
| `go vet ./...` | Passed |
| `go test -race ./...` | Passed |
| `CGO_ENABLED=0 go test ./...` | Passed; core works without desktop libraries |
| `go test -tags desktop ./...` | Passed; includes desktop package compilation |
| `go vet -tags desktop ./...` | Passed |
| `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet -tags desktop ./...` | Passed |
| `npm --prefix frontend test` | Three frontend logic tests passed |
| `npm --prefix frontend run build` | Passed; Bootstrap and Wails runtime bundled locally |
| Linux CLI build | Passed |
| Linux AMD64 desktop build | Passed |
| Windows AMD64 desktop cross-build from Linux | Passed; PE32+ GUI executable produced |

The Go tests cover XML fixtures, incomplete/invalid messages, optional field semantics, MAC normalization, inventory merging and identity promotion, concurrent snapshots, interface eligibility, UUID and adapter correlation, off-subnet response acceptance, timeout/cancellation, repeated scan/socket cleanup, busy rejection, bind/join/send/read failures, partial scans, settings persistence, automatic-scan exclusion, shutdown, bounded logs, export escaping and CLI exit codes. Windows ancillary-data parsing is tested with synthetic 32-bit and 64-bit layouts on Linux.

Fixtures use clearly marked synthetic identities and documentation IPv4 addresses. The parser fuzz target is included and its seed cases execute in normal tests; an extended fuzz campaign has not been run.

Initial package coverage: SADP parser 94.3%, discovery 90.2%, inventory 91.4%, controller 76.9%, platform 43.9%, CLI 47.3%. These are software coverage figures, not hardware compatibility measures.

The GitHub Actions workflow is provided for Linux/Windows core tests and a Windows artifact build. It has not been run remotely; nothing was pushed.

## Live network and Linux GUI checks

- A one-second all-adapter CLI scan received **17 distinct real device MACs** and parsed model, IP, firmware, serial, activation, DHCP and HTTP port information. The observed device-name fields were absent and remained empty.
- The Linux desktop launched using Xvfb, a temporary user configuration directory and a private D-Bus session. The initial inventory was empty. Clicking Discover populated **17 real devices** through the Wails bindings. Selection/details, filtering and the settings dialog worked. Switching to the light theme updated the UI and persisted the preference. The native CSV save dialog opened and exported the 17-device inventory; the resulting CSV was parsed successfully and moved outside the repository.
- A real UDP/37020 bind conflict returned exit 1 with an actionable message and valid empty JSON.
- A live 30-second scan interrupted with SIGINT exited promptly with code 130 and valid partial JSON.
- An unavailable selected adapter returned exit 1 and valid empty JSON.

Real device addresses, serial numbers, raw captures and GUI screenshots were not committed. No device configuration requests, credentials, ISAPI requests or browser requests to devices were used in these checks.

## Build artifacts

- `bin/sadptools` — Linux CLI.
- `bin/SADPTools-Setup-windows-amd64.exe` — Windows offline installer, unsigned, includes the display runtime and licence notices.
- `bin/SADPTools-windows-amd64.exe` — Windows desktop development binary, unsigned; requires an installed display runtime.
- `bin/SADPTools-linux-amd64` — Linux GTK4 desktop.
- `bin/LICENSE`, `bin/THIRD_PARTY_NOTICES.md`, `bin/LICENSES/` — distribute with binaries.

All three original application binaries were also built successfully from a clean export of the staged files, without existing frontend output or node_modules. The installer was added subsequently; see its checks below.

Build outputs are ignored by Git and can be reproduced with `make cli`, `make windows` and `make linux-gui`. Wails emits upstream GTK4 deprecation warnings during Linux compilation; the build succeeds. The virtual display logged unavailable GPU/render-device and desktop-portal service warnings but rendered the application using software graphics.

## Offline Windows packaging checks

Added on 21 October 2026, after the initial discovery milestone:

- `make windows-installer` successfully rebuilt frontend/Windows app and compiled the NSIS installer on Sparx. The installer is approximately 208 MiB; most of that is the bundled Microsoft runtime.
- NSIS 3.10 was downloaded and extracted under the ignored build cache, without installing any system packages. NSIS compilation treats warnings as errors.
- The x64 Evergreen Standalone Installer 155.0.4283.62 was acquired directly from Microsoft over HTTPS. Its SHA-256 and size are pinned and checked before every build; the signed payload is bundled unmodified.
- Five automated packaging tests pass: verified offline cache reuse, corrupt-cache rejection, verified atomic downloads, rejection/cleanup of incomplete/corrupt/oversized downloads, and rejection of insecure redirects. These tests use synthetic bytes and no network.
- `make check` passes after the packaging changes: Go tests/vet/race detector, three frontend tests and five packaging tests.
- `7z t` passes on the resulting installer. Extracted app, runtime and licence files are compared against their build inputs. The generated SHA-256 sidecar verifies successfully.
- Documentation and CI now use the installer as the user-facing Windows artifact. No push or remote Actions run was performed.

These are build/archive checks on Linux, not proof that Windows setup has run successfully. Clean offline Windows 10/11 installation, existing per-user/per-machine runtime detection, runtime failure handling, standard-user permissions, upgrade/uninstall, silent setup, SmartScreen and Authenticode verification remain target-machine checks. See [packaging details](windows-packaging.md).

## Still requires physical/target-platform testing

1. Run the Setup EXE offline on Windows 10 and 11, including a machine without WebView2, and verify application startup, native save dialogs, browser opening, shutdown and firewall prompts/profiles. Users should not need to install the runtime separately.
2. Verify Windows receive-interface attribution with multiple physical adapters, including two adapters on the same LAN, VPNs and adapter removal during a scan. Compilation and synthetic ancillary tests cannot establish Winsock runtime behaviour.
3. Place a real device outside the host IPv4 subnet but on the same Layer 2 network. Confirm discovery without changing host/device IP settings. Synthetic tests establish absence of a subnet filter, not device/network behaviour.
4. Check inactive devices, DHCP changes, repeat scans after IP/firmware changes and older/newer firmware families. The devices observed during initial discovery reported active status.
5. Test multicast restrictions, IGMP snooping, Wi-Fi isolation and absent replies; compare with the official SADP Tool on the same interface. Legacy raw-Ethernet discovery and broadcast-only firmware are outside this UDP milestone.
6. Exercise long-running periodic discovery and GUI usability on a normal desktop at different DPI/scale settings. Unit tests verify repeated socket cleanup and non-overlap, but no multi-hour hardware soak was performed.

At the end of milestone one, device writes and recovery remained future work. Milestone two adds the subset described below; firmware management, ISAPI, ONVIF and other manufacturers remain future work.


## Milestone two: single-device management (0.2.0-dev)

Implemented and validated on Sparx on 21 October 2026:

- Native SADP activation; full IPv4/subnet/gateway/SDK-port/DHCP updates; exchange/encryption-string recovery data requests; support-code password reset using `resetPassword` or `securityCode`.
- Desktop forms, explicit target/adapter selection, operation cancellation, scan/write exclusion, private recovery-data export and CLI `send` commands with stdin/environment credential sources and redacted dry runs.
- Additional discovery fields: advertised SDK command port, password-reset ability and reset mode, retained as supplied without inventing capability meanings.
- `go test ./...`, `go vet ./...`, `go test -race ./...`, `go test -tags desktop ./...`, `go vet -tags desktop ./...` and Windows AMD64 desktop vet passed.
- Five frontend tests and five packaging tests passed. The frontend, Linux CLI, Linux GTK4 desktop and Windows AMD64 offline installer built successfully. NSIS archive integrity and the SHA-256 sidecar passed verification.
- New synthetic tests cover all command XML formats, credential escaping, duplicate/nested/malformed XML rejection, target/UUID/type/receive-interface correlation, explicit success/rejection/unknown statuses, recovery payloads, password/network validation, cancellation/timeouts, repeated socket cleanup, concurrency, shutdown, credential isolation, recovery-export identity checks, CLI dry runs, input sources and exit codes. Configuration package statement coverage was 87.8% in the initial milestone-two run.
- A CLI dry run with a synthetic MAC/password and nonexistent adapter produced a redacted plan and did not open a socket.
- The production Linux GUI ran under Xvfb with temporary settings. Read-only discovery again found 17 devices. Selecting an active device disabled activation and enabled network/recovery controls. The network form showed observed address/mask/gateway/SDK port/DHCP and the discovered adapter; the password recovery form opened and closed correctly. No action-submit or recovery-data request was used. Screenshots remain outside Git because they include real inventory data.

**No physical-device activation, network modification, recovery challenge or password-reset request was sent.** The new commands are implemented, not hardware-certified. Windows startup/installation and live write compatibility are still untested. Firmware requiring encrypted commands, broadcast fallback, different reply correlation or vendor XML/GUID/email recovery is outside the implemented subset. A timeout can follow a successful device change and must not trigger an automatic retry. See [device-management acceptance checks](device-management.md).

## Automatic operation adapter selection — 22 October 2026

- Desktop operations now resolve one adapter from the selected device's discovery sightings. A matching scan preference takes priority; otherwise the first recorded route is used. An optional override appears under **Connection options** only when multiple distinct adapters saw the device.
- `make check` passed: Go tests, vet, race checks, six frontend tests and five packaging tests. Synthetic routing cases cover automatic selection, observed preferences, unrelated preferences, explicit overrides, missing routes, duplicate sightings and no retry after an unconfirmed result.
- `make frontend`, `make windows` and `make linux-gui` passed. The Windows offline installer was rebuilt with `python3 build/windows/package.py`; its SHA-256 sidecar verified successfully. Windows execution remains untested.
- The Linux GUI ran under Xvfb with a private D-Bus session and temporary settings. Read-only discovery found 18 devices, including an inactive camera. Its activation dialog opened directly to the password fields with no adapter prompt. No password was entered and no activation or other device operation was submitted. Screenshots remain outside Git.

## About screen — 22 October 2026

- Added an About button with Paul Willard's biography, locally bundled portrait, website link, project link, application version and credits. Biography and portrait sources are recorded in `THIRD_PARTY_NOTICES.md`; opening the dialog does not fetch remote content.
- All six frontend tests passed. `make frontend`, `make windows` and `make linux-gui` passed; the offline Windows installer was rebuilt and its SHA-256 sidecar verified. No Go service API changed.
- Inspected the production Linux GUI under Xvfb in dark and light themes. The portrait, version, expandable credits, close button and Escape dismissal worked. Website and project buttons handed the expected URLs to a temporary `xdg-open` recorder, verifying the Wails browser bridge without launching an external browser. No discovery or device operations were run for this check. Windows execution remains untested.

## Signal Lens branding — 22 October 2026

- Created the original SVG symbol, dark/light wordmarks, nine-size Windows ICO, embedded PNG, frontend icon, installer/uninstaller artwork and a matching Linux desktop-entry template. The app header, top-right About button, About heading, empty inventory and favicon share the same artwork. Updated both themes to the navy/turquoise/amber palette.
- `python3 build/brand/generate.py --check` verified all 12 generated assets against the source. All nine ICO frames decoded successfully with transparent corners; both installer bitmaps decoded as opaque 24-bit RGB at their intended sizes.
- Six frontend tests, five packaging tests, `go test -tags desktop ./...` and `go vet -tags desktop ./...` passed. `make frontend`, `make windows` and `make linux-gui` passed. NSIS compiled the updated offline installer; archive integrity and the SHA-256 sidecar passed.
- Extracted PE resources from the application and Setup executables: all nine icon payloads in each matched the source ICO exactly. The app binary, sidebar bitmap, header bitmap and attribution file extracted from Setup matched the current build inputs byte for byte.
- Inspected the production Linux app and About screen under Xvfb in both themes. No network discovery or device operation was run. The Linux launcher was supplied but not installed into the user's desktop. Windows installation, taskbar rendering and installer DPI behaviour still need testing on Windows.

See [branding sources and regeneration](branding.md).

## In-app updates — 22 October 2026

- Added a Wails-independent GitHub release checker/downloader, numeric stable/beta/RC version ordering, startup-check preferences, update controls, download progress/cancellation and explicit installer handoff. Checks use the public API without a GitHub account or token.
- `make check` passed: Go tests, vet, race detector, eight frontend tests, five installer-input tests and twelve release-tool tests. Updater fixtures cover release ordering, channels, pagination, drafts, bad/duplicate assets, HTTPS redirect restrictions, HTTP errors, timeouts, partial/corrupt/cancelled downloads, hash revalidation before launch, cleanup and lifecycle cancellation. Controller tests cover excluding active device operations, retaining usability after a failed launch and stopping discovery after successful handoff. Settings tests cover old-file migration and retaining an opt-out.
- `make frontend`, `make windows-installer VERSION=0.2.0-beta.2`, `make linux-gui VERSION=0.2.0-beta.2`, desktop-tag Go tests and desktop-tag vet passed. Generated bindings include the five update methods. The final NSIS package, including `/WAITPID` and staged executable replacement, passed `7z t` and its SHA-256 sidecar check.
- A separate, temporary Go probe queried the real public GitHub API. Installed-version inputs `0.2.0-beta.1` and `0.2.0-beta.2` correctly received no newer release. A deliberately older version input selected the real beta.1 release; its 217,717,210-byte installer was downloaded and verified against `SHA256SUMS` and the GitHub asset digest, then deleted. It was never executed. No synthetic release was published.
- Inspected the production Linux app under Xvfb with isolated preferences and automatic discovery off. The Updates dialog displayed the installed beta.2 version and the real up-to-date result, manual checks worked, Linux download/install controls stayed hidden, and the Settings channel/startup preference saved correctly. Dark/light rendering and Escape dismissal worked. No camera discovery or device writes were run during these checks.
- **Windows process handoff, file replacement, setup/relaunch, antivirus/file-lock behaviour and physical-camera writes remain untested.** The beta.2-labelled files are local candidates; beta.1 remains the public release. See [update acceptance checks](updates.md).

## About version and update controls — 22 October 2026

- About now shows the running build version, shared live update status and a manual check. Windows offers **Update now** for an available or verified download; it enters the existing download/installation confirmation flow. Unsupported platforms link to the release instead.
- All ten frontend tests passed, including synthetic Windows/Linux offers, routing verified downloads to confirmation, busy states and failed checks/downloads. `make frontend`, `make windows-installer VERSION=0.2.0-beta.2` and `make linux-gui VERSION=0.2.0-beta.2` passed. The first frontend build stalled on npm's audit request; builds completed with `npm_config_audit=false`, using the unchanged dependency lockfile.
- Inspected the rebuilt Linux application under Xvfb in dark and light themes, with isolated preferences and automatic discovery disabled. About showed version `0.2.0-beta.2`, the real GitHub up-to-date result and a working manual check; Escape dismissal worked. No physical-camera operations were performed. Windows execution/installation remains untested, and the rebuilt installer is a local candidate, not a published release.

## Beta.2 release preparation — 22 October 2026

- Added the beta.2 release notes and the manual beta.1 upgrade instructions. The release builder runs the full checks again and records the exact source commit and artifact hashes in `build-manifest.json`.
- Microsoft's current x64 Evergreen Standalone download resolves to the same pinned versioned URL and 214,156,024-byte payload as beta.1. The current official Evergreen licence text matches `LICENSES/Microsoft-WebView2.txt` after whitespace normalization. The installer builder verifies the cached payload's pinned SHA-256 again; no runtime or licence change is needed. Authenticode and Windows runtime checks remain outstanding as described above.

## Signing diagnosis — 23 October 2026

- The user reported Windows SmartScreen blocking `SADPTools-windows-amd64.exe` with **Unknown publisher**. Inspection of the local beta.2 desktop/CLI binaries and the released beta.2 setup found empty PE certificate tables: these files are unsigned. This does not constitute Windows signature-trust or installation validation.
- Selected SignPath Foundation's free open-source application route. Added the code signing policy and documented the GitHub-hosted build requirement, signing order, third-party runtime disclosure and Windows verification needed before claiming a signed release.
- Submitted the application once through the official web form. Although the form remained at “Form is submitting”, a subsequent SignPath acknowledgement email shared by Paul on 23 October confirms receipt. Review is pending; SignPath expects to respond within the next few business days. Application details and receipt status are retained privately under ignored `.cache/signpath/`; no approval or certificate is claimed.
- Only documentation changed. `git diff --check` and local documentation-link checks passed; application tests/builds were not repeated. Existing public release files remain unchanged and unsigned.

## Beta.3 update-test release preparation — 23 October 2026

- Prepared `0.2.0-beta.3` to exercise updates from beta.2. Application code and dependency pins are unchanged. The release build stamps the new version into the app, CLIs, Windows resources and installer.
- Documented the standalone-to-installed transition: setup uses the registered installation or the default per-user location; it does not overwrite an arbitrary standalone EXE. The user should launch the updated app from setup or the Start menu and confirm its About version.
- Microsoft's current x64 runtime link now resolves to a different payload. This version-only update test retains beta.2's pinned runtime, whose cached bytes are verified again during packaging. Current Evergreen licence text matches the retained notice after whitespace normalization. A runtime refresh is separate from this release.
- The release builder must pass the full automated checks and verify all downloaded release assets before publication. Windows process handoff, installation and relaunch remain runtime acceptance checks; no camera writes are authorised or performed for this release.

## Security-question recovery — 23 October 2026

- Added native local ISAPI availability checks, question retrieval and an explicit encrypted reset to the desktop and CLI. The protocol uses a fresh RSA challenge and encrypted answers/password, with one reset submission, no automatic retry, and explicit rejected/unconfirmed outcomes. It uses an observed adapter with an on-link IPv4 source address.
- Synthetic tests cover capability and question parsing, unconfigured/unknown/unsupported states, changed question IDs, independently generated wire encryption vectors, secrets excluded from diagnostics, lockout reporting, malformed/oversized replies, redirect rejection, timeout/cancellation, one-use GUI check binding, shutdown/update exclusion, and CLI explicit confirmation/exit codes.
- `make check` passed: Go tests, vet and race checks, 12 frontend tests, five installer-input tests and 12 release-tool tests. Generated bindings, `make frontend`, Windows executable/offline installer and Linux desktop builds passed. Desktop-tag tests and vet passed; `7z t` verified the installer archive.
- Read-only discovery found 18 devices. Native CLI checks returned three configured questions on DS-2CD2385FWD-I (V5.5.61), not-configured status on DS-2CD2363G0-IU (V5.6.6), and unsupported local access on DS-2CD2685FWD-IZS (V5.4.6). These describe the devices' observed responses, not a universal model support list. No credentials were supplied.
- The production Linux GUI was inspected under Xvfb with isolated preferences. Discovery, selecting a real camera, retrieving its three configured question labels, switching to the existing support-code form, and disabling reset for an unsupported camera worked. Captures and vendor web-client research remain outside Git. No real answers or new password were entered, and no challenge exchange or reset request was submitted to physical devices.
- **Physical-camera password reset/login validation and Windows execution remain untested.** The built files are local development candidates, not a new published GitHub release. See [security-question recovery](security-question-recovery.md) for compatibility limits and acceptance checks.

## Routed security-question recovery — 23 October 2026

- Removed the same-subnet prerequisite that incorrectly blocked reachable cameras on routed networks. Automatic HTTP selection uses the OS-selected IPv4 source, including secondary addresses and unicast-only VPN adapters; explicit adapter overrides remain respected. The HTTP recovery API response determines reachability/support. Connection failures identify the attempted address, port, adapter and source without exposing raw errors or secrets.
- Synthetic regression tests cover routed/direct source selection, route lookup failure, a route through an adapter not used for discovery, VPN adapters, explicit overrides, unavailable/invalid adapters, local address conflicts, HTTP success/unsupported/failure/timeout outcomes, routed CLI/controller submissions, and invalidation of a checked reset when its source address changes. The loopback route-helper test verifies that lookup sends no UDP datagrams. No failed HTTP request or reset is retried through another adapter.
- `make check` passed: Go tests, vet and race checks, 12 frontend tests, five installer-input tests and 12 release-tool tests. `make frontend` regenerated bindings and built successfully. Linux CLI, Windows AMD64 desktop/offline installer (`make windows-installer`) and Linux desktop builds passed. Desktop-tag compilation/tests and vet passed. Installer archive (`7z t`) and SHA-256 sidecar verification passed.
- No physical-camera requests, password changes or host network configuration changes were made while validating this fix. Windows runtime, the user's routed camera connection and physical password-reset/login checks remain outstanding. A graphical session was not retested for this backend fix. Artifacts remain local development builds, not a published release.

## HTTP/HTTPS detection and scan feedback — 23 October 2026

- Added a shared, Wails-independent web endpoint detector, HTTPS preference,
  same-camera redirect/port handling, a clickable device IP, bounded probes and
  cancellation, and current-address caching. Recovery keeps the selected
  protocol/port and checks normal TLS trust or an explicitly accepted certificate
  pin for every connection. No recovery write retries or HTTP downgrade occur.
- `make check` passed: `go test ./...`, `go vet ./...`, `go test -race ./...`,
  19 frontend tests, five installer-input tests and twelve release-tool tests.
  New synthetic coverage exercises HTTP-only/HTTPS-only/custom-port services,
  HTTPS preference, redirects, untrusted/changed certificates, recovery endpoint
  binding, cancellation, stale selections, URL validation and per-scan counts.
- `make frontend`, `make windows-installer`, `make linux-gui` and `make cli`
  passed. Windows packaging builds the AMD64 desktop executable and the offline
  installer; `7z t` passed for the resulting installer. Desktop-tag Go compilation
  and vet also passed. Generated Wails bindings are included in source control.
- The production Linux GUI was inspected under Xvfb using isolated preferences.
  Read-only discovery found 18 real devices. Scanning showed elapsed progress,
  live unique-device/new-device counts and retained inventory. A repeat scan
  showed zero new devices, and cancellation showed the cancelled state. The
  activity panel, table and diagnostics fit the normal window together.
- Two physical cameras selected in the GUI were detected over HTTP. Clicking
  the IP handed the detected HTTP origin to the native browser launcher (captured
  with a test-only `xdg-open` stub). Read-only recovery retrieved three configured
  question labels on DS-2CD2385FWD-I V5.5.61 using the detected connection. No
  answers/passwords were entered and no challenge/reset was sent to equipment.
- HTTPS, certificate acceptance and changed-certificate failures were verified
  with synthetic loopback TLS servers, not a physical HTTPS camera. Physical
  HTTPS/recovery writes and Windows runtime/browser/installer execution still
  require testing. Local artifacts remain `0.2.0-dev`; no release was published.

## Technician toolkit 0.3.0-beta.1 — 23 October 2026

- Implemented all seven roadmap groups: persistent sites and protected credentials,
  IPv4 investigation, reviewed rescans/history, explicit bulk provisioning, ONVIF,
  read-only audits and switch/PoE evidence. The
  [implementation guide](toolkit-implementation.md) records architecture, files,
  compatibility limits and a hands-on checklist.
- `make check` passed: Go tests, vet and race detector, 21 frontend tests, five
  installer-input tests and twelve release-tool tests. Desktop-tag Go tests/vet,
  generated bindings, frontend production build, Linux CLI/GUI and Windows
  standalone/offline installer builds passed. Installer integrity and SHA-256
  verification passed; the standalone retains icon/version/manifest resources.
- Linux production GUI checks under Xvfb used isolated preferences and labelled
  synthetic sites. Site creation, vault unlock, saved inventories, reconciliation,
  manual-field protection, saved audit/switch reports, native HTML export and
  persistence after restart worked. Credentials were omitted from ordinary export;
  the vault reopened locked. No synthetic device is added by normal app startup.
- Read-only live discovery returned 18 SADP and 18 ONVIF observations, not 36
  confirmed distinct cameras. Strong identity is required before merging partial
  ONVIF records. No camera or switch writes were sent.
- Setup and standalone are local `0.3.0-beta.1` candidates. Windows runtime,
  DPAPI, native ICMP/IP Helper, installation/update handoff, authenticated physical
  ONVIF access, camera writes and physical switch/PoE mapping still need testing.
  No GitHub publication or signing is claimed; SignPath approval remains pending.

## macOS implementation and portability checks — 27 October 2026

macOS targets are implemented locally; **no Mac GUI executable, app bundle, ZIP
or DMG was produced on Sparx in this session**. `make macos MACOS_ARCH=arm64`
correctly dispatches the ARM64 Darwin build and stops at Docker's unavailable
socket (`permission denied` for the current user). The expected private
`MacOSX14.5.sdk.tar.xz` is also absent. No Docker permissions, privileged packages,
Apple SDK downloads, GitHub pushes, tags or published releases were changed.
See [macOS setup](macos.md) for the exact prerequisite and build commands.

Completed locally on Linux:

- `make check`: all Go tests, `go vet ./...`, `go test -race ./...`, **35 frontend
  tests**, **5 Windows packaging**, **20 release**, **11 Linux packaging** and
  **15 macOS packaging/toolchain** tests passed. Mac package fixtures are
  explicitly synthetic and are never presented as runnable applications.
- `make frontend`: generated bindings and production frontend build passed.
- `go test -tags desktop -race .`: desktop service/lifecycle tests passed on Linux.
- `make windows` and `make windows-installer`: Windows AMD64 desktop and complete
  offline installer built; `7z t` passed. The regression check also fixed copying
  an existing read-only generated licence file by replacing it with `cp -f`.
- `make linux-gui` and `make linux-packages`: Linux desktop, CLI, DEB, RPM and
  manual archive built; all package SHA-256s passed and CLI version was correct.
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -exec /bin/true ./...` and the
  AMD64 equivalent passed **compilation only**. `/bin/true` deliberately avoids
  executing Mac test binaries; these are not native macOS test passes. The new
  Darwin kernel ARP/default-route tests are among the compiled tests.
- `GOOS=darwin GOARCH=<arm64|amd64> CGO_ENABLED=0 go vet ./...` passed.
- Both macOS CLI architectures built under `bin/macos-cli/`; Wails `tool lipo`
  joined them into `sadptools-darwin-universal`. Real Mach-O architecture and
  embedded version checks passed for all three. They have not run on a Mac.
- ICNS generation/structure and `make branding` consistency passed. Task dry runs
  verify requested ARM64/AMD64/universal targets, avoiding a discovered Wails
  `GOARCH` → Task `ARCH` propagation bug. Windows/Linux task paths remain separate.
- The new release importer tests validate universal slices, version/identifier,
  executable modes, licences, ZIP paths, provenance and checksums; corrupt,
  incomplete or stale Mac artifacts are rejected. The original RC1 release
  bundle still passes the extended verifier unchanged.
- Workflow YAML parsed and platform isolation checked. Native macOS ARM64/Intel
  CI, bundle signing, actual `hdiutil` packaging and DMG mounting are configured
  but **not executed** because these changes have not been pushed.

No real camera was contacted or configured for this task. No Windows runtime or
Mac graphical/hardware test is implied by compilation. On actual Macs, verify
Finder/Dock/Retina behavior, Gatekeeper and Local Network permission, adapters,
multicast/ARP/routes, saved sites and vaults, browser/file dialogs, Live View codec
support/autoplay/lifecycle, and manual upgrade preservation using the checklist
in [macos.md](macos.md). Current native packages use ad-hoc signatures for testing;
Developer ID signing and notarization remain a separate distribution step.

### RC2 native CI packaging follow-up

The first native macOS CI run during RC2 preparation compiled both desktop
architectures, combined them into a universal app, verified its ad-hoc signature
and plist, and created both ZIP and DMG. `hdiutil verify` and a read-only mount of
the finished DMG passed; the mounted app's signature and executable matched the
packaged app. The subsequent redundant `lipo -verify_arch` workflow check exposed
an argument-order error (input must precede the variable-length architecture
list); that check was corrected for the release rerun. Publication still requires
the complete native packaging/desktop-test job to succeed and its artifacts to
match the final release commit. This is build/packaging evidence, not Finder,
Gatekeeper, camera or Live View hardware validation.

## Encrypted SADP activation investigation — 28 October 2026

After a reported activation failure, read-only discovery confirmed an IPC-B261H-MU
running V5.7.1 build 220610 still inactive. A key exchange on its discovered
adapter succeeded and exposed two missing requirements in the original code:
RSA/AES activation negotiation and correlated command replies without a MAC.

The corrected Go path passed a guarded live preparation check: fresh inquiry,
inactive-state verification, source/adapter correlation, key exchange, challenge
decryption and encrypted-payload preparation. The transport guard allowed only
`inquiry` and `exchangecode`; it intercepted and blocked the activation datagram.
A final separate discovery still reported inactive. No activation, password,
network configuration or other device write was sent by this investigation.
Private test harnesses and any device identifiers remain in the ignored cache.

Validation passed: `go test ./...`, `go vet ./...`, `go test -race ./...`, all 43
frontend tests, frontend build/binding generation, Linux CLI, Windows AMD64 GUI
and Linux AMD64 GUI builds. Both desktop binaries carry `1.0.2-dev`. New
synthetic tests cover the encryption vector, strict stage correlation, malformed
challenges, password byte limits, explicit rejections, cancellation, send errors,
missing acknowledgement and socket cleanup. No bindings API changed.

These are local development builds, not a tagged release. Full activation,
subsequent password login and Windows GUI runtime verification remain to be
tested on an explicitly authorised physical camera.

## SADP network authentication correction — 28 October 2026

The user reported successful Windows activation after v1.0.2, followed by a DHCP
change returning unconfirmed. Read-only discovery on Sparx confirmed the
IPC-B261H-MU (V5.7.1 build 220610) active, with DHCP off and encrypted/salted
authentication advertised. Its fresh reply passed the new profile parser; a
network command was encoded offline with a synthetic password. The private
harness transmitted **only inquiry**, with no password or configuration write.

The native network transaction now performs that fresh check, computes the
advertised authentication, preserves unrelated settings and sends one write.
Independent synthetic digest/AES vectors were checked against archived SDK
behaviour. Tests cover four authentication combinations, UUID binding, preserved
ports/IPv6, ambiguous/unsupported profiles, source/MAC/adapter correlation,
changed-address acknowledgements, explicit credential/lockout/inactive rejections,
preparation failures, cancellation, lost replies and no retries.

Passed on Sparx: `go test ./...`, `go vet ./...`, `go test -race ./...`, all 43
frontend tests, `make frontend`, `make windows`, `make linux-gui` and `make cli`.
The CLI and both desktop builds at that stage were **1.0.3-dev**; the Windows
output was AMD64 PE32+ GUI. Bindings were regenerated with no API changes. These
were local test builds preceding the v1.0.3 release preparation.

[The management audit](device-management-audit.md) records other write paths and
unverified support-code recovery. No DHCP, static-IP, password-reset, imaging or
PTZ write was performed during this review. DHCP lease acquisition, web login
and Windows runtime behaviour remain physical-device acceptance checks.

## Security-question setup and activation follow-on — 28 October 2026

Implemented native encrypted SADP question setup, a standalone Device details
action and an optional next step after acknowledged single-device activation.
The next step keeps the actual activation adapter and carries the password only
temporarily in the dialog. Saving requires three distinct questions and explicit
submission; replacing existing questions also requires confirmation. Readback
checks configured IDs, never tests answers by resetting the password. Failed or
unconfirmed setup does not cause an automatic retry or another activation.

Passed on Sparx:

- `go test ./...`, `go vet ./...`, `go test -race ./...`.
- All 46 frontend tests, regenerated Wails bindings and production frontend build.
- Synthetic protocol/transaction tests for nested question XML, unknown profiles,
  encrypted password/answers, exact UTF-8 input, changed review, replacement consent,
  source/adapter/UUID/MAC correlation, rejected/lost replies, readback, cancellation,
  socket cleanup and secret redaction. Fresh inventory observations preserve the
  host adapter address rather than substituting the camera address.
- Mocked Chromium dialog checks: activation follow-on, same-adapter selection,
  no automatic setup write, skip and immediate credential clearing, unconfirmed
  activation, existing-question replacement, unsupported and unconfirmed setup,
  single submission and light/dark screenshots. These use clearly labelled
  synthetic devices with intercepted backend calls, not Windows WebView2.
- Windows AMD64 desktop, Linux AMD64 desktop and CLI builds at **1.0.3-beta.1**.
  Binary format/version checks and the standalone ZIP's CRC/content/licences passed.

The local Windows test archive is
`bin/SADPTools-1.0.3-beta.1-standalone-windows-amd64.zip`, with a sibling SHA-256
file and local test-build metadata. Its executable was built at
`bin/SADPTools-windows-amd64.exe`. This validation used an uncommitted local test
build, without rebuilding the installer; it preceded the full v1.0.3 release.
An attempted `1.0.3-dev.2` build was rejected by the existing version validator
(development versions cannot carry sequence numbers); the supported beta version
was used without changing release policy.

A physical **read-only** inquiry and `getsecurityquestion` check after the user's
camera reset confirmed an IPC-B261H-MU, V5.7.1 build 220610, inactive, with question
support and the complete 15-ID list unconfigured. Its real reply passed the
production parser. Independent synthetic encryption vectors were checked against
archived SDK behaviour; no SDK code or binary is included in the application.

No activation, network, question-setup or password-reset write was sent by the
development agent. Successful answer storage, persistence after reopening,
Windows GUI runtime and eventual password recovery still require the user's
physical-device test. Linux graphical-session and macOS runtime validation are
also separate from compilation. See the [setup test checklist](security-question-setup.md).

### v1.0.3 release preparation

The subsequent requested release includes the network correction and question
setup together. All version defaults and platform metadata are set to `1.0.3`.
The release process rebuilds Windows setup/standalone, Linux packages/CLIs and
imports universal Mac artifacts from CI for the exact committed source. It
requires clean sources and verifies uploaded asset checksums before publication.
See [release notes](releases/v1.0.3.md), the release's build manifest and its linked
CI checks for publication evidence; the hardware limitations above still apply.

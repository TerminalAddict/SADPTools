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

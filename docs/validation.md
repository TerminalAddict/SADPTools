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

Device activation, IP/DHCP writes, password recovery, firmware management, ISAPI, ONVIF and other manufacturers remain future work. Their UI controls are disabled and no write path is registered.

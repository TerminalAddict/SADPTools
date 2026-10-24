# SADPTools

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="build/brand/logo-dark.svg">
  <img src="build/brand/logo-light.svg" alt="SADPTools — network device management" width="440">
</picture>

A native Go desktop utility and CLI for CCTV discovery, site inventories and commissioning. Built with **Wails v3**, Bootstrap 5 and vanilla JavaScript. Windows 10/11 AMD64 is the primary desktop target; Linux is also supported.

The CLI and desktop support single-device activation, IPv4/DHCP/SDK-port configuration and code-based password recovery. Security-question recovery is also available for compatible cameras reachable over IPv4, directly or through a router. These write operations have synthetic test coverage but still require physical-device/firmware validation. SADPTools adds no account, cloud service or telemetry. The Windows display runtime is supplied by Microsoft and has its own update and privacy terms, included with setup.

## Install on Windows

Download the **Setup EXE** from [GitHub Releases](https://github.com/TerminalAddict/SADPTools/releases), run it, follow setup, then open **SADPTools** from the Start menu. Release downloads include the version in the filename (for example, `SADPTools-0.2.0-beta.2-Setup-windows-amd64.exe`). Setup includes everything needed to run the application on Windows 10/11 AMD64 and works without an internet connection. There are no separate prerequisites to download or configure.

The original `0.2.0-beta.1` needs one manual upgrade to obtain the in-app updater.
On newer versions, use **About → Check for updates** and **Update now**, or the
**Updates** screen. Local test builds are not available through the updater until
they are published as a GitHub release.

Setup automatically prepares the display component if needed. It installs SADPTools for the current user and provides an entry in Windows' installed-apps list for removal. Builds are currently unsigned, so Windows may show a publisher/SmartScreen warning. Installation and startup still need validation on Windows; see [validation results](docs/validation.md).

See the [code signing policy](docs/code-signing.md) for the SignPath Foundation
application and signing requirements. Current builds remain unsigned while
approval is pending; signing will apply to a new, verified release.

## Install on Linux

Linux AMD64 packages include the desktop app, CLI and a **SADPTools** icon in the
GNOME/KDE application menu. Install the `.deb` with `sudo apt install ./<file>.deb`
or the Fedora `.rpm` with `sudo dnf install ./<file>.rpm`; the package manager
handles the declared desktop libraries. For a manual per-user installation,
extract `SADPTools-<VERSION>-manual-linux-amd64.tar.gz` and run `./install.sh` without
sudo. The script also supports all-users installation and uninstalling.

These packages use GTK4/WebKitGTK 6.0, targeting Ubuntu 24.04+, Debian 13+ and
current Fedora. See [Linux installation and packaging](docs/linux-packaging.md)
for dependencies, upgrades, removal and testing limits. Build local installers
with `make linux-packages`; they appear in `bin/linux/` and are included in
releases starting with **0.3.0-beta.2**.

## What works

The **0.3.0-beta.1** test build adds the complete technician toolkit roadmap. See [the implementation guide and hardware checklist](docs/toolkit-implementation.md).

- Named offline sites, protected shared/device credentials, encrypted portable backups, reviewed inventory changes and history.
- IPv4 investigation with ICMP/TCP and neighbour evidence, a 10-column grid/table, reservations, saved scans and CLI.
- Explicit bulk activation/network plans, serial writes without retries and stable-identity verification.
- ONVIF WS-Discovery and authenticated read-only information queries with certificate trust checks.
- Read-only health audits with HTML/CSV/JSON reporting and explicit unknown/unsupported results.
- Managed switch SNMPv3/v2c inspection, FDB/VLAN/LLDP correlation and supported PoE observations with confidence.

- IPv4 adapter enumeration and selected/all-adapter multicast discovery on `239.255.255.250:37020`.
- `inquiry` and `inquiry_v32` probes, response correlation, normalized MAC deduplication, serial fallback, malformed-response handling, timeouts, cancellation and repeated scans.
- Device model/type, network settings, MAC, serial, firmware, activation, DHCP, HTTP port and device name when advertised. Unknown values remain empty/null.
- CLI table/JSON output and useful diagnostics/exit codes.
- Desktop inventory with sorting, searching, selection, retained results, device details, native CSV/JSON export, web-interface action, diagnostic log, light/dark themes and saved discovery preferences.
- Automatic HTTP/HTTPS detection for the selected camera, with a clickable IP address and a shared endpoint for browser opening and security-question recovery. HTTPS is preferred when available; see [web connections](docs/web-connections.md).
- Visible discovery activity: a spinner, elapsed listening-window progress, devices seen in this scan and newly added devices. Existing inventory stays visible; completion, cancellation and errors are distinguished.
- Device action dialogs, automatic selection of a discovered adapter with an optional override, password input without persistent storage, recovery-data requests/export, support-code submission and [security-question recovery](docs/security-question-recovery.md). See [device management](docs/device-management.md) for workflows and compatibility limits.
- An About screen introducing creator [Paul Willard](https://www.paulwillard.nz/), with the application version, live update status, project link and credits. The biography and credits are available offline.
- An in-app GitHub update checker with stable/beta channels, verified Windows installer downloads, progress/cancellation and explicit installation after closing the app. See [updates](docs/updates.md).
- One active scan or device operation at a time. Automatic scanning starts its interval after the previous scan finishes. Last-seen timestamps identify old observations; missing one scan does not mark a device offline.

A one-second scan on Sparx received **17 real devices** during initial validation. This verifies basic Linux discovery on that network, not every firmware, VLAN, firewall or Windows adapter combination. See [validation results](docs/validation.md).

## Run the CLI on Sparx

From `/home/paul/git-repos/SADPTools`:

```sh
make cli
./bin/sadptools interfaces
./bin/sadptools discover
./bin/sadptools discover --all-interfaces --timeout 10s
./bin/sadptools discover --interface "ADAPTER NAME" --timeout 10s --json
./bin/sadptools discover --protocols sadp,onvif --timeout 5s
./bin/sadptools scan --interface "ADAPTER NAME" --cidr 192.168.10.0/24 --json
./bin/sadptools discover --debug
./bin/sadptools --version
```

Replace `ADAPTER NAME` with an exact name from `interfaces`; spaces are supported. `go run ./cmd/sadptools discover` also works. Ctrl+C cancels a scan and outputs any partial inventory. CLI/engine builds and tests require no graphical display, Wails executable, Node or GTK development packages.

`--json` emits a device array (`[]` when empty) on stdout. Diagnostics go to stderr. Optional booleans/HTTP port are `null` when unknown. `interfaces --json` includes unsuitable adapters with reasons.

| Exit | Meaning |
| --- | --- |
| 0 | Completed successfully, including no devices |
| 1 | Network, socket, I/O or other failure |
| 2 | Invalid command/arguments |
| 3 | Partial scan; one or more adapter operations failed |
| 4 | Device rejected a configuration command |
| 5 | Command outcome unconfirmed; verify before retrying |
| 130 | Cancelled with Ctrl+C; any partial results are still printed |

For activation, network changes and recovery commands, see [the device-management guide](docs/device-management.md). The CLI supports the reference-style `sadptools send <IP> activate --mac ...` syntax, with a required `--interface` and stdin/environment password options.

## Build and run the desktop

Toolchain used: Go 1.26.10, Node 24.21.0, npm 11.16.0, Wails **v3.0.0-beta.34**. Dependencies and npm runtime versions are pinned. Install the matching Wails CLI in your user's Go bin directory:

```sh
make tools
make windows-installer
```

The user-facing artifact is **`bin/SADPTools-Setup-windows-amd64.exe`** (about 210 MiB), with a `.sha256` checksum alongside it. This one file contains the app, licence notices and Microsoft's complete offline WebView2 installer. Users do not need Go, Node, Wails, or a separate runtime download.

Packaging requires Python 3.11+ and NSIS 3.10. On Ubuntu 24.04+ AMD64, missing NSIS tools are downloaded, checksum-verified and extracted into the ignored `.cache/windows-installer/` directory automatically; no administrator rights or system package installation are used. Initial builds need internet access to download dependencies. Subsequent packaging can use the verified cache. See [Windows packaging](docs/windows-packaging.md) for build inputs, maintenance and target-platform checks.

`make windows` still produces `bin/SADPTools-windows-amd64.exe` for development; that raw EXE uses an already installed WebView2 runtime. Distribute the **Setup** EXE to end users.

The Windows cross-build runs entirely on Linux, with `CGO_ENABLED=0`; no Docker, MinGW, administrator rights or display is required. The installer command is `wails3 package GOOS=windows GOARCH=amd64`; the binary-only command is `wails3 build GOOS=windows GOARCH=amd64`. Add `$(go env GOPATH)/bin` to PATH when calling `wails3` directly. See [Wails cross-platform builds](https://v3.wails.io/guides/build/cross-platform/).

For Linux:

```sh
make linux-gui
./bin/SADPTools-linux-amd64
make linux-packages          # DEB, RPM and manual installer archive in bin/linux/
```

Launch from a graphical Linux desktop session. Building needs GTK4 and WebKitGTK 6.0 development libraries; the binary needs their runtime libraries. Current Ubuntu/Debian setup, if missing:

```sh
sudo apt update
sudo apt install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
```

No privileged packages were installed during development. Sparx already had these libraries. Older distributions may need Wails' legacy GTK3 build path; the supplied build targets use GTK4. See [Wails installation requirements](https://v3.wails.io/quick-start/installation/). macOS packaging is a future milestone.

## Development and checks

```sh
make check          # Go tests, vet, race detector; frontend and packaging tests
make frontend       # Generate JS bindings, npm ci, Vite production build
make bindings       # Regenerate committed Go-to-JavaScript bindings
make branding       # Regenerate artwork on a Linux authoring host
make dev            # Wails desktop development, on a graphical Linux host
```

To prepare and publish a tagged beta using `gh`, see the [release process](docs/releases.md).
`make release-build VERSION=0.3.0-beta.2` builds and verifies the downloads from a
clean commit. `make release-publish VERSION=0.3.0-beta.2` pushes the commit/tag,
verifies uploaded files and publishes the GitHub pre-release.

For headless Go validation, use `go test ./...` and `go vet ./...`. The root desktop entry uses `//go:build desktop` to avoid requiring GUI development libraries for ordinary engine checks. With desktop dependencies installed, also run `go test -tags desktop ./...` and `go vet -tags desktop ./...`. Desktop builds include `desktop,production`.

Frontend dependencies are bundled into the executable; there is no separate production web server. Vite runs only during development. Launching the frontend in an ordinary browser does not substitute a mock inventory for the Go backend.

Preferences are stored at `os.UserConfigDir()/SADPTools/settings.json` (normally `~/.config/SADPTools/` on Linux and `%APPDATA%\SADPTools\` on Windows). Live discovery and the bounded 500-entry diagnostic log remain session-only. Explicitly saved sites, encrypted credential profiles, observations, audits and switch assets persist in `sites.sqlite` in the same user configuration directory. Windows uses current-user DPAPI; Linux uses an Argon2id/AES-256-GCM master-passphrase vault. Portable backups use a separate passphrase. Ordinary exports omit secrets. Exports include the currently filtered inventory and all advertised fields; CSV protects spreadsheet cells beginning with formula characters.

## Networking and hardware validation

ONVIF optionally probes multicast `239.255.255.250:3702` and receives correlated unicast replies on a per-adapter socket. SNMP polls only explicitly registered switches; IP investigations use their explicitly approved ranges.

Allow inbound UDP **37020** and outbound multicast to **239.255.255.250:37020** on the adapters used for discovery. On Windows, allow the application for the relevant trusted network profile when prompted. No firewall rules are changed automatically. Close other SADP tools if the port is occupied.

Devices may advertise an address outside the host subnet: the engine does not subnet-filter replies. Discovery still needs a shared, reachable multicast Layer 2 network. VLAN separation, client isolation, IGMP snooping, VPNs, host firewall rules and firmware may affect replies. Opening a discovered device's web interface additionally requires IP reachability.

This implementation uses UDP SADP, not the legacy raw-Ethernet protocol or an ARP sweep. Virtual adapters are listed by capability, not excluded by name; select a physical adapter when appropriate. Each adapter is probed using its OS-selected IPv4 multicast address. All adapter addresses are shown by `interfaces`.

## Source layout

| Path | Responsibility |
| --- | --- |
| `cmd/sadptools/`, `internal/cli/` | Standalone CLI and argument/output handling |
| `internal/protocol/sadp/` | Inquiry XML, response parsing, synthetic fixtures |
| `internal/platform/` | Adapter selection, multicast sockets, Windows packet metadata |
| `internal/discovery/` | Scan lifecycle, retries, cancellation and correlation |
| `internal/devices/` | Normalized device model and synchronized inventory |
| `internal/configuration/` | Single-device validation, SADP transactions and confirmed/rejected/uncertain outcomes |
| `internal/app/` | GUI-independent controller, preferences, diagnostics and export |
| `internal/sites/`, `internal/vault/` | SQLite sites/history/migrations and protected credentials/backups |
| `internal/ipscan/`, `internal/provision/` | Evidence-based IPv4 scans and explicit serial write plans |
| `internal/onvif/`, `internal/multidiscovery/` | Scoped WS-Discovery, authenticated reads and protocol aggregation |
| `internal/audit/`, `internal/switches/`, `internal/reports/` | Read-only health checks, SNMP mapping and offline reports |
| `internal/toolkit/`, `desktop_toolkit.go` | Shared task lifecycle, site workflows and Wails toolkit bindings |
| `internal/update/` | GitHub release selection, verified installer downloads and process handoff |
| `main.go`, `desktop.go` | Wails v3 entry point and desktop service |
| `frontend/` | Bootstrap UI, vanilla JS/CSS and generated bindings |
| `build/`, `Makefile`, `Taskfile.yml` | Repeatable Windows/Linux builds and metadata |
| `docs/`, `LICENSES/` | Architecture, research, validation and attribution |

See [architecture](docs/architecture.md), [reference implementation research](docs/reference-research.md) and [validation and hardware checklist](docs/validation.md).

## Licence and attribution

MIT. Protocol templates and XML mappings are adapted from Cameron Newman's MIT-licensed [hikvision-tooling](https://github.com/cameronnewman/hikvision-tooling). Its `internal` packages cannot be imported from this module, so the relevant source was adapted with attribution, not invoked as an external executable. See [third-party notices](THIRD_PARTY_NOTICES.md) and [preserved upstream licence](LICENSES/hikvision-tooling-MIT.txt).

## Live View (0.4.0-beta.2 test candidate)

Choose **Live View** on a discovered device or saved site record. A separate
desktop window opens and starts the sub stream automatically. You can keep
using discovery and Sites while it plays. If credentials are needed, signing
in starts playback immediately. ONVIF profiles, a selected Hikvision channel/port,
custom RTSP/RTSPS, HTTP MJPEG and snapshot refresh are available. Playback is
video-only; no camera settings are changed and nothing is recorded.

H.264 is the primary preview format. H.265 depends on the installed WebView
codec support and is rejected with guidance when unavailable. Pause/Stop closes
the camera connection; session credentials stay in Go memory until the viewer
closes or you explicitly lock saved passwords. Resume does not require another
login. Reopening the same camera focuses its viewer; opening another replaces
the single live stream. Closing the main app also closes the viewer.

The credential vault encrypts saved passwords. Windows makes these available
automatically through your signed-in account; no routine unlock step is needed.
**Lock now** is optional and remains effective until explicitly unlocked or the
app restarts. Linux still requires the master passphrase, requested directly
in the viewer when needed. Active playback keeps its saved credentials in use.

Windows binaries compile, but Windows playback and real cameras still require
operator testing. See the [engine decision](docs/LIVE_VIEW_ENGINE_DECISION.md)
and [test instructions](docs/LIVE_VIEW_TEST_MATRIX.md).

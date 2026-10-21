# SADPTools

A native Go desktop utility and CLI for discovering Hikvision devices. Built with **Wails v3**, Bootstrap 5 and vanilla JavaScript. Windows 10/11 AMD64 is the primary desktop target; Linux is also supported.

Milestone one implements read-only SADP discovery. Activation, address/DHCP changes, password changes and other configuration are intentionally unavailable. SADPTools adds no account, cloud service or telemetry. The Windows display runtime is supplied by Microsoft and has its own update and privacy terms, included with setup.

## Install on Windows

Run **`SADPTools-Setup-windows-amd64.exe`**, follow setup, then open **SADPTools** from the Start menu. Setup includes everything needed to run the application on Windows 10/11 AMD64 and works without an internet connection. There are no separate prerequisites to download or configure.

Setup automatically prepares the display component if needed. It installs SADPTools for the current user and provides an entry in Windows' installed-apps list for removal. Builds are currently unsigned, so Windows may show a publisher/SmartScreen warning. Installation and startup still need validation on Windows; see [validation results](docs/validation.md).

## What works

- IPv4 adapter enumeration and selected/all-adapter multicast discovery on `239.255.255.250:37020`.
- `inquiry` and `inquiry_v32` probes, response correlation, normalized MAC deduplication, serial fallback, malformed-response handling, timeouts, cancellation and repeated scans.
- Device model/type, network settings, MAC, serial, firmware, activation, DHCP, HTTP port and device name when advertised. Unknown values remain empty/null.
- CLI table/JSON output and useful diagnostics/exit codes.
- Desktop inventory with sorting, searching, selection, retained results, device details, native CSV/JSON export, web-interface action, diagnostic log, light/dark themes and saved discovery preferences.
- One active scan at a time. Automatic scanning starts its interval after the previous scan finishes. Last-seen timestamps identify old observations; missing one scan does not mark a device offline.

A one-second scan on Sparx received **17 real devices** during initial validation. This verifies basic Linux discovery on that network, not every firmware, VLAN, firewall or Windows adapter combination. See [validation results](docs/validation.md).

## Run the CLI on Sparx

From `/home/paul/git-repos/SADPTools`:

```sh
make cli
./bin/sadptools interfaces
./bin/sadptools discover
./bin/sadptools discover --all-interfaces --timeout 10s
./bin/sadptools discover --interface "ADAPTER NAME" --timeout 10s --json
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
| 130 | Cancelled with Ctrl+C; any partial results are still printed |

## Build and run the desktop

Toolchain used: Go 1.26.8, Node 24.21.0, npm 11.16.0, Wails **v3.0.0-beta.34**. Dependencies and npm runtime versions are pinned. Install the matching Wails CLI in your user's Go bin directory:

```sh
make tools
make windows-installer
```

The user-facing artifact is **`bin/SADPTools-Setup-windows-amd64.exe`** (about 208 MiB), with a `.sha256` checksum alongside it. This one file contains the app, licence notices and Microsoft's complete offline WebView2 installer. Users do not need Go, Node, Wails, or a separate runtime download.

Packaging requires Python 3.11+ and NSIS 3.10. On Ubuntu 24.04+ AMD64, missing NSIS tools are downloaded, checksum-verified and extracted into the ignored `.cache/windows-installer/` directory automatically; no administrator rights or system package installation are used. Initial builds need internet access to download dependencies. Subsequent packaging can use the verified cache. See [Windows packaging](docs/windows-packaging.md) for build inputs, maintenance and target-platform checks.

`make windows` still produces `bin/SADPTools-windows-amd64.exe` for development; that raw EXE uses an already installed WebView2 runtime. Distribute the **Setup** EXE to end users.

The Windows cross-build runs entirely on Linux, with `CGO_ENABLED=0`; no Docker, MinGW, administrator rights or display is required. The installer command is `wails3 package GOOS=windows GOARCH=amd64`; the binary-only command is `wails3 build GOOS=windows GOARCH=amd64`. Add `$(go env GOPATH)/bin` to PATH when calling `wails3` directly. See [Wails cross-platform builds](https://v3.wails.io/guides/build/cross-platform/).

For Linux:

```sh
make linux-gui
./bin/SADPTools-linux-amd64
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
make dev            # Wails desktop development, on a graphical Linux host
```

For headless Go validation, use `go test ./...` and `go vet ./...`. The root desktop entry uses `//go:build desktop` to avoid requiring GUI development libraries for ordinary engine checks. With desktop dependencies installed, also run `go test -tags desktop ./...` and `go vet -tags desktop ./...`. Desktop builds include `desktop,production`.

Frontend dependencies are bundled into the executable; there is no separate production web server. Vite runs only during development. Launching the frontend in an ordinary browser does not substitute a mock inventory for the Go backend.

Preferences are stored at `os.UserConfigDir()/SADPTools/settings.json` (normally `~/.config/SADPTools/` on Linux and `%APPDATA%\SADPTools\` on Windows). The device inventory and bounded 500-entry log live only for the session. Exports include the currently filtered inventory and all advertised fields; CSV protects spreadsheet cells beginning with formula characters.

## Networking and hardware validation

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
| `internal/configuration/` | Future configuration contracts; no write implementation |
| `internal/app/` | GUI-independent controller, preferences, diagnostics and export |
| `main.go`, `desktop.go` | Wails v3 entry point and desktop service |
| `frontend/` | Bootstrap UI, vanilla JS/CSS and generated bindings |
| `build/`, `Makefile`, `Taskfile.yml` | Repeatable Windows/Linux builds and metadata |
| `docs/`, `LICENSES/` | Architecture, research, validation and attribution |

See [architecture](docs/architecture.md), [reference implementation research](docs/reference-research.md) and [validation and hardware checklist](docs/validation.md).

## Licence and attribution

MIT. Protocol templates and XML mappings are adapted from Cameron Newman's MIT-licensed [hikvision-tooling](https://github.com/cameronnewman/hikvision-tooling). Its `internal` packages cannot be imported from this module, so the relevant source was adapted with attribution, not invoked as an external executable. See [third-party notices](THIRD_PARTY_NOTICES.md) and [preserved upstream licence](LICENSES/hikvision-tooling-MIT.txt).

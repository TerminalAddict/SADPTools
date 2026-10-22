# Architecture and first milestone

1. Research the reference implementation and current Wails v3 APIs.
2. Implement XML inquiry/parsing, adapter enumeration, multicast transport, cancellable discovery and inventory; expose them through the CLI.
3. Add the desktop service, generated bindings and Bootstrap UI, persistent preferences and bounded diagnostics.
4. Test failure paths and lifecycle; build Linux CLI/desktop and Windows AMD64 desktop; document hardware validation gaps.

`internal/protocol/sadp` owns wire data. `internal/platform` owns adapter enumeration and UDP sockets. `internal/discovery` orchestrates scan lifetimes and correlation through a replaceable transport interface. `internal/devices` owns normalized records and a synchronized inventory. `internal/configuration` validates and executes single-device activation, network updates and recovery-code transactions. See [device-management architecture and limits](device-management.md).

`cmd/sadptools` is a standalone CLI. `internal/app` coordinates the desktop inventory, single active scan or configuration operation, settings, periodic work and diagnostics without importing Wails. Root `main.go` and `desktop.go` adapt this controller to Wails services, events, dialogs and browser opening. They require the `desktop` build tag. Ordinary `go test ./...` works on servers with no display or GTK development packages.

Each scan owns one wildcard UDP/37020 socket. It joins the multicast group on selected IPv4 adapters, sends both inquiry variants with a distinct request UUID per adapter, and retries once per second. It accepts replies on any IPv4 subnet, using received interface metadata for attribution, and correlates echoed UUIDs when present. Missing UUIDs are accepted only with a selected receiving interface. Explicitly mismatched UUIDs are rejected. A future compatibility option may be needed for firmware that supplies its own UUID instead of echoing the probe.

Cancellation closes the socket and joins scan work. Timeout normally completes the receive window. Socket errors fail the scan; adapter-specific failures remain warnings in partial results. Overlapping scans are rejected. The GUI retains devices between scans, with first/last seen timestamps and all observed adapters. Absence from one scan does not prove a device is offline.

No source-address subnet filter, ARP sweep, external reference executable, cloud service or application telemetry is involved. Another IP subnet still requires the same reachable multicast Layer 2 domain and compatible firmware/firewall behaviour; this is not raw Ethernet SADP and does not cross VLANs automatically.

Future discovery providers can feed the same inventory. Further configuration protocols, ISAPI, ONVIF and manufacturer adapters should be separate packages with context-aware interfaces and explicit capabilities.

## Device-management milestone

The desktop controller resolves the selected inventory ID to its normalized MAC and checks the chosen adapter against observed interfaces. It prevents activation of a known active device and network updates to a known inactive device. It rejects operations while discovery is running, and automatic discovery cannot start during a command. Shutdown/cancellation closes the transaction socket and joins pending work.

Configuration sends once, with explicit network values, XML-escaped credentials and exact reply correlation. It never retries a write or treats timeout as rollback. Results distinguish not-sent, acknowledged, rejected, challenge and unconfirmed. The inventory changes only through discovery. Passwords and recovery data are excluded from snapshots and diagnostics; recovery data is exported only through an explicit native save dialog. CLI commands use the same configuration engine without Wails.

## Security-question recovery

`internal/recovery` provides a separate ISAPI capability/question reader and
single-submission encrypted reset client. It binds HTTP connections to an
on-link address of a discovered adapter and does not follow redirects or use
proxies. The controller shares its operation/cancellation guard with SADP writes,
discovery and updates, and binds a short-lived one-use check to the selected
device, route and question IDs. Recovery secrets never enter inventory or
snapshots. Desktop bindings and the standalone `recovery` CLI use the same
engine. See [protocol and compatibility details](security-question-recovery.md).

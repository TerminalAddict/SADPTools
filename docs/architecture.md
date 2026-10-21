# Architecture and first milestone

1. Research the reference implementation and current Wails v3 APIs.
2. Implement XML inquiry/parsing, adapter enumeration, multicast transport, cancellable discovery and inventory; expose them through the CLI.
3. Add the desktop service, generated bindings and Bootstrap UI, persistent preferences and bounded diagnostics.
4. Test failure paths and lifecycle; build Linux CLI/desktop and Windows AMD64 desktop; document hardware validation gaps.

`internal/protocol/sadp` owns wire data. `internal/platform` owns adapter enumeration and UDP sockets. `internal/discovery` orchestrates scan lifetimes and correlation through a replaceable transport interface. `internal/devices` owns normalized records and a synchronized inventory. `internal/configuration` defines future capabilities without implementing device writes.

`cmd/sadptools` is a standalone CLI. `internal/app` coordinates the desktop inventory, single active scan, settings, periodic work and diagnostics without importing Wails. Root `main.go` and `desktop.go` adapt this controller to Wails services, events, dialogs and browser opening. They require the `desktop` build tag. Ordinary `go test ./...` works on servers with no display or GTK development packages.

Each scan owns one wildcard UDP/37020 socket. It joins the multicast group on selected IPv4 adapters, sends both inquiry variants with a distinct request UUID per adapter, and retries once per second. It accepts replies on any IPv4 subnet, using received interface metadata for attribution, and correlates echoed UUIDs when present. Missing UUIDs are accepted only with a selected receiving interface. Explicitly mismatched UUIDs are rejected. A future compatibility option may be needed for firmware that supplies its own UUID instead of echoing the probe.

Cancellation closes the socket and joins scan work. Timeout normally completes the receive window. Socket errors fail the scan; adapter-specific failures remain warnings in partial results. Overlapping scans are rejected. The GUI retains devices between scans, with first/last seen timestamps and all observed adapters. Absence from one scan does not prove a device is offline.

No source-address subnet filter, ARP sweep, external reference executable, cloud service, telemetry or device configuration is involved. Another IP subnet still requires the same reachable multicast Layer 2 domain and compatible firmware/firewall behaviour; this is not raw Ethernet SADP and does not cross VLANs automatically.

Future discovery providers can feed the same inventory. Configuration, ISAPI, ONVIF and manufacturer adapters should be separate packages with context-aware interfaces and explicit capabilities.

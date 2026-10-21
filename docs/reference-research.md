# Reference implementation research

Inspected 21 October 2026: [cameronnewman/hikvision-tooling](https://github.com/cameronnewman/hikvision-tooling), commit `afa00e3db11bb47914e1f0d724c6975e0a41e218`. The checkout used for research was outside SADPTools. No reference executable is invoked by this application.

## Architecture and integration

The upstream `cmd/sadp` dispatches through `internal/cli`. Supporting packages cover configuration, logging, SADP, ARP/network utilities, digest authentication, ISAPI and legacy password-reset cryptography. Its Go `internal` import boundary prevents SADPTools from importing those packages directly. A module dependency alone cannot bypass that boundary.

SADPTools adapts the two inquiry templates and wire field mappings into its own `internal/protocol/sadp`, retaining the upstream MIT copyright in source and the full licence in `LICENSES/hikvision-tooling-MIT.txt`. It uses standard `log/slog`, `encoding/xml`, `crypto/rand`, an inventory model and a new cancellable transport/lifecycle rather than bringing in the complete CLI/logger/configuration/reset stack.

## SADP and interfaces

[`internal/sadp/scanner.go`](https://github.com/cameronnewman/hikvision-tooling/blob/afa00e3db11bb47914e1f0d724c6975e0a41e218/internal/sadp/scanner.go) enumerates up, non-loopback interfaces and scans each IPv4 address concurrently. Each worker binds a local **ephemeral** UDP port, sends XML `Probe` messages to multicast `239.255.255.250:37020`, then sends the same messages to limited broadcast. Both `inquiry` and `inquiry_v32` use a UUID. Responses are `ProbeMatch` XML. Upstream's parser maps `DeviceDescription` to the descriptive model, `DeviceType` to the type code, `DeviceSN` to serial, and the network, firmware and activation fields directly. MAC formatting is normalized before deduplication.

Source-level limitations observed in that revision:

- The scanner does not join the multicast group or listen on the conventional fixed response port. It can receive replies sent back to the ephemeral source port, but cannot rely on receiving fixed-port multicast responses.
- No context-based cancellation; each read loop waits for the configured deadline.
- Bind/send/interface errors are mostly debug logs and do not necessarily make `Discover` fail. This can make an unsuccessful scan look like an empty network.
- Inventory persists in the scanner and only adds unseen MACs, so later changed properties and additional adapter sightings are not merged.
- Discovery does not correlate the response UUID. Missing MACs share the empty map key.
- No explicit multicast-capability check or outgoing interface socket option.
- Limited broadcast delivery depends on platform routing and socket behaviour.

SADPTools joins multicast on each selected interface, binds UDP/37020 once per scan, rejects overlapping scans and reports partial failures. It retries inquiries once per second. It reads the actual receiving interface using Unix control messages or Windows `IP_PKTINFO` with `UDPConn.ReadMsgUDP`; it does not guess the interface from the device's source subnet. Broadcast fallback is not implemented in milestone one.

[`commands.go`](https://github.com/cameronnewman/hikvision-tooling/blob/afa00e3db11bb47914e1f0d724c6975e0a41e218/internal/sadp/commands.go) has a separate multicast/broadcast command path and MAC/IP target matching; direct unicast is optional. It includes activation, configuration and password-related templates. None of those write/reset operations was integrated. Discovery accepts echoed UUIDs from the current scan; missing UUIDs require a recognized receiving adapter. Firmware returning unrelated UUIDs requires further investigation, not silent attribution.

## ARP and device identification

`internal/network/arp.go` combines platform `arp` commands, optional TCP/ping probes, CIDR expansion and a Hikvision OUI list. Linux uses `arp -n`; Windows uses `arp -a`; Darwin has its own command handling. Its parser supports platform-specific output, and test seams replace command factories. This is neighbour-cache based discovery with reachability probes, not native raw-Ethernet SADP. OUI lists may be incomplete and cannot prove model or activation state.

ARP is researched but deferred. Milestone one identifies devices from their protocol responses, using normalized unicast MAC addresses and serial fallback, without guessing manufacturer from a prefix or inventing properties from an IP.

## ISAPI

`internal/isapi/client.go` uses a context-aware HTTP client, configurable timeouts/TLS verification, typed HTTP status errors, and `GET /ISAPI/System/deviceInfo`. `internal/network/digest.go` supplies RFC 2617 Digest authentication, including challenge parsing and retry. Tests use `httptest` to validate XML fields, status handling, cancellation and Digest challenge/response. This provides a useful future read-only management integration; no credentials, ISAPI probing or HTTP requests to devices are used during current discovery.

## Tests and reported issues

The reference includes XML/parser, generated-command, redaction, MAC matching, mocked socket/interface, CLI, configuration, ARP, HTTP and ISAPI tests. Its network tests inject fake packet connections for timeout/send/bind failures and deduplication. Those tests demonstrate software behaviour, not a complete hardware compatibility matrix. SADPTools adds its own synthetic fixtures, deterministic lifecycle tests and OS metadata tests rather than claiming upstream hardware coverage.

[Issue #16](https://github.com/cameronnewman/hikvision-tooling/issues/16) reports a Docker/Make build reaching a directory without Go files; a comment points to PR #19. SADPTools builds explicit entry points with Go/Wails tasks and does not reuse that build stack.

## Primary documentation consulted

- [Wails v3 installation](https://v3.wails.io/quick-start/installation/): GTK4/WebKitGTK 6.0 defaults and Windows WebView2 runtime.
- [Wails v3 cross-platform builds](https://v3.wails.io/guides/build/cross-platform/): native Go Windows cross-compilation with CGO disabled.
- [Wails v3 services](https://v3.wails.io/features/bindings/services/) and [method bindings](https://v3.wails.io/features/bindings/methods/): `application.NewService`, generated ES modules and lifecycle hooks. APIs were checked against pinned beta.34 source.
- [Microsoft IP_PKTINFO](https://learn.microsoft.com/en-us/windows/win32/winsock/ip-pktinfo) and [IN_PKTINFO](https://learn.microsoft.com/en-us/windows/win32/api/ws2ipdef/ns-ws2ipdef-in_pktinfo): receive adapter metadata through `WSARecvMsg`. `x/net/ipv4` beta.34's selected dependency still does not implement Windows receive control options, so a small platform-specific implementation is necessary.

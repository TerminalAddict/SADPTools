# Technician toolkit implementation

This work implements the full `SADPTools-Feature-Roadmap.md` at the user's explicit
request, incrementally in the existing application. Candidate version: 0.3.0-beta.1.

## Plan and boundaries

1. Sites: versioned SQLite, protected credential references, manual inventory,
   portable encrypted backup and restore, secret-free exports.
2. Scanner: bounded IPv4 probes, platform ICMP/neighbour capabilities, evidence,
   reservations, grid/table and CLI. No-response never means free.
3. Reconciliation: stored scan observations, stable identity matching, reviewed
   diffs and persistent history. Never silently replace curated records.
4. Provisioning: deterministic preview, protected-address checks, explicit
   approval, serial writes and stable-identity rediscovery. No write retries.
5. ONVIF: WS-Discovery and authenticated read-only device information, bounded
   same-device URLs and protocol-aware capabilities.
6. Audits: read-only checks with explicit unknown states and safe HTML/CSV/JSON.
7. Switches: protected SNMPv3/v2c profiles, read-only FDB/interface/LLDP/PoE
   evidence and conservative port mapping.
8. Regression checks, UI inspection, Windows setup/standalone and Linux builds.

## Persistence and secret boundary

Operational data lives under the OS user's SADPTools configuration directory in
`sites.sqlite`, independently of installation/update paths. Schema changes are
transactional and versioned. Private database files/directories use restrictive
permissions. Existing preferences and transient live discovery remain separate.

Site/device metadata and observations contain credential profile IDs, never
passwords. Profiles encrypt a credential payload with AES-256-GCM and bind the
ciphertext to its profile ID as authenticated data. Windows protects the random
vault key using current-user DPAPI. Linux uses an explicit master passphrase,
Argon2id and authenticated encryption; no desktop keyring is required. The vault
starts locked and locks on inactivity. No plaintext fallback exists.

Portable backups use a separate user passphrase, fresh salt/nonce, Argon2id and
AES-256-GCM. Restores import a new site with new local IDs and re-encrypt profiles
under the destination vault. A restore never replaces an existing site. Ordinary
exports contain no secret payloads. Explicit password reveal/copy requires an
unlocked vault and clears transient UI values; clipboard history may retain copies.

These measures protect data at rest, not a compromised logged-in process or
operating system. Memory copies in Go/JavaScript cannot be comprehensively erased.
No real credential or customer capture belongs in this repository.

## Validation

Validated on Sparx, 23 October 2026:

- `make check` passed: `go test ./...`, `go vet ./...`, `go test -race ./...`,
  21 frontend tests, five installer-input tests and twelve release-tool tests.
  New fixtures cover migrations, vault inheritance/expiry/encryption/restore,
  conservative identities, scanner evidence/ranges/pagination/cancellation,
  ONVIF XML/authentication/TLS pins, mixed provisioning outcomes with no retries,
  audit partial/error results, SNMP/PoE correlation and report escaping.
- `make frontend`, `make cli`, `make linux-gui` and `make windows-installer`
  passed. Packaging includes the Windows AMD64 desktop cross-build and offline
  WebView2 runtime. `go test -tags desktop ./...` and
  `go vet -tags desktop ./...` also passed. Generated Wails bindings are included.
- The Windows standalone PE is AMD64 and retains its icon, icon-group, version
  and manifest resources. Version stamps in the Windows desktop, Linux desktop
  and CLI are `0.3.0-beta.1`. Installer `7z t` and SHA-256 checks passed.
  The standalone is 18,161,152 bytes; setup is 220,130,624 bytes. Both are local,
  unsigned test candidates, not a published GitHub release.
- Production Linux GUI checks ran under Xvfb with isolated preferences. Empty
  views, site creation and vault unlock worked. A separate, clearly labelled
  synthetic site database exercised saved inventories, inherited/none credentials,
  before/after review, audit reports, switch/PoE displays and native HTML export.
  Accepting IP/firmware changes preserved the manual name, notes and credential
  assignment. Restart retained both sites, records, observations and accepted
  changes, and the vault reopened locked. Exports used private file permissions
  and omitted the synthetic credential. Fixtures are outside the application and
  repository source; normal startup does not insert sample devices.
- A read-only CLI scan with `discover --protocols sadp,onvif --timeout 3s --json`
  returned 18 SADP and 18 ONVIF observations, with no identity-conflict flags.
  These are protocol observations, not 36 proven distinct cameras. Unauthenticated
  ONVIF discovery lacked the strong identifiers needed to merge the two groups;
  IP equality alone is deliberately insufficient. No device-write request was sent.
- Windows execution, DPAPI use on Windows, ICMP/IP Helper runtime behaviour,
  installation/update handoff, authenticated physical ONVIF queries, physical
  camera writes and switch/PoE correspondence remain untested. Use the hands-on
  checklist below before treating these as hardware-validated capabilities.

Build outputs are `bin/SADPTools-Setup-windows-amd64.exe`,
`bin/SADPTools-windows-amd64.exe`, `bin/SADPTools-linux-amd64` and `bin/sadptools`,
with SHA-256 sidecars. On Sparx run `./bin/sadptools interfaces` or
`./bin/sadptools discover --timeout 5s`; launch `./bin/SADPTools-linux-amd64` from
a graphical Linux session. Windows testers can run setup or use the standalone
EXE where WebView2 is already installed.

## Implemented workflows (0.3.0-beta.1)

- **Sites:** create/edit/archive/delete, persistent offline device inventories,
  save selected/all live devices, cross-site search, manual metadata and credential
  assignments, explicit merge/move, reservations, ordinary exports and encrypted
  portable backups. The most recently visited site and IP scan preferences persist.
- **IP scanner:** selected adapter and CIDR/range, native Windows ICMP / Linux
  datagram ICMP with raw fallback, neighbour-cache evidence, optional TCP probes,
  on-demand DNS, bounded workers/rate, cancellation, 10-column grid and paginated
  table, reservation/association actions, saved scans and CLI `scan`.
- **Site review:** explicit saved scope approval, SADP/ONVIF protocol selection,
  timestamped observations, per-field before/after review, manual values kept by
  default, accept/ignore/intentional removal and auditable history export.
- **Provision:** select live devices or resolve selected saved devices against
  current discovery; preview activation/network changes and sequential assignments;
  exclude reserved, observed-owned, responding, local, gateway and subnet-boundary
  addresses; explicit one-use plan approval and credentials; serial execution,
  fresh identity/occupancy checks and post-write SADP verification. Unconfirmed
  actions are never retried and stop that device's dependent actions. Rejected
  devices do not prevent the queue from reporting other devices individually.
  Only independently verified results may update the approved site inventory.
- **ONVIF:** per-adapter WS-Discovery, bounded/correlated SOAP parsing, partial
  vendor-neutral records, same-IP service URL validation, authenticated read-only
  information/network/protocol queries using HTTP Digest and WS-Security, explicit
  certificate inspection/trust, strong-identity merging and Hikvision action gates.
- **Audit:** saved-site/subset read-only network/duplicate/firmware observation,
  advertised HTTP/HTTPS/SDK/RTSP reachability, web/TLS checks, and ONVIF authentication,
  DNS/NTP/clock checks where supported. Status/evidence/source/time accompany each
  finding; cancellation preserves a partial report. HTML/CSV/JSON exports are offline.
- **Switches:** registered site assets, vault references, SNMPv3 authPriv using
  SHA-256 or SHA-512 and AES-128 (explicit legacy SHA-1 option), opt-in SNMPv2c,
  fixed read-only IF/FDB/VLAN/LLDP/PoE queries, conservative camera-to-port evidence,
  uplink annotations and explicit PoE/LLDP index bindings. Polls and port mappings
  persist and appear in saved device details; reports export without secrets.

## Source map

| Component | Main new files |
|---|---|
| Desktop integration | `desktop_toolkit.go`, `internal/app/tasks.go`, generated `frontend/bindings/.../toolkitservice.js` and model bindings |
| Persistence | `internal/sites/{store,models,inventory,credentials,backup,preferences,switches}.go` and migration/store tests |
| Secret protection | `internal/vault/{crypto,platform_windows,platform_other}.go` and crypto tests |
| IPv4 scanner | `internal/ipscan/{range,scan,ping_windows,ping_unix,neighbours_windows,neighbours_linux,neighbours_other}.go`, tests, `internal/cli/scan.go` |
| Adapter gateways | `internal/platform/gateway_{windows,linux,other}.go` |
| ONVIF discovery/information | `internal/onvif/{discovery,client,certificate}.go`, tests, `internal/multidiscovery/engine.go` and tests |
| Provisioning | `internal/provision/{plan,runner}.go` and tests |
| Audits and exports | `internal/audit/audit.go`, `internal/reports/export.go` and tests |
| Switch diagnostics | `internal/switches/{models,snmp,decode}.go` and tests |
| Job coordination | `internal/toolkit/{manager,discovery,scanner,provision,audit,switches}.go` and tests |
| UI | `frontend/src/{toolkit,toolkit-ui,sites,scanner,reconciliation,provision,audit,switches}.js`, `toolkit.css`, helper tests |

Existing entry points, device model/inventory, SADP parser, web/recovery selection,
settings, live UI, version/build files and dependency/attribution records were
extended in place. The original feature roadmap is retained as the specification;
this guide records the delivered behaviour and validation limits.

## Compatibility and operational limits

- No camera or switch write protocol was invented. Bulk provisioning reuses the
  existing SADP commands and inherits their firmware limitations. An acknowledgement
  alone is insufficient for a confirmed result. A cancelled/timed-out command may
  already have changed the device. No rollback is promised.
- Linux ICMP availability depends on ping-socket/raw-socket permissions; unavailable
  methods report limitations. Windows uses built-in ICMP and IP Helper APIs without
  installing a packet driver. Neighbour cache entries are labelled cached and do
  not become fresh successful probes. No response never means free.
- IP scans are capped at 65,536 addresses, 64 workers and 200 hosts/second. Per-host
  timeouts are 100–3,000 ms, with at most eight optional TCP ports. Public/routed/
  large scopes require explicit confirmation. Saved site multicast scans use one
  selected adapter and retain only observations inside the site's approved ranges.
- ONVIF depends on device support and enabled services/accounts. Only numeric IPv4
  HTTP(S) service URLs on the responding device are accepted, with ONVIF paths and
  no redirects, URL credentials or cross-device forwarding. Self-signed certificates
  require explicit fingerprint acceptance before authenticated access. ONVIF-only
  devices cannot use Hikvision activation/network/recovery controls.
- The audit checks implemented ONVIF services and network evidence. It does not
  guess ISAPI health endpoints, firmware upgrade suitability, storage health or
  uptime: unsupported checks remain Not checked/Unknown. PC clock correctness is
  a prerequisite for interpreting clock skew and WS-Security timestamps.
- SNMP requires an existing switch account/view. Only GET/GETBULK walks of fixed
  diagnostic columns are sent; no SNMP SET, PoE cycling or port configuration exists.
  Polling is bounded to 90 seconds / 50,000 values, with no transport retries.
  Forwarding-table entry age is generally unknown. Multiple MACs, trunks, uplinks,
  port moves and contradictory evidence produce Ambiguous results, not a direct-port
  claim. LLDP chassis-MAC plus single learned FDB MAC on an up interface can support
  Direct/strong confidence. VLAN-specific contexts must be supplied when required.
- Standard POWER-ETHERNET-MIB supplies PoE detection state, not generic per-port
  watts. Cisco's documented consumption MIB is read on Cisco enterprise agents;
  milliwatts are converted to watts. PoE indices are mapped through Cisco physical/
  ENTITY-MIB evidence or explicit operator bindings, never assumed to be ifIndex.
  Unsupported power values remain null/Unknown rather than 0 W.
- Schema 2 adds switch assets to schema 1. Migration makes a private SQLite backup
  before transactional changes. Portable backups include assets and rebind profile
  references on restore into a new site. A newer unknown schema is rejected.
- Vault inactivity lock is 15 minutes. Windows protection is tied to the current
  Windows user; use a portable encrypted backup before changing PCs/accounts.
  There is no master-passphrase recovery mechanism; keep encrypted backups and
  their passphrases safe. Explicit clipboard copies may remain in OS history.

## Protocol references

- [ONVIF Core specification](https://www.onvif.org/specs/core/ONVIF-Core-Specification.pdf)
  and [Device Management WSDL](https://www.onvif.org/ver10/device/wsdl/devicemgmt.wsdl).
- [IF-MIB, RFC 2863](https://www.rfc-editor.org/rfc/rfc2863.html),
  [BRIDGE-MIB, RFC 4188](https://www.rfc-editor.org/rfc/rfc4188.html),
  [Q-BRIDGE-MIB, RFC 4363](https://www.rfc-editor.org/rfc/rfc4363.html),
  [Power Ethernet MIB, RFC 3621](https://www.rfc-editor.org/rfc/rfc3621.html).
- [LLDP MIB reference from H3C](https://www.h3c.com/en/d_202606/2850257_294551_0.htm)
  and [Cisco's published POWER-ETHERNET-EXT MIB](https://github.com/cisco/cisco-mibs/blob/main/v2/CISCO-POWER-ETHERNET-EXT-MIB.my).

## Hands-on acceptance checklist

Use a clearly labelled test site and authorised equipment; take an encrypted site
backup before upgrading a PC with real saved inventories.

1. Windows 10/11 AMD64: install the offline Setup EXE, including on a clean machine;
   launch the standalone EXE on a machine with the display runtime already present.
   Check icons, About version, light/dark themes, shutdown and existing updater flow.
2. Create two sites, save separate real inventories, restart and verify isolation.
   Test DPAPI unlock, inherited/override/none credentials and explicit copy/reveal.
   Restore an encrypted backup on another user/PC with the backup passphrase.
3. Scan a known /24 and /25, check /31 boundary display, try a routed scope and an
   unavailable adapter, cancel part-way through, save/export and reopen observations.
   Verify a ping-blocked host with an explicitly enabled TCP probe.
4. Enable ONVIF on compatible Hikvision/other-vendor devices; inspect partial
   discovery first, then query with a dedicated ONVIF account. Check wrong credentials,
   HTTPS fingerprint acceptance, mixed SADP/ONVIF identities and action restrictions.
5. Rescan a site after an authorised address change. Review IP/firmware/activation
   diffs, keep a manual value, ignore an observation and verify history after restart.
6. On specifically authorised spare cameras, preview a small batch; test activation
   then static/DHCP settings, a rejection, interruption after send and rediscovery.
   Never repeat an unconfirmed action before manually establishing the actual state.
7. Run an audit against reachable, unreachable and authentication-denied devices.
   Check statuses and evidence, cancel, and open exported HTML/CSV/JSON reports.
8. Poll a switch using a read-only SNMPv3 account; test explicit v2c separately if
   needed. Compare FDB/VLAN/LLDP mappings with physical cabling, uplinks, a moved MAC,
   and PoE readings. Validate any manual PoE/LLDP bindings against switch documentation.

# Moving sites between laptops

In **Sites**, select a site and choose **Export site**. Set a site file password
(at least 12 characters) and save the `.sadpsite` file. On the other laptop choose
**Sites → Import site**, enter that file password, choose the file, review the
counts/name and select **Import as new site**.

The encrypted file contains the complete saved site: camera records, known saved
passwords and usernames, shared/default credentials and per-camera overrides,
notes, tags, address reservations, switches and their credentials, history, saved
scans/reports, plans, documents and attachments, including encrypted device
configuration backups. Unknown passwords stay unknown. One-session sign-ins that
were never saved to the site's credentials are not part of a saved site.

Import creates new identifiers and remaps references in one transaction. It
preserves other sites, including sites with the same name, and performs no camera
connections or configuration writes. Network adapter bindings are cleared because
they belong to the old laptop; select the local adapter in **Edit site** before
rescanning. Saved IPv4 networks remain available. No IP address is used to merge
two records automatically.

The file password travels with the file, independently of each laptop's local
credential protection. Windows protects imported passwords for the current Windows
user. Linux/macOS may ask you to create or unlock their local saved-password
passphrase. The importer encrypts secrets again with that laptop's key; copying a
Windows-user-specific database/key is unnecessary. Existing `.sadpsite` backups
(payload versions 1 and 2) remain accepted.

**Export report** still produces CSV, JSON or HTML inventory reports without
passwords. Those reports are distinct from complete site transfer files.

## Import an existing NX Camera Info export

The importer accepts the existing JSON `Report` from
[NX Camera Info](https://github.com/TerminalAddict/nx-camera-info), including its
current CLI and GUI output. No changes to NX Camera Info are required:

1. Open the Nx database in NX Camera Info.
2. Reveal passwords if they should be transferred and clear any filter if the
   entire camera list should be included.
3. Export **JSON**. Alternatively use its CLI:

   ```sh
   nx-camera-info --db /private/path/ecs.sqlite --show-passwords --format json --output /private/path/cameras.json
   ```

4. In SADPTools choose **Import site**, leave the site file password blank, and
   select that JSON file. Choose the site name in the review screen.

NX JSON with revealed credentials is plaintext: keep the source file private.
SADPTools reads it directly in the Go backend; passwords and raw file contents
are not sent to the WebView, diagnostic logs or ordinary reports. Imported saved
credentials use the existing encrypted vault. You can subsequently export the
site as an encrypted `.sadpsite` for further transfers.

The NX adapter uses `summary.passwords_revealed`, `credential_status`,
`has_password` and `password` together. Masked/unavailable passwords are never
saved as placeholders. A known empty password can be preserved. Virtual cameras
are omitted with a count. Hostname/IPv6-only records retain their metadata with
an empty IPv4 field and a review note; no DNS lookup is performed. Camera IP,
name, vendor/model, MAC, serial, firmware, group and server references are mapped
where available. NX record IDs stay source references, not camera UUIDs.

HTTP/HTTPS/RTSP ports are taken only from identified services; NX `media_port`
is not assumed to mean RTSP. Raw URLs, URL credentials/tokens, arbitrary Nx
properties and credential decode errors are not copied into site metadata.
Known RTSP stream addresses (`stream_urls`, or an RTSP camera `url`) and a valid
`onvif_url` are retained separately as encrypted camera attachments. Live View
selects **Imported camera stream** automatically and uses the assigned camera
credential. These addresses remain in Go, never in the WebView or ordinary
reports, and travel with an encrypted `.sadpsite` export. Numeric NX stream slots
are ordered main, sub, then additional streams; duplicate/empty URLs are omitted.
The sub stream is preferred when present. An endpoint must address the selected
camera's numeric IPv4 address; foreign hosts and unsupported URLs are omitted
with a count-only warning. URL userinfo is discarded in favour of the assigned
camera credential. No URL guessing or automatic authentication retries occur.
An inventory IP change requires fresh stream addresses or a custom URL.

Sites imported before stream-address support need to be re-imported from the
original NX export. Import creates a new site; it does not overwrite the old one.
If a camera has neither saved endpoints nor advertised ONVIF services, Live View
only assumes Hikvision paths for identified Hikvision/SADP cameras. Other cameras
show the custom URL field with instructions.

Imported inventory does not claim discovery, activation status or connectivity
that the source did not establish. Discovery capabilities are verified later.

## A stable format for other applications

For an explicit site export in NX Camera Info or another tool, generate the
versioned **SADPTools site JSON v1** contract below. This is an import format for
site metadata/camera inventory/credentials. Complete SADPTools exports continue
using the encrypted `.sadpsite` format to preserve every associated resource.

See [the JSON Schema](schemas/site-transfer-v1.schema.json) and
[the synthetic example](examples/site-transfer-v1.json). All addresses and
passwords in the example are synthetic test data.

```json
{
  "format": "sadptools-site",
  "version": 1,
  "site": {"name": "Synthetic workshop", "networks": ["192.0.2.0/24"]},
  "cameras": [{
    "name": "Synthetic entrance",
    "ipv4": "192.0.2.10",
    "vendor": "Example manufacturer",
    "model": "Synthetic model",
    "credentials": {"username": "fixture", "password": "SYNTHETIC-example-only"}
  }]
}
```

- Top-level `format`, `version`, `site` and `cameras` are required. Unknown fields
  and unsupported versions are rejected instead of silently discarding data.
- `site.name` is required. Optional site metadata: `customer`, `address`, `notes`,
  `tags`, `networks` (IPv4 CIDRs) and `defaultCredentials`.
- Camera fields: `name`, `ipv4`, `mac`, `serial`, `vendor`, `model`, `firmware`,
  `subnetMask`, `gateway`, `httpPort`, `httpsPort`, `rtspPort`, `activated`, `dhcp`,
  `location`, `notes`, `tags`, `credentials` and `inheritCredentials`.
- Omit unknown fields. Ports and booleans may be `null`; do not substitute `0`,
  `false`, guessed addresses or asterisks. Ports must be 1–65535 when supplied.
- `credentials` and `defaultCredentials` contain `username` and `password`.
  The password field is required whenever a credential object is supplied. An
  empty string means a known empty password; omitted/null credentials mean no
  supplied credential. Strings are preserved, including Unicode and colons.
- A camera's explicit credentials override the site default. Otherwise the
  default is inherited unless `inheritCredentials` is `false`. Explicit
  credentials with `inheritCredentials: true` are rejected as contradictory.
- Device IDs and database keys are assigned by SADPTools. Import preserves
  separate camera records rather than silently merging cameras sharing an IP.
- Limits: 10,000 camera records, JSON input/payload 64 MiB, encrypted file 128 MiB.
  The site name is limited to 200 UTF-8 bytes, username to 256 bytes, password to
  4096 bytes and a device's metadata to 64 KiB. Existing attachment limits apply
  to full backups (16 MiB per attachment, 32 MiB combined).

For existing NX JSON, the schema marker is absent and the top-level `summary`
and `cameras` objects distinguish its report format. Standard inventory report
JSON/CSV, an `ecs.sqlite` database, and arbitrary vendor exports are not accepted
as site files.

## Implementation and verification

`internal/sites/transfer*.go` parses/bounds/normalises imports and creates a
private immutable snapshot. `internal/toolkit/transfer.go` issues a one-use review
token, clears the snapshot on cancel/replacement/ten-minute expiry/shutdown, and
commits via the same transaction as the existing portable restore. Failed vault
unlock or validation can be retried without partial sites. The desktop service
uses Wails v3 native file dialogs; the frontend receives only safe summary counts.

Run engine and UI checks:

```sh
go test ./...
go vet ./...
go test -race ./...
npm --prefix frontend test
make windows VERSION=1.0.0
make linux-gui VERSION=1.0.0
# Optional browser tests use the website's pinned development dependencies:
npm --prefix web ci
node frontend/tests/site-transfer-browser.mjs
```

Tests cover two independent vaults, shared/overridden/unknown/empty passwords,
encrypted attachments, reference remapping, existing-site isolation, NX masked
and revealed exports, invalid inputs, cancellation, expiry and replay rejection.
All test credentials and cameras are synthetic. Test with your saved site files
on two actual laptops before relying on the transfer as your only backup.

Initial site-transfer validation on Sparx (2026-10-28): full Go tests, vet and race checks passed;
36 frontend tests and four light/dark desktop/mobile browser scenarios passed,
including axe checks. The local NX Camera Info CLI generated
a JSON export from its synthetic demo database; SADPTools imported its eight
physical camera records and all six recovered credential pairs. This implementation
does not require changes to NX Camera Info. JSON Schema validation tooling was not installed; the schema
and example parse as JSON and the Go importer tests the documented example.

Initial local test artifacts were stamped **0.6.1-dev**; the stable release is **1.0.0**:

- `bin/SADPTools-windows-amd64.exe`
- `bin/SADPTools-Setup-windows-amd64.exe` (includes the offline WebView2 runtime)
- `bin/SADPTools-linux-amd64`

Compilation and synthetic tests do not verify native Windows/macOS file dialogs,
Windows DPAPI between real user accounts, or transfers of your real saved sites.
Those still need testing on the destination laptops. The site-transfer tests did
not contact cameras. Subsequent read-only hardware playback checks are recorded
in [Live View compatibility](live-view-compatibility.md).

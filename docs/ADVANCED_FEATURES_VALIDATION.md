# v0.5.0-beta.1 validation

Validated on Sparx on 26 October 2026; prepared for release on 27 October 2026.
This file separates implementation and synthetic evidence from hardware claims.

## Implemented scope

All ten roadmap workflows are integrated with the existing services, sites and
separate Live View window. See [the user guide](ADVANCED_FEATURES.md). Production
configuration restore remains gated exactly as the roadmap requires. Full vendor
config export, verified recording status, RTSP UDP, preset-only rename, arbitrary
font-script coverage and Windows named-pipe SSH agent support are not claimed.

## Automated checks completed during development

- `go test ./...`, `go vet ./...`, `go test -race ./...` through `make check`.
- Frontend Node tests, including coordinate/resize/history/focus math and missing
  checklist evidence and WebKit playback liveness: **31 passed**.
- Existing Windows and Linux packaging Python tests through `make check`: **34 passed**.
- `go test -tags desktop -race .` on Linux (actual Wails ownership tests).
- Desktop-tagged `go vet` on Linux and Windows AMD64.
- `make frontend`: regenerated and retained Wails bindings; npm clean install
  and production Vite build.
- Windows AMD64 standalone and offline Setup build succeeded; Linux GUI, DEB,
  RPM and manual archive builds succeeded after the interaction-review fixes.
- Windows installer archive integrity: `7z t` passed. Linux CLI reports
  `0.5.0-beta.1` and versioned `capabilities --json` schema 1.
- Final Windows binary is a GUI PE32+ AMD64 executable; Linux GUI/CLI are ELF64
  AMD64. Version strings and installer/menu/icon payloads were checked.

## Feature fixtures

| Area | Evidence |
|---|---|
| NVR | Generated 0/4/16/32-channel XML, missing status/identity, duplicate addresses/names, actual stream mapping, stable matching; ISAPI method/endpoint/auth/redirect failures |
| Streams | Authenticated loopback RTSP H.264/H.265 fixtures; codec/size checks, actual RTP counters, unknown decoded metrics, bounded state history, auth/redirect/resource cleanup |
| PTZ | Fake SOAP target/profile and timeout checks, mismatched serial blocks writes, unsupported profile/fixed camera, stale preset, lost/malformed write response remains unconfirmed; fake-provider Stop/watchdog and no retry |
| Events | Synthetic motion active/inactive, line crossing, unknown labels, malformed XML, subscription reference/action and unsubscribe; finished-session/evidence-ID validation |
| Configuration | Encrypted package round trip, wrong passphrase, corruption, future version, structured secret redaction, firmware mismatch, production gate, changed target and uncertain write halting |
| Storage | Deterministic single/many/mixed channels, schedule/duty/calendar/audio, VBR ranges, TB/TiB conversion, missing/zero/invalid/nonfinite values |
| Plans/storage | Bounded image decode, invalid/active content, normalised coordinates, undo/redo isolation, cross-site document/image rejection, portable backup/re-encryption/remapping, referenced image deletion blocked |
| Reports | 200-device multi-page PDF, Māori/Latin text, image inclusion, stable repeated output, HTML escaping, secret/client redaction and immutable snapshots |
| SSH | Real in-process loopback SSH server: correct/wrong host key, capabilities/schema, missing executable, partial results, malformed report, cancellation, hostile arguments; import stays in selected site and out of live discovery |

All fixtures are synthetic. No camera configuration or movement was exercised on
physical equipment during development.

## Native Linux GUI checks

An isolated `XDG_CONFIG_HOME` and Xvfb session on Sparx use a conspicuously labelled
SYNTHETIC site, generated plan and cached NVR/channel/scenario data. Automatic
network discovery and update checks are disabled in this fixture profile. Normal
application data is not changed and no fake result is shipped as a real device.

The native main window and commissioning tabs rendered. The 16-channel cache,
unknown/offline status, saved scenario comparisons and plan image/marker rendered.
An older checklist with null evidence revealed a frontend loading bug; it was
fixed with a regression test. Plan geometry was corrected to preserve image
aspect ratio. The final integration smoke check also:

- Opened a cached advertised NVR channel in the separate viewer and automatically
  played the loopback H.264 fixture. No physical NVR was contacted.
- Fixed a WebKit case where frame callbacks exist but never fire. Playback
  liveness uses advancing time; displayed FPS correctly remains Unknown.
- Completed a 15-second connection test and saved its real RTP bitrate, codec,
  state timeline and channel ID through the Wails service into the isolated site.
- Rendered thirds guides and the relative sharpness trend, captured before/after
  frames and saved a PNG snapshot associated with NVR stream 101.
- Closed the viewer and verified no TCP connection remained to the test camera.
- Saved a commissioning checklist and immutable report, then exported a **7-page
  PDF through the native save dialog**, with NVR/stream/storage/plan/audit tables
  and two figures. PDF text extraction checked every page and confirmed the
  selected IP/serial redaction. A separate automated fixture covers 200 devices.

Xvfb uses software rendering. These checks do not establish Windows/WebView2,
Wayland, accelerated graphics or physical-camera compatibility.

## Required human-operated validation — NOT yet passed

- [ ] Windows 10/11 standalone and offline Setup install/upgrade/uninstall;
  embedded icon, WebView2 setup, notifications/Stop/close and saved credentials.
- [ ] Real camera: model/firmware, H.264 and H.265/smart-codec limitations,
  source-vs-displayed metrics, 15-second tests, snapshots/alignment/fullscreen.
- [ ] PTZ/focus camera: explicit authorised movement, release/blur/close/watchdog
  Stop, reconnect after loss, preset add/recall/delete and one-pass tour.
- [ ] Real Hikvision NVR: internal PoE channels, connection/recording evidence,
  advertised stream mappings and manual identity conflicts.
- [ ] ONVIF event camera: topics, motion start/end, clock offsets, timed stop,
  unsubscribe/expiry and a saved graded test.
- [ ] Read-only configuration capture/diff and encrypted package on named firmware.
  Do not enable a production restore provider without separately approved tests.
- [ ] Other-vendor ONVIF profile/PTZ/imaging/event interoperability.
- [ ] Real remote SSH host: key verification, CLI capabilities, partial results,
  cancellation and explicitly reviewed site import over the intended route.
- [ ] Linux distribution desktop install/menu icon and real-camera preview.

The app remains unsigned while the SignPath Foundation application is pending;
this work does not establish Windows SmartScreen reputation.

## Release artifacts

The release workflow rebuilds from the clean committed source into
`bin/releases/v0.5.0-beta.1/`, reruns the required checks and records that exact
commit in `build-manifest.json`. Publication downloads every GitHub asset and
checks it against the local SHA-256 before making the beta public. See
[release notes](releases/v0.5.0-beta.1.md) for downloads and update instructions.

## Earlier local test candidate

The original pre-commit test downloads remain in `bin/test-builds/v0.5.0-beta.1/`:

- `SADPTools-0.5.0-beta.1-Setup-windows-amd64.exe` — offline Windows installer.
- `SADPTools-windows-amd64.exe` and the standalone Windows ZIP.
- Windows and Linux CLI archives; `SADPTools-linux-amd64` desktop executable.
- Linux DEB, RPM and manual `install.sh` archive.
- `README.txt`, the technician guide, this validation record, licences,
  `build-manifest.json` and `SHA256SUMS`.

The manifest identifies the baseline Git commit, an exact working-tree source
fingerprint and artifact hashes; it does not mislabel uncommitted work as a
release commit. `SHA256SUMS` can be checked with `sha256sum -c SHA256SUMS` inside
that directory. That development-stage manifest describes the earlier test
candidate; the release manifest above identifies the committed public build.

Linux packaging uses the nFPM tool bundled with the pinned Wails CLI; system
`rpm` is not installed on Sparx. RPM creation and packaging tests passed, but a
Fedora installation has not been exercised. No privileged packages were installed.

# Live View test matrix — 0.4.0-beta.1

This is a local test candidate. No physical camera credentials or captures were
used during development, and no camera configuration was written.

## Automated and local checks

| Check | Result / boundary |
|---|---|
| Selected-IP URL validation, userinfo/alternate host/redirect rejection | Automated tests pass |
| H.264 and H.265 authenticated RTSP over TCP | Synthetic loopback source; RTP depayload, fMP4 init and fragment validation pass |
| RTSP authentication rejection and redirect target isolation | Pass; target receives no redirected requests |
| TLS untrusted/wrong pin/approved exact fingerprint | Synthetic HTTPS tests pass |
| HTTP snapshot and multipart MJPEG | Synthetic JPEG tests pass; oversized image rejected |
| ONVIF Media1/Media2 profiles and stream URI | Generated SOAP tests pass; XML token escaping, authority restrictions and authentication failure handling checked |
| ONVIF malformed/oversized metadata and clock correction | Pass |
| Inventory identity and vault assignment | Pass; contradictory identities rejected |
| Locking vault while streaming | Pass; fake engine drains after lock; discovery task remains available |
| 100 start/stop cycles | In-process session/fake-engine lifecycle passes, including single-use tickets and no concurrent engines; not 100 physical-camera cycles |
| Concurrent starts, expired ticket, renderer heartbeat timeout | Pass |
| Cancellation during connection | Pass |
| Frontend unsupported codec, queue bounds and disposal | Node tests pass |
| Linux integrated H.264 video | Native Wails/WebKitGTK 2.52.6 under Xvfb; moving synthetic RTSP pattern, 320 × 180, 10 displayed frames/s |
| Linux integrated H.265 video | Same native runtime, moving 320 × 180 HEVC pattern at 10 displayed frames/s; Windows HEVC remains unverified |
| Pause, reopen, fullscreen and close | Exercised in the Linux integrated panel; Stop removes the RTSP TCP connection |
| Windows standalone and offline setup | Cross-compilation/package build succeeds; runtime tests pending |
| Linux GUI | Builds and starts under Xvfb; physical desktop / Wayland validation pending |

## Required Windows / camera acceptance

1. Run the new standalone executable, or install the new setup package. Confirm
   About shows **0.4.0-beta.1**. Existing saved sites should still be present.
2. Discover a known, activated Hikvision camera. Select it and choose **Live View**.
   The same action is available on saved-site records and device context menus.
3. If a matching saved device has assigned credentials, select that credential
   source and unlock the vault from Sites if needed. Otherwise enter the camera
   username/password for this session. Nothing is saved automatically.
4. Begin with **Sub stream**. If ONVIF is advertised, the default source reads
   its profiles. If ONVIF is unavailable, explicitly select **Hikvision RTSP
   endpoint**. Press **Play / Retry**. Observe actual movement, not just a still
   frame. Do not count successful RTSP setup as playback success.
5. Stop, restart, pause, close, reopen, select another camera, resize and use
   fullscreen. Discovery and the site manager should remain responsive. Manual
   session passwords must be re-entered after stopping/retrying; vault passwords
   are fetched again in Go.
6. Try Main and an explicit ONVIF profile. For NVRs choose an explicit profile or
   use the **Hikvision channel** control; automatic ONVIF selection cannot infer
   the operator's intended NVR channel from resolution alone.
7. If the stream is H.265, record the Windows version, WebView2 version, camera
   model/firmware and whether real frames appear. A decoder limitation message
   is expected on unsupported machines. Try an H.264 sub stream without changing
   camera encoding through SADPTools.
8. Exercise incorrect credentials, blocked RTSP port, offline camera, network
   interruption and vault lock. Expect a bounded error and explicit Retry.
   Check Task Manager for lingering SADPTools processes after exit.
9. Try routed cameras, custom RTSP ports, self-signed HTTPS/RTSPS, HTTP MJPEG and
   ONVIF snapshots where the equipment supports them. Verify fingerprints using
   a trusted channel. Do not accept a certificate solely because it was shown.
10. On a clean Windows machine, test setup's WebView2 handling. Test both Windows
    10 and 11 and different GPU drivers/DPI scales. Test Linux under a real
    desktop, including Wayland; Xvfb alone does not validate those environments.

Report model, firmware, selected source/profile, codec, OS/WebView version and
the sanitised Diagnostics text. **Do not include passwords, security answers,
credential-bearing URLs, sensitive screenshots or raw network captures.**

## Developer commands

```sh
go test ./...
go vet ./...
go test -race ./...
npm --prefix frontend test
make frontend
make windows-installer VERSION=0.4.0-beta.1
make linux-packages VERSION=0.4.0-beta.1
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -tags desktop,production ./...
```

An optional test-only command serves a synthetic loopback RTSP pattern and
creates an isolated test site directory:

```sh
go run -tags testcamera ./cmd/synthetic-camera
# For HEVC, add -hevc. Keep this command running during the GUI check.
# Use the exact temporary directory printed by the command:
XDG_CONFIG_HOME=/tmp/sadptools-synthetic-preview-PRINTED-VALUE ./bin/SADPTools-linux-amd64
```

Open Sites → SYNTHETIC PLAYBACK TEST → Live View. No camera password is needed
for this isolated source. Normal SADPTools builds never launch this server or
populate synthetic devices. Test fixtures and generation commands are in
`internal/streaming/testcamera/testdata/`. FFmpeg is not required to run tests.

Recorded/generated inputs exercise protocol and lifecycle behaviour only.
Physical Hikvision, smart-codec, HEVC hardware decoding, Windows install/update,
firewall and long-duration resource behaviour remain separate acceptance tasks.

## Final local build results

- `make check` passed: all Go tests, `go vet`, race tests, 25 frontend tests,
  and 34 Python build/packaging tests. Desktop-tagged vet passed for Linux and
  Windows; Linux reported only upstream GTK deprecation warnings.
- Windows AMD64 standalone EXE and offline Setup EXE built with Go 1.26.10.
  Executable size: 20,410,880 bytes (+2,249,728 bytes versus 0.3.0-beta.2).
  Setup size: 220,694,968 bytes (+564,439 bytes). Windows runtime,
  signing/SmartScreen, installer execution and update acceptance are pending.
- Linux AMD64 GUI/CLI, DEB, RPM and manual installer archive built. The manual
  archive was installed and removed under a temporary private prefix; launcher,
  icons, CLI version and installed GUI hash were checked. No privileged package
  installation was performed. This local DEB needs glibc >= 2.38 and GTK >= 4.14.
- Test artifacts and SHA-256 checksums are under
  `bin/test-builds/v0.4.0-beta.1/`. These are local test builds, not a published
  GitHub release.

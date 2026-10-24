# Live View test matrix — 0.4.0-beta.2

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
| Separate native viewer, automatic start and login | Linux Xvfb: authenticated H.264 pattern starts after sign-in without a Play click |
| Main-window navigation during playback | Sites/IP scanner navigation leaves the viewer playing |
| Pause/resume and cached login | Linux Xvfb: resumes without another login; Go ownership/cache regression passes |
| Native window close / main-app exit | Linux Xvfb: native close removes viewer and RTSP socket; closing main app closes both windows and exits successfully |
| Viewer ownership and stale-close isolation | Desktop-tagged tests pass; a different or stale window cannot read/start/stop the current viewer |
| Explicit lock and active saved-credential use | Tests pass; lock clears cached credentials, active use refreshes the idle timer |
| Automatic Windows DPAPI access | Windows-specific regression added to CI; cross-compiled here, Windows execution pending |
| Fullscreen and H.265 decoder | Exercised in beta.1 on the same Linux runtime; Windows remains pending |
| Windows standalone and offline setup | Cross-compilation/package build succeeds; runtime tests pending |
| Linux GUI | Builds and starts under Xvfb; physical desktop / Wayland validation pending |

## Required Windows / camera acceptance

1. Run the new standalone executable, or install the new setup package. Confirm
   About shows **0.4.0-beta.2**. Existing saved sites should still be present.
2. Discover a known, activated Hikvision camera. Select it and choose **Live View**.
   The same action is available on saved-site records and device context menus.
3. A separate native window should open and start its sub stream automatically.
   A single matching assigned credential is selected automatically. Windows
   should use it without an unlock dialog. If the camera requests credentials,
   enter its username/password and choose **Sign in and view**. Playback should
   begin immediately; nothing is saved automatically. Linux may first ask for
   the master passphrase when using saved credentials.
4. If ONVIF is advertised, the default source reads its profiles. If ONVIF is
   unavailable, select **Hikvision RTSP endpoint** and apply/retry. Observe real
   movement, not just a still frame. RTSP setup alone is not playback success.
5. Move the viewer beside the main app and navigate Sites/discovery while video
   continues. Pause/resume should not ask for the same session password again.
   Opening Live View on the same device should focus its existing window;
   another device should replace it. Close/reopen, resize and use fullscreen.
   Closing the main app must close the viewer and its camera connection.
6. Try Main and an explicit ONVIF profile. For NVRs choose an explicit profile or
   use the **Hikvision channel** control; automatic ONVIF selection cannot infer
   the operator's intended NVR channel from resolution alone.
7. If the stream is H.265, record the Windows version, WebView2 version, camera
   model/firmware and whether real frames appear. A decoder limitation message
   is expected on unsupported machines. Try an H.264 sub stream without changing
   camera encoding through SADPTools.
8. Exercise incorrect credentials, blocked RTSP port, offline camera, network
   interruption and vault lock. Expect a bounded error and explicit Retry.
   Manually locking saved passwords must stop playback and clear session
   credentials. Windows must honour this lock until you unlock or restart.
   Restarting normally must make saved passwords available automatically again.
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
go test -tags desktop -race . # Native desktop build dependencies required
npm --prefix frontend test
make frontend
make windows-installer VERSION=0.4.0-beta.2
make linux-packages VERSION=0.4.0-beta.2
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -tags desktop,production ./...
```

An optional test-only command serves a synthetic loopback RTSP pattern and
creates an isolated test site directory:

```sh
go run -tags testcamera ./cmd/synthetic-camera
# Add -hevc for HEVC or -auth to require fixture / synthetic-password.
# Keep this command running during the GUI check.
# Use the exact temporary directory printed by the command:
XDG_CONFIG_HOME=/tmp/sadptools-synthetic-preview-PRINTED-VALUE ./bin/SADPTools-linux-amd64
```

Open Sites → SYNTHETIC PLAYBACK TEST → Live View. No camera password is needed
for this isolated source unless -auth was selected. Playback starts automatically. Normal SADPTools builds never launch this server or
populate synthetic devices. Test fixtures and generation commands are in
`internal/streaming/testcamera/testdata/`. FFmpeg is not required to run tests.

Recorded/generated inputs exercise protocol and lifecycle behaviour only.
Physical Hikvision, smart-codec, HEVC hardware decoding, Windows install/update,
firewall and long-duration resource behaviour remain separate acceptance tasks.

## Final local build results

- `make check` passed: all Go tests, `go vet`, race tests, 25 frontend tests,
  and 34 Python build/packaging tests. Desktop-tagged race tests passed on Linux.
  Desktop-tagged vet passed for Linux and
  Windows; Linux reported only upstream GTK deprecation warnings.
- Windows AMD64 standalone EXE and offline Setup EXE built with Go 1.26.10.
  Windows runtime,
  signing/SmartScreen, installer execution and update acceptance are pending.
- Linux AMD64 GUI/CLI, DEB, RPM and manual installer archive built. The manual
  archive install/remove, launcher and icon handling were checked under a
  private prefix in beta.1; packaging regression tests pass again in beta.2. No privileged package
  installation was performed. This local DEB needs glibc >= 2.38 and GTK >= 4.14.
- Test artifacts and SHA-256 checksums are under
  `bin/test-builds/v0.4.0-beta.2/`. These are local test builds, not a published
  GitHub release.

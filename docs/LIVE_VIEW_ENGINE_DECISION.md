# Live View engine decision — 0.4.0-beta.2 test build

## Decision and evidence

Use an in-process Go RTSP client and fragmented MP4 remuxer, with a video-only
MediaSource player carried over the pinned Wails v3 Streams transport. Use the
same private transport for HTTP JPEG images. No additional network listener or
media subprocess runs in the application.

The proposed go2rtc path was investigated at **v1.9.14**. Its RTSP `Conn.Do`
implementation automatically follows 301/302 responses and transfers credentials
into a redirect URL. The connection path does not expose the required endpoint
restriction hook. Embedding that client unchanged would violate this plan's
destination restrictions. Instead, **gortsplib/v5 v5.6.7** provides an explicit
dial callback and response interception. The implementation fixes each dial to
the selected numeric IPv4 endpoint, rejects redirects, validates SDP control
addresses and selects one video track. **mediacommon/v2 v2.9.6** supplies RTP
codec support and MP4 muxing, with **go-mp4 v1.7.3** underneath.

This is an adjustment to the proposed engine choice, supported by these local
results:

- A minimal Wails **v3.0.0-beta.34** prototype decoded 45 moving H.264 frames
  through `HandleStream` into MSE. This was a native Linux Wails process, not a
  standalone browser page.
- The integrated application's saved-device Live View displayed a synthetic
  320 × 180 H.264 RTSP feed at 10 displayed frames/s under Xvfb, with software
  rendering. A moving test-pattern counter was visible. Pause, reopen and close
  were exercised. Playback was visible within the first four-second observation
  window; this is not a precision startup-latency benchmark.
- Synthetic authenticated RTSP integration tests remux both H.264 and H.265 and
  parse the generated MP4 init/fragments. This alone is not decoder proof.
- The integrated H.265 RTSP test also displayed moving frames at 10 fps in
  Linux WebKitGTK 2.52.6. Fullscreen and Stop were exercised; no RTSP socket
  remained after Stop. This does not establish Windows HEVC support.
- See [the test matrix](LIVE_VIEW_TEST_MATRIX.md) for codec/runtime results and
  the checks still required on Windows and physical cameras.

**Windows playback acceptance is pending.** Sparx cannot run the Windows WebView2
binary. This build is an operator test candidate, not a claim that the plan's
Windows production gate or clean-install acceptance has passed.

### Resource observations

The Xvfb/software-rendering HEVC run sampled about 49% of one CPU core in the
application and 244% across WebKit renderer threads during/after fullscreen;
combined process RSS was about 815 MiB (shared pages may be counted twice).
After Stop both main and renderer CPU samples fell below 1%, and the RTSP TCP
connection was absent. RSS remained reserved by GTK/WebKit in the short sample.
These are three-second observations, not a leak soak test or hardware-decoding
benchmark. Software rendering is expensive; Windows GPU and long-duration
resource tests remain required before calling playback performance validated.

## Native alternatives

LibVLC 3 was investigated through the VideoLAN APIs and `adrg/libvlc-go`.
`libvlc_media_player_set_hwnd` requires a native Windows window, and
`set_xwindow` requires an X11 drawable; neither targets a DOM video tile. An
overlay needs focus, DPI, clipping and lifecycle handling, while a separate
native window changes the user experience. No libVLC headers/runtime or Windows
runtime were available here, and no privileged packages were installed. A
LibVLC proof of concept remains unverified, rather than an advertised fallback.

The test build does **not** transcode. Unsupported HEVC gets a specific message
to choose an H.264 sub stream or a JPEG endpoint. It never changes a camera's
encoding. H.264+/H.265+, GPU acceleration, HDR, interlacing, uncommon profiles and
large NVR streams need device-specific validation. Audio is not selected.

## Component boundaries

- `internal/streaming`: URL/TLS validation, RTSP and HTTP clients, video mux,
  ephemeral sessions and bounds; no Wails dependency.
- `internal/onvif/media.go`: read-only Media2/Media service discovery, profile
  metadata, stream and snapshot URI requests. Media2 is preferred; unsupported
  operations may fall back to Media1. Rejected credentials do not trigger an
  authentication fallback. WS-Security clock correction is limited to ±300 s.
- `internal/toolkit/preview.go`: selected live/saved identity, explicit vault
  credential assignment, session credentials, ONVIF profile selection or
  operator-selected Hikvision/custom endpoint. No shared discovery/write lock.
- `desktop_liveview.go`: native viewer window, window-owned bindings, ephemeral
  login cache and the single private media stream.
- `frontend/src/live-view*.js`: viewer, automatic startup, capability checks, bounded MSE append
  queue and JPEG object URLs. No camera credential retrieval or camera requests.

## Security and limits

Only a selected current or saved device can be previewed. Conflicting identities
and inactive cameras are rejected. Cross-site credentials require a matching
stable identity without contradictory MAC/serial fields. Nothing is inferred
from an IP match alone. Vault credentials stay in Go; session credentials cross
the normal binding once and are cleared from password inputs immediately.
Credential strings cannot be securely overwritten in Go/JavaScript memory;
cleanup releases references, not a forensic erasure guarantee.

Camera endpoints must use the selected numeric IPv4 address. DNS aliases,
alternate hosts, userinfo, fragments, executable schemes and redirects are
rejected. A custom URL and port are explicitly selected by the operator.
ONVIF service delegation is additionally restricted to the discovered HTTP
authority and scheme. Cameras advertising another media-service port require a
custom stream URL in this version. ONVIF-returned stream URLs are independently
validated before RTSP/HTTP authentication. No network-wide endpoint guessing.

TLS uses system trust or an explicitly approved exact leaf SHA-256 fingerprint.
Certificate inspection sends no HTTP/RTSP authentication. ONVIF and stream pins
are separate, session-only controls. Normal RTSP/HTTP traffic is unencrypted;
the panel advises using a trusted management network. No global TLS override.

The Wails channel uses origin-bound IPC, a random 256-bit single-use ticket and
no TCP listening port. Unclaimed tickets expire in 20 seconds. Handshake timeout
is five seconds; a missing renderer heartbeat stops the session within about
eight seconds. Close/Stop cancels sockets and waits up to four seconds; a still
stopping engine prevents a replacement stream. Active playback refreshes the
vault idle deadline; status polling alone does not. The explicit Lock action
stops every preview and clears the viewer login cache. Window destruction also
cancels the Wails stream. Local processes with access to the
user's memory or an already compromised renderer remain outside this boundary.

One session/video track is allowed. RTSP uses interleaved TCP only. Connect and
TLS handshakes are bounded; ONVIF resolution has a 24-second overall deadline.
RTSP's first usable keyframe must arrive within 12 seconds and subsequent video
within six seconds. HTTP JPEG has a first-frame and subsequent-frame watchdog,
including slow trickle responses. Retry is explicit, with no automatic reconnect
or password cycling.

Frames are capped at 4 MiB, codec parameter sets at 4 KiB, video dimensions at
4096 × 2160, ONVIF replies at 1 MiB and profiles at 64. Wails has an 8 MiB
per-window outgoing queue; full queues terminate playback. The MSE append queue
is capped at 8 MiB/160 chunks; old buffered video is trimmed and excessive lag
seeks forward. HTTP multipart JPEG is capped at 10 fps and snapshot refresh at
1 fps. These are encoded-buffer bounds, not a total WebView process memory cap.
No images, video, stream URLs or passwords are written to disk by Live View.

## Native viewer and credential behaviour (beta.2)

Live View has its own Wails window and frontend entry point. Opening the same
camera focuses it; opening a different camera replaces the single session.
Main-window selection and navigation do not affect playback. Closing the main
window closes the viewer. Ownership is registered before its renderer starts.
Binding calls and the media handshake check the actual Wails calling window;
a stale closing window cannot stop a newer viewer.

Startup is automatic. Assigned saved credentials are resolved in Go; without
an assignment the first attempt is anonymous. An explicit authentication error
opens a sign-in prompt. Submitting it starts playback, and the entered login
remains only in the viewer's Go state for Pause/Resume and profile changes.
Passwords are not returned to the renderer. Closing/replacing the viewer,
explicit Lock and app shutdown release this cache. No password guessing or
background retry loop was added.

Windows automatically unwraps its existing current-user DPAPI key when a saved
credential is requested. The encrypted database format is unchanged. Idle key
eviction remains, with automatic re-opening on use. Explicit **Lock now** blocks
automatic access for that app session until explicitly unlocked. This is an
optional session control, not a separate Windows authentication boundary.
Linux continues to require its master passphrase and prompts in the viewer
when needed. The app never persists that passphrase.

Linux Xvfb checks exercised authenticated automatic playback, main-page
navigation during playback, pause/resume with the cached login and viewer
close. Native main-window close exited successfully with no remaining viewer
or RTSP connection. Windows DPAPI automatic access has a Windows-only regression test;
cross-compilation here does not establish its runtime result.

## Builds, security scan and redistribution

Go **1.26.10** is pinned in `go.mod` after vulnerability scanning found standard
library fixes missing in 1.26.8. It downloads through Go's normal verified
toolchain mechanism without changing system packages. Existing CI consumes
`go-version-file: go.mod`.

`govulncheck v1.9.0` reported no reachable vulnerabilities in headless and Linux
desktop packages. It flagged the unused `golang.org/x/crypto/openpgp` package at
module level; SADPTools does not import it. The scan is point-in-time evidence,
not a guarantee. npm audit during the pinned frontend install reported zero
vulnerabilities. Dependencies and notices are recorded in `go.mod`, `go.sum`
and `LICENSES/dependencies.txt`.

No new Windows media runtime is bundled. Setup continues to include the pinned
offline WebView2 installer. The standalone executable needs the normal WebView2
runtime already installed. Installed codec availability can differ by Windows
version and machine. The installer remains unsigned while signing approval is
pending.

## Primary references

- [Wails v3 Streams](https://v3.wails.io/guides/streams/), checked against pinned
  `pkg/application/stream.go` and `@wailsio/runtime` types.
- [go2rtc v1.9.14](https://github.com/AlexxIT/go2rtc/tree/v1.9.14), especially
  `pkg/rtsp/client.go`.
- [gortsplib](https://github.com/bluenviron/gortsplib) and
  [mediacommon](https://github.com/bluenviron/mediacommon), pinned source inspected
  in the Go module cache.
- [VideoLAN media-player API](https://videolan.videolan.me/vlc-3.0/group__libvlc__media__player.html)
  and [libvlc-go](https://github.com/adrg/libvlc-go).
- [Go 1.26.10 release history](https://go.dev/doc/devel/release#go1.26.10).

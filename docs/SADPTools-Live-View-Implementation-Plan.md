# SADPTools — Live Camera View: Research and Implementation Plan

**Project:** [TerminalAddict/SADPTools](https://github.com/TerminalAddict/SADPTools)  
**Target:** Existing Go + Wails v3 + Bootstrap 5 + vanilla JavaScript desktop application  
**Primary OS:** Windows 10/11 AMD64, built primarily on Sparx (Linux)  
**Secondary OS:** Linux  
**Feature:** Authenticated, low-latency live video preview for discovered or saved IP cameras  
**Status:** Implementation proposal; playback and packaging choices require proof-of-concept validation.

## 1. Outcome and scope

Add **Live View** to SADPTools so a technician can select a discovered camera or saved site device, authenticate using existing protected credentials (or enter credentials for this session), choose a stream, and **watch actual live video inside the application**.

The first release should:

- Support one video tile at a time (expand later to 2×2 and 3×3).
- Work with Hikvision IP cameras first, but use a vendor-independent architecture for ONVIF-capable and other IP cameras.
- Support RTSP/RTP and common camera codecs, especially H.264/AVC and H.265/HEVC, subject to verified decoder capability.
- Support HTTP multipart MJPEG and snapshot refresh as useful fallbacks where available.
- Respect camera main/sub streams, RTSP port overrides, ONVIF media profiles and unusual camera endpoints.
- Use encrypted site/device credentials through the existing repository's vault mechanisms; never persist plaintext credentials.
- Open and close streams without modifying camera configuration.
- Show actionable errors instead of indefinitely spinning or returning a black preview.
- Keep headless discovery, CLI, and all existing SADPTools functions working.

**Out of scope for first release:** Recording, playback of NVR archives, PTZ controls, microphone/two-way audio, multi-camera wall, cloud relaying, exposing a public streaming service, automatically changing camera codecs, or authorising a camera with credentials the user has not provided.

## 2. Research findings and technology recommendation

### 2.1 Camera protocols vs codecs

Do not treat these as equivalent:

- **Discovery/control:** Hikvision SADP, ONVIF SOAP Media/Media2, vendor ISAPI APIs.
- **Transport:** RTSP/RTP over TCP/UDP; HTTP multipart MJPEG; optionally WebRTC and MSE/fMP4 on the *application-to-frontend* side.
- **Encoding:** H.264, H.265, MJPEG and possibly vendor “smart codec” variations H.264+/H.265+.

RTSP is generally **not directly playable** in an HTML `<video>` element inside a Wails WebView. An intermediary media engine or native renderer is required. A successful RTSP connection does not imply that the installed WebView decoder can decode its video.

### 2.2 Engines considered

| Candidate | Pros | Challenges | Decision |
|---|---|---|---|
| **go2rtc** | Go-based; RTSP, HTTP/MJPEG, ONVIF integrations; WebRTC/MSE playback designed for browser frontends; little transcoding for ordinary streams | Default API/service configuration is unsafe for an untrusted environment; H.265 browser support varies; integration/embedding requires a security design | **Preferred first research/prototype path** |
| **LibVLC 3.x** | Mature RTSP playback; native H.264/H.265 decoding with software/hardware options; supports multiple Windows/Linux outputs | Native video output is not an HTML DOM node; needs native HWND/X11 integration or separate window; CGo/native library packaging and redistribution licenses | **Serious fallback/second backend, prototype native rendering** |
| **FFmpeg-based local conversion** | Broad codec and RTSP support; can produce browser-compatible output | Transcoding consumes CPU; subprocess management, startup delay, licensing/build complexity | **Optional, explicitly controlled compatibility fallback** |
| Direct WebView `<video>` | Simple for supported HTTP formats | No general native RTSP playback; HEVC availability/platform variation | **Not sufficient alone** |

**Recommendation:** Implement a media-engine abstraction. Conduct short proof-of-concept spikes for (A) a secured go2rtc-backed Wails web video tile and (B) a LibVLC native video window/host. Choose the first-production backend using actual Windows test results. Do **not** commit to a heavy native integration before demonstrating that it works with the pinned Wails revision.

#### A critical LibVLC point

`libvlc_media_player_set_hwnd` (Windows) and `libvlc_media_player_set_xwindow` (X11) render into **native windows**, not directly into a Bootstrap div. A Wails WebView does not automatically expose an embeddable video surface. Synchronising a native child-window overlay with page layout, clipping, scrolling, DPI scaling, resize/fullscreen and focus can be complicated. On Linux/Wayland, assuming an X11 drawable is also unsafe. Test either a genuinely integrated native surface or a **separate native playback window**; do not promise in-page LibVLC support until it is demonstrated.

For LibVLC research, inspect `github.com/adrg/libvlc-go/v3` (CGo + libVLC development headers) and current VideoLAN C API; pin verified versions. Investigate licensing for each redistributed plugin, not merely `libvlc.dll`.

#### A critical go2rtc point

The upstream README warns that default ports can expose video and administrative functions to other devices, and that localhost API access may bypass authentication. Merely binding `127.0.0.1` or using an unpredictable port **is not sufficient** to protect site credentials or an API which permits arbitrary sources/commands.

A usable production solution **must** restrict the data plane and management plane. Preferred outcome: embed only necessary media functionality behind a constrained in-process Wails route/stream, or isolate an explicitly configured helper behind a strict, per-session authenticated proxy that exposes only whitelisted live playback capabilities. No externally accessible default go2rtc UI, arbitrary stream URL input, exec/echo/source creation, shell execution, RTSP server, or uncontrolled management API. Evaluate local process threat boundaries honestly. If this cannot be done safely and reliably, do **not** ship the go2rtc backend; use the native playback strategy instead.

Wails v3 offers origin-bound, bidirectional **Streams** without a listening TCP port; these could carry playback control/status and possibly media packets if an appropriate MSE muxing and buffering layer is implemented. Wails Streams by themselves are **not** an RTSP decoder or plug-and-play video player. A separate raw WebSocket media server should not be added casually.

### 2.3 Codecs / compatibility policy

| Camera stream | MVP strategy | Notes |
|---|---|---|
| H.264/AVC over RTSP | Ingest RTSP; remux to supported MSE/fMP4 or negotiate WebRTC | Aim for copy/remux, not transcoding |
| H.265/HEVC over RTSP | Try native supported decode path; native LibVLC or explicit transcoding fallback when unavailable | **Do not** assume all Windows WebView2/Linux WebKitGTK installs can decode HEVC |
| MJPEG over HTTP | Authenticated HTTP multipart stream, or convert frames to browser-safe display | Handle digest/basic auth in Go; bounded memory and framerate |
| H.264+/H.265+ | Detect reported encoding where possible; attempt playback, collect sanitised diagnostics | Smart-codec modes are not necessarily independent standard codecs. Compatibility varies with model, settings, decoder and firmware. Do not claim universal support |
| H.264/H.265 audio | Video-first; audio muted/off in MVP | Avoid inadvertently playing microphones/surveillance audio |

**Important:** Never auto-disable H.264+/H.265+, switch a camera's codec, or save new encoding settings merely to make Live View work. If that is the only resolution, show a recommendation and require a separate, explicit user-authorised configuration operation.

## 3. Stream discovery and authentication

### 3.1 Identity and credential resolution

1. Start from the selected SADPTools device ID / stable MAC or serial and its current IP; do not key exclusively by IP because it changes.
2. If it belongs to a saved site, request access to the existing per-device credential or inherited site credential **using the current vault abstraction**.
3. If no credentials exist or the vault is locked, prompt for username/password with a **Use for this session** option; explicitly ask before storing them in the vault.
4. Never embed `username:password@` in URLs passed to the frontend, Wails event payloads, renderer logs, browser network inspector, exception messages, or subprocess command lines.
5. Separate RTSP, ONVIF and HTTP authentication domains. Attempt permitted configured credentials without indiscriminate password spraying; show an authenticated retry prompt after rejection.
6. Treat ONVIF WS-Security UsernameToken, HTTP Digest/Basic, RTSP Digest/Basic and HTTPS certificate trust as distinct protocols. Support camera time offset in a narrow, controlled way if ONVIF authentication requires it.
7. A successful SADP/ONVIF discovery reply does **not** confer streaming authorisation.

### 3.2 Stream URI resolution (read-only)

Preferred order:

1. **ONVIF Media2 / Media:** authenticate and request available video profiles and `GetStreamUri`; display profile labels/resolution where available; respect a URI/port returned by the camera after validating that the destination is the selected device or an explicitly authorised endpoint.
2. **Hikvision fallback:** propose standard RTSP endpoints for selected channel and stream when ONVIF is unavailable, using an RTSP port learned from device configuration where available or the user-provided/default value.
3. **Saved custom URL:** user may set a per-device source/path/port for known unsupported brands or unusual installations, subject to strict validation and an explicit trust/approval step.
4. **MJPEG/snapshot:** look for vendor/ONVIF-supported read-only HTTP image endpoints; handle authenticated HTTP requests in Go.

Hikvision's documented RTSP convention commonly uses `/Streaming/channels/101` for camera/channel 1 **main stream**, `/Streaming/channels/102` for **sub stream**. For NVR channels, derive channel numbers carefully; do not hardcode all equipment as channel 1 or assume one universally fixed port. A documented default RTSP port is 554, but devices may differ.

RTSP endpoint probing is explicitly opt-in when no advertised URI is available; apply a bounded candidate list, finite timeouts and rate limits. Use `OPTIONS`/`DESCRIBE` and playback setup without writing camera settings. Do not scan arbitrary networks or follow untrusted redirects to internal/external destinations.

### 3.3 Session model

Create a backend-managed ephemeral session. Example (illustrative interfaces, not an instruction to overwrite existing APIs):

```go
type StreamProfile struct {
    ID         string
    Name       string
    Channel    int
    Width      int
    Height     int
    Codec      string // "h264", "h265", "mjpeg", "unknown"
    IsSubstream bool
}

type PreviewSession interface {
    Start(ctx context.Context, deviceID, profileID string) (sessionID string, err error)
    Stop(ctx context.Context, sessionID string) error
    Status(sessionID string) PreviewStatus
}

type PlaybackEngine interface {
    Name() string
    Probe(ctx context.Context, source ProtectedSource) (StreamInfo, error)
    Open(ctx context.Context, source ProtectedSource) (PreviewSession, error)
}
```

Only opaque `sessionID`, safe codec/status/profile metadata, and session-scoped playback handles should reach JS. Backend owns secret retrieval, stream credentials, connections and cleanup. Do not hand raw RTSP URLs with embedded credentials to JS.

Connection state should be explicit:

`idle → authenticating → resolving_profile → connecting → buffering → playing → reconnecting / error → stopped`

Support context cancellation, connection limits, finite connect/read timeouts, a bounded reconnect policy (read-only connection retries are acceptable), cleanup on dialog closing, device selection changes, application exit and lock/vault closure. Do not reconnect forever or leak sockets/processes.

## 4. Desktop UI specification

### 4.1 Entry points

- Add **Live View** button to device details and appropriate device row context menus.
- Add same action to saved-site device entries.
- If device is inactive/unreachable or no usable endpoint is known, disable or explain the action without hiding the reason.

### 4.2 Preview panel

Prefer a resizable **Live View** panel in the main Wails interface; allow a dedicated window/fullscreen mode if it is easier or more robust. The panel should show:

- Camera name, manufacturer/model, IP (never credentials).
- Live video tile at correct aspect ratio, with letterboxing and resize handling.
- Stream selector: **Auto / Sub / Main / available ONVIF profiles**.
- Status: Connecting, Authenticating, Buffering, Playing, Reconnecting, Unavailable.
- Codec, resolution, approximate FPS/bitrate/latency **only where measured or reported**, never fabricated.
- Controls: Play/Pause/Stop, Retry, Mute (future audio), Snapshot (second milestone), Fullscreen, Diagnostics.
- A readable error: bad credentials, stream not found, connection refused, RTSP port blocked, unsupported codec, H.265 browser limitation, unsupported smart-codec, certificate error, timed out, or unknown.
- Optional **Use external VLC** fallback only if implemented without leaking credentials (never launch a credential-bearing URL in process arguments).

Start with **sub stream** for rapid previews on constrained links, with a selectable main-stream option. Do not silently change the camera's permanent stream configuration.

### 4.3 Experience guarantees

- The UI remains responsive while connecting, playing or stopping.
- Closing Live View stops the camera connection within a defined timeout.
- A failed preview never blocks discovery, the site manager, activation dialogs or the IP scanner.
- Do not store preview frames or video recordings by default.
- Honor user preference for selected stream, but do not persist sensitive stream URLs.

## 5. Security requirements — release blockers

- All site/device credentials remain behind the established encrypted vault. Never write them in cleartext to disk, environment dumps, diagnostic bundles, telemetry, browser storage, logs or issue reports.
- Do not expose unrestricted go2rtc HTTP/WebSocket admin/API endpoints to the LAN or to untrusted local processes. Binding localhost alone does not satisfy this requirement.
- If an HTTP loopback bridge is unavoidable, implement a narrow media-only proxy: session-specific unguessable access tokens, strict Origin checks where applicable, no general URL-to-stream endpoint, no unauthenticated privileged API, service lifetime tied to active previews, automatic cleanup and tests proving unauthorised access is rejected. Document residual local-process risk.
- Validate all URLs from ONVIF/vendor responses, custom inputs and redirects. Reject unexpected destinations without explicit user approval to avoid SSRF and credential exfiltration.
- Support certificate validation and an explicit per-device trust decision for a self-signed or mismatched HTTPS endpoint; never globally disable TLS verification.
- Limit concurrent streams, decoded resolution, memory and buffer sizes; reject oversized MJPEG frames and malicious stream metadata.
- Treat media decoder inputs as untrusted. Pin libraries, use maintained builds, review CVEs and ship security updates promptly.
- RTSP transport over a trusted local management network may be unencrypted. Warn when applicable; do not imply RTSP authentication encrypts media.
- Do not automatically modify camera encoding, user settings or stream profiles. Follow `AGENTS.md` for all device writes.
- Streaming helper processes (if needed) must not allow arbitrary shell commands; avoid credentials on argv and avoid writable config files containing secrets. If a required helper cannot accept credentials securely, fail closed rather than compromise secrets.

## 6. Packaging, builds and licences

- Preserve current Wails v3 desktop build tags and headless Go CLI/testing architecture.
- Windows cross-compilation happens on Sparx. Native Windows playback, firewall, GPU acceleration, DPI and installer behaviour require **separate validation on actual Windows**, not just `make windows`.
- Linux playback must be tested on a graphical Linux machine separately; consider Wayland versus X11 explicitly if using LibVLC native surfaces.
- Evaluate WebView2 H.265 support across Windows 10/11 and the WebView2 runtime actually packaged by SADPTools. Test without assuming OS codec packs are present.
- If using go2rtc, pin exact source/version and audit bundled/static assets and licensing. Avoid silently launching a standalone network service.
- If using LibVLC, pin libVLC major/minor and a Go wrapper or custom bridge. Distribute exact native DLLs/plugins required (not necessarily the whole VLC program), handle lookup/loading and `VLC_PLUGIN_PATH` safely, and audit `LGPL`/`GPL` obligations for **every** bundled library/plugin plus relevant codec patents where applicable. Add copyright/source/notice instructions to `THIRD_PARTY_NOTICES.md` and `LICENSES/`.
- If using FFmpeg fallback, choose and document the actual build/licence combination (LGPL vs GPL-enabled), codec availability, executable packaging and update policy.
- No surprise prerequisites: an installed Windows SADPTools build should either include the media runtime it uses or clearly and safely detect/guide installation. Measure installer-size increase and startup overhead.
- Cross-built binaries compiling is **not** evidence that decoding, native rendering or authentication works.

## 7. Implementation phases and deliverables

### Phase 0 — Inspect current code and prove playback (no broad refactor)

1. Read `AGENTS.md`, current README, existing site vault, ONVIF device service, Wails bindings, UI event flow, tests and build scripts.
2. Identify the exact pinned Wails revision, OS target and how the desktop app currently accesses the protected credentials.
3. Create a minimal H.264 RTSP test source (synthetic/authorised fixture) and confirm a visible, moving video inside the **actual Windows Wails WebView**, not merely in Chrome/VLC externally.
4. Prototype the go2rtc media path with strict access controls. Record whether MSE/WebRTC works using the real Wails origin and WebView2.
5. Independently investigate a LibVLC RTSP proof-of-concept: native Windows playback and actual feasibility of a Wails-integrated tile vs separate native window.
6. Test an H.265 source to learn whether the chosen browser runtime really decodes it. Measure startup latency, resource use, hardware decoding and failure reporting.
7. Write `docs/LIVE_VIEW_ENGINE_DECISION.md` with measured results, dependencies, packaging implications, security review and chosen primary/fallback strategy.

**Gate:** Do not proceed to production integration until at least one path renders live video in the real application without unsafe unauthenticated media-control endpoints.

### Phase 1 — Single-camera MVP

1. Integrate the chosen safe playback engine behind `internal/streaming` (or similarly named independent Go package).
2. Reuse existing saved-site credential/vault abstractions. Add session credentials only when needed.
3. Query ONVIF profiles/stream URI where supported, with Hikvision RTSP fallback when necessary.
4. Add Live View action and one resizable preview tile with status, stop, retry and stream selector.
5. Implement bounded read-only connection/retry behaviour, cancellation and resource cleanup.
6. Add sanitised diagnostics and unit/integration tests.
7. Produce a Windows test build; validate on a physical Hikvision H.264 camera.

**Acceptance:** Windows tester can pick a known camera, unlock vault or authenticate, select sub/main stream, view live moving video in SADPTools, close it, reopen it, and continue using discovery unchanged. No credential leaks or lingering stream/server processes.

### Phase 2 — Wider protocol/codec coverage

1. Native H.265 path or conditional fallback when WebView decoding fails, without altering camera configuration.
2. HTTP MJPEG support and ONVIF snapshot URI support.
3. NVR channel selection; custom RTSP port/path and validation.
4. Test Hikvision smart-codec modes and record model-specific results accurately.
5. Better stream health: reconnect reason, connection state, measured latency/FPS where possible.
6. Linux GUI support; Windows hardware decoder and clean-install tests.

**Acceptance:** Failure cases are explained; one unsupported stream cannot crash the app or affect other features. Protocol/codec support is documented as tested, conditional or unsupported, not overclaimed.

### Phase 3 — Technician-friendly enhancements (separate scope)

- 2×2 grid of substreams with a concurrency/CPU budget.
- Per-camera snapshots saved to the selected site's export folder, with explicit user action and no automatic sensitive-content storage.
- Optional audio, mute-by-default, and precise permission handling.
- Expand/fullscreen and saved layout preferences.
- Camera diagnostics: codec, profile, resolution, negotiated transport, optional packet-loss/RTT where measurable.
- Optional authorised PTZ; keep it separate from the playback engine.

## 8. Test matrix

Use synthetic/local fixtures first and a test lab with devices the user owns or is authorised to manage. Test representative:

| Case | Expected |
|---|---|
| Hikvision RTSP H.264 main/sub | Plays both; starts sub by default |
| Hikvision RTSP H.265 | Plays with supported backend, or clear fallback/error |
| H.264+/H.265+ enabled | Attempts supported mode and reports tested limitations honestly |
| ONVIF camera from another vendor | Gets profiles/URI; plays if supported |
| HTTP multipart MJPEG camera | Plays or shows defined compatibility status |
| Password missing/wrong | Prompts or reports authentication failure; no leaking credentials |
| Saved device inherits site credentials | Uses vault correctly without prompting when unlocked |
| Self-signed HTTPS ONVIF | Explicit per-device trust decision; no global insecure bypass |
| Custom path/port | Validated; works when authorised and reachable |
| NVR different channel | Selects correct profile/channel; never assumes 101 |
| RTSP TCP vs UDP/firewall | Tries safe transport options; clear timeout/error |
| Select a new device while playing | Previous session stopped/cleaned up |
| Window closed mid-connection | All connections/goroutines/helpers end cleanly |
| Repeated 100× open/close | No material socket/process/memory leak |
| Malformed RTSP/ONVIF/MJPEG responses | Errors, bounds checks, never app crash |
| Untrusted local HTTP client | Cannot access/control other streams or credentials |
| Windows 10/11 clean install | No hidden dependencies, expected codec fallback |
| Linux GUI session | Works or documents platform-specific backend constraints |

Automated: Go unit/race tests; integration tests with a local synthetic RTSP stream; negative security tests; frontend tests. Manual: real camera firmware combinations, actual Windows WebView, install/uninstall, GPU/CPU profiling and native runtime packaging.

## 9. Suggested backend organisation (adapt to current project)

```text
internal/streaming/
    engine.go             # playback-engine abstraction
    manager.go            # session lifetime, limits, cancellation
    profiles.go           # codec/profile representation
    resolver.go           # ONVIF Media/Media2 + vendor stream resolution
    credentials.go        # vault integration; secrets never returned to JS
    diagnostics.go        # sanitised connection/status events
    backend_go2rtc.go     # only if validated and secured
    backend_libvlc.go     # optional platform-specific native backend
    backend_mjpeg.go      # HTTP MJPEG implementation/fallback

frontend/                # modify the existing UI, not a second app
    ...                  # Live View panel/modal + player adapter

docs/
    LIVE_VIEW_ENGINE_DECISION.md
    LIVE_VIEW_TEST_MATRIX.md
```

Do not copy this structure mechanically if existing packages offer better seams. Keep CLI and headless tests free of GUI-native dependencies using existing build tags.

## 10. Instructions for Codex CLI

Paste the following after copying this file to `docs/LIVE_VIEW_PLAN.md` in the existing repository:

> Read `AGENTS.md`, `README.md`, the current site/credential vault implementation, ONVIF integration and `docs/LIVE_VIEW_PLAN.md`. Implement the SADPTools Live View feature in phases, **extending** the current Wails v3 application rather than creating another application. Start with Phase 0 playback/security spikes and a written engine decision. After a safe path is proven, implement Phase 1 single-camera MVP, real backend sessions, actual Wails UI, automatic profile resolution, secure credentials, cancellation, logs, tests and a Windows build. Do not pretend a mock image or snapshots are live video. Do not add an unrestricted localhost go2rtc service. Do not auto-change camera settings. Respect AGENTS.md safety rules and the existing build/test conventions. Commit only if consistent with repository policy; do not push. Report exactly what compiled, what played on real hardware, what was synthetic, what remains untested and the commands needed to verify it on Windows.

### First actions Codex should perform

```bash
cd ~/git-repos/SADPTools
cat AGENTS.md
git status --short
git log -5 --oneline
# Inspect actual Wails package versions and repository structure.
# Review existing credential/site storage and ONVIF implementation before modifying them.
```

## 11. Primary references (verified during research, 2026-10-24)

- SADPTools source and current features: https://github.com/TerminalAddict/SADPTools
- SADPTools repository instructions: https://raw.githubusercontent.com/TerminalAddict/SADPTools/main/AGENTS.md
- go2rtc upstream and **security warnings**: https://github.com/AlexxIT/go2rtc
- go2rtc codec support/limitations: https://github.com/AlexxIT/go2rtc/blob/master/README.md
- go2rtc API security and integration: https://go2rtc.org/internal/api/
- VideoLAN libVLC 3.0 media-player API (HWND and X11 rendering): https://videolan.videolan.me/vlc-3.0/group__libvlc__media__player.html
- libVLC licence details (LGPL + some GPL plugins): https://videolan.videolan.me/vlc/master/index.html
- Go libVLC bindings: https://github.com/adrg/libvlc-go
- Wails v3 Streams and transport constraints: https://v3.wails.io/guides/streams/
- Wails v3 window management: https://v3.wails.io/features/windows/basics/
- ONVIF Media profile and stream URI specification: https://www.onvif.org/specs/srv/media/ONVIF-Media-Service-Spec-v221.pdf
- Hikvision documented RTSP URI conventions: https://supportusa.hikvision.com/a/solutions/articles/17000129064

---

**Definition of success:** Select camera → authenticate safely → choose stream → see real moving video *in SADPTools* → close video with no lingering resources. Maintain all previous device/site functions. Avoid overpromising codec compatibility and never reduce the security of the existing inventory/credential vault.

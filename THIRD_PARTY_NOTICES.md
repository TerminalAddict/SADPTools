# Third-party notices

SADP inquiry and device-management command wire formats and response field mappings in `internal/protocol/sadp/` are adapted from [cameronnewman/hikvision-tooling](https://github.com/cameronnewman/hikvision-tooling), commit `afa00e3db11bb47914e1f0d724c6975e0a41e218` (MIT, Copyright (c) 2025 Cameron Newman). Its full licence is retained in `LICENSES/hikvision-tooling-MIT.txt`. The scanner lifecycle and transport are implemented here; no upstream executable or `internal` package is imported.

The application uses Wails v3 (MIT), Bootstrap 5 (MIT), Go's x/net, x/sys and x/crypto packages (BSD-3-Clause), modernc.org/sqlite and its pure-Go runtime dependencies (BSD-3-Clause and their included third-party notices), gosnmp (BSD-2-Clause, including its Go ASN.1 attribution), icholy/digest (MIT), and their transitive dependencies. Frontend assets retain upstream licence comments; dependency versions are recorded in `go.sum` and `frontend/package-lock.json`. Bundled dependency licence texts are collected in `LICENSES/dependencies.txt` for distribution. Build tooling includes Vite (MIT); its licence is also included there.

Wails generated the initial desktop scaffold and Windows manifest/resource configuration. The original **Signal Lens** mark, wordmarks, installer artwork and interface are SADPTools assets, covered by the project's MIT licence. Their editable source and generation instructions are described in [the branding guide](docs/branding.md). Exported branding artwork uses system DejaVu Sans; that font is not redistributed. PDF reports embed the separately licensed Go fonts described below.

## Live View

Live View uses **bluenviron/gortsplib v5.6.7** (MIT) for RTSP/RTP, **bluenviron/mediacommon v2.9.6** (MIT) for H.264/H.265 parsing and fragmented MP4, **abema/go-mp4 v1.7.3** (MIT), Pion RTP/RTCP and their pinned transitive dependencies. Their full licence texts are included in `LICENSES/dependencies.txt`. The session manager, constrained camera client, ONVIF media resolver and Wails player integration are implemented in this repository.

go2rtc and LibVLC were researched but are not redistributed or launched. FFmpeg generated the clearly labelled synthetic test patterns; it is neither bundled nor required to run SADPTools. Video decoding uses the existing operating system WebView component. Codec availability, especially HEVC, depends on that component; no additional native codec package is redistributed by this version.

## Author profile

The About screen's biography is adapted from [Paul Willard's website](https://www.paulwillard.nz/), accessed 22 October 2026. The unmodified portrait in `frontend/src/assets/paul-willard.webp` comes from [the author profile image](https://www.paulwillard.nz/assets/images/optimized/paul-lw-220.webp) on that site and is bundled locally for offline display.

## Windows installer

The Windows Setup executable redistributes the **unmodified Microsoft Edge WebView2 Runtime Evergreen Standalone Installer (x64)**. This Microsoft component is separately licensed, not covered by SADPTools' MIT licence. The full Microsoft terms, obtained on 21 October 2026 from the [official licence endpoint](https://developer.microsoft.com/microsoft-edge/api/eula/webview2?locale=en-us), are retained in `LICENSES/Microsoft-WebView2.txt` and displayed during setup. The payload comes directly from Microsoft's download service; its version, URL, size and SHA-256 are pinned in `build/windows/installer-inputs.json`.

WebView2 includes Microsoft Defender SmartScreen, which collects and sends information to Microsoft as disclosed in [Microsoft's Privacy Statement](https://aka.ms/privacy) and the [Microsoft Edge Privacy Whitepaper](https://learn.microsoft.com/en-us/microsoft-edge/privacy-whitepaper#smartscreen). Microsoft supplies runtime updates separately. SADPTools itself does not implement telemetry. Uninstalling SADPTools leaves this shared Microsoft component installed.

The installer is compiled with **NSIS 3.10**, whose installer stub, Modern UI and standard plugins carry their respective open-source notices. The full copyright/licence file from the pinned Ubuntu NSIS packages is retained in `LICENSES/NSIS.txt`. No Wails NSIS helper code is copied; the installer script is maintained in this repository.

## Commissioning reports and site images

Offline PDF generation uses **codeberg.org/go-pdf/fpdf v0.12.0** (MIT), the maintained successor of go-pdf/fpdf. Reports embed **Go Regular and Go Bold**, copyright (c) 2016 Bigelow & Holmes Inc., under their BSD-style licence in `LICENSES/Go-fonts.txt`. Fonts are supplied by **golang.org/x/image v0.47.0** (BSD-3-Clause), also used to decode WebP site images. Full module licences are included in `LICENSES/dependencies.txt`. No online PDF service, external renderer, PDF input engine or additional media decoder is bundled.

SSH discovery uses the existing pinned **golang.org/x/crypto/ssh** implementation and its agent/known_hosts packages (BSD-3-Clause). ONVIF PTZ, imaging and events support was implemented against published ONVIF WSDLs; no third-party camera-control source was copied. ISAPI channel parsing follows the vendor-authored protocol documentation cited in `docs/ADVANCED_FEATURES_GAP_ANALYSIS.md`.

## Field diagnostics and equipment labels

Offline QR label generation uses **skip2/go-qrcode**, pinned to commit `da1b6568686e` (MIT, Copyright (c) 2014 Tom Harwood). Its full licence is included in `LICENSES/go-qrcode-MIT.txt`. QR SVG layout, asset identifiers and diagnostics are implemented here.

Recording metadata and imaging operations follow the published ONVIF recording/search/imaging WSDLs and the vendor-authored ISAPI Content Management specification cited in `docs/v0.6-diagnostics.md`. No vendor firmware, camera recordings or production packet captures are redistributed. Optional packet tracing invokes a separately installed `dumpcap`; Wireshark/Npcap binaries or drivers are not bundled or installed by SADPTools.

macOS cross-build tooling adapts the Docker/Zig approach from **Wails v3.0.0-beta.34** (`internal/commands/build_assets/docker/Dockerfile.cross` and Darwin tasks), under Wails' MIT licence in `LICENSES/wails-MIT.txt`. The private build container uses **Zig 0.14.0** (MIT; compiler and bundled third-party notices accompany its distribution). Neither Zig nor an Apple SDK is shipped in the app. The macOS SDK must be supplied separately under Apple's terms; it and any container containing it must not be committed or published. The macOS icon is an export of SADPTools' existing Signal Lens artwork.

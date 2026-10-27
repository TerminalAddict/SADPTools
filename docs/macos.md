# macOS builds, installation and validation

SADPTools uses the same Go backend and Wails v3 frontend on Windows, Linux and
macOS. macOS builds target **12.0 or later**, Apple Silicon (`arm64`) and Intel
(`amd64`). A universal app contains both native slices. This is a deployment
floor, not a claim that every codec or device feature has been tested on 12.0.
WKWebView is part of macOS; WebView2 and a separate video player are not required.

## Implementation and milestone plan

1. Keep Windows/Linux tasks independent; add native and Docker Darwin targets.
2. Export the existing icon as ICNS and assemble versioned `.app` bundles.
3. Review sockets, routes/ARP, paths, credentials, playback and updates.
4. Validate universal ZIP/DMG artifacts on native CI and import them into the
   existing immutable release process with commit/version/checksum checks.
5. Run headless/synthetic checks on Sparx, attempt the ARM64 GUI cross-build,
   then perform the hardware and Finder acceptance checks below on actual Macs.

No new backend, camera protocol implementation or web server is introduced.

## Build on a Mac

Install Xcode Command Line Tools (`xcode-select --install`), the Go version in
`go.mod`, Node 24/npm, Python 3.11+ and make. Then:

```sh
make tools
make macos MACOS_ARCH=arm64
make macos MACOS_ARCH=amd64
make macos                       # universal by default
make macos-packages              # universal .app, ZIP and DMG
open bin/macos/universal/SADPTools.app
```

`VERSION=0.6.0-rc.1` overrides the version. Do not replace an existing published
release with modified source; use the next release version when publishing.
Wails equivalents: `wails3 package GOOS=darwin GOARCH=arm64`,
`wails3 task darwin:package:universal`, `wails3 task darwin:package:dmg`.
The frontend is built once before Go compilation. Native builds use Apple clang
with CGO and a 12.0 deployment target. Universal slices are joined with `lipo`.

Generated output (ignored by Git):

- `bin/macos/SADPTools-darwin-<arch>` and `build-<arch>.json`: binary and provenance.
- `bin/macos/<arch>/SADPTools.app`: native application bundle.
- `bin/macos/<arch>/artifacts/SADPTools-<VERSION>-macos-<arch>.zip`.
- `bin/macos/<arch>/artifacts/SADPTools-<VERSION>-macos-<arch>.dmg` (Mac only).
- `macos-build-manifest.json` and `SHA256SUMS` alongside those archives.

The plist contains a numeric `CFBundleShortVersionString`/`CFBundleVersion` and
full semantic `SADPToolsVersion`; About uses the embedded full Go version.
Beta/RC ordering must use semantic versions, not the numeric Finder version.
ZIPs preserve executable permissions and bundle signatures. Both formats include
licences. DMGs contain the app and an Applications shortcut; packaging mounts the
finished DMG read-only, verifies its signature and compares its app to the ZIP.

## Cross-compile on Sparx (no display required)

The implementation follows the pinned Wails **v3.0.0-beta.34** Docker/Zig approach
and the current [Wails cross-platform guide](https://v3.wails.io/guides/build/cross-platform/).
It uses Go **1.26.10**, Zig **0.14.0** (pinned SHA-256) and a private **macOS 14.5 SDK**.
The Docker image is isolated from all Windows/Linux tasks. Frontend compilation
runs on the host; Docker compiles the same `desktop,production` Go entry point.
Only source, a generated output directory and private build caches are mounted;
no Docker socket, SSH keys or signing credentials are mounted into the builder.

1. `docker info` must succeed for the current user. On Sparx Docker is installed
   but the current session cannot access `/var/run/docker.sock`. An administrator
   can grant trusted-user access with `sudo usermod -aG docker paul`, followed by
   a new login. Membership grants control of Docker/the host; alternatively use
   an approved rootless Docker context. No privilege changes are made by builds.
2. Obtain `MacOSX14.5.sdk` from your authorized Xcode SDK installation and review
   Apple's SDK terms. Keep the SDK and Docker image private. On the Mac that
   has that SDK: `COPYFILE_DISABLE=1 tar -cJf MacOSX14.5.sdk.tar.xz MacOSX14.5.sdk`.
   Record `shasum -a 256 MacOSX14.5.sdk.tar.xz` there, then transfer the archive to
   Sparx's ignored `.cache/macos-sdk/`. SDK archives are never downloaded from an
   unverified mirror or added to the repository by the build script.
3. Build the private toolchain and application:

```sh
export MACOS_SDK_ARCHIVE="$PWD/.cache/macos-sdk/MacOSX14.5.sdk.tar.xz"
export MACOS_SDK_SHA256='<the independently recorded 64-character SHA-256>'
make macos-docker
make macos MACOS_ARCH=arm64
make macos MACOS_ARCH=amd64
make macos MACOS_ARCH=universal
```

The Dockerfile uses a small context containing only its scripts and the SDK.
Builds run as the calling UID/GID, so they do not leave root-owned output. The
compiler wrapper preserves paths with spaces and targets macOS 12.0. Wails'
`tool lipo` combines cross-built slices without Apple tools on Linux.

Linux can create genuine Mach-O executables, `.app` directories and ZIPs. It
cannot run Apple's `codesign`, `notarytool` or `hdiutil`: **DMG packaging and bundle
signature validation run on macOS**. A missing Docker daemon/image/SDK fails the
macOS target with instructions; it never changes Windows/Linux toolchains.
`make macos-packages` on Linux deliberately reports that DMG creation needs a Mac.

## Install and update

Open the DMG and drag **SADPTools.app** into Applications, or extract the ZIP and
move the app there. Open the app from Finder. A user-writable `~/Applications`
folder is also suitable. Quit the old app before replacing it during an upgrade.
Do not run it from inside the mounted DMG when testing installation or updates.

Current native packages are **ad-hoc signed, not Developer ID signed or notarized**.
Cross-built bundles are unsigned at the bundle level. Neither is a claim of
Gatekeeper acceptance. For a trusted local cross-build, a Mac tester can run:

```sh
codesign --force --sign - /path/to/SADPTools.app
codesign --verify --deep --strict /path/to/SADPTools.app
```

Downloaded test builds may require macOS **Privacy & Security → Open Anyway**
after an attempted launch. Do not disable Gatekeeper globally. For normal public
distribution, arrange Developer ID signing and notarization on a trusted Mac,
then repackage and reverify the signed/stapled app before computing checksums.
See [Wails macOS packaging](https://v3.wails.io/guides/build/macos/) and
[Apple's notarization guide](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution).
CI only uses ad-hoc signing and never requests Apple account secrets.

The app checks for `SADPTools-<VERSION>-macos-universal.dmg`, validates release
metadata/checksums and offers **View release**. It cannot download an EXE or call
Windows setup on macOS. Install manually for now; the planned safe automatic
handoff is in [updates.md](updates.md). Windows' existing download/verify/WAITPID
workflow remains unchanged. Removing the app leaves user data intact.

## Platform review

| Area | macOS behavior |
| --- | --- |
| Discovery | Existing Go interface enumeration, IPv4 multicast groups and `x/net/ipv4` interface ancillary data. No Linux naming assumptions or root requirement. UDP 37020 remains exclusive, with explicit conflict/cancellation/timeout handling. |
| Routed cameras | Existing UDP route-source selection and bounded HTTP/HTTPS probes. SADP multicast stays on its local link; use the existing routed IP/range discovery for other networks. |
| Diagnostics | Kernel routing table supplies default gateways and ARP evidence; `/sbin/route -n get -inet -ifscope` gives read-only destination routes. Missing/ambiguous results stay unknown. Cached ARP does not prove reachability. |
| ICMP | Existing unprivileged Darwin datagram ICMP via Go `x/net/icmp`, with bounded fallback/error reporting. TCP remains available when ICMP is blocked. |
| Settings/sites | Go `os.UserConfigDir`: `~/Library/Application Support/SADPTools/` (settings and `sites.sqlite`). No writes inside the app bundle. |
| Cache | Go `os.UserCacheDir`: `~/Library/Caches/SADPTools/`; disposable update cache stays separate from sites. |
| Credentials | Existing passphrase-encrypted vault on macOS, session-only login when desired; no secrets in preferences, inventory, logs or app bundle. Windows DPAPI remains Windows-only. macOS Keychain auto-unlock is not implemented. Move credentials via encrypted site backup, not a copied Windows DPAPI database. |
| Live View | Existing native RTSP/ONVIF Go engine; WKWebView consumes bounded fMP4/JPEG data via the Wails stream. No FFmpeg subprocess or credential URLs in JavaScript. Dedicated viewer enables WKWebView autoplay; H.264 is the first test target, HEVC depends on the Mac/OS codec. Unsupported codecs fail visibly and suggest H.264/JPEG. |
| Packet capture | Optional existing Wireshark `dumpcap`, including Finder-safe fixed application/Homebrew locations. No driver/helper installation, `sudo`, permission changes or automatic capture. |
| GUI | Wails' native dialogs, browser opening, application/Edit menu and separate Live View window; verify Finder launching, Command-key shortcuts, Retina rendering and lifecycle on hardware. |

On macOS versions with Local Network privacy controls, allow SADPTools under
**System Settings → Privacy & Security → Local Network** and allow incoming
connections if the firewall prompts. The bundle includes the purpose string for
both direct unicast and multicast. Do not disable the firewall or privacy system.
If discovery is empty, check permissions, selected adapters, VPNs, UDP 37020 and
same-link access before blaming camera state. A CLI run from Terminal can have
different privacy behavior from Finder: it is not proof of GUI permission.
[Apple local-network privacy guidance](https://developer.apple.com/documentation/technotes/tn3179-understanding-local-network-privacy)
also recommends an Apple-issued signing identity for reliable app identity.

## CI and release integration

`check.yml` adds native headless tests on **macos-15 (ARM64)** and
**macos-15-intel**, plus a separate universal app/ZIP/DMG job on macos-15. It checks
both Mach-O slices, plist, signature and desktop service tests. It uploads
`SADPTools-macOS-universal`; it does not publish a release. Windows/Linux jobs have
no dependency on the macOS job or SDK. See [validation results](validation.md) for which checks have actually run.

Once a clean commit has passed that trusted CI job, download its artifacts:

```sh
gh run download <run-id> -n SADPTools-macOS-universal -D .cache/macos-release
make release-build VERSION=<new-version> MACOS_RELEASE_DIR=.cache/macos-release
# Review the complete bundle before any separately authorised publish:
# make release-publish VERSION=<new-version>
```

The import requires the **same clean commit and version**, universal architecture,
native bundle-verification manifest, ZIP+DMG, and matching SHA-256s. It inspects
the ZIP's Mach-O slices, plist, icon, licences, executable mode and paths, and the
DMG container header. Linux cannot independently run Apple's signature or DMG
mount verification; import only trusted CI artifacts, never an arbitrary upload.
Old Windows/Linux release bundles remain verifiable. Omitting `MACOS_RELEASE_DIR`
keeps the old release process; supplying an invalid/incomplete directory fails
rather than silently publishing without the requested Mac files.

## Actual-Mac acceptance checklist

Compilation and synthetic tests do not establish physical-device or macOS UI
compatibility. Test both Apple Silicon and Intel, including the oldest supported
OS where practical:

- Install ZIP/DMG; validate icon/Dock, About/version, Finder launch and Gatekeeper.
- Allow/deny/re-enable Local Network access and firewall prompts; compare Finder
  versus CLI behavior and ensure actionable diagnostics after denial.
- Discover on Ethernet/Wi-Fi/USB adapters, multiple adapters and VPNs; test an
  off-subnet same-link camera, cancellation, repeated scans and port conflicts.
- Compare route/ARP/gateway diagnostics with OS output; exercise routed camera
  discovery and HTTP/HTTPS/security-question reachability without camera writes.
- Test Live View H.264, HEVC where supported, JPEG, autoplay, separate-window
  navigation, resize/fullscreen, stream change, reconnect and closing all windows.
- Verify saving/reopening sites and passphrase credentials, session-only login,
  encrypted backup restore, Unicode/spaced paths and denied file/capture access.
- Confirm update checks select Mac artifacts; replacing the closed app preserves
  settings/sites. Verify Windows updates separately on Windows.
- Camera activation/network/password/firmware/PTZ writes require an explicitly
  authorized test device and action; no such operations run as build validation.

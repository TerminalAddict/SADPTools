# Imported camera Live View

NX Camera Info exports can supply the exact stream addresses a camera uses.
SADPTools retains those addresses as encrypted camera attachments and defaults to
**Imported camera stream** when opening Live View from the imported site. It uses
the camera's assigned credential and prefers the exported sub stream when there
is one. No vendor path guessing or automatic authentication retries occur.

For sites imported before this support was added, import the original NX JSON
again and open Live View from the new site. Existing sites are retained. Cameras
without imported endpoints use advertised ONVIF services, identified
Hikvision/SADP endpoints, or an explicitly entered custom URL.

See [Site transfer](site-transfer.md) for credential handling and portable site
exports. Endpoints stay out of inventory and ordinary reports, and an inventory
IP change invalidates the saved endpoints until they are refreshed.

## Physical camera checks — 28 October 2026

The user's three identified test cameras were checked from Sparx using the
supplied export. Only read-only ONVIF discovery/profile requests, RTSP session
requests and video playback were performed. No camera configuration was changed,
and no screenshots, recordings, credentials or raw replies were saved as test
artifacts. Addresses and credentials are deliberately omitted from this report.

| User-identified camera | Imported stream played | Browser result |
| --- | --- | --- |
| Grandstream GDS3727 | H.264, 1920 × 1080; one distinct exported stream | 25 displayed frames |
| Milesight MS-C2964-RFLPE | H.264, 640 × 360 sub stream | 25 displayed frames |
| Tapo C100 | H.264, 640 × 360 sub stream | 25 displayed frames |

The full check used the real NX importer, a temporary encrypted store in RAM,
the toolkit's saved-endpoint/credential selection, the native RTSP/fMP4 engine,
and the existing `CameraPlayer` in Linux Chromium. Its temporary store was removed
after the check. Separate native-engine checks also received the exported main
streams from all three cameras.

Milesight and Tapo returned H.264 main/sub ONVIF profiles. Milesight initially
failed because its service advertised HTTP port 80 explicitly while the selected
device URL omitted the default port. That comparison now uses effective HTTP(S)
ports; different hosts, schemes and non-default ports remain rejected. The
Grandstream's default ONVIF endpoint returned unsupported/rejected in this check;
its exported RTSP stream worked.

This verifies these physical cameras and Linux browser decoding. It does not
establish compatibility with other firmware, long-running sessions, Windows
WebView2 playback, or macOS WebKit. The rebuilt Windows app still needs a normal
desktop check.

## Automated regression checks

- Imported endpoints remain encrypted locally and through `.sadpsite` transfer;
  public metadata and reports contain no stream URLs or tokens.
- Reject foreign destinations, malformed protected assets, stale camera IPs and
  locked-vault access; preserve valid main/sub selection and deduplicate URLs.
- Import-to-playback test using the authenticated, loopback-only synthetic RTSP
  camera and the native streaming engine.
- ONVIF explicit/default HTTP(S) port equivalence and authority restrictions.
- Source selection, automatic start, protected endpoint unlocking and guidance
  for unknown vendors in light/dark browser UI checks.

Run `go test ./...`, `go vet ./...`, `go test -race ./...`,
`npm --prefix frontend test`, and
`node frontend/tests/live-view-source-browser.mjs` (the latter uses the website's
pinned Playwright dependency). These automated tests never contact real cameras.

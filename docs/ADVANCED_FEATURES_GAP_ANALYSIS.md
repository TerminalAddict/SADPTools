# v0.5 implementation map

Scope: all F01–F10 in `SADPTools-v0.5-Feature-Roadmap.md`, as explicitly requested
by Paul. Extend the existing app; continue through the complete test candidate.
Baseline: v0.4.0-beta.2, clean source except the user-supplied roadmap. No device
writes, unsolicited scans, privileged installs, GitHub pushes or releases are
part of development validation.

| Feature | Existing integration | Implementation gap and approach | Verification / compatibility boundary |
|---|---|---|---|
| F01 NVR channels | `sites.Match`, `toolkit`, ONVIF media, viewer | Read-only ISAPI channel/status and streaming inventory; stable NVR/channel keys, explicit links, timestamped cache; open channel through existing viewer | Generated 0/4/16/32-channel fixtures, identity conflicts, missing status and auth failures; model/firmware hardware checks remain |
| F02 stream analyser | `streaming.Manager`, gortsplib, native viewer | Instrument actual RTP/access-unit and mux counters, state timeline, startup latency, 15 s captures and main/sub comparisons; safe exports | Synthetic RTSP and lifecycle tests; decoded frame counters only from player telemetry; TCP supported, UDP shown unsupported until safely integrated |
| F03 focus/alignment | existing video/JPEG player | Canvas overlays, digital pan/zoom, relative Laplacian trend, explicit snapshots, comparison and site evidence; capability-gated ONVIF imaging | Generated images, browser/native GUI checks; no optical controls on fixed cameras |
| F04 PTZ/presets | ONVIF authenticated SOAP, viewer ownership | Capability/profile-bound PTZ session, explicit movement/preset confirmation, watchdog and release/blur/close Stop; preset tour test | Fake SOAP/provider lifecycle and no-retry tests; physical movement only by operator |
| F05 walk-test | ONVIF transport, site runs | PullPoint discovery/subscription/poll/unsubscribe, bounded event timeline, clock offsets, explicit grading/evidence and reports | Generated motion/line/unknown/malformed/disconnect responses; no analytics configuration |
| F06 config backup/diff/clone | vault crypto, site backup/history | Encrypted readable settings export (full vendor blob export remains unsupported), sanitised structured diff, compatibility/identity-bound preview and a gated restore executor | Encrypted round-trip/damage/wrong passphrase, incompatibility and uncertain outcome fixtures. Production restore remains disabled until approved physical validation, as the roadmap explicitly requires |
| F07 retention | sites, reports | Deterministic units/ranges/duty-cycle/calendar/audio/reserve calculator; saved comparison scenarios | Pure Go calculations and invalid/unknown input tests; measured/configured/manual provenance |
| F08 plans | sites, saved device actions | Managed decoded/re-encoded PNG/JPEG/WebP assets, normalised markers, layers/search/zoom/pan/history, backup ID remapping | Image size/corruption, cross-site refs, replacement and backup tests; offline only, no remote map or PDF renderer |
| F09 handover | `internal/reports`, audit/site runs | Immutable commissioning snapshot, checklist/branding, evidence sections, redaction; offline HTML/PDF | Large multi-page reports, Unicode/image fixtures, secret exclusion; pure-Go PDF and embedded redistributable font |
| F10 SSH discovery | CLI, x/crypto, reconciliation | Explicit site SSH target, pinned host key, key/agent or vault authentication, fixed safe remote commands, versioned bounded JSON; reviewed remote imports | In-process SSH server and hostile-input/key/cancellation tests; real remote host remains separate |

## Shared design

- Keep networking and business logic outside Wails. Extend ONVIF and streaming
  clients; add small ISAPI, installation-data, retention and SSH packages.
- Add site-scoped, versioned documents/assets using the existing SQLite store
  and migration backup mechanism. Encrypt sensitive configuration data with the
  existing vault. Include new content in encrypted site backups and remap local
  references on restore; ordinary exports omit blobs and credentials.
- Use existing toolkit tasks/events for bounded reads and cancellation. Keep
  viewer controls owned by the actual Wails window, with Go-only credentials.
- No inference of offline/recording/success from missing information. Reports
  retain observation timestamps, provenance and explicit unknown/not-tested.
- New dependencies are limited to a pinned pure-Go PDF renderer and x/image
  for WebP and an embedded Go font. Existing x/crypto provides SSH; no media
  subprocess, cloud renderer, extra web server or privileged dependency install.

## Work sequence

1. Stream telemetry plus shared persistence and read-only NVR/events.
2. PTZ/focus, retention and commissioning/reporting.
3. Encrypted configuration tooling, offline plans and SSH discovery.
4. Integrate complete GUI/CLI flows, regenerate bindings, run engine/vet/race,
   frontend/packaging checks, native synthetic GUI checks and Windows/Linux builds.

Each feature receives implementation and validation notes in
`docs/ADVANCED_FEATURES_VALIDATION.md`. Compilation and synthetic tests will be
reported separately from physical-camera and Windows graphical validation.

## Protocol references

- ONVIF PTZ WSDL: https://www.onvif.org/ver20/ptz/wsdl/
- ONVIF Events WSDL: https://www.onvif.org/ver10/events/wsdl/event.wsdl
- ONVIF Imaging WSDL: https://www.onvif.org/ver20/imaging/wsdl/imaging.wsdl
- ONVIF specifications: https://github.com/onvif/specs
- Hikvision ISAPI General Application Developer Guide (vendor-authored archived
  documentation): https://raw.githubusercontent.com/loozhengyuan/hikvision-sdk/master/resources/isapi.pdf
- Pinned Wails v3 source and https://v3.wails.io/features/windows/multiple/
- PDF renderer documentation: https://codeberg.org/go-pdf/fpdf

ISAPI availability is checked at runtime. A documented endpoint is not evidence
that every Hikvision firmware implements it. No production restore compatibility
entry will be invented from a model-name guess.

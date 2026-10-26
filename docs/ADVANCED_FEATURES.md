# SADPTools v0.5 technician tools

The v0.5 beta adds the ten roadmap workflows to the existing application. Start
with **Sites**: save the equipment and assign the device/site credential that has
the appropriate read or control permissions. Then open **Commissioning**. Reads
are explicit, use the operating system's route and preserve unknown values.
Certificates require normal system trust or an explicitly approved exact
SHA-256 fingerprint. No new listener, cloud service or media executable is used.

## NVR channels

Choose a saved recorder and **Read NVR channels**. Compatible Hikvision firmware
can expose input-proxy channel inventory, connection status and advertised stream
IDs through ISAPI. The sortable table shows recorder → channel → saved camera,
including disabled, empty, offline, connected and unknown states. Channel reads
prove capability; a model name alone is not treated as proof of an NVR.

Stable MAC/serial matches take precedence over IP candidates. **Set link** lets
you link or unlink a camera manually. Links survive rescans; an identity conflict
is flagged for review. Internal PoE addresses are displayed only as reported by
the NVR. They are not assumed directly reachable. A stream button opens that
advertised stream on the NVR in the existing separate Live View window. No stream
number is invented when the mapping is absent. Refresh channels if firmware has
changed the mapping. JSON, CSV and text exports include observation timestamps.

Recording status remains Unknown where no verified recording-status provider is
available. Unsupported/authentication/connection failures are distinct. Saved
channel inventories remain available after a failed refresh.

## Stream diagnostics and focus

In Live View expand **Stream Info & connection tests**. Codec and resolution come
from media metadata; RTP bitrate, encoded access units, loss and decode-error
counts come from packet/parser observations. Displayed FPS uses actual video
frame callbacks or decoded JPEG frames; it is unavailable on WebViews without
those callbacks. First media/playing latency and a bounded state timeline are
recorded. RTP bitrate includes RTP headers, not TCP/Ethernet overhead. Player
frame-drop counters are saved only when the video element exposes them.

Use **Run 15-second connection test**, or **Compare main and sub**. Tests do not
change camera encoding or retry automatically. NVR channels with explicitly
advertised stream IDs are tested separately using their stream buttons. Copy
sanitised diagnostics, or save an observation to the site for reports/exports.
TCP is supported. UDP selection is unavailable in this beta. Nominal FPS and
jitter are not fabricated. H.264 is generally the most compatible starting point;
H.265 depends on the operating system WebView decoder. Proprietary H.264+/H.265+
encoding can remain incompatible even when ordinary H.264/H.265 works.

**Focus & alignment assistant** supplies thirds/grid/crosshair/horizon guides,
digital pan/zoom/reset, an optional relative Laplacian sharpness trend, before/
after capture and a zoomable split comparison. The score samples the central
80% of the decoded frame at reduced resolution: compare adjacent views of the
same scene, not unrelated cameras. It is a heuristic, not calibrated focus or
optical coverage. Capture/save is always explicit. Images can be saved to a site
or a local PNG file; there is no continuous recording. Saved site evidence is
associated with the camera, capture time and advertised NVR stream where relevant.

## PTZ, motor focus and presets

Use an ONVIF video profile, then **Read supported controls**. Physical controls
remain disabled until capabilities are read and you explicitly arm them for
that camera. A verified serial is required. Each write rechecks the serial, uses
the selected profile and is rate limited. Fixed cameras have disabled motor
controls. Optical movement is separate from digital preview zoom.

Directional/zoom buttons request bounded steps. Mouse/key release, blur, Stop,
viewer close and a backend watchdog attempt Stop. The device receives a 0.5 s
continuous-movement timeout too. Stop errors are unconfirmed and must be checked
at the camera. No uncertain write is retried automatically. Relative focus uses
small supported steps; autofocus selects the supported AUTO mode. Iris steps
require an already manual exposure mode; SADPTools does not switch exposure mode
on your behalf.

Read/recall/add/delete presets are supported where the profile permits them.
Adding captures the current position; deletion requires confirmation. Pure
rename is unavailable because ONVIF SetPreset can also overwrite the position.
Tour preview visits up to eight checked presets once, with five-second dwell;
it does not program a camera patrol. Stop/blur cancels the preview.

## Event walk-test

Choose a saved camera, duration (10–300 seconds), and **Start walk-test**. The app
queries ONVIF event topics, creates a PullPoint subscription and displays received
notifications. It records device and local receipt times, source channel,
normalised type/state, original topic and suspicious clock offsets. Unknown
vendor events remain visible. No event means **No event observed**, not failure.

Only the latest 500 notifications remain in memory; the UI shows the latest 100.
The total counter continues. Optional in-app visual notification is opt-in.
Stop, timeout and application shutdown cancel polling and attempt unsubscribe.
A disconnected feed stops with an error; there is no silent reconnect. If
unsubscribe fails the outcome is marked unconfirmed; the requested subscription
lifetime is two minutes.

After stopping, grade individual events and/or the overall test, add notes and
an optional explicitly captured snapshot, then **Save graded summary**. Live
notifications are not automatically stored. The test never enables analytics or
changes motion/line-crossing settings.

## Configuration backup, diff and restore preview

**Capture encrypted settings** reads documented ONVIF NTP/DNS/time configuration
and profile metadata. Available, unknown, unreadable, excluded and unsupported
sections are recorded individually. This is a partial settings backup, not a
claim that the vendor supports a complete configuration export. The raw package
is encrypted with the existing vault; ordinary document lists contain section
status, metadata and a checksum, not setting values or secrets.

Capture another snapshot for a current-versus-backup comparison. Structured diffs
show added/removed/changed/unchanged/unknown values and omit secret or nonportable
values. Portable encrypted `.sadpconfig` import/export requires a separate
12-character-or-longer backup passphrase. Wrong passphrases, corruption and
unsupported package versions are rejected.

**Preview restore / clone** explicitly identifies the selected target and shows
portable candidates and exclusions. Model/manufacturer/firmware mismatch or
unknown identity blocks compatibility. Networking, MAC/serial, accounts, keys,
recovery material and device-specific profiles are excluded.

**Production restore is deliberately disabled**, as required by the roadmap,
until an approved model/firmware provider has passed physical-device validation.
The isolated executor has identity, verification and stop-on-uncertain-write
fixture coverage; there is no user-accessible bypass. No opaque vendor export or
blind cross-model cloning is claimed.

## Storage planner

The calculator works without a site. Enter channel groups, typical and optional
low/high video bitrates, audio, hours/day or a seven-day calendar, recording duty
fraction, usable capacity after RAID, reserve, overhead, safety margin and target
days. Units distinguish decimal TB/GB from binary TiB/GiB. A zero/missing bitrate
is Unknown, never infinite retention.

Prefill only from verified NVR targets or saved stream measurements. Each bitrate
is labelled configured, measured or estimate; resolution is never used to invent
a bitrate. Save/copy scenarios for side-by-side comparison and report inclusion.
Actual NVR rules, scene activity, pre/post-event buffers and chosen streams still
affect real retention. Saving a scenario does not modify the NVR.

## Offline site plans

Attach PNG/JPEG/WebP images. Decoding is bounded (16 MiB input, 16 million pixels,
maximum dimension 8192); images are re-encoded to PNG to remove metadata and
active content. PDF input, geolocation and remote maps are not used.

Drag saved devices onto a plan and move markers, or click one for its identity,
notes, layer, direction/cone and Live View. Coordinates are normalised and survive
resizing and image replacement. Search, layer visibility, zoom/pan and bounded
undo/redo are available. Save placements explicitly. Image replacement preserves
coordinates; review them against the new drawing. Archived/missing cameras are
identified. Removing a plan requires confirmation; images referenced by another
plan or immutable report cannot be deleted.

## Commissioning reports

Complete the per-camera checklist using Pass / Fail / Not tested / Not applicable,
with technician/date/project, notes, summary and optional signoff. New checks
start Not tested. Saved playback evidence and graded walk-tests can populate
matching checks with an evidence reference; other checks remain manual. Company
branding templates and local logos are configurable.

Save the checklist, select channel/diagnostic/walk-test/retention/plan evidence
and snapshots, then generate an immutable report snapshot. Export offline PDF,
HTML, CSV or JSON. Reports include inventory, checklist/provenance, outstanding
issues, selected evidence, latest saved audit and signoff. Re-exporting a snapshot
does not silently substitute newer inventory. Go fonts support Latin/Māori and
other included glyphs; arbitrary world-script coverage is not promised.

Optional client-facing redaction removes known customer/network/serial values
from report text, including free-form labels. **Image pixels are unchanged**:
review screenshots and plan drawings for OSD addresses or confidential content
before handover. Credentials and authenticated stream URLs are excluded from
ordinary reports. Report snapshots and their referenced images remain immutable.

## Remote discovery over SSH

Install the headless `sadptools` v0.5 CLI on an explicitly trusted remote Linux
host. Configure the saved site's host/port/user, executable, adapter/protocols,
scan timeout and either a known_hosts file or independently verified SHA256 host
key fingerprint. Wrong/changed keys are rejected. An explicit fingerprint plus a
known_hosts file must satisfy both checks.

Use a private key under your control, `SSH_AUTH_SOCK` where available, or a saved
vault password profile. For an encrypted private key, its passphrase uses the
profile's privacy-password field. No password/key contents are saved in SSH target
metadata. Windows users can use an identity file or a vault profile; this beta
does not implement the Windows OpenSSH named-pipe agent.

The app probes `sadptools capabilities --json` and invokes only a validated fixed
`discover --report` argument set. It never accepts an arbitrary shell command or
remote device writes. Output is versioned and bounded; operations have deadlines,
channel cancellation and partial-result handling. Results show source host,
adapter, time and SSH handshake latency. Review/select observations, then import
into that same site. Remote data never joins the local live-discovery inventory.
An existing VPN route is supported; VPN provisioning is outside this feature.

```bash
make cli
./bin/sadptools --version
./bin/sadptools capabilities --json
./bin/sadptools discover --interface eth0 --protocols sadp,onvif --timeout 10s --report
```

Replace `eth0` with the remote host's actual adapter. Existing `--json` remains the
plain device-array format; `--report` is the versioned remote envelope. Empty
successful scans return success, partial scans return 3, and Ctrl+C cancels.

## Data and compatibility

SQLite schema 3 adds site documents/assets through the existing transactional
migration and private pre-migration backup. Portable encrypted site backup v2
includes new evidence, encrypted configuration assets, plans and report snapshots;
restore accepts v1 and v2 and remaps site/device/credential/document/image links.
Attachments are bounded to 32 MiB per site. Cross-site references are rejected.
Images are excluded from ordinary inventory CSV.

A device with referenced commissioning evidence cannot be moved/merged out from
under that evidence. Archive it and create the destination inventory record, or
review/remove the references first. Deleting a whole site cascades its documents
and assets through existing confirmation. Local backups still need normal care.

See [validation and hardware checklist](ADVANCED_FEATURES_VALIDATION.md) for the
exact boundary between automated checks and physical/runtime validation.

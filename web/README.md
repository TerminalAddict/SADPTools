# SADPTools marketing website

Public site: **https://sadptools.com/**

The website promotes camera installation, management and diagnostics on **Windows, Linux and Apple Mac**. It includes a searchable 36-area features page with 72 light/dark desktop screenshots, Windows and Apple Mac installation help, a privacy page, social sharing metadata and a 1200 × 630 social image, JSON-LD, a sitemap, and first-party downloads. No source-code links or upstream release-provider links are exposed in pages, scripts, metadata or download responses.

## Layout

- `public/`: the only directory deployed to the document root. Static HTML/CSS/JS and images plus the PHP download endpoints.
- `public/_app/ReleaseProxy.php`: release resolution, checksum validation, cache and bounded HTTPS transfers. Apache denies public access to this directory.
- `scripts/mac_help.py`: Mac installation, Gatekeeper, network access, credentials and manual-update guidance.
- `scripts/build-pages.py`: page content and shared metadata/header/footer. Regenerate HTML after editing.
- `content/features.json` and `scripts/feature_page.py`: the full feature catalogue, capability boundaries, captions and static features-page renderer.
- `content/screenshots.json`: reviewed capture dimensions, provenance and SHA-256 hashes.
- `public/assets/features.{css,js}`: responsive feature guide, local text search and accessible screenshot dialog.
- `public/assets/screens/`: 72 native PNG captures and 72 compact WebP previews. Full PNGs load when a visitor opens the gallery.
- `scripts/social-card.html` and `social-card.mjs`: editable social-card artwork and PNG export.
- `scripts/seed-demo.go` and `capture-app.py`: isolated native desktop screenshots with synthetic inventory. No live devices, credentials or customer captures.
- `scripts/check-browser.mjs`: desktop/mobile screenshots, accessibility, theme persistence, image/metadata and first-party request checks.
- `scripts/check-downloads.py`: end-to-end full download hashes, HEAD, resumed ranges, malformed requests and private-route checks.
- `tests/proxy.php`: synthetic proxy tests, including automatic new-release selection, cache expiry/outages and corrupt files.
- `screenshots/`: site screenshots in both themes and validation reports. These are generated review artifacts, ignored by Git and not deployed.

## Connection details

`web/.credentials.json` is the local connection file requested by the maintainer. It contains the SSH destination, document root and site URL, and uses the existing SSH key/agent configuration. It contains no password or private-key material. It is mode `0600`, explicitly excluded by the repository `.gitignore`, and outside `public/`.

For another checkout, copy `.credentials.example.json` to `.credentials.json`, fill in the connection details and run `chmod 600 web/.credentials.json`. Do not copy private keys into this project. The deployment script requires the credentials file to be ignored and private.

## Develop and deploy

Runtime: Apache with `mod_rewrite`, `mod_headers`, PHP 8.2+ and PHP cURL. No production Node service, database, scheduled job, API token or system-package installation is needed on the configured host.

```sh
npm --prefix web ci
npm --prefix web run build
npm --prefix web test
npm --prefix web run preview
```

The preview is at `http://127.0.0.1:8790`. In a second terminal:

```sh
node web/scripts/social-card.mjs
npm --prefix web run build # refresh content-hashed sharing image and asset URLs
node web/scripts/check-browser.mjs
java -jar /home/paul/bin/vnu.jar --stdout web/public/*.html
java -jar /home/paul/bin/vnu.jar --stdout --css web/public/assets/site.css web/public/assets/features.css
python3 web/scripts/deploy.py
bash web/scripts/validate.sh https://sadptools.com/
SITE_URL=https://sadptools.com node web/scripts/check-browser.mjs
python3 web/scripts/check-downloads.py
```

Playwright and axe are pinned in `package-lock.json`. A compatible Playwright Chromium installation is required only for authoring/testing. The host already has it. If setting up a new development machine, `cd web && npx playwright install chromium` downloads the user-local browser; it does not install system packages.

`validate.sh` invokes the maintainer's actual `/home/paul/bin/validate-site.sh --mode both`. Set `SADPTOOLS_SITE_VALIDATOR` to override that path. Its Nu validator blocks loopback URLs by default, so validate local files directly before deployment, then run the crawler against the public HTTPS site. The public crawl covers the homepage, features page, privacy page, Windows help, Mac help and both linked stylesheets. The 404 page is also validated directly.

Deployment uses `rsync --delay-updates`, sends **only public application files**, and does not delete existing remote files or modify Apache/system configuration. It installs the download-directory access guard separately and excludes all runtime records and report credentials. Existing log ownership and permissions are preserved. Existing ACME files are preserved. Review removed public paths manually if a future site change retires a file. No Git push or app release is involved.

### Host compatibility

The existing vhost has a legacy `php_admin_value auto_prepend_file` referencing `wordfence-waf.php`, but the document root was empty and no WordPress/Wordfence installation existed. The supplied file at that path sets response defaults and suppresses public PHP error details; it is **not** a Wordfence installation or a claimed application firewall. Direct web access is denied. An administrator can remove the obsolete prepend directive when tidying the vhost; the website does not depend on WordPress.

## Automatic downloads

The stable public URLs are:

| Route | Download |
| --- | --- |
| `/download/windows-setup.exe` | Windows AMD64 Setup with offline display runtime |
| `/download/windows-portable.zip` | Windows AMD64 standalone desktop ZIP with notices |
| `/download/linux.deb` | Linux AMD64 DEB |
| `/download/linux.rpm` | Linux x86-64 RPM |
| `/download/linux.tar.gz` | Linux AMD64 manual installer with `install.sh` |
| `/download/macos.dmg` | Universal Apple Silicon + Intel app in a drag-to-Applications DMG |
| `/download/macos.zip` | The same universal Mac app in a ZIP |
| `/download/checksums.txt` | Hashes and original versioned names for these seven files |
| `/api/releases` | Public version, date, sizes, hashes and first-party URLs |

The proxy reads the fixed project's public release API, includes beta/RC releases, excludes drafts and sorts valid semantic versions. It requires all seven desktop formats and their checksum manifest in the newest release. The release builder's established filenames are the contract; no version is hardcoded in the website.

Metadata refreshes on demand every **five minutes**. A cached catalogue missing a current target is invalidated immediately, including migration from the original five Windows/Linux formats. No redeployment or cron job is needed after a normal release. An upstream outage can use last-confirmed metadata for up to 24 hours, labelled as stale by the UI; requests back off for a minute after failure. An incomplete newest release does not silently masquerade as complete. If no usable metadata remains, the endpoints return `503` and a retry hint.

Downloads are streamed to a private temporary file, checked against the release SHA-256 manifest (and API digest where supplied), then atomically cached and delivered from this domain. Browser-visible responses never redirect to the upstream provider or relay its headers. Cache files are outside the document root in the PHP user's temporary directory, with a private directory and private files. A shared lock prevents concurrent duplicate transfers. Unused binaries expire after seven days when metadata refreshes; the OS may also clear its temporary directory. Each upstream asset is bounded to 512 MiB and timed out; corrupt/interrupted files are removed.

Only the fixed repository and allowlisted HTTPS release/CDN hosts are accepted, including at each redirect. Visitors cannot supply an upstream URL. Metadata fetches are bounded, certificate verification stays enabled, and full binary hashes are checked before sending any executable bytes. Downloads support HEAD and single byte ranges with an ETag/If-Range handoff. GET/HEAD are the only accepted methods. The proxy does not repack, modify or sign release archives; existing attribution inside those archives remains intact.

## Private download records

The package proxy records each valid package GET in
`public/assets/private-downloads/downloads.jsonl`: one JSON object per line.
The file starts empty; records are appended under an exclusive file lock, without
replacing the file or its owner. The directory and log are **paul:paul, mode 0777**,
as requested by the maintainer. Both the directory's `.htaccess` and a rule in the
parent `.htaccess` deny every HTTP request to this directory. PHP reads/writes it
directly through the filesystem. The preview router enforces the same exclusion.

Rows contain a request ID, UTC start/finish timestamps, direct peer IP, bounded
user agent, inferred browser/OS, selected release version and filename, requested
range, HTTP status, bytes sent, duration and outcome. Only `REMOTE_ADDR` is used;
untrusted forwarded headers do not override the address. Authentication headers,
cookies, query strings and referrers are not collected. HEAD requests, metadata,
checksum-file requests and invalid package names are not download records.

`sent` means the server finished writing the requested bytes, not that the visitor
saved or installed the app. Range requests are separate from full transfers;
failed and detected disconnected transfers retain their outcomes. Browser/user
agent labels are estimates, and automated requests can appear. Tests use the
distinctive `SADPTools-Website-Download-Check/1.0` agent on the live site. There is
no automatic deletion of records. Logging errors go to the server error log with
a generic message and do not break package delivery.

The report is at **https://sadptools.com/assets/downloads_report.php**. It uses an
HTTPS browser password prompt and displays all-time totals with 200 records per
page in Pacific/Auckland time. Search covers the entire history, case-insensitively,
with each word matching across the recorded fields (including raw user agents and
UTC/Auckland dates). Column-heading links toggle ascending/descending order;
dates, IP addresses, release versions and byte counts use their appropriate data
ordering. Search/sort choices persist when paging, refreshing or exporting, and
the controls also work without JavaScript.

Each row has a **Delete** button and a checkbox. The table-header checkbox selects
the current page; **Select all matching records (every page)** includes the entire
current search, even beyond the 200 visible rows. Individual visible rows can be
unchecked to exclude them from that selection. Clear selection resets both modes;
paging, searching and sorting start a fresh selection. Without JavaScript, row
checkboxes and select-all-matching still work, but the latter overrides individual
checkboxes.

Deletion first opens a confirmation showing the exact count, search and sample
records. **Cancel** leaves the log unchanged. Confirmation captures specific record
IDs, so later matching downloads are excluded, and expires after ten minutes or one
use. Only authenticated POST requests with a session CSRF token can preview or
confirm deletion; GET requests cannot delete records. The report's session cookie
is scoped to this endpoint, Secure on HTTPS, HttpOnly and SameSite=Strict.

Readers, writers and deletion share an exclusive file lock. Deletion rewrites the
existing file in place, preserving its inode, owner and permissions. A temporary,
durable recovery copy in the private directory protects interrupted rewrites; the
next read/write restores an uncommitted deletion. Successful deletion removes this
copy, so there is no permanent archive of deleted records. Keep both the log and
any recovery file together if moving them while the application is stopped.

Authenticated CSV and JSON Lines exports contain **every matching record**, in
the selected sort order, regardless of the page currently displayed. Clear the
search to export the full history. CSV includes UTC and Auckland start times,
finish time, request ID, IP, browser/OS, version/package/filename, transfer/range,
result, HTTP status, byte counts, duration and raw user agent. It uses UTF-8 with
a BOM, quoted CSV fields and CRLF row endings. Formula-like cells receive a text
prefix for initial spreadsheet import; JSON preserves the original values for
machine processing. No CSV file is saved under the public assets directory.
No GET parameter bypasses authentication. Anonymous and wrong-password
requests return an empty `401`; missing login configuration fails closed. Report
responses use `no-store` and `noindex`. Use a private browser window and close it
when finished to discard remembered HTTP Basic credentials.

Set up the records/login after deploying the access guards:

```sh
python3 web/scripts/setup-download-logging.py --local  # optional local preview
python3 web/scripts/deploy.py
python3 web/scripts/setup-download-logging.py          # production
```

The setup command generates a strong password once and saves it in the local,
mode-0600 **`web/.download-report-credentials.json`** file. Open that file privately
to obtain the username/password. Only its hash and username are sent over SSH to
`public/_app/download-report-auth.json`, outside the writable records directory
and blocked from HTTP. No password is printed or passed in shell arguments.
Keep the private local credential file to reuse the login on subsequent setup;
the command refuses to replace a different existing login. To rotate it, choose a
new password/hash and deliberately replace both private configurations together.
Provisioning never truncates existing records.

The records directory (except `.htaccess`), backups, local login and server hash
configuration are all ignored by Git. The deployment script excludes runtime
records and the server hash even if they exist locally, and does not chmod or
overwrite them during later website deployments. The report is not linked from
public pages or included in the sitemap. The existing privacy notice is unchanged.

Additional verification:

```sh
REPORT_BROWSER_CHECKS=1 npm --prefix web test
java -jar /home/paul/bin/vnu.jar --stdout web/screenshots/download-report-fixture.html web/screenshots/delete-confirmation-fixture.html
java -jar /home/paul/bin/vnu.jar --stdout --css web/public/assets/download-report.css
```

The isolated tests use synthetic packages and logins, concurrent writers, forced
disconnects, broken log/auth files, hostile user agents, resumed transfers and
permission-preserving setup, deletion snapshots, cancellation, replay protection,
CSRF rejection, concurrent logging and interrupted-rewrite recovery. Optional
browser checks cover individual/bulk selection and deletion and capture only synthetic report
data in both themes and mobile/desktop layouts. Keep real visitor data out of test
fixtures and screenshots.

Validation for the logging change: 51 existing proxy assertions, 25 log/auth
assertions, 31 query/sorting/CSV assertions, 24 deletion/recovery/concurrency assertions
and 82 isolated HTTP assertions passed. The report and deletion confirmation passed automated
accessibility checks in four desktop/mobile theme views and Nu HTML/CSS checks.
The live site passed `validate-site.sh`; all seven full package downloads matched
their checksums. Production checks verified report authentication and `403`
responses for the private log and password-hash paths.
Deletion deployment checks verified authenticated selection previews and cancellation,
CSRF rejection, session-cookie attributes and denied recovery-file access. Existing
production records, ownership, permissions and login configuration were preserved;
actual deletion was tested only with isolated synthetic records.

## Screenshots and social assets

Native screenshots use an isolated Xvfb display, private DBus session and XDG directories under ignored `web/.cache/`. The fixture is clearly marked as demonstration data and uses documentation-only addresses; discovery is not started. Recreate them after building the Linux GUI:

```sh
xvfb-run -a -s '-screen 0 1440x980x24' dbus-run-session -- python3 web/scripts/capture-app.py
node web/scripts/social-card.mjs
```

### Full feature gallery

The page at `/features.html` groups 36 feature areas into eight workflows. It includes all ten v0.6 diagnostic tools, the v0.5 commissioning/Live View tools, discovery, activation, network configuration, authorised recovery and saved-site management. A new platform section covers universal Mac packaging, native network diagnostics and platform-aware updates. The catalogue documents Mac/Linux passphrase credentials and Mac codec/capture limits. All app images remain Linux captures; none are presented as evidence of Mac hardware testing. The CLI is described separately with example commands. Features that depend on device support and planning-only tools have explicit boundaries.

The page is static HTML and remains readable without JavaScript. Screenshot links fall back to original images. With JavaScript enabled, search filters the catalogue locally; category links clear the filter; the native dialog supports Light/Dark, Previous/Next, arrow keys, Escape, focus return and a full-resolution image link. Thumbnail themes follow the website preference. Images are served from this site, without third-party requests.

Capture tooling uses **Go build overlays**, an ignored separate binary and an isolated XDG database. The overlays supply synthetic live inventory, a documentation-only adapter, synthetic security-question readiness and the saved synthetic audit report. They do not edit production application sources or replace `bin/SADPTools-linux-amd64`. The normal app never loads these fixtures. The synthetic capture workflow connects only to the repository's local test-pattern RTSP source. It performs no physical camera actions, discovery, write operations, event subscriptions or packet captures. The four real-camera views described below use a separate workflow. The settings capture is limited to the appearance/discovery portion of the native dialog. Full image captions disclose setup-only, calculated or synthetic evidence as appropriate.

Recreate the full gallery on a Linux host with the desktop build dependencies and an existing `frontend/dist` build:

```sh
mkdir -p web/.cache
magick web/scripts/demo-plan.svg web/.cache/demo-plan.png
go build -o web/.cache/feature-fixture web/scripts/seed-feature-demo.go
python3 web/scripts/build-capture-app.py
xvfb-run -a -s '-screen 0 1440x980x24' dbus-run-session -- python3 web/scripts/capture-features.py
# Visually review every raw capture under web/.cache/feature-capture/raw first.
python3 web/scripts/prepare-feature-images.py
python3 web/scripts/build-pages.py
```

Review raw captures before publishing, especially if the application layout has changed: the native X11 automation uses fixed coordinates and can otherwise photograph the wrong panel. Do not publish real addresses, customer details or secrets. The builder refuses missing theme images; the preparation script checks that the theme pairs differ and have matching dimensions. Review the gallery whenever a UI or feature description changes; recapture altered screens and keep existing platform-neutral images when still representative. Real-camera recaptures require new authorisation; release downloads continue updating automatically through the existing proxy.

### Authorised real-camera views

The `live-view`, `stream-info`, `focus` and `ptz` entries are marked `capture: real-camera` in the catalogue. Their light/dark PNGs show the released Linux application playing an explicitly authorised physical camera. Gallery captions, image alternatives and the viewer distinguish these from synthetic fixtures. Screen URLs include content-derived version tokens to refresh cached previews when an image changes.

`scripts/capture-live-camera.py` prompts without terminal echo for the target and session credentials; they are never passed in process arguments, environment variables or saved credential profiles. It creates XDG state under a private `/dev/shm` directory, discards application output and disables core dumps. `seed-live-capture.go` creates only a manual loopback device entry. A temporary in-memory TCP relay forwards RTSP to the authorised target, so the camera address is absent from published images. There is no disk protocol logging, footage recording, ONVIF capability probe or camera write. The app uses its existing session-only sign-in flow. On exit the helper closes the app and relay and removes its temporary state. Runtime strings are released on process exit, not a claim about guaranteed OS memory erasure.

```sh
go build -o web/.cache/seed-live-capture web/scripts/seed-live-capture.go
xvfb-run -a -s '-screen 0 1440x1600x24' dbus-run-session -- python3 web/scripts/capture-live-camera.py
```

The helper is interactive: `main`, `click X Y main`, `login`, `resize WIDTH HEIGHT`, `move X Y`, `scroll STEPS`, `key NAME`, `shot NAME`, `next` and `quit`. Enter credentials only at the hidden prompts. Never put them in these commands or a file. Review the native sign-in form before `login`. The `login` keystroke helper currently targets the US keyboard layout and alphanumeric input; extend and test its character mapping before using other characters. Take the four named shots in each theme under ignored `.cache/live-camera-capture/`, visually inspect for sensitive details, then run `prepare-feature-images.py` and the page builder. The preparation script selects the correct raw source per catalogue entry and cannot silently replace these with the synthetic test pattern. Actual rendering/metrics are evidence for this tested stream, not proof of all codecs, models or PTZ capabilities.

The supplied Windows SmartScreen screenshot is copied without alteration from `docs/SADPTools-windows smartscreen warning.png`. The installation article distinguishes a reputation warning from a malware verdict and links to Microsoft's primary guidance. It does not tell users to disable protection or evade managed policies.

## Validation — 27 October 2026

- PHP lint and **51** synthetic proxy assertions passed, including universal Mac target selection, legacy-cache refresh, missing-Mac rejection and archive verification.
- Public `validate-site.sh --mode both`: **PASS**, five HTML pages and two stylesheets; no warnings/errors. Six local HTML files (including 404) and both stylesheets also passed Nu validation.
- 72 feature-gallery screenshots in light and dark mode: 64 synthetic demonstration screens and eight authorised physical-camera captures. The latter show read-only H.265 playback, real stream metrics, preview-only alignment/sharpness and the unarmed PTZ interface on Linux. No camera settings or physical controls were changed. Original homepage screenshots are retained.
- Chromium desktop/mobile checks passed for both themes, theme persistence, navigation, same-origin requests, metadata and all feature thumbnails. Gallery checks cover search/no-results/reset, theme changes, previous/next and arrow-key navigation, Escape, focus return and responsive layout. axe WCAG 2 A/AA and 2.1 AA checks reported no violations in the checked views. This is automated testing, not a claim of complete accessibility certification.
- All seven real production downloads completed and matched the published release hashes. HEAD, range resume, invalid-range, invalid-method and private-path checks passed.
- Public metadata and page links contain no upstream-provider/source links. Social image is served successfully and has the expected dimensions. Search/social platforms' eventual indexing is outside these checks.
- Credentials file is ignored, mode `0600`, and not deployed. No private SSH keys copied, privileged packages installed, Git commits pushed, or new desktop release created by this website task.

Saved evidence: `screenshots/browser-report.json`, `screenshots/download-report.json`, `screenshots/validation-report.txt`, plus full-page light/dark desktop/mobile screenshots.

# In-app updates

Open **Updates** in the toolbar to check GitHub releases. Checks also run once
at startup unless disabled in Settings. Downloads and installation are explicit
actions; an available update never closes the app automatically.

**About** also displays the running version and live update status. Use **Check
for updates** there to check manually. When a newer Windows release is available,
**Update now** opens the Updates screen and starts downloading it. Once verified,
choose **Install and close** to continue. If the download is already ready,
**Update now** goes straight to that confirmation screen. Linux offers **View
release** instead. Opening About alone does not start a network check when
startup checks are disabled.

1. **Check now** compares published versions with the running version.
2. **Download update** downloads the complete offline Windows installer, shows
   progress and verifies its SHA-256. You can cancel and retry a failed download.
3. **Install and close** verifies the cached file again, starts setup, stops
   discovery and exits SADPTools. Export any inventory or recovery data needed
   before closing; those live in memory. Preferences are preserved.
4. Setup waits for the old process to exit, installs the update and offers to
   launch SADPTools on its Finish page. The normal setup terms still apply.

Installation is blocked while an activation, network or recovery operation is
active. An installer launch failure leaves SADPTools open. The updater never
kills processes, retries camera writes or silently accepts setup terms.

**Beta.1 predates this feature. Its users need one manual upgrade to a release
containing the updater.** Do not replace beta.1's published assets with a changed
build; publish a new version through the [release process](releases.md).

### Updating from a standalone EXE

A versioned standalone `SADPTools-windows-amd64.exe` uses the same installer-based
update flow. Setup installs into `%LOCALAPPDATA%\Programs\SADPTools` by default,
or uses an existing installation registered for the current user. It does not
replace the original standalone file in Downloads or another arbitrary folder.
After setup, launch from its Finish page or the **SADPTools** Start menu shortcut
and confirm the new version in About. Reopening the original standalone EXE
still runs the old version. Preferences are shared for the same Windows user.

## Channels and platforms

Stable builds default to stable releases. Beta/RC builds default to stable and
beta releases (including RCs). The channel is saved in Settings. Changing it
requires a new check before downloading or installing. Existing preferences
survive upgrades. Version comparison is numeric: beta.10 follows beta.2, stable
follows RC/beta for the same version, and older/equal versions are never offered.

Tags must follow the project's `vMAJOR.MINOR.PATCH[-beta.N|-rc.N]` convention and
agree with GitHub's pre-release flag. Drafts and unrelated tags are ignored.
Development builds do not install updates and skip the startup check, avoiding
an accidental replacement of newer development code with an older public beta.

Windows AMD64 supports download/install. Linux can check and open release notes;
install updates using the DEB, RPM or manual installer matching the original
installation method. The updater does not install Linux packages or substitute
the Linux CLI archive for a desktop upgrade. macOS installation is also unsupported.

## Network, integrity and lifecycle

`internal/update` is independent of Wails and device networking. It uses the
public [GitHub releases API](https://docs.github.com/en/rest/releases/releases),
including paginated pre-releases, without `gh`, tokens or an account on the
user's computer. Only update-related requests go to GitHub and its asset hosts;
device inventory, credentials and recovery data are never included. Disable
startup checks in Settings for offline use. Manual checks remain available.

Checks time out after 30 seconds. Downloads have a 15-minute HTTP timeout, a
512 MiB maximum and an exact release-declared size. HTTPS redirects are limited
to GitHub's API, website and documented asset hosts. The installer must have the
expected repository/tag/filename, a unique entry in `SHA256SUMS`, and a matching
GitHub asset digest when supplied. Wrong sizes, hashes, duplicate assets, foreign
URLs and non-EXE payloads fail closed. Hashes establish integrity relative to the
GitHub release, not independent publisher signing: the app remains unsigned and
trusts this repository and HTTPS. Code signing is a future improvement.

Downloads use private temporary directories under
`os.UserCacheDir()/SADPTools/updates` (normally `%LOCALAPPDATA%\SADPTools\updates`
on Windows). Partial/cancelled downloads are removed. A ready file is removed
when discarded or when the app exits without starting setup. A launched
installer is retained so it can outlive the app; directories older than seven
days are pruned on a later launch. Nothing is persisted as an automatically
trusted installer: each session downloads and verifies anew.

The Go backend retains the chosen release and path; JavaScript cannot pass an
arbitrary executable or download URL to the install method. Installation
rechecks the file, uses `exec.Command` without a shell, and passes only the
current numeric process ID as `/WAITPID=<PID>`. Wails' pinned `App.Quit()` and
service shutdown hooks close the application and join outstanding work.

The independent NSIS process obtains a SYNCHRONIZE process handle and uses
[WaitForSingleObject](https://learn.microsoft.com/en-us/windows/win32/api/synchapi/nf-synchapi-waitforsingleobject)
with a 30-second limit. A process that has already exited is accepted. Access
errors and timeouts stop setup before installation; there is no forced kill.
Setup extracts `SADPTools.update.exe` beside the installed executable, then uses
[MoveFileExW](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw)
to replace it on the same volume. Extraction/replacement failures do not truncate
the existing executable. Another running instance or a file lock can prevent
replacement; close it and run setup again. No replacement is deferred to reboot.
This protects the executable replacement, not a transactional rollback of all
installer files, registry entries or future data migrations.

## Release contract and Windows acceptance checks

Future installers must retain `/WAITPID`, staged replacement and the existing
per-user installation location/registry key. Each release needs the full
`SADPTools-<VERSION>-Setup-windows-amd64.exe` plus `SHA256SUMS`; the release script
already supplies both. Keep assets in draft until all uploads are verified.

Before claiming Windows update support is runtime-tested:

- Upgrade an installed earlier version to a newer published test release; verify
  that setup waits for process exit, replaces the file and relaunches the new
  version, with settings intact.
- Test cancellation, offline/rate-limited checks, a corrupt download and a failed
  installer launch. The existing app must remain usable.
- Keep another instance/file lock open and verify replacement fails with the
  old executable intact. Test disk-full/staging failure and non-ASCII/spaced paths.
- Test waiting for a running/exited PID, invalid PID input, access denial and a
  wait timeout. Check exit codes 70 (invalid PID) and 71 (wait failed).
- Confirm no update can interrupt an active device operation. Use synthetic
  operations unless a specific device/action is authorised for a hardware test.

Linux compilation, synthetic tests, Xvfb UI checks and a successful GitHub
download do not establish Windows process-handoff or replacement behaviour.

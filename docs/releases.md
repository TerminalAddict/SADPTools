# GitHub releases

Release from Sparx using the installed `gh` CLI. `make release-build` prepares
files locally; `make release-publish` pushes the source commit and annotated tag,
uploads a draft, downloads every asset to verify its SHA-256, then publishes.
Beta and release-candidate versions are marked **Pre-release**, never **Latest**.
Ordinary commits and CI builds do not publish releases.

## Prepare a release

Use `MAJOR.MINOR.PATCH-beta.N`, `MAJOR.MINOR.PATCH-rc.N` or `MAJOR.MINOR.PATCH`.
The first beta is `0.2.0-beta.1`; the Git tag is `v0.2.0-beta.1`.
Choose a new version for each public build. Do not move tags or replace published
downloads. Next beta example: `0.2.0-beta.2`.

1. Finish and review changes on `main`. Pull remote changes with
   `git pull --ff-only` before preparing the release.
2. Write `docs/releases/v<VERSION>.md`: user-facing changes, recommended download,
   tested platforms, hardware evidence and known limitations. Keep credentials,
   captures and device identifiers out of the notes.
3. Review the pinned runtime and its notices as described in
   [Windows packaging](windows-packaging.md). Record any target-platform checks
   that remain outstanding; beta status does not imply Windows or camera testing.
4. Commit the changes and notes. The builder refuses a dirty checkout, including
   untracked source files. Do not edit source during a release build.
5. Build, inspect and publish:

```sh
make tools                         # Once, or after a pinned Wails update
make release-build VERSION=0.2.0-beta.1
cat bin/releases/v0.2.0-beta.1/build-manifest.json
(cd bin/releases/v0.2.0-beta.1 && sha256sum -c SHA256SUMS)
make release-publish VERSION=0.2.0-beta.1
```

Publishing requires `gh auth login` for an account with repository write access
and Git push access to `origin`. The script targets `TerminalAddict/SADPTools`,
requires `main`, verifies source/notes/checksums, and refuses mismatched tags.
The publish command is an explicit instruction to push and publish; it does not
prompt again. A future protected `main` branch may require pushing through a PR
before running it.

## Build inputs and downloads

The release builder runs on Linux AMD64 with the normal Go/Node/npm toolchain,
Python 3.11+, Make, Git, the pinned Wails CLI and `7z`. For this first beta the
tested host versions are Go 1.26.8, Node 24.21.0, npm 11.16.0 and Wails
v3.0.0-beta.34. NSIS 3.10 and the offline runtime are pinned and verified by
`build/windows/installer-inputs.json`. No system packages are installed by the
release command. If `7z` is missing on Ubuntu, install `7zip` separately with
administrator approval (`sudo apt install 7zip`).

The build runs `make check`, regenerates bindings, installs the locked frontend
dependencies, builds the frontend and Windows app, packages/tests the offline
installer, and builds Windows/Linux AMD64 CLIs. It checks the embedded Go version
string and executes the Linux CLI's `--version`. Generated bindings must match
the committed bindings or the clean-checkout check fails.

The ignored `bin/releases/v<VERSION>/` directory contains:

| Download | Purpose |
| --- | --- |
| `SADPTools-<VERSION>-Setup-windows-amd64.exe` | Recommended Windows desktop download; includes the offline display runtime and notices |
| `sadptools-<VERSION>-windows-amd64.zip` | Standalone Windows CLI, usage notes and licences |
| `sadptools-<VERSION>-linux-amd64.tar.gz` | Static Linux CLI, usage notes and licences |
| `build-manifest.json` | Source commit, version, toolchain details, pinned installer inputs and artifact hashes |
| `SHA256SUMS` | SHA-256 checksums for all four files above |

GitHub also provides the tagged source archives. Linux desktop users currently
build from source with `make linux-gui`; the GTK/WebKit libraries differ by
distribution, so this beta does not ship a generic Linux desktop binary.

`VERSION` stamps the Go app/CLI, Windows resources, installer properties and
installed-apps entry. Windows numeric versions use `MAJOR.MINOR.PATCH.REVISION`:
development `0`, beta `N`, RC `20000+N`, stable `65535` (N is 1–19999).
The displayed version retains the full `-beta.N` or `-rc.N` suffix. Unversioned
development builds still use `0.2.0-dev`. Examples:

```sh
make windows-installer VERSION=0.2.0-beta.1
make linux-gui VERSION=0.2.0-beta.1
SADPTOOLS_VERSION=0.2.0-beta.1 wails3 build GOOS=windows GOARCH=amd64
python3 build/windows/package.py --version 0.2.0-beta.1
```

Package an existing binary only with the same version used to build it. Prefer
the release command, which rebuilds and verifies the binaries. PE, NSIS and
archive timestamps may vary; identical inputs, not byte-identical rebuilds, are
promised. Keep the verified release directory as the local copy of what shipped.

## Interrupted uploads and failures

An interrupted publish can be resumed with the same `make release-publish`
command. Missing draft assets are uploaded; existing assets are downloaded and
verified, never overwritten. The release remains a draft until every download
matches. Re-running publication of an unchanged published release verifies it.
Unexpected or corrupt remote assets stop publication for manual review.

If code or notes change, rebuild from their new commit. The builder refuses to
overwrite an existing release directory: move aside or explicitly remove an
**unpublished** local directory first. If the tag already exists on a different
commit, use a new version. No script force-pushes, deletes a tag or changes a
published asset.

This follows GitHub's [draft-first publication guidance](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository?tool=cli)
and uses [`gh release create --verify-tag`](https://cli.github.com/manual/gh_release_create)
to bind the release to the explicitly pushed tag. It also works when immutable
releases are enabled because all uploads precede publication.

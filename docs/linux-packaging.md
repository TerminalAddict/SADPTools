# Linux installation and packaging

SADPTools provides three Linux AMD64/x86_64 desktop installers. All include the
same GUI, CLI, logo and licence notices. They register **SADPTools** in the
application menus used by GNOME, KDE Plasma, Xfce and other freedesktop desktops.
There is no background service or automatic startup. Run the GUI as your normal
user in a graphical session; the CLI also works over SSH.

## Install

Download the appropriate file from the release. Use the actual version in place
of `<VERSION>` below. Local development packages are under `bin/linux/` until a
new release is published. Verify `SHA256SUMS` against your downloads if desired.

**Ubuntu/Debian:**

```sh
sudo apt install ./SADPTools-<VERSION>-linux-amd64.deb
```

**Fedora:**

```sh
sudo dnf install ./SADPTools-<VERSION>-linux-x86_64.rpm
```

APT/DNF resolve the declared runtime dependencies from your distribution's
repositories. Internet access may be needed for missing libraries. These are
standalone local packages; they do not register a package repository. RPMs are
currently unsigned; the GitHub release's SHA-256 manifest verifies downloaded
bytes. Local signing requirements may be enforced by your system administrator.

**Manual installation:**

```sh
tar -xzf SADPTools-<VERSION>-manual-linux-amd64.tar.gz
cd SADPTools-<VERSION>-linux-amd64
./install.sh
```

This installs for the current user without sudo. For all users, choose
`sudo ./install.sh --system` instead. `./install.sh --help` lists the options.
The archive contains the application, so installation needs no network. The
script verifies its payload checksums and uses `ldd -r` to check libraries and
symbols before changing the installation. If libraries are missing it prints
the distribution's install command; it never runs a package manager itself.
The same checksummed archive can be used to install or upgrade.

Open **SADPTools** from your application menu, or run `SADPTools`. Use `sadptools`
for the terminal CLI. Manual per-user installs also print the complete CLI path
if `~/.local/bin` is not in PATH. The menu uses an absolute, correctly quoted
executable path, so it works without changing your shell configuration.

## Supported runtime

The default build uses **GTK4 >= 4.14 and WebKitGTK 6.0**. Ubuntu 24.04+,
Debian 13+ and current Fedora releases provide this stack. Older Ubuntu/Debian,
RHEL 9 and Alpine/musl are not targets for these prebuilt packages. Packaging on
a newer build host can raise the minimum versions; each build derives exact
Debian library dependencies from ELF symbol metadata and translates them into
Fedora package requirements. Both formats enforce these minimums. The manual
archive includes `payload/share/doc/sadptools/runtime-requirements.json`.

For current binaries built on Sparx the direct glibc requirement is >= 2.38.
GTK and WebKit remain distribution-managed so they receive the distribution's
security updates. They are not bundled inside SADPTools. File dialogs use the
desktop portal; use your desktop's portal backend (GNOME/KDE/etc.).

SADPTools does not request root privileges, add capabilities, alter firewall
rules or enable raw sockets during installation. If unprivileged ICMP is
disabled by your OS, the scanner retains its existing TCP/neighbour fallback.

## Files, upgrades and removal

Native packages own `/usr/bin/SADPTools`, `/usr/bin/sadptools`, the desktop entry
under `/usr/share/applications`, hicolor PNG/SVG icons and
`/usr/share/doc/sadptools` licence/documentation files. Install/upgrade/removal
hooks refresh the menu and icon caches where the corresponding tools exist.
The desktop ID matches the Wails ApplicationID for Wayland/window grouping.

Manual installs default to `~/.local/lib/sadptools`, with launchers in
`~/.local/bin`, and menu/icons in `${XDG_DATA_HOME:-~/.local/share}`. System
manual installs use `/usr/local/lib/sadptools`, `/usr/local/bin` and
`/usr/local/share`. `--prefix /absolute/path` selects a custom prefix and puts
menu/icons in its `share` directory. A nonstandard prefix must be in your
desktop's `XDG_DATA_DIRS` to appear in the menu. Standard defaults need no such
configuration. Keep the same prefix and scope when upgrading. Paths may contain
spaces and quotes; `%`, `=`, tabs and newlines are rejected because desktop
launchers do not reliably support them in executable paths.

Close SADPTools before upgrading, then install the new package/archive using
the same method. Manual upgrades replace the application directory instead of
writing through an open executable. The manual installer refuses to overwrite
unrelated files or shadow an existing native package. To switch methods, remove
the previous installation first. The Linux update screen links to releases;
automatic Windows installer handoff is not used on Linux.

Remove with one of:

```sh
sudo apt remove sadptools
sudo dnf remove sadptools
./install.sh --uninstall
sudo ./install.sh --system --uninstall
```

The manual install also prints an installed `uninstall.sh` path, which remains
available after you delete the extracted download. Removal deletes only files
owned by the installer. All methods preserve `~/.config/SADPTools` (or the
configured XDG config directory), including sites, encrypted credentials and
preferences. Back up important sites using SADPTools' encrypted export before
moving to a different PC/user account.

## Build on Sparx

```sh
make tools                  # pinned Wails v3.0.0-beta.34
make linux-packages         # rebuild GUI/frontend/CLI, then all three installers
make linux-packages VERSION=0.3.0-beta.2
```

Output: `bin/linux/`, containing the `.deb`, `.rpm`, manual `.tar.gz` and
`SHA256SUMS`. `wails3 package GOOS=linux GOARCH=amd64` is equivalent. To repackage
already-built binaries of the same version, use
`python3 build/linux/package.py --version 0.3.0-beta.2`. The packager checks ELF
architecture and embedded version before accepting either binary.

The builder targets Debian/Ubuntu AMD64 and uses Python 3.11+, the project's
pinned Go/Node/Wails tools, `dpkg-shlibdeps` and the GTK/WebKit development
libraries. nFPM **2.44.1**, embedded in our pinned Wails CLI, creates both native
formats without `rpmbuild`, Docker, root or a graphical display. No new runtime
Go dependencies are added. If prerequisites are missing, install them separately
with administrator approval:

```sh
sudo apt update
sudo apt install build-essential pkg-config dpkg-dev libgtk-4-dev libwebkitgtk-6.0-dev
```

For package inspection/desktop linting: `desktop-file-utils`, `rpm` and
`shellcheck` are optional developer tools. Prefer building release packages on
Ubuntu 24.04 for the oldest supported library baseline. Dependency checks fail
closed if new native libraries need a reviewed Fedora name mapping.

`make release-build` includes these three installers in future release bundles,
with the normal manifest/checksums and draft-download verification. Existing
published assets and tags are not modified. Beta/RC versions use the native
`~beta.N`/`~rc.N` package version ordering, so the final release sorts newer.

## Validation scope

Local validation on Sparx for this packaging change:

- `make check` passed: Go tests, vet, race detector, 21 frontend tests, 5 Windows
  installer tests, 18 release tests and 11 Linux packaging/installer tests.
- `make linux-packages` produced all three installers. SHA-256 verification,
  DEB payload/metadata/licence inspection and RPM digest/metadata checks passed.
- ShellCheck and `desktop-file-validate` passed. A real GIO launcher test covered
  executable paths containing spaces, quotes, dollar signs and backslashes.
- The actual manual archive installed in an isolated prefix, ran the CLI,
  opened a SADPTools window under Xvfb, then uninstalled while retaining user
  configuration. Library checking used the real compiled binary in this test.
- The already-published `v0.3.0-beta.1` bundle remains verifiable by the updated
  release tool. No existing release files or tags were changed.

Automated tests cover dependency/version metadata, manual install, upgrade,
uninstall, checksum failure, missing libraries, path quoting and unrelated-file
protection using isolated synthetic payloads. The Linux CI job is configured
to build all formats, check native package metadata and install/remove the DEB
on its disposable runner. See [GitHub Actions](https://github.com/TerminalAddict/SADPTools/actions)
for results on the release commit. No privileged
system installation was performed on Sparx. Native APT/DNF installation/removal,
real GNOME/KDE menus, Wayland and Fedora runtime still require checks;
successful packaging/Xvfb startup is not proof of those tests.

References: [Wails v3 Linux packaging](https://v3.wails.io/guides/build/linux/),
[Wails installation requirements](https://v3.wails.io/quick-start/installation/),
[nFPM configuration](https://nfpm.goreleaser.com/docs/configuration/),
[freedesktop executable quoting](https://specifications.freedesktop.org/desktop-entry/latest/exec-variables.html).

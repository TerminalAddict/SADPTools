#!/usr/bin/env bash
# Offline manual installer. Native packages should be managed with apt/dnf.
set -euo pipefail

die() { printf 'SADPTools: %s\n' "$*" >&2; exit 1; }
usage() {
    cat <<'EOF'
Usage: ./install.sh [--user | --system] [--prefix /absolute/path] [--uninstall]

Default: install for this user in ~/.local, with an application-menu icon.
--system: install in /usr/local for all users; run with sudo.
--prefix: choose another installation prefix (also use it when uninstalling).
--uninstall: remove only this manual installation; preserve all user data.

The .deb and .rpm are managed with apt/dnf, not with this script.
No packages are downloaded, no firewall changes are made, and the app is not
started by this installer. Close SADPTools before upgrading or removing it.
EOF
}

scope=user
prefix=''
remove=false
while (($#)); do
    case "$1" in
        --user) scope=user ;;
        --system) scope=system ;;
        --prefix) (($# >= 2)) || die '--prefix needs an absolute path'; prefix=$2; shift ;;
        --uninstall) remove=true ;;
        -h|--help) usage; exit 0 ;;
        *) die "Unknown option: $1 (use --help)" ;;
    esac
    shift
done

if [[ $scope == system ]]; then
    ((EUID == 0)) || die 'Use sudo ./install.sh --system for an all-users installation.'
    prefix=${prefix:-/usr/local}
elif ((EUID == 0)); then
    die 'Run without sudo for a per-user install, or select --system explicitly.'
fi

custom_prefix=$prefix
prefix=${prefix:-"$HOME/.local"}
[[ $prefix == /* ]] || die '--prefix must be absolute'
prefix=$(realpath -m -- "$prefix")
[[ $prefix != / && $prefix != /usr ]] || die 'Use /usr/local or a private prefix; /usr belongs to the package manager.'
data_root="$prefix/share"
if [[ $scope == user && -z $custom_prefix && -n ${XDG_DATA_HOME:-} ]]; then
    data_root=$XDG_DATA_HOME
fi
[[ $data_root == /* ]] || die 'XDG_DATA_HOME must be absolute'
data_root=$(realpath -m -- "$data_root")
for path in "$prefix" "$data_root"; do
    [[ $path != *$'\n'* && $path != *$'\r'* && $path != *$'\t'* && $path != *'='* && $path != *'%'* ]] || die 'Installation paths cannot contain tabs, newlines, = or % (desktop launcher limitations).'
done
app_dir="$prefix/lib/sadptools"
app_id=io.github.terminaladdict.sadptools
marker="$app_dir/.sadptools-manual-install"
expected_marker=$(printf 'SADPTools manual install v1\n%s\n%s\n%s\n' "$scope" "$prefix" "$data_root")

# Symlinks let menu/CLI entry points survive an atomic replacement of binaries.
targets=("$prefix/bin/SADPTools" "$prefix/bin/sadptools"
    "$data_root/applications/$app_id.desktop"
    "$data_root/icons/hicolor/scalable/apps/$app_id.svg"
    "$data_root/icons/hicolor/512x512/apps/$app_id.png")
sources=("$app_dir/bin/SADPTools" "$app_dir/bin/sadptools"
    "$app_dir/share/applications/$app_id.desktop"
    "$app_dir/share/icons/hicolor/scalable/apps/$app_id.svg"
    "$app_dir/share/icons/hicolor/512x512/apps/$app_id.png")

refresh_desktop() {
    if command -v update-desktop-database >/dev/null 2>&1; then
        update-desktop-database "$data_root/applications" >/dev/null 2>&1 || true
    fi
    if command -v gtk-update-icon-cache >/dev/null 2>&1; then
        gtk-update-icon-cache -q -t "$data_root/icons/hicolor" >/dev/null 2>&1 || true
    fi
}

if [[ -e $app_dir || -L $app_dir ]]; then
    [[ ! -L $app_dir && -f $marker && $(cat -- "$marker") == "$expected_marker" ]] ||
        die "Refusing to replace an unrecognised installation at $app_dir"
elif $remove; then
    printf 'No manual SADPTools installation at %s\n' "$app_dir"
    exit 0
fi

if $remove; then
    for i in "${!targets[@]}"; do
        if [[ -L ${targets[i]} && $(readlink -- "${targets[i]}") == "${sources[i]}" ]]; then
            rm -- "${targets[i]}"
        elif [[ -e ${targets[i]} || -L ${targets[i]} ]]; then
            printf 'Preserving changed entry: %s\n' "${targets[i]}" >&2
        fi
    done
    rm -rf -- "$app_dir"
    refresh_desktop
    printf 'SADPTools removed. Saved sites, credentials and preferences were preserved.\n'
    exit 0
fi

# Avoid shadowing a package-manager installation with a stale manual version.
if [[ -e /usr/share/applications/$app_id.desktop || -e /usr/bin/SADPTools ]]; then
    die 'A native SADPTools package is installed. Upgrade it with apt/dnf, or remove it before using the manual installer.'
fi
for i in "${!targets[@]}"; do
    if [[ -e ${targets[i]} || -L ${targets[i]} ]]; then
        [[ -L ${targets[i]} && $(readlink -- "${targets[i]}") == "${sources[i]}" ]] ||
            die "Refusing to overwrite an unrelated file: ${targets[i]}"
    fi
done

bundle=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || die 'This bundle is for Linux AMD64/x86_64.'
[[ -f $bundle/SHA256SUMS && -f $bundle/payload/bin/SADPTools ]] || die 'Extract the complete manual-install archive before running install.sh.'
command -v sha256sum >/dev/null || die 'sha256sum is required (GNU coreutils).'
(cd -- "$bundle" && sha256sum --check --quiet SHA256SUMS) || die 'Archive checksum verification failed. Download and extract it again.'
command -v ldd >/dev/null || die 'ldd is required to check runtime libraries (libc-bin on Debian/Ubuntu, glibc-common on Fedora).'
if ! dependencies=$(LC_ALL=C ldd -r "$bundle/payload/bin/SADPTools" 2>&1) ||
    [[ $dependencies == *'not found'* || $dependencies == *'undefined symbol:'* ]]; then
    printf '%s\n' "$dependencies" >&2
    cat >&2 <<'EOF'
SADPTools needs GTK >= 4.14, WebKitGTK 6.0 and compatible glibc libraries.
On a supported distribution, install the runtime once, then retry:
  Debian/Ubuntu: sudo apt install libgtk-4-1 libwebkitgtk-6.0-4 xdg-utils ca-certificates hicolor-icon-theme xdg-desktop-portal
  Fedora: sudo dnf install gtk4 webkitgtk6.0 xdg-utils ca-certificates hicolor-icon-theme xdg-desktop-portal
  Arch: sudo pacman -S gtk4 webkitgtk-6.0 xdg-utils ca-certificates hicolor-icon-theme xdg-desktop-portal
See payload/share/doc/sadptools/runtime-requirements.json for this build's exact
minimums. Older distributions may require an OS upgrade or a separate source build.
EOF
    exit 1
fi

# No privileged operations are invoked internally. Stage before replacing the
# owned app directory, so upgrades can replace even an open executable safely.
umask 022
mkdir -p -- "$prefix/lib"
work=$(mktemp -d "$prefix/lib/.sadptools-install.XXXXXXXX")
cleanup() {
    if [[ -d $work/previous && ! -e $app_dir ]]; then
        mv -- "$work/previous" "$app_dir"
    fi
    rm -rf -- "$work"
}
trap cleanup EXIT
cp -R -- "$bundle/payload" "$work/new"
cp -- "$bundle/install.sh" "$work/new/install.sh"
chmod 755 "$work/new/install.sh" "$work/new/bin/SADPTools" "$work/new/bin/sadptools"
printf '%s\n' "$expected_marker" > "$work/new/.sadptools-manual-install"

# Desktop Exec values have both string escaping and command-line escaping.
# Percent signs in executable paths are rejected above: GIO checks executable
# availability before decoding %% and otherwise rejects a valid-looking entry.
desktop_path="$app_dir/bin/SADPTools"
desktop_path=${desktop_path//\\/\\\\}
desktop_path=${desktop_path//\"/\\\"}
desktop_path=${desktop_path//\$/\\\$}
desktop_path=${desktop_path//\`/\\\`}
desktop_path=${desktop_path//%/%%}
desktop_path=${desktop_path//\\/\\\\}
desktop_file="$work/new/share/applications/$app_id.desktop"
while IFS= read -r line; do
    case "$line" in
        Exec=*) printf 'Exec="%s"\n' "$desktop_path" ;;
        *) printf '%s\n' "$line" ;;
    esac
done < "$bundle/payload/share/applications/$app_id.desktop" > "$desktop_file"

# Store exact locations so removal still works after XDG environment changes.
{
    printf '#!/usr/bin/env bash\n'
    printf 'export XDG_DATA_HOME=%q\n' "$data_root"
    printf 'exec bash %q --%s --prefix %q --uninstall\n' "$app_dir/install.sh" "$scope" "$prefix"
} > "$work/new/uninstall.sh"
# --prefix normally selects prefix/share; explicit stored data_root is needed
# by the uninstall helper if XDG_DATA_HOME was used at original installation.
if [[ $data_root != "$prefix/share" ]]; then
    # Keep the original default-prefix form so XDG_DATA_HOME is honoured.
    printf '#!/usr/bin/env bash\nexport XDG_DATA_HOME=%q\nexec bash %q --user --uninstall\n' \
        "$data_root" "$app_dir/install.sh" > "$work/new/uninstall.sh"
fi
chmod 755 "$work/new/uninstall.sh"
for target in "${targets[@]}"; do mkdir -p -- "$(dirname -- "$target")"; done
if [[ -d $app_dir ]]; then mv -- "$app_dir" "$work/previous"; fi
mv -- "$work/new" "$app_dir"
for i in "${!targets[@]}"; do ln -sfn -- "${sources[i]}" "${targets[i]}"; done
refresh_desktop
printf 'SADPTools installed. Open SADPTools from your application menu.\n'
printf 'CLI: %s/bin/sadptools\nUninstall: %s/uninstall.sh\n' "$prefix" "$app_dir"
if [[ $scope == user && :$PATH: != *":$prefix/bin:"* ]]; then
    printf 'Add %s/bin to your terminal PATH if desired; the menu launcher already works.\n' "$prefix"
fi

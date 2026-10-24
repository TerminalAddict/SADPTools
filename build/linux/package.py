#!/usr/bin/env python3
"""Package existing, version-stamped Linux AMD64 binaries without installing them.

Uses the nFPM bundled into the pinned Wails CLI, not system rpm/rpmbuild.
Debian symbol metadata supplies minimum library versions for both formats.
"""
import argparse
import hashlib
import json
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile

BUILD = Path(__file__).resolve().parents[1]
ROOT = BUILD.parent
sys.path.insert(0, str(BUILD))
from release_version import windows_version

WAILS_VERSION = "v3.0.0-beta.34"
APP_ID = "io.github.terminaladdict.sadptools"
# Names for the supported Fedora RPM stack. Unknown ELF dependencies fail the
# build instead of silently producing packages with incomplete requirements.
RPM_NAMES = {
    "libc6": "glibc", "libglib2.0-0t64": "glib2", "libgtk-4-1": "gtk4",
    "libjavascriptcoregtk-6.0-1": "javascriptcoregtk6.0",
    "libsoup-3.0-0": "libsoup3", "libwebkitgtk-6.0-4": "webkitgtk6.0",
    "libx11-6": "libX11",
}
# Round development-version symbol entries to their first stable API release.
MINIMUMS = {"libgtk-4-1": "4.14", "libwebkitgtk-6.0-4": "2.40",
            "libjavascriptcoregtk-6.0-1": "2.40", "libsoup-3.0-0": "3.0"}


def run(*args, **kwargs):
    return subprocess.run([str(a) for a in args], check=True, text=True, **kwargs)


def asset_names(version):
    return {f"SADPTools-{version}-linux-amd64.deb",
            f"SADPTools-{version}-linux-x86_64.rpm",
            f"SADPTools-{version}-linux-amd64.tar.gz"}


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def verify_binary(path, version):
    data = path.read_bytes()
    if data[:6] != b"\x7fELF\x02\x01" or int.from_bytes(data[18:20], "little") != 62:
        raise ValueError(f"Expected a Linux AMD64 ELF executable: {path}")
    if b"\0" + version.encode() + b"\0" not in data:
        raise ValueError(f"{path} is not stamped {version}; rebuild both Linux binaries")


def dependency_lists(value):
    deb, rpm = [], []
    for dependency in value.split(", "):
        match = re.fullmatch(r"([a-z0-9.+-]+)(?: \(>= ([0-9.]+)\))?", dependency)
        if not match or match[1] not in RPM_NAMES:
            raise ValueError(f"Review the Linux runtime mapping for: {dependency}")
        name, minimum = match.groups()
        floor = MINIMUMS.get(name)
        if floor and (not minimum or tuple(map(int, floor.split("."))) > tuple(map(int, minimum.split(".")))):
            minimum = floor
        deb.append(name + (f" (>= {minimum})" if minimum else ""))
        rpm.append(RPM_NAMES[name] + (f" >= {minimum}" if minimum else ""))
    for name in ("ca-certificates", "xdg-utils", "hicolor-icon-theme"):
        deb.append(name)
        rpm.append(name)
    return {"deb": deb, "rpm": rpm}


def dependencies(binary, work):
    control = work / "debian/control"
    control.parent.mkdir()
    control.write_text("Source: sadptools\nSection: net\nPriority: optional\n"
                       "Maintainer: Paul Willard <paul@paulwillard.nz>\n\n"
                       "Package: sadptools\nArchitecture: amd64\n"
                       "Description: CCTV discovery and technician toolkit\n")
    result = run("dpkg-shlibdeps", "-O", f"-e{binary}", cwd=work, capture_output=True)
    prefix = "shlibs:Depends="
    for line in result.stdout.splitlines():
        if line.startswith(prefix):
            return dependency_lists(line[len(prefix):])
    raise ValueError("dpkg-shlibdeps did not report runtime dependencies")


def copy(source, destination, mode=0o644):
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, destination)
    destination.chmod(mode)


def stage_payload(stage, version):
    copy(ROOT / "bin/SADPTools-linux-amd64", stage / "bin/SADPTools", 0o755)
    copy(ROOT / "bin/sadptools", stage / "bin/sadptools", 0o755)
    copy(BUILD / f"linux/{APP_ID}.desktop", stage / f"share/applications/{APP_ID}.desktop")
    copy(BUILD / "appicon.svg", stage / f"share/icons/hicolor/scalable/apps/{APP_ID}.svg")
    copy(BUILD / "appicon.png", stage / f"share/icons/hicolor/512x512/apps/{APP_ID}.png")
    docs = stage / "share/doc/sadptools"
    for filename in ("LICENSE", "THIRD_PARTY_NOTICES.md"):
        copy(ROOT / filename, docs / filename)
    for path in sorted((ROOT / "LICENSES").rglob("*")):
        if path.is_file():
            copy(path, docs / "LICENSES" / path.relative_to(ROOT / "LICENSES"))
    for filename in ("linux-packaging.md", "device-management.md"):
        copy(ROOT / "docs" / filename, docs / filename)
    (docs / "version.txt").write_text(version + "\n")


def package_config(payload, version, deps, kind):
    return {
        "name": "sadptools", "arch": "amd64", "platform": "linux",
        "version": version, "version_schema": "semver", "release": "1",
        "section": "net", "priority": "optional", "license": "MIT",
        "maintainer": "Paul Willard <paul@paulwillard.nz>", "vendor": "Paul Willard",
        "homepage": "https://github.com/TerminalAddict/SADPTools",
        "description": "CCTV discovery and technician toolkit\n"
                       "Desktop application and CLI for camera discovery, saved sites,\n"
                       "configuration, audits and switch diagnostics.",
        "depends": deps[kind],
        "recommends": ["xdg-desktop-portal", "desktop-file-utils"],
        "contents": [{"src": str(path), "dst": "/usr/" + str(path.relative_to(payload)),
                      "file_info": {"mode": path.stat().st_mode & 0o777, "owner": "root", "group": "root"}}
                     for path in sorted(payload.rglob("*")) if path.is_file()],
        "scripts": {key: str(BUILD / "linux/refresh-desktop.sh")
                    for key in ("postinstall", "postremove")},
        "rpm": {"compression": "gzip"},
    }


def manual_archive(output, payload, version, work):
    name = f"SADPTools-{version}-linux-amd64"
    stage = work / name
    stage.mkdir()
    shutil.copytree(payload, stage / "payload")
    copy(BUILD / "linux/install.sh", stage / "install.sh", 0o755)
    (stage / "README.txt").write_text(
        f"SADPTools {version} — Linux AMD64 desktop and CLI\n\n"
        "Run ./install.sh for a per-user installation (no sudo).\n"
        "Or run sudo ./install.sh --system to install for all users.\n"
        "Then open SADPTools from your desktop's application menu.\n"
        "The installer checks the payload checksums and runtime libraries first.\n"
        "It never installs system dependencies or launches the application as root.\n"
        "Use ./install.sh --uninstall to remove a per-user installation;\n"
        "use sudo ./install.sh --system --uninstall for an all-users installation.\n"
        "Saved sites, credentials and preferences are preserved.\n\n"
        "See payload/share/doc/sadptools/linux-packaging.md for dependencies,\n"
        "custom prefixes, upgrade/removal, distro support and limitations.\n")
    (stage / "SHA256SUMS").write_text("".join(
        f"{digest(p)}  {p.relative_to(stage)}\n" for p in sorted(stage.rglob("*")) if p.is_file()))
    # Root ownership is archive metadata only; no elevated build privileges.
    def attributes(info):
        info.uid = info.gid = 0
        info.uname = info.gname = "root"
        info.mode = 0o755 if info.isdir() or info.name.endswith(("/install.sh", "/bin/SADPTools", "/bin/sadptools")) else 0o644
        return info
    with tarfile.open(output / f"{name}.tar.gz", "w:gz") as archive:
        archive.add(stage, arcname=name, filter=attributes)


def build(version, output, wails):
    windows_version(version)
    if platform.system() != "Linux" or platform.machine() not in ("x86_64", "amd64"):
        raise ValueError("Package on a Debian/Ubuntu AMD64 build host; see docs/linux-packaging.md")
    if not shutil.which("dpkg-shlibdeps"):
        raise ValueError("Missing dpkg-shlibdeps. Build prerequisite: sudo apt install dpkg-dev")
    result = run(wails, "version", capture_output=True)
    if (result.stdout + result.stderr).strip() != WAILS_VERSION:
        raise ValueError("Run make tools to install the pinned Wails CLI")
    for binary in (ROOT / "bin/SADPTools-linux-amd64", ROOT / "bin/sadptools"):
        verify_binary(binary, version)
    output.mkdir(parents=True, exist_ok=True)
    # Only write final output after all three formats have built successfully.
    with tempfile.TemporaryDirectory(prefix="sadptools-linux-") as temporary:
        work = Path(temporary)
        payload, packages = work / "payload", work / "packages"
        packages.mkdir()
        stage_payload(payload, version)
        deps = dependencies(payload / "bin/SADPTools", work)
        (payload / "share/doc/sadptools/runtime-requirements.json").write_text(json.dumps(deps, indent=2) + "\n")
        for kind, arch in (("deb", "amd64"), ("rpm", "x86_64")):
            config = work / f"nfpm-{kind}.json"
            config.write_text(json.dumps(package_config(payload, version, deps, kind), indent=2))
            run(wails, "tool", "package", "-format", kind, "-config", config,
                "-name", f"SADPTools-{version}-linux-{arch}", "-out", packages)
        manual_archive(packages, payload, version, work)
        for name in sorted(asset_names(version)):
            copy(packages / name, output / name)
        (output / "SHA256SUMS").write_text("".join(
            f"{digest(output / name)}  {name}\n" for name in sorted(asset_names(version))))
    print(f"Linux installers ready: {output}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--output", type=Path, default=ROOT / "bin/linux")
    parser.add_argument("--wails", default=shutil.which("wails3") or "wails3")
    args = parser.parse_args()
    build(args.version, args.output.resolve(), args.wails)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        raise SystemExit(f"Linux packaging failed: {error}")

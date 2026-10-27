#!/usr/bin/env python3
"""Build a release on Linux AMD64, or publish its verified files with gh.

No GitHub changes happen during build. Publish pushes main and an annotated tag,
uploads to a draft, downloads and verifies every asset, then publishes it.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import tempfile
import zipfile

from release_version import windows_version
from linux.package import asset_names as linux_asset_names
from darwin.common import asset_names as macos_asset_names
from darwin.package import verify_import as verify_macos_import
from darwin.package import verify_payloads as verify_macos_payloads

ROOT = Path(__file__).resolve().parents[1]
REPO = "TerminalAddict/SADPTools"
VERSION_SYMBOL = "github.com/TerminalAddict/SADPTools/internal/version.Value"


def run(*args, capture=False, env=None):
    result = subprocess.run([str(arg) for arg in args], cwd=ROOT, env=env,
                            check=True, text=True, stdout=subprocess.PIPE if capture else None)
    return result.stdout.strip() if capture else None


def gh(*args, capture=False):
    return run("gh", *args, "--repo", REPO, capture=capture)


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def verify_binary_version(binary, version):
    # -trimpath deliberately omits ldflags from `go version -m`. The linker
    # stores the -X string separately with NUL padding; PE resource strings are
    # UTF-16 and cannot accidentally satisfy this UTF-8 Go-string check.
    if b"\0" + version.encode() + b"\0" not in binary.read_bytes():
        raise ValueError(f"Missing release version in {binary}")


def clean_commit():
    if run("git", "status", "--porcelain", "--untracked-files=all", capture=True):
        raise ValueError("Commit all source changes before building or publishing a release")
    return run("git", "rev-parse", "HEAD", capture=True)


def verify_publish_branch(commit):
    branch = run("git", "branch", "--show-current", capture=True)
    # A detached worktree allows a clean release without moving or including
    # unrelated local work. It must identify main's exact current commit.
    if branch == "main":
        return
    if branch == "" and run("git", "rev-parse", "refs/heads/main", capture=True) == commit:
        return
    raise ValueError("Publish from main or a clean detached worktree at main's current commit")


def asset_names(version, *, standalone_desktop=True, linux_installers=False, macos_installers=False):
    names = {f"SADPTools-{version}-Setup-windows-amd64.exe",
             f"sadptools-{version}-windows-amd64.zip",
             f"sadptools-{version}-linux-amd64.tar.gz"}
    if standalone_desktop:
        names.add(f"SADPTools-{version}-standalone-windows-amd64.zip")
    if linux_installers:
        names.update(linux_asset_names(version))
    if macos_installers:
        names.update(macos_asset_names(version))
    return names


def checksum_text(folder, names):
    return "".join(f"{digest(folder / name)}  {name}\n" for name in sorted(names))


def verify_bundle(folder, version, commit):
    manifest = json.loads((folder / "build-manifest.json").read_text())
    # Older releases predate the standalone desktop download. Keep their
    # immutable bundles verifiable without changing or replacing public files.
    names = asset_names(version, standalone_desktop=manifest.get("standalone_desktop", False),
                        linux_installers=manifest.get("linux_installers", False),
                        macos_installers=manifest.get("macos_installers", False))
    expected = names | {"build-manifest.json", "SHA256SUMS"}
    if {p.name for p in folder.iterdir()} != expected:
        raise ValueError("Release directory has missing or unexpected files")
    if any(not p.is_file() or p.is_symlink() for p in folder.iterdir()):
        raise ValueError("Release assets must be regular files")
    if manifest["version"] != version or manifest["commit"] != commit:
        raise ValueError("Release files were built from a different version or commit; rebuild")
    if set(manifest["sha256"]) != names:
        raise ValueError("Manifest does not describe the expected release assets")
    for name in names:
        if digest(folder / name) != manifest["sha256"][name]:
            raise ValueError(f"Release checksum mismatch: {name}")
    if (folder / "SHA256SUMS").read_text() != checksum_text(folder, names | {"build-manifest.json"}):
        raise ValueError("SHA256SUMS does not match the release files")
    if manifest.get("macos_installers"):
        verify_macos_payloads(folder, manifest.get("macos_build") or {}, version, commit)
    return manifest


def verify_download(local, downloaded):
    if {p.name for p in local.iterdir()} != {p.name for p in downloaded.iterdir()}:
        raise ValueError("GitHub asset list differs from the verified local release")
    for path in local.iterdir():
        if digest(path) != digest(downloaded / path.name):
            raise ValueError(f"GitHub asset checksum mismatch: {path.name}; draft left unpublished")


def notes_path(version):
    path = ROOT / "docs/releases" / f"v{version}.md"
    if not path.is_file() or not path.read_text().strip():
        raise ValueError(f"Write and commit release notes in {path.relative_to(ROOT)} first")
    return path


def cli_archive(folder, version, target, binary):
    name = f"sadptools-{version}-{target}-amd64"
    with tempfile.TemporaryDirectory(prefix="sadptools-cli-") as temporary:
        stage = Path(temporary)
        executable = "sadptools.exe" if target == "windows" else "sadptools"
        shutil.copy2(binary, stage / executable)
        for notice in ("LICENSE", "THIRD_PARTY_NOTICES.md"):
            shutil.copy2(ROOT / notice, stage / notice)
        shutil.copytree(ROOT / "LICENSES", stage / "LICENSES")
        shutil.copy2(ROOT / "docs/device-management.md", stage / "device-management.md")
        prefix = ".\\" if target == "windows" else "./"
        (stage / "README.txt").write_text(
            f"SADPTools CLI {version} ({target} AMD64)\n\n"
            f"Extract this archive, open a terminal here, then run:\n"
            f"  {prefix}{executable} --version\n  {prefix}{executable} interfaces\n"
            f"  {prefix}{executable} discover --timeout 10s\n\n"
            "The CLI needs no GUI runtime. Windows desktop users should download the Setup EXE.\n"
            "Allow UDP 37020 on your trusted network. See device-management.md before writes.\n"
            f"Release notes: https://github.com/{REPO}/releases/tag/v{version}\n")
        if target == "windows":
            with zipfile.ZipFile(folder / f"{name}.zip", "w", zipfile.ZIP_DEFLATED) as archive:
                for path in sorted(stage.rglob("*")):
                    if path.is_file():
                        archive.write(path, str(Path(name) / path.relative_to(stage)))
        else:
            with tarfile.open(folder / f"{name}.tar.gz", "w:gz") as archive:
                archive.add(stage, arcname=name)


def desktop_archive(folder, version, binary):
    name = f"SADPTools-{version}-standalone-windows-amd64.zip"
    with zipfile.ZipFile(folder / name, "w", zipfile.ZIP_DEFLATED) as archive:
        archive.write(binary, "SADPTools-windows-amd64.exe")
        for notice in ("LICENSE", "THIRD_PARTY_NOTICES.md"):
            archive.write(ROOT / notice, notice)
        for path in sorted((ROOT / "LICENSES").rglob("*")):
            if path.is_file():
                archive.write(path, str(path.relative_to(ROOT)))
        archive.writestr(
            "README.txt",
            f"SADPTools desktop {version} (Windows AMD64)\n\n"
            "Extract this ZIP and run SADPTools-windows-amd64.exe.\n"
            "WebView2 must already be present. Use the offline Setup EXE if needed.\n"
            "This ZIP contains the desktop application; the separate lowercase\n"
            "sadptools archive contains command-line tools.\n\n"
            "Saved sites and credentials live in the current user's SADPTools\n"
            "configuration directory, outside this folder. Use encrypted site\n"
            "backups to move protected credentials between PCs or accounts.\n\n"
            f"Release notes: https://github.com/{REPO}/releases/tag/v{version}\n")


def build(version, folder, macos_dir=None):
    commit = clean_commit()
    notes = notes_path(version)
    if platform.system() != "Linux" or platform.machine() not in ("x86_64", "amd64"):
        raise ValueError("The release builder currently runs on Linux AMD64 (Sparx)")
    if folder.exists():
        raise ValueError(f"{folder} already exists; preserve published builds or remove an unpublished build explicitly")
    # Import that exact commit's native CI artifacts; never silently omit macOS
    # when it was explicitly requested as part of a release.
    macos_manifest = verify_macos_import(macos_dir, version, commit) if macos_dir else None
    for tool in ("go", "npm", "node", "make", "7z", "dpkg-shlibdeps"):
        if not shutil.which(tool):
            raise ValueError(f"Missing build tool: {tool}; see docs/releases.md")
    wails = Path(run("go", "env", "GOPATH", capture=True)) / "bin/wails3"
    # This pinned CLI writes its version to stderr, not stdout.
    wails_result = subprocess.run([str(wails), "version"], check=True, text=True, capture_output=True)
    if (wails_result.stdout + wails_result.stderr).strip() != "v3.0.0-beta.34":
        raise ValueError("Run make tools to install the pinned Wails CLI")
    env = dict(os.environ, SADPTOOLS_VERSION=version, GOAMD64="v1")
    run("make", "check", env=env)
    run("make", "windows-installer", f"VERSION={version}", env=env)
    run("7z", "t", ROOT / "bin/SADPTools-Setup-windows-amd64.exe")
    run("make", "linux-packages", f"VERSION={version}", env=env)
    if run(ROOT / "bin/sadptools", "--version", capture=True) != f"sadptools {version}":
        raise ValueError("Linux CLI does not report the release version")
    # NSIS resolves File inputs case-insensitively even on Linux. Keep the CLI
    # apart from SADPTools-windows-amd64.exe so repeated builds stay unambiguous.
    windows_cli = ROOT / "bin/cli-windows-amd64/sadptools.exe"
    windows_cli.parent.mkdir(parents=True, exist_ok=True)
    run("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", f"-s -w -X {VERSION_SYMBOL}={version}",
        "-o", windows_cli, "./cmd/sadptools",
        env=dict(env, GOOS="windows", GOARCH="amd64", CGO_ENABLED="0"))
    for binary in (ROOT / "bin/sadptools", windows_cli, ROOT / "bin/SADPTools-windows-amd64.exe",
                   ROOT / "bin/SADPTools-linux-amd64"):
        verify_binary_version(binary, version)
    if clean_commit() != commit:
        raise ValueError("Source changed during the release build; rebuild from a clean commit")
    folder.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=folder.parent, prefix="release-") as temporary:
        stage = Path(temporary)
        shutil.copy2(ROOT / "bin/SADPTools-Setup-windows-amd64.exe",
                     stage / f"SADPTools-{version}-Setup-windows-amd64.exe")
        cli_archive(stage, version, "linux", ROOT / "bin/sadptools")
        cli_archive(stage, version, "windows", windows_cli)
        desktop_archive(stage, version, ROOT / "bin/SADPTools-windows-amd64.exe")
        for name in linux_asset_names(version):
            shutil.copy2(ROOT / "bin/linux" / name, stage / name)
        if macos_dir:
            if verify_macos_import(macos_dir, version, commit) != macos_manifest:
                raise ValueError("macOS artifacts changed during the release build")
            for name in macos_asset_names(version):
                shutil.copy2(Path(macos_dir) / name, stage / name)
                if digest(stage / name) != macos_manifest["sha256"][name]:
                    raise ValueError("macOS artifact changed while staging")
        names = asset_names(version, linux_installers=True, macos_installers=bool(macos_dir))
        manifest = {
            "version": version, "tag": f"v{version}", "commit": commit,
            "standalone_desktop": True,
            "linux_installers": True,
            "macos_installers": bool(macos_dir),
            "macos_build": macos_manifest,
            "notes_sha256": digest(notes),
            "toolchain": {"go": run("go", "version", capture=True),
                          "node": run("node", "--version", capture=True),
                          "npm": run("npm", "--version", capture=True),
                          "wails": "v3.0.0-beta.34",
                          "python": platform.python_version(),
                          "installer": json.loads((ROOT / "build/windows/installer-inputs.json").read_text())},
            "sha256": {name: digest(stage / name) for name in sorted(names)},
        }
        (stage / "build-manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
        (stage / "SHA256SUMS").write_text(checksum_text(stage, names | {"build-manifest.json"}))
        verify_bundle(stage, version, commit)
        stage.rename(folder)
    print(f"Release files verified: {folder}\nPublish with: make release-publish VERSION={version}")


def publish(version, folder):
    commit = clean_commit()
    notes = notes_path(version)
    manifest = verify_bundle(folder, version, commit)
    if manifest["notes_sha256"] != digest(notes):
        raise ValueError("Release notes changed since the build")
    verify_publish_branch(commit)
    remote = run("git", "remote", "get-url", "--push", "origin", capture=True)
    if remote not in (f"git@github.com:{REPO}.git", f"https://github.com/{REPO}.git", f"https://github.com/{REPO}"):
        raise ValueError("Origin must point to the SADPTools GitHub repository")
    run("gh", "auth", "status", "--hostname", "github.com")
    tag = f"v{version}"
    if run("git", "tag", "--list", tag, capture=True):
        if run("git", "rev-parse", f"{tag}^{{commit}}", capture=True) != commit:
            raise ValueError("Existing tag points to another commit; choose a new version")
    else:
        run("git", "tag", "-a", tag, "-m", f"SADPTools {tag}", commit)
    # Normal pushes deliberately refuse divergence or replacement of a remote tag.
    run("git", "push", "origin", "HEAD:refs/heads/main")
    run("git", "push", "origin", f"refs/tags/{tag}")
    remote_tag = run("git", "ls-remote", "origin", f"refs/tags/{tag}^{{}}", capture=True).split()
    if not remote_tag or remote_tag[0] != commit:
        raise ValueError("Remote annotated tag does not identify the build commit")
    tags = run("gh", "api", f"repos/{REPO}/releases", "--paginate", "--jq", ".[].tag_name", capture=True).splitlines()
    prerelease = "-" in version
    if tag not in tags:
        gh("release", "create", tag, "--verify-tag", "--draft", f"--prerelease={str(prerelease).lower()}",
           "--latest=false", "--title", f"SADPTools {tag}", "--notes-file", notes)
    state = json.loads(gh("release", "view", tag, "--json", "isDraft,isPrerelease,assets,url", capture=True))
    local = {p.name for p in folder.iterdir()}
    remote_assets = {a["name"] for a in state["assets"]}
    if remote_assets - local:
        raise ValueError("Unexpected GitHub assets; inspect the release before continuing")
    missing = local - remote_assets
    if missing:
        if not state["isDraft"]:
            raise ValueError("Published release is incomplete; it will not be modified automatically")
        gh("release", "upload", tag, *(folder / name for name in sorted(missing)))
    with tempfile.TemporaryDirectory(prefix="sadptools-release-verify-") as temporary:
        gh("release", "download", tag, "--dir", temporary)
        verify_download(folder, Path(temporary))
    if state["isDraft"]:
        gh("release", "edit", tag, "--draft=false", "--verify-tag",
           f"--prerelease={str(prerelease).lower()}", f"--latest={str(not prerelease).lower()}",
           "--notes-file", notes)
    state = json.loads(gh("release", "view", tag, "--json", "isDraft,isPrerelease,url", capture=True))
    if state["isDraft"] or state["isPrerelease"] != prerelease:
        raise ValueError("GitHub release state needs review")
    print(f"Published and verified: {state['url']}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("build", "publish"))
    parser.add_argument("version")
    parser.add_argument("--macos-dir", type=Path, help="Import verified universal ZIP/DMG artifacts from the native macOS CI job")
    args = parser.parse_args()
    windows_version(args.version, release=True)
    folder = ROOT / "bin/releases" / f"v{args.version}"
    if args.action == "build":
        build(args.version, folder, args.macos_dir)
    else:
        if args.macos_dir:
            parser.error("--macos-dir is a build option; publish uses the verified bundle")
        publish(args.version, folder)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, subprocess.SubprocessError) as error:
        raise SystemExit(f"Release failed: {error}") from error

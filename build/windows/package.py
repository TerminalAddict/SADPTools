#!/usr/bin/env python3
"""Build the offline Windows installer; downloads are pinned and never installed.

Python's standard library, NSIS 3.10, and the built Windows app are sufficient.
Ubuntu 24.04+ AMD64 can bootstrap NSIS into the ignored cache without sudo.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
INPUTS = Path(__file__).with_name("installer-inputs.json")


def sha256(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def verify(path, asset):
    if path.stat().st_size != asset["size"] or sha256(path) != asset["sha256"]:
        raise ValueError(f"Integrity check failed for {path}; remove it and retry. "
                         "Do not change the pinned checksum to bypass this check.")


def fetch(asset, cache):
    """Verify cached input or atomically download and verify a new one."""
    name = asset["filename"]
    if Path(name).name != name or not asset["url"].startswith("https://"):
        raise ValueError("Installer inputs must use a plain filename and HTTPS")
    cache.mkdir(parents=True, exist_ok=True)
    destination = cache / name
    if destination.exists():
        verify(destination, asset)
        return destination
    temporary = None
    try:
        print(f"Downloading {name} ({asset['size'] / 1024 / 1024:.1f} MiB)", flush=True)
        with urllib.request.urlopen(asset["url"], timeout=60) as response:
            if not response.url.startswith("https://"):
                raise ValueError("Refusing an insecure download redirect")
            with tempfile.NamedTemporaryFile(dir=cache, suffix=".part", delete=False) as out:
                temporary = Path(out.name)
                size = 0
                while block := response.read(1024 * 1024):
                    size += len(block)
                    if size > asset["size"]:
                        raise ValueError(f"Download exceeds pinned size: {name}")
                    out.write(block)
        verify(temporary, asset)
        temporary.replace(destination)
        return destination
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


def compiler(inputs, cache):
    env = os.environ.copy()
    executable = env.get("MAKENSIS") or shutil.which("makensis")
    if not executable:
        if platform.system() != "Linux" or platform.machine() not in ("x86_64", "amd64"):
            raise RuntimeError("Install NSIS 3.10 and add makensis to PATH, or set MAKENSIS.")
        if not shutil.which("dpkg-deb"):
            raise RuntimeError("Local NSIS setup needs dpkg-deb; alternatively supply NSIS 3.10 on PATH.")
        target = cache / "nsis"
        # Re-extract verified packages, so stale or incomplete tools are repaired.
        for asset in inputs["nsis_linux_amd64"]:
            archive = fetch(asset, cache)
            subprocess.run(["dpkg-deb", "-x", str(archive), str(target)], check=True)
        executable = str(target / "usr/bin/makensis")
        env["NSISDIR"] = str(target / "usr/share/nsis")
    flag = "/VERSION" if os.name == "nt" else "-VERSION"
    version = subprocess.check_output([executable, flag], env=env, text=True).strip()
    if version not in ("v" + inputs["nsis_version"], "v" + inputs["nsis_version"] + "-2"):
        raise RuntimeError(f"Expected NSIS {inputs['nsis_version']}, found {version} at {executable}")
    return executable, env


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prepare", action="store_true", help="Cache and verify build inputs only")
    args = parser.parse_args()
    inputs = json.loads(INPUTS.read_text())
    cache = Path(os.environ.get("SADPTOOLS_BUILD_CACHE", ROOT / ".cache/windows-installer")).resolve()
    runtime = fetch(inputs["webview2"], cache)
    makensis, env = compiler(inputs, cache)
    if args.prepare:
        print(f"Installer inputs ready in {cache}")
        return
    binary = ROOT / "bin/SADPTools-windows-amd64.exe"
    if not binary.is_file():
        raise RuntimeError("Windows application is missing; run make windows-installer.")
    metadata = json.loads((ROOT / "build/windows/info.json").read_text())
    version = metadata["info"]["0409"]["ProductVersion"]
    output = ROOT / "bin/SADPTools-Setup-windows-amd64.exe"
    # Compile to a temporary output; a failed build cannot replace a working installer.
    with tempfile.TemporaryDirectory(dir=output.parent, prefix="installer-") as stage:
        stage = Path(stage)
        terms = stage / "setup-terms.txt"
        terms.write_text(
            "SADPTools is MIT-licensed; its licence and third-party notices are installed with the app.\n\n"
            "Setup includes Microsoft's display component, Microsoft Edge WebView2 Runtime. "
            "It is installed automatically when missing. Microsoft supplies its own updates. "
            "This component includes Microsoft Defender SmartScreen, which collects and sends "
            "information to Microsoft as described at https://aka.ms/privacy and "
            "https://learn.microsoft.com/en-us/microsoft-edge/privacy-whitepaper#smartscreen .\n\n"
            "The following Microsoft terms apply to that component; SADPTools source remains MIT-licensed.\n\n"
            + (ROOT / "LICENSES/Microsoft-WebView2.txt").read_text(), encoding="utf-8-sig")
        pending = stage / output.name
        prefix = "/" if os.name == "nt" else "-"
        defines = {
            "PROJECT_ROOT": ROOT, "APP_BINARY": binary, "WEBVIEW2_INSTALLER": runtime,
            "SETUP_TERMS": terms, "OUTPUT_FILE": pending, "PRODUCT_VERSION": version,
        }
        command = [makensis, prefix + "V3", prefix + "WX"]
        command += [f"{prefix}D{key}={value}" for key, value in defines.items()]
        command.append(str(ROOT / "build/windows/installer.nsi"))
        subprocess.run(command, env=env, check=True, cwd=ROOT)
        pending.replace(output)
    checksum = output.with_suffix(output.suffix + ".sha256")
    checksum.write_text(f"{sha256(output)}  {output.name}\n")
    print(f"Created {output} ({output.stat().st_size / 1024 / 1024:.1f} MiB)")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        raise SystemExit(f"Installer build failed: {error}") from error

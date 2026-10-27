"""Version validation shared by release builds, Windows resources and NSIS."""
import argparse
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]
PATTERN = re.compile(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(dev|beta|rc)(?:\.([1-9][0-9]*))?)?")


def windows_version(value, *, release=False):
    match = PATTERN.fullmatch(value)
    if not match:
        raise ValueError("Use a version such as 0.2.0-beta.1, 0.2.0-rc.1 or 0.2.0 (no leading v)")
    major, minor, patch, channel, sequence = match.groups()
    if any(int(part) > 65535 for part in (major, minor, patch)):
        raise ValueError("Windows version components must be at most 65535")
    if channel in ("beta", "rc") and (sequence is None or int(sequence) > 19999):
        raise ValueError("Beta/RC versions require a sequence from 1 to 19999")
    if channel == "dev" and (sequence or release):
        raise ValueError("Development versions cannot be released")
    revision = 65535 if channel is None else 0
    if channel in ("beta", "rc"):
        revision = int(sequence) + (20000 if channel == "rc" else 0)
    return f"{major}.{minor}.{patch}.{revision}"


def version_stamp(value):
    windows_version(value)
    return f"SADPToolsVersion[{value}]"


def has_version_stamp(data, value):
    # The exact value consumed by version.init, including its frame. Neither
    # Go string padding nor incidental dependency/resource versions prove it.
    if version_stamp(value).encode() in data:
        return True
    # Retain validation of immutable pre-1.0 archives built with the old marker.
    return (tuple(map(int, value.split('-')[0].split('.'))) < (1, 0, 0)
            and b"\0" + value.encode() + b"\0" in data)


def metadata(value):
    numeric = windows_version(value)
    result = json.loads((ROOT / "build/windows/info.json").read_text())
    result["fixed"].update(file_version=numeric, product_version=numeric)
    result["info"]["0409"].update(ProductVersion=value, FileVersion=numeric)
    return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version")
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    data = metadata(args.version)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(data, indent=2) + "\n")

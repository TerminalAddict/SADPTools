"""Shared macOS artifact validation. Standard library only; runs on Linux too."""
import hashlib
import json
from pathlib import Path
import plistlib
import struct
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'build'))
from release_version import windows_version, version_stamp, has_version_stamp

ARCHES = {'arm64': 0x100000c, 'amd64': 0x1000007}
MINIMUM = '12.0'
IDENTIFIER = 'io.github.terminaladdict.sadptools'


def run(*args, capture=False, env=None):
    result = subprocess.run([str(a) for a in args], cwd=ROOT, env=env, check=True,
                            text=True, stdout=subprocess.PIPE if capture else None)
    return result.stdout.strip() if capture else None


def digest(path):
    with Path(path).open('rb') as f:
        return hashlib.file_digest(f, 'sha256').hexdigest()


def source_state():
    return {'commit': run('git', 'rev-parse', 'HEAD', capture=True),
            'dirty': bool(run('git', 'status', '--porcelain', '--untracked-files=all', capture=True))}


def architectures(arch):
    if arch == 'universal':
        return set(ARCHES)
    if arch not in ARCHES:
        raise ValueError('macOS architecture must be arm64, amd64 or universal')
    return {arch}


def asset_names(version, arch='universal', dmg=True):
    windows_version(version)
    architectures(arch)
    stem = f'SADPTools-{version}-macos-{arch}'
    return {stem + '.zip', stem + '.dmg'} if dmg else {stem + '.zip'}


def macho_slices(data):
    """Reject malformed/wrong-platform binaries, including misleading fat headers."""
    if len(data) < 32:
        raise ValueError('Truncated Mach-O executable')
    if data[:4] == b'\xcf\xfa\xed\xfe':
        cpu, _, kind = struct.unpack_from('<III', data, 4)
        if cpu not in ARCHES.values() or kind != 2:  # MH_EXECUTE
            raise ValueError('Expected an ARM64 or AMD64 Mach-O executable')
        return {next(a for a, c in ARCHES.items() if c == cpu): data}
    if data[:4] != b'\xca\xfe\xba\xbe':
        raise ValueError('Expected Mach-O, not a Windows/Linux executable')
    count = struct.unpack_from('>I', data, 4)[0]
    if count != 2 or len(data) < 8 + count * 20:
        raise ValueError('Universal Mach-O must contain exactly two architectures')
    result, spans = {}, []
    for i in range(count):
        cpu, _, offset, size, alignment = struct.unpack_from('>IIIII', data, 8 + i * 20)
        if offset < 8 + count * 20 or size < 32 or offset + size > len(data) or alignment > 30 or offset % (1 << alignment):
            raise ValueError('Invalid universal Mach-O slice bounds')
        if any(offset < end and start < offset + size for start, end in spans):
            raise ValueError('Overlapping universal Mach-O slices')
        spans.append((offset, offset + size))
        part = data[offset:offset + size]
        if part[:4] != b'\xcf\xfa\xed\xfe':
            raise ValueError('Invalid universal Mach-O slice')
        slices = macho_slices(part)
        arch = next(iter(slices))
        if ARCHES[arch] != cpu or arch in result:
            raise ValueError('Duplicate or mismatched Mach-O architecture')
        result.update(slices)
    return result


def verify_binary(data, arch, version):
    slices = macho_slices(data)
    if set(slices) != architectures(arch):
        raise ValueError('Mach-O architectures do not match the requested build')
    for value in slices.values():
        if not has_version_stamp(value, version):
            raise ValueError('Missing embedded release version in Mach-O slice')


def info_plist(version):
    windows_version(version)
    value = plistlib.loads((ROOT / 'build/darwin/Info.plist').read_bytes())
    # Finder uses numeric marketing/build versions. Keep the complete semantic
    # prerelease version separately and in Go/About; never order RCs by plist.
    value['CFBundleShortVersionString'] = version.split('-')[0]
    value['CFBundleVersion'] = version.split('-')[0]
    value['SADPToolsVersion'] = version
    value['CFBundleGetInfoString'] = f'SADPTools {version}'
    return value


def checksums(folder, names):
    return ''.join(f'{digest(folder / n)}  {n}\n' for n in sorted(names))


def write_json(path, value):
    path.write_text(json.dumps(value, indent=2) + '\n')

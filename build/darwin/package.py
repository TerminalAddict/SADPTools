#!/usr/bin/env python3
"""Package a verified macOS binary as .app/ZIP; DMG creation requires macOS."""
import argparse
import json
from pathlib import Path, PurePosixPath
import platform
import plistlib
import shutil
import stat
import subprocess
import tempfile
import zipfile
if __package__:
    from .common import ROOT, IDENTIFIER, MINIMUM, architectures, asset_names, checksums, digest, info_plist, run, source_state, verify_binary, write_json
else:
    from common import ROOT, IDENTIFIER, MINIMUM, architectures, asset_names, checksums, digest, info_plist, run, source_state, verify_binary, write_json

MANIFEST = 'macos-build-manifest.json'


def verify_zip(path, version, arch):
    with zipfile.ZipFile(path) as z:
        names = z.namelist()
        if len(names) != len(set(names)) or sum(i.file_size for i in z.infolist()) > 512 << 20:
            raise ValueError('Duplicate or oversized macOS ZIP contents')
        for i in z.infolist():
            p = PurePosixPath(i.filename)
            if p.is_absolute() or '..' in p.parts or '\\' in i.filename or not p.parts or p.parts[0] != 'SADPTools.app':
                raise ValueError('Unsafe macOS ZIP path')
            mode = i.external_attr >> 16
            if stat.S_ISLNK(mode) or (stat.S_IFMT(mode) not in (0, stat.S_IFREG, stat.S_IFDIR)):
                raise ValueError('macOS ZIP contains a link or special file')
        prefix = 'SADPTools.app/Contents/'
        info = plistlib.loads(z.read(prefix + 'Info.plist'))
        expected = info_plist(version)
        for k in ('CFBundleIdentifier', 'CFBundleExecutable', 'CFBundleVersion',
                  'CFBundleShortVersionString', 'SADPToolsVersion', 'LSMinimumSystemVersion',
                  'NSLocalNetworkUsageDescription', 'CFBundleIconFile', 'CFBundlePackageType'):
            if info.get(k) != expected[k]:
                raise ValueError(f'Unexpected macOS bundle metadata: {k}')
        executable = prefix + 'MacOS/SADPTools'
        if not (z.getinfo(executable).external_attr >> 16) & 0o111:
            raise ValueError('macOS executable permission was lost')
        verify_binary(z.read(executable), arch, version)
        icon = z.read(prefix + 'Resources/icons.icns')
        if icon[:4] != b'icns' or int.from_bytes(icon[4:8], 'big') != len(icon):
            raise ValueError('Invalid macOS icon')
        for n in ('LICENSE', 'THIRD_PARTY_NOTICES.md'):
            if not z.read(prefix + 'Resources/' + n):
                raise ValueError('Missing macOS licence notices')
        if not any(n.startswith(prefix + 'Resources/LICENSES/') for n in names):
            raise ValueError('Missing bundled third-party licences')
        if z.testzip() is not None:
            raise ValueError('Corrupt macOS ZIP')


def verify_import(folder, version, commit):
    """Validate native CI artifacts before adding them to a Linux release bundle.

    Checksums prove integrity against this manifest, not publisher authenticity.
    Only import artifacts obtained from this repository's trusted native CI job.
    """
    folder = Path(folder)
    manifest = json.loads((folder / MANIFEST).read_text())
    names = asset_names(version)
    expected = names | {MANIFEST, 'SHA256SUMS'}
    if {p.name for p in folder.iterdir()} != expected or any(p.is_symlink() or not p.is_file() for p in folder.iterdir()):
        raise ValueError('macOS import requires exactly ZIP, DMG, manifest and SHA256SUMS regular files')
    verify_payloads(folder, manifest, version, commit)
    if (folder / 'SHA256SUMS').read_text() != checksums(folder, names | {MANIFEST}):
        raise ValueError('macOS SHA256SUMS mismatch')
    return manifest


def verify_payloads(folder, manifest, version, commit):
    """Reused during release publish, including after copying Mac artifacts."""
    names = asset_names(version)
    if manifest.get('version') != version or manifest.get('commit') != commit or manifest.get('dirty') is not False:
        raise ValueError('macOS artifacts are from a different version/commit or a dirty checkout')
    if manifest.get('arch') != 'universal' or manifest.get('architectures') != ['amd64', 'arm64']:
        raise ValueError('Release requires a universal macOS app')
    if manifest.get('host') != 'Darwin' or manifest.get('signing') not in ('adhoc', 'developer-id'):
        raise ValueError('Release requires macOS bundle signature verification on a native Mac')
    if set(manifest.get('sha256', {})) != names:
        raise ValueError('macOS manifest has missing or unexpected assets')
    for name in names:
        if digest(folder / name) != manifest['sha256'][name]:
            raise ValueError(f'macOS checksum mismatch: {name}')
    verify_zip(folder / next(n for n in names if n.endswith('.zip')), version, 'universal')
    dmg = folder / next(n for n in names if n.endswith('.dmg'))
    with dmg.open('rb') as f:
        if dmg.stat().st_size < 512:
            raise ValueError('Truncated DMG')
        f.seek(-512, 2)
        if f.read(4) != b'koly':
            raise ValueError('Expected a native UDIF DMG')


def create_bundle(binary, bundle, version):
    resources = bundle / 'Contents/Resources'
    executable = bundle / 'Contents/MacOS/SADPTools'
    resources.mkdir(parents=True)
    executable.parent.mkdir()
    shutil.copy2(binary, executable)
    executable.chmod(0o755)
    (bundle / 'Contents/Info.plist').write_bytes(plistlib.dumps(info_plist(version)))
    shutil.copy2(ROOT / 'build/darwin/icons.icns', resources)
    for name in ('LICENSE', 'THIRD_PARTY_NOTICES.md'):
        shutil.copy2(ROOT / name, resources / name)
    shutil.copytree(ROOT / 'LICENSES', resources / 'LICENSES')
    shutil.copy2(ROOT / 'docs/macos.md', resources / 'macOS-installation.md')


def archive_bundle(bundle, output):
    with zipfile.ZipFile(output, 'w', zipfile.ZIP_DEFLATED) as z:
        for p in sorted(bundle.rglob('*')):
            if p.is_symlink():
                raise ValueError('Unexpected link inside app bundle')
            if p.is_file():
                z.write(p, p.relative_to(bundle.parent).as_posix())


def package(version, arch, dmg=False):
    names = asset_names(version, arch, dmg)
    native = platform.system() == 'Darwin'
    if dmg and not native:
        raise ValueError('DMGs require hdiutil on macOS. The cross-built .app/ZIP remains available; use the native CI artifacts for releases.')
    output = ROOT / 'bin/macos'
    receipt = json.loads((output / f'build-{arch}.json').read_text())
    binary = output / f'SADPTools-darwin-{arch}'
    before = source_state()
    if receipt.get('version') != version or receipt.get('arch') != arch or any(receipt.get(k) != v for k, v in before.items()):
        raise ValueError('macOS binary provenance is stale; rebuild with make macos')
    if digest(binary) != receipt['sha256']:
        raise ValueError('macOS binary changed since compilation')
    verify_binary(binary.read_bytes(), arch, version)
    with tempfile.TemporaryDirectory(dir=output, prefix='package-') as directory:
        stage = Path(directory)
        bundle = stage / 'SADPTools.app'
        create_bundle(binary, bundle, version)
        signing = 'unsigned'
        if native:
            # Local/CI testing only: ad-hoc is NOT Developer ID or notarization.
            run('codesign', '--force', '--sign', '-', bundle)
            run('codesign', '--verify', '--deep', '--strict', bundle)
            run('plutil', '-lint', bundle / 'Contents/Info.plist')
            signing = 'adhoc'
        assets = stage / 'artifacts'
        assets.mkdir()
        zip_path = assets / next(n for n in names if n.endswith('.zip'))
        archive_bundle(bundle, zip_path)
        verify_zip(zip_path, version, arch)
        if dmg:
            with tempfile.TemporaryDirectory(dir=stage, prefix='volume-') as volume_name:
                volume = Path(volume_name)
                # ditto preserves the verified signed bundle metadata.
                run('ditto', bundle, volume / bundle.name)
                (volume / 'Applications').symlink_to('/Applications')
                dmg_path = assets / next(n for n in names if n.endswith('.dmg'))
                run('hdiutil', 'create', '-volname', 'SADPTools', '-srcfolder', volume,
                    '-format', 'UDZO', '-fs', 'HFS+', dmg_path)
                run('hdiutil', 'verify', dmg_path)
                # Validate the actual mounted contents, not just the source app.
                mount = stage / 'mounted'
                mount.mkdir()
                run('hdiutil', 'attach', '-readonly', '-nobrowse', '-mountpoint', mount, dmg_path)
                try:
                    run('codesign', '--verify', '--deep', '--strict', mount / bundle.name)
                    if (mount / bundle.name / 'Contents/MacOS/SADPTools').read_bytes() != (bundle / 'Contents/MacOS/SADPTools').read_bytes():
                        raise ValueError('DMG app differs from ZIP app')
                    if (mount / 'Applications').readlink() != Path('/Applications'):
                        raise ValueError('DMG Applications link is incorrect')
                finally:
                    run('hdiutil', 'detach', mount)
        if source_state() != before:
            raise ValueError('Source state changed while packaging; rebuild')
        manifest = dict(before, version=version, arch=arch, architectures=sorted(architectures(arch)),
                        host=platform.system(), signing=signing, notarized=False,
                        toolchain=receipt['toolchain'], minimum_macos=MINIMUM,
                        bundle_identifier=IDENTIFIER,
                        sha256={name: digest(assets / name) for name in sorted(names)})
        write_json(assets / MANIFEST, manifest)
        (assets / 'SHA256SUMS').write_text(checksums(assets, names | {MANIFEST}))
        if dmg and arch == 'universal' and not before['dirty']:
            verify_import(assets, version, before['commit'])
        # Replace only generated output for this architecture, never release bundles.
        destination = output / arch
        if destination.exists():
            shutil.rmtree(destination)
        destination.mkdir()
        shutil.move(bundle, destination / bundle.name)
        shutil.move(assets, destination / 'artifacts')
    print(f'Packaged {destination}/SADPTools.app ({signing}, not notarized)')
    print(f'Archives: {destination}/artifacts')


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--version', required=True)
    p.add_argument('--arch', choices=['arm64', 'amd64', 'universal'], default='universal')
    p.add_argument('--dmg', action='store_true')
    a = p.parse_args()
    try:
        package(a.version, a.arch, a.dmg)
    except (OSError, ValueError, KeyError, subprocess.SubprocessError) as e:
        raise SystemExit(f'macOS packaging failed: {e}') from e

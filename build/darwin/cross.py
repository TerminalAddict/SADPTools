#!/usr/bin/env python3
"""Wails v3 macOS build: native Apple clang or a pinned Docker/Zig toolchain."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
from common import ROOT, MINIMUM, architectures, digest, run, source_state, verify_binary, windows_version, version_stamp, write_json

IMAGE = 'sadptools-macos:go1.26.10-zig0.14.0-sdk14.5-v1'
SDK = 'MacOSX14.5.sdk.tar.xz'
CACHE = ROOT / '.cache/macos'


def recipe_hash():
    return hashlib.sha256(b''.join((ROOT / 'build/darwin' / n).read_bytes() for n in ('Dockerfile', 'zig-cc.py'))).hexdigest()


def docker_ready(image=True):
    if not shutil.which('docker'):
        raise ValueError('Docker is required for Linux → macOS builds; see docs/macos.md')
    result = subprocess.run(['docker', 'info'], capture_output=True, text=True)
    if result.returncode:
        raise ValueError('Docker daemon is unavailable to this user. Check docker info and socket permissions; see docs/macos.md. No elevation was attempted.')
    if image:
        inspection = subprocess.run(['docker', 'image', 'inspect', IMAGE], capture_output=True, text=True)
        if inspection.returncode:
            raise ValueError(f'Missing {IMAGE}. Supply the macOS 14.5 SDK, then run make macos-docker; see docs/macos.md')
        details = json.loads(inspection.stdout)[0]
        labels = details.get('Config', {}).get('Labels') or {}
        if labels.get('io.sadptools.recipe-sha256') != recipe_hash() or len(labels.get('io.sadptools.sdk-sha256', '')) != 64:
            raise ValueError('macOS Docker image is stale or missing provenance; rerun make macos-docker')
        return details['Id']


def setup():
    docker_ready(False)
    sdk = Path(os.environ.get('MACOS_SDK_ARCHIVE', ROOT / '.cache/macos-sdk' / SDK)).resolve()
    expected = os.environ.get('MACOS_SDK_SHA256', '')
    if len(expected) != 64 or any(c not in '0123456789abcdefABCDEF' for c in expected):
        raise ValueError('Set MACOS_SDK_SHA256 to the independently verified SDK archive SHA-256; see docs/macos.md')
    if not sdk.is_file() or digest(sdk) != expected.lower():
        raise ValueError('Missing SDK or SDK checksum mismatch; set MACOS_SDK_ARCHIVE to your MacOSX14.5.sdk.tar.xz')
    # Send ONLY toolchain scripts and the SDK to Docker, never the checkout,
    # credentials or unrelated ignored files.
    with tempfile.TemporaryDirectory(prefix='sadptools-macos-docker-') as directory:
        context = Path(directory)
        for name in ('Dockerfile', 'zig-cc.py'):
            shutil.copy2(ROOT / 'build/darwin' / name, context / name)
        shutil.copy2(sdk, context / SDK)
        run('docker', 'build', '--platform', 'linux/amd64', '--build-arg', f'SDK_SHA256={expected.lower()}',
            '--label', f'io.sadptools.sdk-sha256={expected.lower()}',
            '--label', f'io.sadptools.recipe-sha256={recipe_hash()}', '-t', IMAGE, context)
    print(f'Prepared {IMAGE}; the private SDK/image must not be published.')


def compiler_environment(arch):
    return dict(os.environ, GOOS='darwin', GOARCH=arch, CGO_ENABLED='1',
                CC='clang', CXX='clang++', MACOSX_DEPLOYMENT_TARGET=MINIMUM,
                CGO_CFLAGS=f'-mmacosx-version-min={MINIMUM}',
                CGO_LDFLAGS=f'-mmacosx-version-min={MINIMUM}')


def stage_sources(destination):
    # A repository mount would expose ignored web credentials/SDKs/caches.
    # Copy only non-ignored Go sources/resources and generated frontend assets.
    paths = run('git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard', capture=True).split('\0')
    for name in paths:
        path = Path(name)
        if not (name in ('go.mod', 'go.sum', 'build/appicon.png') or
                (len(path.parts) == 1 and path.suffix == '.go') or
                name.startswith(('internal/', 'cmd/'))):
            continue
        source = ROOT / path
        if source.is_symlink():
            raise ValueError('Build sources must not contain symlinks')
        if source.is_file():
            target = destination / path
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, target)
    frontend = ROOT / 'frontend/dist'
    if not (frontend / 'index.html').is_file() or any(p.is_symlink() for p in frontend.rglob('*')):
        raise ValueError('Run make frontend first; generated frontend must contain no symlinks')
    shutil.copytree(frontend, destination / 'frontend/dist')


def build(arch, version):
    windows_version(version)
    requested = architectures(arch)
    native = platform.system() == 'Darwin'
    if native:
        run('xcrun', '--sdk', 'macosx', '--find', 'clang', capture=True)
    else:
        image_id = docker_ready()
    before = source_state()
    output = ROOT / 'bin/macos'
    output.mkdir(parents=True, exist_ok=True)
    cache = CACHE / 'go'
    cache.mkdir(parents=True, exist_ok=True)
    flags = ['-tags', 'desktop,production', '-trimpath', '-buildvcs=false', '-ldflags',
             f'-s -w -X github.com/TerminalAddict/SADPTools/internal/version.Value={version_stamp(version)}']
    # A failed rebuild invalidates provenance, so packaging cannot use stale files.
    receipt = output / f'build-{arch}.json'
    receipt.unlink(missing_ok=True)
    with tempfile.TemporaryDirectory(dir=output, prefix='compile-') as directory:
        stage = Path(directory) / 'output'
        stage.mkdir()
        sources = Path(directory) / 'source'
        if not native:
            stage_sources(sources)
        for target in sorted(requested):
            binary = stage / f'SADPTools-darwin-{target}'
            if native:
                run('go', 'build', *flags, '-o', binary, '.', env=compiler_environment(target))
            else:
                run('docker', 'run', '--rm', '--platform', 'linux/amd64',
                    '--user', f'{os.getuid()}:{os.getgid()}',
                    '--mount', f'type=bind,src={sources},dst=/src,readonly',
                    '--mount', f'type=bind,src={stage},dst=/out',
                    '--mount', f'type=bind,src={cache},dst=/cache',
                    '-e', 'HOME=/cache/home', '-e', 'GOCACHE=/cache/build',
                    '-e', 'GOMODCACHE=/cache/mod', '-e', 'ZIG_GLOBAL_CACHE_DIR=/cache/zig',
                    '-e', 'ZIG_LOCAL_CACHE_DIR=/cache/zig-local', '-e', 'GOTOOLCHAIN=local',
                    '-e', 'GOOS=darwin', '-e', f'GOARCH={target}', '-e', 'CGO_ENABLED=1',
                    '-e', 'CC=/usr/local/bin/zig-cc.py', '-e', 'CXX=/usr/local/bin/zig-cc.py',
                    '-e', f'MACOSX_DEPLOYMENT_TARGET={MINIMUM}', image_id,
                    'go', 'build', *flags, '-o', f'/out/{binary.name}', '.')
            verify_binary(binary.read_bytes(), target, version)
        binary = stage / f'SADPTools-darwin-{arch}'
        if arch == 'universal':
            inputs = [stage / f'SADPTools-darwin-{a}' for a in sorted(requested)]
            if native:
                run('xcrun', 'lipo', '-create', *inputs, '-output', binary)
            else:
                wails = Path(run('go', 'env', 'GOPATH', capture=True)) / 'bin/wails3'
                run(wails, 'tool', 'lipo', '-output', binary, '-input', inputs[0], '-input', inputs[1])
            verify_binary(binary.read_bytes(), arch, version)
        if source_state() != before:
            raise ValueError('Source state changed while compiling; rebuild')
        destination = output / binary.name
        shutil.copy2(binary, destination)
        destination.chmod(0o755)
        write_json(receipt, dict(before, version=version, arch=arch, sha256=digest(destination),
                                toolchain='native Xcode' if native else IMAGE + '@' + image_id))
    print(f'Built {destination}')


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=['setup', 'build'])
    p.add_argument('--arch', choices=['arm64', 'amd64', 'universal'], default='universal')
    p.add_argument('--version')
    a = p.parse_args()
    if a.action == 'setup':
        setup()
    else:
        if not a.version:
            p.error('--version is required for build')
        build(a.arch, a.version)


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, subprocess.SubprocessError) as e:
        raise SystemExit(f'macOS build failed: {e}') from e

"""Synthetic Mach-O/ZIP/DMG fixtures: these are never runnable applications."""
import importlib.util
import json
from pathlib import Path
import plistlib
import shutil
import subprocess
import struct
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import common
import package
import cross

VERSION = '0.6.0-rc.1'


def thin(arch, version=VERSION):
    return struct.pack('<8I', 0xfeedfacf, common.ARCHES[arch], 0, 2, 0, 0, 0, 0) + b'\0' + version.encode() + b'\0synthetic only'


def fat():
    a, b = thin('amd64'), thin('arm64')
    first, second = 16384, 32768
    header = struct.pack('>II', 0xcafebabe, 2)
    header += struct.pack('>5I', common.ARCHES['amd64'], 0, first, len(a), 14)
    header += struct.pack('>5I', common.ARCHES['arm64'], 0, second, len(b), 14)
    return header + bytes(first - len(header)) + a + bytes(second - first - len(a)) + b


class MacTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def test_thin_and_universal_architecture_and_version(self):
        common.verify_binary(thin('arm64'), 'arm64', VERSION)
        common.verify_binary(thin('amd64'), 'amd64', VERSION)
        common.verify_binary(fat(), 'universal', VERSION)
        for binary, arch, version in [(thin('arm64'), 'amd64', VERSION), (thin('amd64'), 'universal', VERSION), (fat(), 'universal', '0.6.0-rc.2'), (b'MZ' * 100, 'arm64', VERSION)]:
            with self.assertRaises(ValueError):
                common.verify_binary(binary, arch, version)

    def test_malformed_fat_slices_rejected(self):
        for position, replacement in [(8, common.ARCHES['arm64']), (8+8, 1), (28+8, 16384), (28+12, 0xffffffff), (28+16, 31)]:
            binary = bytearray(fat())
            struct.pack_into('>I', binary, position, replacement)
            with self.assertRaises(ValueError):
                common.verify_binary(binary, 'universal', VERSION)

    def test_plist_and_names(self):
        for version in (VERSION, '0.6.0-beta.1', '0.6.0'):
            info = common.info_plist(version)
            self.assertEqual(info['CFBundleShortVersionString'], '0.6.0')
            self.assertEqual(info['SADPToolsVersion'], version)
            self.assertTrue(info['NSLocalNetworkUsageDescription'])
            self.assertEqual(info['LSMinimumSystemVersion'], '12.0')
        self.assertEqual(common.asset_names(VERSION), {f'SADPTools-{VERSION}-macos-universal.zip', f'SADPTools-{VERSION}-macos-universal.dmg'})
        with self.assertRaises(ValueError):
            common.asset_names('../../bad')
        with self.assertRaises(ValueError):
            common.architectures('386')

    def fixture(self):
        binary = self.root / 'synthetic'
        binary.write_bytes(fat())
        bundle = self.root / 'SADPTools.app'
        package.create_bundle(binary, bundle, VERSION)
        folder = self.root / 'artifacts'
        folder.mkdir()
        names = common.asset_names(VERSION)
        zipname = next(n for n in names if n.endswith('.zip'))
        package.archive_bundle(bundle, folder / zipname)
        (folder / next(n for n in names if n.endswith('.dmg'))).write_bytes(b'koly' + bytes(508))
        manifest = {'version': VERSION, 'commit': 'a'*40, 'dirty': False, 'arch': 'universal',
                    'architectures': ['amd64', 'arm64'], 'host': 'Darwin', 'signing': 'adhoc',
                    'sha256': {n:common.digest(folder / n) for n in names}}
        self.save_manifest(folder, manifest)
        return folder, manifest, folder / zipname

    def save_manifest(self, folder, value):
        common.write_json(folder / package.MANIFEST, value)
        (folder / 'SHA256SUMS').write_text(common.checksums(folder, common.asset_names(VERSION) | {package.MANIFEST}))

    def test_bundle_archive_preserves_permissions_metadata_and_notices(self):
        folder, _, archive = self.fixture()
        package.verify_zip(archive, VERSION, 'universal')
        package.verify_import(folder, VERSION, 'a'*40)

    def test_import_rejects_dirty_wrong_commit_unsigned_and_nonuniversal(self):
        folder, manifest, _ = self.fixture()
        for key, value in [('dirty', True), ('commit', 'b'*40), ('version', '0.6.0'), ('arch', 'arm64'), ('host', 'Linux'), ('signing', 'unsigned')]:
            with self.subTest(key=key):
                self.save_manifest(folder, dict(manifest, **{key:value}))
                with self.assertRaises(ValueError):
                    package.verify_import(folder, VERSION, 'a'*40)

    def test_import_rejects_corrupt_assets(self):
        folder, _, archive = self.fixture()
        archive.write_bytes(b'corrupted')
        with self.assertRaisesRegex(ValueError, 'checksum'):
            package.verify_import(folder, VERSION, 'a'*40)

    def test_import_rejects_missing_dmg(self):
        folder, _, _ = self.fixture()
        next(folder.glob('*.dmg')).unlink()
        with self.assertRaises(ValueError):
            package.verify_import(folder, VERSION, 'a'*40)

    def test_import_rejects_mislabeled_dmg(self):
        folder, manifest, _ = self.fixture()
        dmg = next(folder.glob('*.dmg'))
        dmg.write_bytes(b'synthetic-not-a-dmg' * 40)
        manifest['sha256'][dmg.name] = common.digest(dmg)
        self.save_manifest(folder, manifest)
        with self.assertRaisesRegex(ValueError, 'UDIF'):
            package.verify_import(folder, VERSION, 'a'*40)

    def test_zip_rejects_unsafe_paths_links_and_lost_executable_mode(self):
        _, _, archive = self.fixture()
        original = archive.read_bytes()
        for name, mode in [('../escape', 0o100644), ('SADPTools.app/link', 0o120777), ('/absolute', 0o100644)]:
            archive.write_bytes(original)
            with zipfile.ZipFile(archive, 'a') as z:
                info = zipfile.ZipInfo(name)
                info.external_attr = mode << 16
                z.writestr(info, 'synthetic')
            with self.assertRaises(ValueError):
                package.verify_zip(archive, VERSION, 'universal')
        archive.write_bytes(original)
        binary = self.root / 'SADPTools.app/Contents/MacOS/SADPTools'
        binary.chmod(0o644)
        package.archive_bundle(self.root / 'SADPTools.app', archive)
        with self.assertRaisesRegex(ValueError, 'permission'):
            package.verify_zip(archive, VERSION, 'universal')

    def test_bundle_rejects_wrong_identifier(self):
        _, _, archive = self.fixture()
        infofile = self.root / 'SADPTools.app/Contents/Info.plist'
        info = plistlib.loads(infofile.read_bytes())
        info['CFBundleIdentifier'] = 'invalid.example'
        infofile.write_bytes(plistlib.dumps(info))
        package.archive_bundle(self.root / 'SADPTools.app', archive)
        with self.assertRaisesRegex(ValueError, 'metadata'):
            package.verify_zip(archive, VERSION, 'universal')

    def test_dmg_requires_native_host(self):
        with patch.object(package.platform, 'system', return_value='Linux'), self.assertRaisesRegex(ValueError, 'hdiutil'):
            package.package(VERSION, 'universal', True)

    def test_zig_wrapper_preserves_spaces_and_deployment_floor(self):
        spec = importlib.util.spec_from_file_location('zig_cc', Path(__file__).with_name('zig-cc.py'))
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        args = module.arguments(['-target', 'arm64-apple-macos', '-mmacosx-version-min=11.0', '-I/path with spaces', '-c', 'file.mm'], 'arm64')
        self.assertIn('aarch64-macos.12.0', args)
        self.assertIn('-I/path with spaces', args)
        self.assertIn('c++', args)
        self.assertNotIn('arm64-apple-macos', args)
        self.assertNotIn('-mmacosx-version-min=11.0', args)

    def test_docker_source_staging_excludes_private_and_unrelated_files(self):
        source = self.root / 'source-repository'
        source.mkdir()
        for name in ['go.mod', 'go.sum', 'main.go', 'internal/example/code.go',
                     'internal/example/testdata/fixture.xml', 'build/appicon.png',
                     'frontend/dist/index.html', 'web/.credentials.json', '.cache/private',
                     'docs/private-notes.md']:
            path = source / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text('synthetic')
        listing = '\0'.join(['go.mod', 'go.sum', 'main.go', 'internal/example/code.go',
                              'internal/example/testdata/fixture.xml', 'build/appicon.png',
                              'docs/private-notes.md']) + '\0'
        destination = self.root / 'staged'
        with patch.object(cross, 'ROOT', source), patch.object(cross, 'run', return_value=listing):
            cross.stage_sources(destination)
        self.assertTrue((destination / 'frontend/dist/index.html').is_file())
        self.assertTrue((destination / 'internal/example/testdata/fixture.xml').is_file())
        self.assertFalse((destination / 'web').exists())
        self.assertFalse((destination / '.cache').exists())
        self.assertFalse((destination / 'docs').exists())

    def test_wails_task_preserves_requested_architecture(self):
        wails = shutil.which('wails3')
        if not wails:
            self.skipTest('Pinned Wails CLI unavailable; make tools enables task integration check')
        for arch in ('arm64', 'amd64', 'universal'):
            result = subprocess.run([wails, 'task', '--dry', 'package', 'GOOS=darwin', 'ARCH='+arch],
                                    cwd=common.ROOT, capture_output=True, text=True, check=True)
            output = result.stdout + result.stderr
            self.assertIn("build/darwin/cross.py build --arch '" + arch + "'", output)
            self.assertIn("build/darwin/package.py --version", output)
        for target in ('windows', 'linux'):
            result = subprocess.run([wails, 'task', '--dry', 'package', 'GOOS='+target, 'ARCH=amd64'],
                                    cwd=common.ROOT, capture_output=True, text=True, check=True)
            output = result.stdout + result.stderr
            self.assertNotIn('build/darwin/', output)
            self.assertIn('build/'+target+'/package.py', output)

    def test_icon_contains_retina_and_standard_sizes(self):
        data = (common.ROOT / 'build/darwin/icons.icns').read_bytes()
        self.assertEqual(data[:4], b'icns')
        self.assertEqual(struct.unpack_from('>I', data, 4)[0], len(data))
        offset, types = 8, set()
        while offset < len(data):
            kind, size = struct.unpack_from('>4sI', data, offset)
            self.assertGreater(size, 8)
            self.assertEqual(data[offset+8:offset+16], b'\x89PNG\r\n\x1a\n')
            types.add(kind)
            offset += size
        self.assertEqual(offset, len(data))
        self.assertTrue({b'ic07', b'ic08', b'ic09', b'ic10'} <= types)


if __name__ == '__main__':
    unittest.main()

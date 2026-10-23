"""Synthetic release files only: no GitHub writes, builds or network requests."""
import json
from pathlib import Path
import shutil
import tempfile
import unittest
import zipfile
from unittest.mock import patch

import release
from release_version import metadata, windows_version


class VersionTests(unittest.TestCase):
    def test_full_beta_label_and_numeric_windows_version(self):
        result = metadata("0.2.0-beta.1")
        self.assertEqual(result["fixed"]["file_version"], "0.2.0.1")
        self.assertEqual(result["info"]["0409"]["ProductVersion"], "0.2.0-beta.1")

    def test_windows_version_order(self):
        versions = ["0.2.0-dev", "0.2.0-beta.1", "0.2.0-beta.2", "0.2.0-rc.1", "0.2.0"]
        numeric = [tuple(map(int, windows_version(v).split("."))) for v in versions]
        self.assertEqual(numeric, sorted(set(numeric)))

    def test_invalid_versions_and_shell_metacharacters_rejected(self):
        for version in ("v0.2.0", "01.2.0", "0.2.0-beta", "0.2.0-rc.0", "0.2.0-dev.1",
                        "0.2.0-beta.20000", "65536.0.0", "0.2.0+metadata", "../v1", "$(id)", "0.2.0\n"):
            with self.subTest(version=version), self.assertRaises(ValueError):
                windows_version(version)

    def test_development_build_cannot_be_released(self):
        with self.assertRaisesRegex(ValueError, "Development"):
            windows_version("0.2.0-dev", release=True)

    def test_binary_version_requires_exact_go_string_not_windows_resource(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "synthetic.exe"
            path.write_bytes(b"synthetic\0" + "0.2.0-beta.1".encode("utf-16-le") + b"\0\0")
            with self.assertRaisesRegex(ValueError, "Missing release version"):
                release.verify_binary_version(path, "0.2.0-beta.1")
            path.write_bytes(b"synthetic\0" + b"0.2.0-beta.10\0")
            with self.assertRaisesRegex(ValueError, "Missing release version"):
                release.verify_binary_version(path, "0.2.0-beta.1")
            path.write_bytes(b"synthetic\0" + b"0.2.0-beta.1\0")
            release.verify_binary_version(path, "0.2.0-beta.1")


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.folder = Path(self.temporary.name) / "local"
        self.folder.mkdir()
        self.version, self.commit = "0.2.0-beta.1", "a" * 40
        self.names = release.asset_names(self.version)
        for name in self.names:
            (self.folder / name).write_bytes(b"synthetic release bytes " + name.encode())
        manifest = {"version": self.version, "commit": self.commit, "standalone_desktop": True,
                    "sha256": {n: release.digest(self.folder / n) for n in self.names}}
        (self.folder / "build-manifest.json").write_text(json.dumps(manifest))
        (self.folder / "SHA256SUMS").write_text(release.checksum_text(self.folder, self.names | {"build-manifest.json"}))

    def verify(self):
        return release.verify_bundle(self.folder, self.version, self.commit)

    def test_complete_bundle_is_accepted(self):
        self.assertEqual(self.verify()["commit"], self.commit)

    def test_older_bundle_without_desktop_remains_verifiable(self):
        (self.folder / f"SADPTools-{self.version}-standalone-windows-amd64.zip").unlink()
        manifest_path = self.folder / "build-manifest.json"
        manifest = json.loads(manifest_path.read_text())
        manifest.pop("standalone_desktop")
        names = release.asset_names(self.version, standalone_desktop=False)
        manifest["sha256"] = {n: release.digest(self.folder / n) for n in names}
        manifest_path.write_text(json.dumps(manifest))
        (self.folder / "SHA256SUMS").write_text(release.checksum_text(self.folder, names | {"build-manifest.json"}))
        self.assertEqual(self.verify()["commit"], self.commit)

    def test_standalone_archive_contains_gui_and_licence_notices(self):
        root = Path(self.temporary.name) / "synthetic-source"
        root.mkdir()
        (root / "LICENSES").mkdir()
        for path in (root / "LICENSE", root / "THIRD_PARTY_NOTICES.md", root / "LICENSES/fixture.txt"):
            path.write_text("Synthetic licence fixture")
        binary = root / "fixture.exe"
        binary.write_bytes(b"Synthetic desktop executable")
        with patch("release.ROOT", root):
            release.desktop_archive(self.folder, self.version, binary)
        with zipfile.ZipFile(self.folder / f"SADPTools-{self.version}-standalone-windows-amd64.zip") as archive:
            self.assertIsNone(archive.testzip())
            self.assertEqual(archive.read("SADPTools-windows-amd64.exe"), binary.read_bytes())
            self.assertEqual(set(archive.namelist()), {
                "SADPTools-windows-amd64.exe", "LICENSE", "THIRD_PARTY_NOTICES.md",
                "LICENSES/fixture.txt", "README.txt"})
            self.assertIn("WebView2", archive.read("README.txt").decode())

    def test_damaged_asset_rejected(self):
        (self.folder / sorted(self.names)[0]).write_bytes(b"corrupt")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            self.verify()

    def test_stale_build_rejected(self):
        with self.assertRaisesRegex(ValueError, "different version or commit"):
            release.verify_bundle(self.folder, self.version, "b" * 40)

    def test_missing_or_extra_assets_rejected(self):
        (self.folder / "surprise.txt").write_text("not a release file")
        with self.assertRaisesRegex(ValueError, "unexpected"):
            self.verify()
        (self.folder / "surprise.txt").unlink()
        (self.folder / "SHA256SUMS").unlink()
        with self.assertRaisesRegex(ValueError, "missing"):
            self.verify()

    def test_modified_checksums_rejected(self):
        (self.folder / "SHA256SUMS").write_text("not a checksum list")
        with self.assertRaisesRegex(ValueError, "SHA256SUMS"):
            self.verify()

    def test_download_verification_rejects_wrong_bytes_and_missing_files(self):
        downloaded = Path(self.temporary.name) / "download"
        shutil.copytree(self.folder, downloaded)
        release.verify_download(self.folder, downloaded)
        target = downloaded / sorted(self.names)[0]
        target.write_bytes(b"wrong upload")
        with self.assertRaisesRegex(ValueError, "GitHub asset checksum mismatch"):
            release.verify_download(self.folder, downloaded)
        target.unlink()
        with self.assertRaisesRegex(ValueError, "asset list differs"):
            release.verify_download(self.folder, downloaded)

    def test_dirty_checkout_rejected_before_publish(self):
        with patch("release.run", return_value=" M README.md") as command:
            with self.assertRaisesRegex(ValueError, "Commit all source changes"):
                release.publish(self.version, self.folder)
            command.assert_called_once_with("git", "status", "--porcelain", "--untracked-files=all", capture=True)


if __name__ == "__main__":
    unittest.main()

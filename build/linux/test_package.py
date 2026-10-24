"""Isolated synthetic payloads only; no system installs or real camera access."""
import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location("linux_package", Path(__file__).with_name("package.py"))
package = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(package)


class MetadataTests(unittest.TestCase):
    def test_runtime_requirements_translate_minimums_and_reject_unmapped_libraries(self):
        deps = package.dependency_lists("libc6 (>= 2.38), libgtk-4-1 (>= 4.13.8), libwebkitgtk-6.0-4 (>= 2.39.91), libsoup-3.0-0 (>= 2.4.0)")
        self.assertIn("libgtk-4-1 (>= 4.14)", deps["deb"])
        self.assertIn("webkitgtk6.0 >= 2.40", deps["rpm"])
        self.assertIn("libsoup3 >= 3.0", deps["rpm"])
        self.assertIn("glibc >= 2.38", deps["rpm"])
        with self.assertRaisesRegex(ValueError, "Review"):
            package.dependency_lists("new-unknown-library (>= 1.0)")

    def test_newer_symbol_requirements_are_not_lowered(self):
        deps = package.dependency_lists("libgtk-4-1 (>= 4.20), libc6 (>= 2.43)")
        self.assertIn("gtk4 >= 4.20", deps["rpm"])
        self.assertIn("libc6 (>= 2.43)", deps["deb"])

    def test_wrong_architecture_and_version_fail_before_packaging(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = Path(tmp) / "synthetic-elf"
            binary.write_bytes(b"not ELF\0" + b"0.3.0-beta.1\0")
            with self.assertRaisesRegex(ValueError, "AMD64"):
                package.verify_binary(binary, "0.3.0-beta.1")
            binary.write_bytes(b"\x7fELF\x02\x01" + b"\0" * 12 + b"\x3e\0" + b"\0" * 12 + b"0.3.0-beta.1\0")
            package.verify_binary(binary, "0.3.0-beta.1")
            with self.assertRaisesRegex(ValueError, "stamped"):
                package.verify_binary(binary, "0.3.0-beta.2")

    def test_native_layout_permissions_and_upgrade_hooks(self):
        with tempfile.TemporaryDirectory() as tmp:
            payload = Path(tmp)
            binary = payload / "bin/SADPTools"
            binary.parent.mkdir()
            binary.write_text("synthetic")
            binary.chmod(0o755)
            config = package.package_config(payload, "0.3.0-beta.1", {"deb": ["libgtk-4-1"]}, "deb")
            self.assertEqual(config["version_schema"], "semver")
            self.assertEqual(config["contents"][0]["dst"], "/usr/bin/SADPTools")
            self.assertEqual(config["contents"][0]["file_info"], {"mode": 0o755, "owner": "root", "group": "root"})
            self.assertEqual(set(config["scripts"]), {"postinstall", "postremove"})


@unittest.skipUnless(sys.platform == "linux" and os.geteuid() != 0, "manual tests use a normal Linux user")
class ManualInstallerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="sadptools-installer-test-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.bundle = self.root / "bundle"
        self.bundle.mkdir()
        self.prefix = self.root / "install with spaces"
        self.tools = self.root / "tools"
        self.tools.mkdir()
        # Synthetic runtime probe: real library resolution is checked separately
        # on the compiled app. This test must not depend on GUI libraries.
        (self.tools / "ldd").write_text('#!/bin/sh\nif [ "${SADPTOOLS_FIXTURE_MISSING:-}" = 1 ]; then\n echo "libsynthetic.so => not found"\n exit 1\nfi\n')
        (self.tools / "ldd").chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.tools) + os.pathsep + os.environ["PATH"])
        self.env.pop("XDG_DATA_HOME", None)
        self.payload = self.bundle / "payload"
        for relative in ("bin/SADPTools", "bin/sadptools",
                         f"share/icons/hicolor/scalable/apps/{package.APP_ID}.svg",
                         f"share/icons/hicolor/512x512/apps/{package.APP_ID}.png",
                         "share/doc/sadptools/LICENSE"):
            path = self.payload / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("synthetic fixture v1\n")
            path.chmod(0o755 if relative.startswith("bin/") else 0o644)
        desktop = self.payload / f"share/applications/{package.APP_ID}.desktop"
        desktop.parent.mkdir(parents=True)
        shutil.copyfile(package.BUILD / f"linux/{package.APP_ID}.desktop", desktop)
        shutil.copyfile(package.BUILD / "linux/install.sh", self.bundle / "install.sh")
        self.checksums()

    def checksums(self):
        (self.bundle / "SHA256SUMS").write_text("".join(
            f"{package.digest(p)}  {p.relative_to(self.bundle)}\n"
            for p in sorted(self.bundle.rglob("*")) if p.is_file() and p.name != "SHA256SUMS"))

    def install(self, *args, success=True, env=None):
        result = subprocess.run(["bash", str(self.bundle / "install.sh"), "--prefix", str(self.prefix), *args],
                                text=True, capture_output=True, env=env or self.env)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0)
        return result

    def test_install_upgrade_and_stored_uninstaller_preserve_user_data(self):
        settings = self.prefix / "config/SADPTools/sites.sqlite"
        settings.parent.mkdir(parents=True)
        settings.write_text("synthetic saved data")
        self.install()
        launcher = self.prefix / "bin/SADPTools"
        self.assertTrue(launcher.is_symlink())
        self.assertTrue(os.access(launcher, os.X_OK))
        self.assertEqual(launcher.read_text(), "synthetic fixture v1\n")
        desktop = self.prefix / f"share/applications/{package.APP_ID}.desktop"
        self.assertIn(f'Exec="{self.prefix}/lib/sadptools/bin/SADPTools"', desktop.read_text())
        self.assertTrue((self.prefix / f"share/icons/hicolor/scalable/apps/{package.APP_ID}.svg").is_file())
        (self.payload / "bin/SADPTools").write_text("synthetic fixture v2\n")
        self.checksums()
        self.install()
        self.assertEqual(launcher.read_text(), "synthetic fixture v2\n")
        result = subprocess.run([str(self.prefix / "lib/sadptools/uninstall.sh")], text=True, capture_output=True, env=self.env)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(launcher.is_symlink())
        self.assertFalse(desktop.is_symlink())
        self.assertFalse((self.prefix / "lib/sadptools").exists())
        self.assertEqual(settings.read_text(), "synthetic saved data")
        self.install("--uninstall")  # Repeated uninstall is harmless.

    def test_checksum_failure_makes_no_installation(self):
        (self.payload / "bin/SADPTools").write_text("damaged fixture")
        result = self.install(success=False)
        self.assertIn("checksum", result.stderr.lower())
        self.assertFalse(self.prefix.exists())

    def test_missing_runtime_makes_no_installation_and_explains_dependencies(self):
        result = self.install(success=False, env=dict(self.env, SADPTOOLS_FIXTURE_MISSING="1"))
        self.assertIn("sudo apt install", result.stderr)
        self.assertIn("sudo dnf install", result.stderr)
        self.assertFalse(self.prefix.exists())

    def test_unrelated_launcher_and_unowned_install_directory_are_preserved(self):
        launcher = self.prefix / "bin/SADPTools"
        launcher.parent.mkdir(parents=True)
        launcher.write_text("another program")
        self.install(success=False)
        self.assertEqual(launcher.read_text(), "another program")
        launcher.unlink()
        app = self.prefix / "lib/sadptools"
        app.mkdir(parents=True)
        (app / "keep.txt").write_text("unowned")
        self.install(success=False)
        self.install("--uninstall", success=False)
        self.assertTrue((app / "keep.txt").exists())

    def test_changed_launcher_is_not_deleted_by_uninstall(self):
        self.install()
        launcher = self.prefix / "bin/SADPTools"
        launcher.unlink()
        launcher.write_text("replacement owned by user")
        self.install("--uninstall")
        self.assertEqual(launcher.read_text(), "replacement owned by user")

    @unittest.skipUnless(shutil.which("gio"), "GIO is optional for the real desktop launcher parsing test")
    def test_desktop_exec_escaping_using_real_gio_launcher(self):
        self.prefix = self.root / 'space $dollar `tick` "quote" \\ slash'
        marker = self.root / "launched"
        binary = self.payload / "bin/SADPTools"
        binary.write_text('#!/bin/sh\nprintf "synthetic-launch" > "$SADPTOOLS_FIXTURE_MARKER"\n')
        self.checksums()
        self.install()
        desktop = self.prefix / f"share/applications/{package.APP_ID}.desktop"
        result = subprocess.run(["gio", "launch", str(desktop)], text=True, capture_output=True,
                                env=dict(self.env, SADPTOOLS_FIXTURE_MARKER=str(marker)))
        self.assertEqual(result.returncode, 0, result.stderr)
        # GIO launches asynchronously; communicate with its synthetic child.
        import time
        deadline = time.monotonic() + 3
        while not marker.exists() and time.monotonic() < deadline:
            time.sleep(0.02)
        self.assertEqual(marker.read_text(), "synthetic-launch")

    def test_invalid_arguments_and_prefix_are_rejected(self):
        for args in (("--unknown",), ("--prefix",), ("--prefix", "/"), ("--prefix", "relative"),
                     ("--prefix", str(self.root / "percent %f"))):
            self.install(*args, success=False)


if __name__ == "__main__":
    unittest.main()

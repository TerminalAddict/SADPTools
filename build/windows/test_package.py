"""Input-integrity tests use synthetic bytes, never executable fixtures or a network."""
import hashlib
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import package


class DownloadTests(unittest.TestCase):
    def setUp(self):
        self.folder = tempfile.TemporaryDirectory()
        self.addCleanup(self.folder.cleanup)
        self.cache = Path(self.folder.name)
        self.payload = b"synthetic redistributable test data"
        self.asset = {"filename": "fixture.exe", "url": "https://example.invalid/fixture.exe",
                      "size": len(self.payload), "sha256": hashlib.sha256(self.payload).hexdigest()}

    def response(self, data):
        response = io.BytesIO(data)
        response.url = self.asset["url"]
        return response

    def test_verified_cache_works_without_network(self):
        path = self.cache / self.asset["filename"]
        path.write_bytes(self.payload)
        with patch("package.urllib.request.urlopen", side_effect=AssertionError("network used")):
            self.assertEqual(package.fetch(self.asset, self.cache), path)

    def test_corrupt_cache_is_rejected_without_silent_replacement(self):
        path = self.cache / self.asset["filename"]
        path.write_bytes(b"x" * len(self.payload))
        with patch("package.urllib.request.urlopen", side_effect=AssertionError("network used")):
            with self.assertRaisesRegex(ValueError, "Integrity check failed"):
                package.fetch(self.asset, self.cache)

    def test_download_committed_only_after_integrity_check(self):
        with patch("package.urllib.request.urlopen", return_value=self.response(self.payload)):
            path = package.fetch(self.asset, self.cache)
        self.assertEqual(path.read_bytes(), self.payload)
        self.assertEqual(list(self.cache.iterdir()), [path])

    def test_incomplete_corrupt_and_oversized_downloads_leave_no_files(self):
        for data in (self.payload[:-1], b"x" * len(self.payload), self.payload + b"extra"):
            with self.subTest(data=data):
                with patch("package.urllib.request.urlopen", return_value=self.response(data)):
                    with self.assertRaises(ValueError):
                        package.fetch(self.asset, self.cache)
                self.assertEqual(list(self.cache.iterdir()), [])

    def test_insecure_redirect_rejected(self):
        response = self.response(self.payload)
        response.url = "http://example.invalid/insecure"
        with patch("package.urllib.request.urlopen", return_value=response):
            with self.assertRaisesRegex(ValueError, "insecure"):
                package.fetch(self.asset, self.cache)
        self.assertEqual(list(self.cache.iterdir()), [])


if __name__ == "__main__":
    unittest.main()

"""Synthetic search-submission tests; never call search engines."""
import io
import json
from pathlib import Path
import sys
import tempfile
import unittest
import urllib.robotparser
from html.parser import HTMLParser
from unittest.mock import patch
from xml.etree import ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'scripts'))
from search_metadata import PUBLIC_PAGES, write_discovery_files
import submit_search as submit


class SearchSubmissionTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        root = Path(self.temp.name)
        self.public = root / 'public'
        self.public.mkdir()
        (root / 'content').mkdir()
        self.state = root / '.cache/submitted.json'
        for name in PUBLIC_PAGES:
            (self.public / name).write_text('<html><title>Synthetic ' + name + '</title></html>')
        (self.public / 'indexnow-key.txt').write_text('a' * 32 + '\n')
        write_discovery_files(self.public, submit.SITE)
        self.addCleanup(patch.stopall)
        patch('sys.stdout', new=io.StringIO()).start()

    def deployed(self, url):
        name = url.removeprefix(submit.SITE + '/') or 'index.html'
        return (self.public / name).read_bytes()

    def test_no_timestamp_churn_on_rebuild(self):
        state_path = self.public.parent / 'content/page-updates.json'
        state = json.loads(state_path.read_text())
        for row in state.values():
            row['lastmod'] = '2026-01-01T00:00:00Z'
        state_path.write_text(json.dumps(state))
        write_discovery_files(self.public, submit.SITE)
        self.assertEqual(json.loads(state_path.read_text()), state)
        (self.public / 'features.html').write_text('Synthetic revised features')
        write_discovery_files(self.public, submit.SITE)
        new = json.loads(state_path.read_text())
        self.assertNotEqual(new['features.html']['lastmod'], state['features.html']['lastmod'])
        self.assertEqual(new['index.html'], state['index.html'])

    def test_accepted_batch_then_only_changed_page(self):
        with patch.object(submit, 'fetch_exact', side_effect=self.deployed), patch.object(submit, 'post', return_value=202) as post:
            self.assertEqual(submit.submit(self.public, self.state), 202)
            self.assertEqual(len(post.call_args.args[0]['urlList']), len(PUBLIC_PAGES))
            self.assertEqual(post.call_args.args[0]['keyLocation'], submit.SITE + '/indexnow-key.txt')
            self.assertIsNone(submit.submit(self.public, self.state))
            post.assert_called_once()
            (self.public / 'features.html').write_text('Synthetic changed features')
            post.return_value = 200
            submit.submit(self.public, self.state)
            self.assertEqual(post.call_args.args[0]['urlList'], [submit.SITE + '/features.html'])
        records = [json.loads(line) for line in (self.state.parent / 'search-submissions.jsonl').read_text().splitlines()]
        self.assertEqual([row['http_status'] for row in records], [202, 200])

    def test_no_submission_or_checkpoint_for_undeployed_page(self):
        def old(url):
            return b'not deployed yet' if url.endswith('/features.html') else self.deployed(url)
        with patch.object(submit, 'fetch_exact', side_effect=old), patch.object(submit, 'post') as post:
            with self.assertRaisesRegex(ValueError, 'deploy first'):
                submit.submit(self.public, self.state)
            post.assert_not_called()
            self.assertFalse(self.state.exists())

    def test_rejected_batch_can_be_retried_manually(self):
        with patch.object(submit, 'fetch_exact', side_effect=self.deployed), patch.object(submit, 'post', return_value=429):
            with self.assertRaises(ValueError):
                submit.submit(self.public, self.state)
            self.assertFalse(self.state.exists())

    def test_private_external_or_noindex_pages_never_submit(self):
        for url in ['https://example.invalid/', submit.SITE + '/assets/downloads_report.php', submit.SITE + '/404.html', submit.SITE + '/features.html?password=synthetic']:
            with self.subTest(url=url):
                (self.public / 'sitemap.xml').write_text('<urlset><url><loc>' + url + '</loc></url></urlset>')
                with self.assertRaises(ValueError):
                    submit.candidates(self.public)
        write_discovery_files(self.public, submit.SITE)
        (self.public / 'features.html').write_text('<meta name="robots" content="noindex">')
        with self.assertRaises(ValueError):
            submit.candidates(self.public)

    def test_dry_run_uses_no_network_or_state(self):
        with patch.object(submit, 'fetch_exact') as fetch, patch.object(submit, 'post') as post:
            submit.submit(self.public, self.state, dry_run=True)
            fetch.assert_not_called()
            post.assert_not_called()
            self.assertFalse(self.state.exists())

    def test_actual_sitemap_has_only_indexable_canonical_pages(self):
        urls = submit.candidates(ROOT / 'public')
        self.assertEqual(len(urls), len(PUBLIC_PAGES))
        self.assertNotIn(submit.SITE + '/index.html', urls)
        for name in PUBLIC_PAGES:
            data = (ROOT / 'public' / name).read_text()
            url = submit.SITE + ('/' if name == 'index.html' else '/' + name)
            self.assertIn('<link rel="canonical" href="' + url + '">', data)
            self.assertEqual(data.count('<h1>'), 1)
            self.assertNotIn('github.com', data.lower())
        tree = ET.parse(ROOT / 'public/sitemap.xml')
        self.assertEqual(len(tree.findall('{*}url/{*}lastmod')), len(urls))

    def test_search_bots_can_read_guides_but_private_paths_stay_excluded(self):
        rules = urllib.robotparser.RobotFileParser()
        rules.parse((ROOT / 'public/robots.txt').read_text().splitlines())
        for agent in ['OAI-SearchBot', 'Googlebot', 'bingbot', 'GPTBot', 'SyntheticCrawler']:
            for name in PUBLIC_PAGES:
                self.assertTrue(rules.can_fetch(agent, submit.SITE + '/' + name), (agent, name))
            for path in ['/_app/download-report-auth.json', '/assets/private-downloads/downloads.jsonl', '/assets/downloads_report.php', '/download/windows-setup.exe']:
                self.assertFalse(rules.can_fetch(agent, submit.SITE + path), (agent, path))

    def test_guides_are_readable_without_scripts_and_have_valid_section_links(self):
        from discovery_guides import GUIDES
        class Content(HTMLParser):
            def __init__(self):
                super().__init__(); self.ids=set(); self.links=[]; self.headings=0; self.nav_labels=[]
            def handle_starttag(self, tag, attrs):
                data=dict(attrs)
                if 'id' in data:
                    if data['id'] in self.ids: raise AssertionError('Duplicate document ID')
                    self.ids.add(data['id'])
                if tag == 'a': self.links.append(data.get('href', ''))
                if tag == 'h2': self.headings += 1
                if tag == 'nav': self.nav_labels.append(data.get('aria-label', ''))
        hub=(ROOT/'public/guides.html').read_text()
        for guide in GUIDES:
            self.assertIn('href="/' + guide['file'] + '"', hub)
            p=Content(); p.feed((ROOT/'public'/guide['file']).read_text())
            self.assertGreaterEqual(p.headings, 4)
            self.assertIn('On this page', p.nav_labels)
            for link in p.links:
                if link.startswith('#'): self.assertIn(link[1:], p.ids, guide['file'])
                elif link.startswith('/') and '.html' in link:
                    self.assertTrue((ROOT/'public'/link[1:].split('#')[0]).is_file(), link)


if __name__ == '__main__':
    unittest.main()

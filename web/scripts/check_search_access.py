#!/usr/bin/env python3
"""Audit published HTML using a search-crawler user agent; no indexing claims.

This makes ordinary requests from the development host, not from OpenAI's IPs.
"""
import hashlib
from html.parser import HTMLParser
import json
import urllib.request
import urllib.robotparser

from submit_search import ROOT, SITE, candidates

AGENT = 'Mozilla/5.0 (compatible; OAI-SearchBot/1.4; +https://openai.com/searchbot)'


class Page(HTMLParser):
    def __init__(self):
        super().__init__()
        self.canonical = ''
        self.noindex = False
        self.h1 = 0
        self.script = False
        self.text = []

    def handle_starttag(self, tag, attrs):
        data = dict(attrs)
        if tag == 'script': self.script = True
        if tag == 'h1': self.h1 += 1
        if tag == 'link' and data.get('rel') == 'canonical': self.canonical = data.get('href')
        if tag == 'meta' and data.get('name', '').lower() in ('robots', 'googlebot', 'oai-searchbot'):
            self.noindex |= 'noindex' in data.get('content', '').lower()

    def handle_endtag(self, tag):
        if tag == 'script': self.script = False

    def handle_data(self, text):
        if not self.script: self.text.append(text)


def fetch(url):
    request = urllib.request.Request(url, headers={'User-Agent': AGENT, 'Cache-Control': 'no-cache'})
    with urllib.request.urlopen(request, timeout=30) as response:
        if response.status != 200 or response.url != url:
            raise ValueError('Expected a direct HTTP 200 response: ' + url)
        if 'noindex' in response.headers.get('X-Robots-Tag', '').lower():
            raise ValueError('Crawler response excludes indexing: ' + url)
        return response.read(2 * 1024 * 1024 + 1)


def main():
    rules = urllib.robotparser.RobotFileParser()
    rules.parse(fetch(SITE + '/robots.txt').decode().splitlines())
    rows = []
    for url, digest in candidates(ROOT / 'public').items():
        if not rules.can_fetch('OAI-SearchBot', url): raise ValueError('Robots exclusion: ' + url)
        data = fetch(url)
        if hashlib.sha256(data).hexdigest() != digest: raise ValueError('Published HTML differs from build: ' + url)
        page = Page(); page.feed(data.decode())
        if page.noindex or page.h1 != 1 or page.canonical != url:
            raise ValueError('Check heading/canonical/indexing metadata: ' + url)
        rows.append({'url': url, 'http': 200, 'words_without_scripts': len(' '.join(page.text).split())})
        print('Crawler-access check passed:', url)
    for path in ['/_app/download-report-auth.json', '/assets/private-downloads/downloads.jsonl', '/assets/downloads_report.php']:
        if rules.can_fetch('OAI-SearchBot', SITE + path): raise ValueError('Private path lost robots exclusion')
    output = ROOT / '.cache/search-access.json'
    output.parent.mkdir(exist_ok=True)
    output.write_text(json.dumps({'note': 'Requests from Sparx with a crawler user agent; not proof of an OpenAI crawl or indexing.', 'pages': rows}, indent=2) + '\n')
    print(f'All {len(rows)} public pages are readable without JavaScript; private-path exclusions preserved.')


if __name__ == '__main__':
    main()

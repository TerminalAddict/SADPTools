#!/usr/bin/env python3
"""Notify IndexNow after deployment. Acceptance is not a ranking/indexing result."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import secrets
import sys
import urllib.error
import urllib.parse
import urllib.request
from xml.etree import ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
SITE = 'https://sadptools.com'
ENDPOINT = 'https://api.indexnow.org/indexnow'
AGENT = 'SADPTools-Site-Submission/1.0 (+https://sadptools.com/)'


def candidates(public):
    urls = {}
    tree = ET.parse(public / 'sitemap.xml')
    for element in tree.findall('{*}url/{*}loc'):
        url = element.text or ''
        parsed = urllib.parse.urlsplit(url)
        if parsed.scheme != 'https' or parsed.netloc != 'sadptools.com' or parsed.query or parsed.fragment:
            raise ValueError('Sitemap contains a URL outside the canonical website')
        name = 'index.html' if parsed.path == '/' else parsed.path.removeprefix('/')
        if not re.fullmatch(r'[a-z0-9-]+\.html', name) or name == '404.html':
            raise ValueError('Only public HTML pages may be submitted')
        payload = (public / name).read_bytes()
        if re.search(rb'<meta\b[^>]*content=["\'][^"\']*noindex', payload, re.I):
            raise ValueError('An excluded page appears in the sitemap')
        urls[url] = hashlib.sha256(payload).hexdigest()
    if not 1 <= len(urls) <= 10000:
        raise ValueError('Invalid number of sitemap URLs')
    return urls


def fetch_exact(url):
    request = urllib.request.Request(url, headers={'User-Agent': AGENT, 'Cache-Control': 'no-cache'})
    with urllib.request.urlopen(request, timeout=30) as response:
        if response.status != 200 or response.url != url:
            raise ValueError('Expected a directly reachable canonical URL: ' + url)
        body = response.read(2 * 1024 * 1024 + 1)
        if len(body) > 2 * 1024 * 1024:
            raise ValueError('Unexpectedly large public response')
        if 'noindex' in response.headers.get('X-Robots-Tag', '').lower() and url != SITE + '/indexnow-key.txt':
            raise ValueError('Published page is excluded from indexing')
        return body


def post(payload):
    request = urllib.request.Request(ENDPOINT, data=json.dumps(payload).encode(),
        headers={'Content-Type': 'application/json; charset=utf-8', 'User-Agent': AGENT}, method='POST')
    with urllib.request.urlopen(request, timeout=45) as response:
        return response.status


def submit(public, state_path, all_pages=False, dry_run=False):
    current = candidates(public)
    previous = json.loads(state_path.read_text()) if state_path.exists() else {}
    changed = {url: digest for url, digest in current.items() if all_pages or previous.get(url) != digest}
    if not changed:
        print('IndexNow: no changed pages since the last accepted submission.')
        return None
    print(('Would submit' if dry_run else 'Checking deployed copies of') + f' {len(changed)} public pages:')
    for url in changed:
        print('  ' + url)
    if dry_run:
        return None
    key = (public / 'indexnow-key.txt').read_text().strip()
    if not re.fullmatch(r'[a-f0-9]{32}', key):
        raise ValueError('Invalid IndexNow key; run --init once and deploy the key file')
    key_url = SITE + '/indexnow-key.txt'
    if fetch_exact(key_url).decode().strip() != key:
        raise ValueError('Deploy the IndexNow key file before submitting')
    for url, digest in changed.items():
        if hashlib.sha256(fetch_exact(url)).hexdigest() != digest:
            raise ValueError('Published HTML differs from the local build; deploy first: ' + url)
    status = post({'host': 'sadptools.com', 'key': key, 'keyLocation': key_url, 'urlList': list(changed)})
    if status not in (200, 202):
        raise ValueError(f'IndexNow did not accept the batch (HTTP {status})')
    state_path.parent.mkdir(parents=True, exist_ok=True)
    temporary = state_path.with_suffix('.tmp')
    temporary.write_text(json.dumps(current, indent=2) + '\n')
    temporary.replace(state_path)
    record = {'at': datetime.now(timezone.utc).isoformat(), 'endpoint': ENDPOINT,
              'http_status': status, 'urls': list(changed)}
    with (state_path.parent / 'search-submissions.jsonl').open('a') as log:
        log.write(json.dumps(record) + '\n')
    print(f'IndexNow HTTP {status}: ' + ('accepted; key validation pending.' if status == 202 else 'submission received.'))
    print('Participating engines share submissions. Receipt does not confirm crawling, indexing or ranking.')
    print('Google: submit https://sadptools.com/sitemap.xml in a verified Search Console property.')
    return status


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--init', action='store_true', help='Create a public verification key once, without submitting')
    parser.add_argument('--all', action='store_true', help='Resubmit all sitemap pages when a deliberate refresh is needed')
    parser.add_argument('--dry-run', action='store_true', help='List candidate pages without network requests')
    args = parser.parse_args()
    public = ROOT / 'public'
    if args.init:
        key = public / 'indexnow-key.txt'
        if key.exists():
            print('Existing IndexNow key retained.')
        else:
            key.write_text(secrets.token_hex(16) + '\n')
            print('Created public IndexNow ownership file. Deploy before submitting.')
        return
    try:
        submit(public, ROOT / '.cache/indexnow-submitted.json', args.all, args.dry_run)
    except (OSError, ValueError, ET.ParseError) as error:
        if isinstance(error, urllib.error.HTTPError):
            print(f'Search submission failed: HTTP {error.code}. Check verification or rate limits; no automatic retry.', file=sys.stderr)
        else:
            print('Search submission failed: ' + str(error), file=sys.stderr)
        raise SystemExit(1)


if __name__ == '__main__':
    main()

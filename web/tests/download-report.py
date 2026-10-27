#!/usr/bin/env python3
"""Isolated HTTP tests with synthetic packages, users and logs. No external network."""
import base64, csv, fcntl, hashlib, io, json, os, pathlib, shutil, socket, struct, subprocess, tempfile, time
import http.cookiejar, urllib.error, urllib.parse, urllib.request
from html.parser import HTMLParser

source = pathlib.Path(__file__).resolve().parents[1]
checks = 0

def check(value, message):
    global checks
    assert value, message
    checks += 1

with tempfile.TemporaryDirectory(prefix='sadptools-report-test-') as tmp:
    root = pathlib.Path(tmp)
    public = root / 'public'
    for folder in ('_app', 'assets/private-downloads', '../scripts'):
        (public / folder).mkdir(parents=True, exist_ok=True)
    for file in ('download.php', 'wordfence-waf.php', '.htaccess', 'assets/downloads_report.php', 'assets/private-downloads/.htaccess', 'assets/theme-init.js', 'assets/site.css', 'assets/download-report.css', 'assets/download-report.js', 'assets/appicon.svg'):
        shutil.copyfile(source / 'public' / file, public / file)
    for file in (source / 'public/_app').glob('*.php'):
        shutil.copyfile(file, public / '_app' / file.name)
    shutil.copyfile(source / 'scripts/router.php', root / 'scripts/router.php')
    (public / '404.html').write_text('Not found')
    log = public / 'assets/private-downloads/downloads.jsonl'
    log.touch()
    hashed = subprocess.run(['php', '-r', "echo password_hash('synthetic-test-only', PASSWORD_DEFAULT);"], capture_output=True, text=True, check=True).stdout
    auth_path = public / '_app/download-report-auth.json'
    auth = {'username': 'fixture', 'password_hash': hashed}
    auth_path.write_text(json.dumps(auth))
    targets = json.loads(subprocess.run(['php', '-r', 'require $argv[1]; echo json_encode(ReleaseProxy::TARGETS);', str(public / '_app/ReleaseProxy.php')], capture_output=True, text=True, check=True).stdout)
    # ReleaseProxy reads a private, fresh cache with all seven synthetic packages.
    cache = root / ('sadptools-downloads-' + str(os.geteuid()))
    cache.mkdir()
    payload = b'SADPTools synthetic package fixture\n' * 1024
    digest = hashlib.sha256(payload).hexdigest()
    (cache / (digest + '.bin')).write_bytes(payload)
    version = '0.6.0-rc.2'
    assets = {key: {'name': pattern % version, 'type': mime, 'size': len(payload), 'sha256': digest, 'url': 'https://example.invalid/not-used'} for key, (pattern, mime) in targets.items()}
    release = {'version': version, 'publishedAt': '2026-10-27T00:00:00Z', 'prerelease': True, 'assets': assets}
    (cache / 'release.json').write_text(json.dumps({'fetchedAt': int(time.time()), 'release': release}))
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0)); port = sock.getsockname()[1]
    env = os.environ | {'TMPDIR': tmp}
    (root / 'sessions').mkdir()
    server = subprocess.Popen(['php', '-d', 'session.save_path='+str(root / 'sessions'), '-S', f'127.0.0.1:{port}', '-t', str(public), str(root / 'scripts/router.php')], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    base = f'http://127.0.0.1:{port}'
    credentials = {'Authorization': 'Basic ' + base64.b64encode(b'fixture:synthetic-test-only').decode()}

    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, *args): return None
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), NoRedirect())
    def fetch(path, headers=None, method='GET', data=None):
        try:
            body = urllib.parse.urlencode(data, doseq=True).encode() if data is not None else None
            response = opener.open(urllib.request.Request(base + path, headers=headers or {}, method=method, data=body), timeout=10)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            return response.status, response.headers, response.read()

    def records():
        return [json.loads(line) for line in log.read_text().splitlines() if line]

    def hidden(body):
        class Fields(HTMLParser):
            def __init__(self): super().__init__(); self.values = {}
            def handle_starttag(self, tag, attrs):
                attrs = dict(attrs)
                if tag == 'input' and attrs.get('type') == 'hidden': self.values[attrs.get('name')] = attrs.get('value', '')
        parser = Fields(); parser.feed(body.decode()); return parser.values

    try:
        for attempt in range(100):
            try:
                with socket.create_connection(('127.0.0.1', port), timeout=.1): break
            except OSError: time.sleep(.02)
        report = '/assets/downloads_report.php'
        for suffix in ('', '?view=1', '?export=jsonl', '?export=csv', '?export=csv&q=Safari', '?password=synthetic-test-only'):
            status, headers, body = fetch(report + suffix)
            check(status == 401 and body == b'' and 'Basic' in headers.get('WWW-Authenticate', ''), 'Anonymous report/export denied')
            check('no-store' in headers['Cache-Control'], 'Unauthenticated report cannot be cached')
        status, _, _ = fetch(report, {'Authorization': 'Basic ' + base64.b64encode(b'fixture:wrong').decode()})
        check(status == 401, 'Wrong password denied')
        status, headers, body = fetch(report, credentials)
        check(status == 200 and b'No downloads have been recorded yet' in body, 'Authenticated empty report')
        check('no-store' in headers['Cache-Control'] and 'noindex' in headers['X-Robots-Tag'], 'Report is private and not indexed')
        status, headers, body = fetch(report + '?export=csv', credentials)
        check(status == 200 and len(list(csv.reader(io.StringIO(body.decode('utf-8-sig'))))) == 1, 'Empty CSV contains its header')
        for path in ('/assets/private-downloads/', '/assets/private-downloads/downloads.jsonl', '/assets/private-downloads/.htaccess', '/assets/%70rivate-downloads/downloads.jsonl', '/assets/screens/../private-downloads/downloads.jsonl', '/_app/download-report-auth.json'):
            check(fetch(path)[0] in (403, 404), 'Private file blocked: ' + path)
        agent = 'Synthetic <script>alert(1)</script> Firefox/130.0 (Linux)'
        for key in targets:
            check(fetch('/download/' + key, {'User-Agent': agent})[2] == payload, 'Unchanged package bytes: ' + key)
        rows = records()
        check(len(rows) == 7 and all(row['outcome'] == 'sent' for row in rows), 'All seven downloads recorded')
        check(all(row['version'] == version and row['bytes_sent'] == len(payload) for row in rows), 'Actual release and byte counts recorded')
        check(all(row['filename'] == assets[row['target']]['name'] for row in rows), 'Versioned filenames match selected assets')
        for path, method in (('/download/macos.dmg', 'HEAD'), ('/download/checksums.txt', 'GET'), ('/download/not-a-package', 'GET'), ('/download/linux.deb', 'POST')):
            fetch(path, method=method)
        check(len(records()) == 7, 'HEAD, checksums and invalid requests excluded')
        status, _, body = fetch('/download/macos.zip', {'Range': 'bytes=0-15'})
        check(status == 206 and body == payload[:16], 'Range still works')
        row = records()[-1]
        check(row['transfer'] == 'range' and row['bytes_sent'] == 16 and row['http_status'] == 206, 'Range recorded separately')
        check(fetch('/download/macos.zip', {'Range': 'bytes=999999999-'})[0] == 416, 'Invalid range rejected')
        check(records()[-1]['outcome'] == 'invalid_range' and records()[-1]['transfer'] == 'range', 'Invalid range outcome retained')
        fetch('/download/macos.zip', {'Range': 'bytes=0-15', 'If-Range': 'wrong-etag'})
        check(records()[-1]['transfer'] == 'full' and records()[-1]['bytes_sent'] == len(payload), 'Ignored range recorded as full transfer')
        with open(cache / (digest + '.bin.lock'), 'w') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            check(fetch('/download/linux.deb')[0] == 503, 'Busy package fails gracefully')
        check(records()[-1]['outcome'] == 'failed' and records()[-1]['bytes_sent'] == 0, 'Failed transfer recorded')
        # Disconnect during a large cached transfer; shutdown must still log it.
        large = root / 'large-fixture.bin'; large_hash = hashlib.sha256()
        with large.open('wb') as file:
            for _ in range(64):
                chunk = b'X' * 1048576; file.write(chunk); large_hash.update(chunk)
        large_digest = large_hash.hexdigest(); large.rename(cache / (large_digest + '.bin'))
        release['assets']['windows-setup.exe'] = assets['windows-setup.exe'] | {'sha256': large_digest, 'size': 64 * 1048576}
        (cache / 'release.json').write_text(json.dumps({'fetchedAt': int(time.time()), 'release': release}))
        before_count = len(records())
        with socket.create_connection(('127.0.0.1', port), timeout=10) as client:
            client.sendall(b'GET /download/windows-setup.exe HTTP/1.1\r\nHost: localhost\r\nUser-Agent: Synthetic-Abort-Test\r\nConnection: close\r\n\r\n')
            check(bool(client.recv(1024)), 'Interrupted transfer started')
            client.setsockopt(socket.SOL_SOCKET, socket.SO_LINGER, struct.pack('ii', 1, 0))
        for attempt in range(100):
            if len(records()) > before_count: break
            time.sleep(.02)
        check(records()[-1]['outcome'] == 'interrupted' and records()[-1]['bytes_sent'] < 64 * 1048576, 'Disconnected download recorded as interrupted')
        status, _, body = fetch(report, credentials)
        check(status == 200 and b'&lt;script&gt;alert(1)&lt;/script&gt;' in body and b'<script>alert(1)' not in body, 'User agent escaped in report')
        check(b'Pacific/Auckland' in body and version.encode() in body, 'Report shows local time zone and release')
        status, headers, body = fetch(report + '?export=jsonl', credentials)
        check(status == 200 and len(body.splitlines()) == len(records()), 'Authenticated JSON export contains all records')
        check('attachment;' in headers['Content-Disposition'], 'Export is an attachment')
        auth_path.unlink()
        check(fetch(report, credentials)[0] == 503, 'Missing auth config fails closed')
        auth_path.write_text(json.dumps(auth))
        # A broken/missing log must not prevent users from downloading the app.
        log.rename(log.with_suffix('.saved'))
        check(fetch('/download/macos.zip')[2] == payload, 'Logging failure cannot break a download')
        check(fetch(report, credentials)[0] == 503, 'Unavailable log is reported without revealing paths')
        log.with_suffix('.saved').rename(log)
        before = log.read_bytes()
        # Provisioning preserves data, ownership and requested permissions.
        provision = json.dumps({'document_root': str(public), 'auth': auth})
        subprocess.run(['php', str(source / 'scripts/prepare-download-logging.php')], input=provision, text=True, stdout=subprocess.DEVNULL, check=True)
        check(log.read_bytes() == before, 'Reprovisioning never truncates records')
        check(log.stat().st_mode & 0o777 == 0o777 and log.parent.stat().st_mode & 0o777 == 0o777, 'Requested writable modes preserved')
        check(log.stat().st_uid == os.geteuid(), 'Log ownership preserved')
        # Keep memory bounded and render only the newest 200 rows in a large log.
        sample = records()[0]
        with log.open('a') as file:
            file.write(json.dumps(sample | {'id': 'early-history-fixture', 'started_at': '2026-01-01T12:30:00+00:00',
                'browser': 'Safari', 'os': 'macOS', 'ip': '192.0.2.2', 'target': 'macos.dmg', 'bytes_sent': 9,
                'user_agent': 'Synthetic early-history, "quoted"\n<script>alert(1)</script>'}) + '\n')
            file.write(json.dumps(sample | {'id': 'csv-formula-fixture', 'user_agent': '=HYPERLINK("https://example.invalid", "synthetic")'}) + '\n')
            for index in range(220):
                file.write(json.dumps(sample | {'id': f'synthetic-extra-{index}', 'bytes_sent': 100 + index,
                    'version': '0.6.0-rc.10' if index % 2 else '0.6.0-rc.2'}) + '\n')
        status, headers, body = fetch(report + '?export=csv&q=EARLY-HISTORY%20safari&sort=bytes&direction=asc&page=2', credentials)
        exported = list(csv.DictReader(io.StringIO(body.decode('utf-8-sig'))))
        check(status == 200 and headers['Content-Type'].startswith('text/csv') and 'no-store' in headers['Cache-Control'], 'CSV is authenticated, non-cacheable and correctly typed')
        check(len(exported) == 1 and exported[0]['Request ID'] == 'early-history-fixture', 'Search covers older records and CSV ignores page limits')
        check(exported[0]['User agent'] == 'Synthetic early-history, "quoted"\n<script>alert(1)</script>', 'CSV preserves quotes, commas, Unicode-compatible text and newlines')
        status, _, body = fetch(report + '?export=csv&sort=bytes&direction=desc', credentials)
        exported = list(csv.DictReader(io.StringIO(body.decode('utf-8-sig'))))
        values = [int(row['Bytes sent']) for row in exported]
        check(len(exported) == len(records()) and values == sorted(values, reverse=True), 'CSV exports every page in selected numeric order')
        formula = next(row for row in exported if row['Request ID'] == 'csv-formula-fixture')
        check(formula['User agent'].startswith("'=HYPERLINK"), 'Formula-like agent exported as text')
        status, _, body = fetch(report + '?export=jsonl&q=early-history', credentials)
        check(status == 200 and len(body.splitlines()) == 1 and json.loads(body)['id'] == 'early-history-fixture', 'JSON export honours the same search')
        for query in ('?q%5B%5D=bad', '?sort=unknown', '?direction=invalid', '?page=-1', '?export=bad'):
            check(fetch(report + query, credentials)[0] == 400, 'Invalid query rejected')
        check(b'No download records match your search.' in fetch(report + '?q=no-such-record', credentials)[2], 'Empty search result message')
        status, _, body = fetch(report + '?q=%22%3E%3Cscript%3Ebad%3C%2Fscript%3E', credentials)
        check(status == 200 and b'<script>bad</script>' not in body and b'&lt;script&gt;bad&lt;/script&gt;' in body, 'Search text is HTML-escaped')
        if os.environ.get('REPORT_BROWSER_CHECKS') == '1':
            subprocess.run(['node', str(source / 'tests/report-browser.mjs')],
                           env=os.environ | {'REPORT_TEST_URL': base + report}, check=True)
        check(fetch(report, credentials)[2].count(b'<tr>') == 201, 'Report bounds rendered rows to latest 200 plus header')
        # Deletion is POST-only, session-bound and confirmed with an exact snapshot.
        status, headers, body = fetch(report, credentials); fields = hidden(body)
        before = log.read_bytes()
        check(fetch(report, method='POST', data={'action':'preview'})[0] == 401, 'Anonymous deletion denied')
        check(fetch(report, credentials, 'POST', {'action':'confirm'})[0] == 403, 'Missing CSRF denied')
        check(fetch(report, credentials, 'POST', fields | {'csrf':'wrong','single_id':'early-history-fixture'})[0] == 403, 'Wrong CSRF denied')
        check(fetch(report, credentials | {'Origin':'https://evil.invalid'}, 'POST', fields | {'single_id':'early-history-fixture'})[0] == 403, 'Cross-site deletion denied')
        fetch(report+'?action=confirm&single_id=early-history-fixture', credentials)
        check(log.read_bytes() == before, 'GET and rejected requests never change the log')
        status, _, body = fetch(report, credentials, 'POST', fields | {'ids[]':['synthetic-extra-0','synthetic-extra-1']})
        check(status == 200 and b'Delete 2 download records?' in body and log.read_bytes() == before, 'Selection previews without deleting')
        confirmation = hidden(body)
        check(fetch(report, credentials, 'POST', confirmation | {'action':'cancel'})[0] == 303 and log.read_bytes() == before, 'Cancel preserves all records')
        check(fetch(report, credentials, 'POST', confirmation | {'action':'confirm'})[0] == 400, 'Cancelled confirmation cannot be replayed')
        match_count = sum(r['id'].startswith('synthetic-extra-') for r in records()) - 1
        status, _, body = fetch(report, credentials, 'POST', fields | {'q':'synthetic-extra','all_matching':'1','excluded[]':['synthetic-extra-0']})
        check(status == 200 and f'Delete {match_count} download records?'.encode() in body, 'Select all matches spans every page and honours exceptions')
        confirmation = hidden(body)
        with log.open('a') as file: file.write(json.dumps(sample | {'id':'synthetic-extra-late'}) + '\n')
        original_stat = log.stat()
        check(fetch(report, credentials, 'POST', confirmation | {'action':'confirm','ids[]':['early-history-fixture']})[0] == 303, 'Confirmed snapshot deleted')
        kept = {r['id'] for r in records()}
        check('synthetic-extra-0' in kept and 'synthetic-extra-late' in kept and 'early-history-fixture' in kept and 'synthetic-extra-1' not in kept, 'New arrivals, excluded rows and nonmatching rows survive; injected IDs ignored')
        check(log.stat().st_ino == original_stat.st_ino and log.stat().st_uid == original_stat.st_uid and log.stat().st_mode & 0o777 == 0o777, 'Deletion preserves owner, inode and permissions')
        before = log.read_bytes()
        check(fetch(report, credentials, 'POST', confirmation | {'action':'confirm'})[0] == 400 and log.read_bytes() == before, 'Repeated confirm cannot delete again')
        status, _, body = fetch(report, credentials, 'POST', fields | {'all_matching':'1','single_id':'early-history-fixture'})
        check(status == 200 and b'Delete 1 download record?' in body, 'Individual delete ignores bulk selection')
        check(fetch(report, credentials, 'POST', hidden(body) | {'action':'confirm'})[0] == 303, 'Individual row deleted')
        check('early-history-fixture' not in {r['id'] for r in records()}, 'Individual deletion changes only its row')
        print(f'{checks} isolated download/report HTTP assertions passed')
    finally:
        server.terminate(); server.wait(timeout=5)

#!/usr/bin/env python3
"""Create a private report login once; provision records without truncating them."""
import argparse, json, os, pathlib, re, secrets, shlex, subprocess

root = pathlib.Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--local', action='store_true', help='prepare the local PHP preview instead of the web host')
args = parser.parse_args()
secret = root / '.download-report-credentials.json'
subprocess.run(['git', 'check-ignore', '--quiet', str(secret)], cwd=root, check=True)
if not secret.exists():
    password = secrets.token_urlsafe(24)
    hashed = subprocess.run(['php', '-r', 'echo password_hash(stream_get_contents(STDIN), PASSWORD_DEFAULT);'],
                            input=password, text=True, capture_output=True, check=True).stdout
    credentials = {'username': 'paul', 'password': password, 'password_hash': hashed}
    fd = os.open(secret, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as file:
        json.dump(credentials, file, indent=2)
        file.write('\n')
if secret.is_symlink() or secret.stat().st_mode & 0o077:
    raise SystemExit('Keep the report credentials file private (mode 600, no symlink).')
credentials = json.loads(secret.read_text())
auth = {key: credentials[key] for key in ('username', 'password_hash')}
if args.local:
    document_root = str(root / 'public')
    command = ['php', str(root / 'scripts/prepare-download-logging.php')]
else:
    connection = root / '.credentials.json'
    if connection.stat().st_mode & 0o077:
        raise SystemExit('Keep the SSH connection file private (mode 600).')
    config = json.loads(connection.read_text())
    host, document_root = config['ssh'], config['document_root']
    if not re.fullmatch(r'[A-Za-z0-9_.-]+@[A-Za-z0-9_.-]+', host) or not re.fullmatch(r'/var/www/html/[A-Za-z0-9_.-]+', document_root):
        raise SystemExit('Invalid deployment destination.')
    code = (root / 'scripts/prepare-download-logging.php').read_text().removeprefix('<?php')
    command = ['ssh', '-o', 'BatchMode=yes', host, 'php -r ' + shlex.quote(code)]
subprocess.run(command, input=json.dumps({'document_root': document_root, 'auth': auth}), text=True, check=True)
print('Report username: ' + credentials['username'])
print('Password saved privately in ' + str(secret) + ' (not printed or deployed).')

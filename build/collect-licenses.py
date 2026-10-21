#!/usr/bin/env python3
"""Collect pinned runtime/build dependency licences for portable distribution."""
import json
import os
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parent.parent
os.chdir(root)
raw = ''
for platform, cgo in [('windows', '0'), ('linux', '1')]:
    env = dict(os.environ, GOOS=platform, GOARCH='amd64', CGO_ENABLED=cgo)
    raw += subprocess.check_output(['go', 'list', '-deps', '-tags', 'desktop', '-f', '{{if .Module}}{{.Module.Dir}}{{end}}', '.'], env=env, text=True)
entries = []
for directory in sorted(set(raw.splitlines())):
    if not directory or Path(directory) == root:
        continue
    folder = Path(directory)
    for path in sorted(folder.iterdir()):
        if path.is_file() and path.name.lower().startswith(('license', 'copying')):
            entries.append((folder.name + '/' + path.name, path.read_text(errors='replace')))
for package in ['@wailsio/runtime', 'bootstrap', '@popperjs/core', 'vite', 'rolldown', 'lightningcss']:
    folder = root / 'frontend/node_modules' / package
    if not folder.exists():
        continue
    for path in sorted(folder.iterdir()):
        if path.is_file() and path.name.lower().startswith(('license', 'copying')):
            entries.append((package + '/' + path.name, path.read_text(errors='replace')))
(root / 'LICENSES/dependencies.txt').write_text('\n\n'.join(name + '\n' + '=' * len(name) + '\n\n' + body for name, body in entries) + '\n')
print(f'Collected {len(entries)} dependency licence files.')

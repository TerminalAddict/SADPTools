#!/usr/bin/env python3
"""Publish only public/ using an ignored SSH connection file. No sudo or deletion."""
import json, pathlib, re, shlex, subprocess
root=pathlib.Path(__file__).resolve().parents[1]
credentials=root/'.credentials.json'
if credentials.stat().st_mode & 0o077:
 raise SystemExit('Keep .credentials.json private: chmod 600 web/.credentials.json')
config=json.loads(credentials.read_text())
host=config['ssh']; target=config['document_root']
if not re.fullmatch(r'[A-Za-z0-9_.-]+@[A-Za-z0-9_.-]+',host): raise SystemExit('Invalid SSH target')
if not re.fullmatch(r'/var/www/html/[A-Za-z0-9_.-]+',target): raise SystemExit('Invalid document root')
subprocess.run(['git','check-ignore','--quiet',str(credentials)],cwd=root,check=True)
subprocess.run(['ssh','-o','BatchMode=yes',host,'test -d '+shlex.quote(target)+' && test -w '+shlex.quote(target)],check=True)
subprocess.run(['rsync','-az','--delay-updates','--chmod=D755,F644','-e','ssh -o BatchMode=yes',str(root/'public')+'/',host+':'+target+'/'],check=True)
print('Published public files to '+config['site_url'])

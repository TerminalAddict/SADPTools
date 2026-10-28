#!/usr/bin/env python3
"""Publish only public/ using an ignored SSH connection file. No sudo or deletion."""
import argparse, json, pathlib, re, shlex, subprocess, sys
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--skip-search-submit',action='store_true',help='Deploy without notifying IndexNow')
args=parser.parse_args()
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
# Install the directory guard first. Never sync data, backups or report secrets,
# or reset the maintainer's writable directory permissions during deployment.
private=target+'/assets/private-downloads'
subprocess.run(['ssh','-o','BatchMode=yes',host,'mkdir -p '+shlex.quote(private)],check=True)
subprocess.run(['rsync','-az','--chmod=F644','-e','ssh -o BatchMode=yes',str(root/'public/assets/private-downloads/.htaccess'),host+':'+private+'/.htaccess'],check=True)
subprocess.run(['rsync','-az','--delay-updates','--chmod=D755,F644','--exclude=/assets/private-downloads/***','--exclude=/_app/download-report-auth.json*','-e','ssh -o BatchMode=yes',str(root/'public')+'/',host+':'+target+'/'],check=True)
print('Published public files to '+config['site_url'],flush=True)
if not args.skip_search_submit:
 result=subprocess.run([sys.executable,str(root/'scripts/submit_search.py')])
 if result.returncode:
  raise SystemExit('Deployment completed, but IndexNow notification failed. Run python3 web/scripts/submit_search.py after checking the reported error.')

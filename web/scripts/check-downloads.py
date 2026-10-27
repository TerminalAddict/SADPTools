#!/usr/bin/env python3
"""Exercise deployed routes and hash every full download without retaining binaries."""
import hashlib,json,os,pathlib,urllib.request,urllib.error
base=os.environ.get('SITE_URL','https://sadptools.com').rstrip('/')
class NoRedirect(urllib.request.HTTPRedirectHandler):
 def redirect_request(self,req,fp,code,msg,headers,newurl):
  raise AssertionError('A first-party endpoint redirected to another URL')
opener=urllib.request.build_opener(NoRedirect)
def fetch(path,headers=None,method='GET'):
 return opener.open(urllib.request.Request(base+path,headers=headers or {},method=method),timeout=240)
with fetch('/api/releases') as r:
 metadata=r.read(); assert r.status==200; release=json.loads(metadata)
assert b'github' not in metadata.lower()
results=[]
for key,asset in release['assets'].items():
 path=asset['download']; assert path=='/download/'+key
 with fetch(path,method='HEAD') as r:
  assert r.status==200 and int(r.headers['Content-Length'])==asset['size']
  assert r.headers['X-Download-SHA256']==asset['sha256']
 digest=hashlib.sha256(); size=0; first=b''
 with fetch(path) as r:
  assert r.status==200 and r.url==base+path
  assert asset['name'] in r.headers['Content-Disposition']
  etag=r.headers['ETag']
  while chunk:=r.read(1048576):
   if not first:first=chunk[:16]
   size+=len(chunk);digest.update(chunk)
 assert size==asset['size'] and digest.hexdigest()==asset['sha256'],key
 with fetch(path,{'Range':'bytes=0-15','If-Range':etag}) as r:
  assert r.status==206 and r.read()==first and r.headers['Content-Range']==f'bytes 0-15/{size}'
 try:
  fetch(path,{'Range':f'bytes={size}-'})
  raise AssertionError('Out-of-range request accepted')
 except urllib.error.HTTPError as e: assert e.code==416
 results.append({'route':path,'filename':asset['name'],'bytes':size,'sha256':digest.hexdigest(),'fullDownload':'passed','rangeResume':'passed'})
 print('Verified',key,flush=True)
for path in ['/download/not-a-package.exe','/download.php?target=https://example.com','/download.php?target%5B%5D=linux.deb','/not-a-page']:
 try:fetch(path);raise AssertionError('Missing route accepted')
 except urllib.error.HTTPError as e: assert e.code==404,(path,e.code)
for path in ['/_app/ReleaseProxy.php','/.credentials.json','/.git/config','/wordfence-waf.php']:
 try:fetch(path);raise AssertionError('Private route exposed')
 except urllib.error.HTTPError as e:assert e.code in (403,404),(path,e.code)
try:fetch('/download/linux.deb',method='POST');raise AssertionError('POST accepted')
except urllib.error.HTTPError as e:assert e.code==405
with fetch('/download/checksums.txt') as r:
 hashes=r.read().decode();assert 'github' not in hashes.lower()
 for a in release['assets'].values():assert a['sha256']+'  '+a['name'] in hashes
out=pathlib.Path(__file__).resolve().parents[1]/'screenshots/download-report.json'
out.write_text(json.dumps({'base':base,'version':release['version'],'results':results,'negativeChecks':'passed'},indent=2)+'\n')
print('All five downloads, checksums, range requests and private-route protections passed.')

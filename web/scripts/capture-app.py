#!/usr/bin/env python3
"""Capture the actual native app on an isolated display with synthetic site data.
Run under xvfb-run and dbus-run-session. Never uses the normal app profile.
"""
import ctypes, json, os, pathlib, re, subprocess, time
root=pathlib.Path(__file__).resolve().parents[2]
state=root/'web/.cache/app-capture'
state.mkdir(parents=True,exist_ok=True)
config=state/'config'; (config/'SADPTools').mkdir(parents=True,exist_ok=True)
if not (config/'SADPTools/sites.sqlite').exists():
 subprocess.run(['go','run',str(root/'web/scripts/seed-demo.go'),str(config)],cwd=root,check=True)
x=ctypes.CDLL('libX11.so.6'); xt=ctypes.CDLL('libXtst.so.6')
x.XOpenDisplay.argtypes=[ctypes.c_char_p]; x.XOpenDisplay.restype=ctypes.c_void_p
d=x.XOpenDisplay(None)
x.XFlush.argtypes=[ctypes.c_void_p]
xt.XTestFakeMotionEvent.argtypes=[ctypes.c_void_p,ctypes.c_int,ctypes.c_int,ctypes.c_int,ctypes.c_ulong]
xt.XTestFakeButtonEvent.argtypes=[ctypes.c_void_p,ctypes.c_uint,ctypes.c_int,ctypes.c_ulong]
def click(a,b):
 xt.XTestFakeMotionEvent(d,0,a,b,0); xt.XTestFakeButtonEvent(d,1,1,0); xt.XTestFakeButtonEvent(d,1,0,0); x.XFlush(d); time.sleep(1)
for theme in ['light','dark']:
 (config/'SADPTools/settings.json').write_text(json.dumps({'protocols':['sadp'],'theme':theme,'timeoutSeconds':3,'intervalSeconds':30,'automatic':False,'checkUpdates':False,'updateChannel':'beta'}))
 env=dict(os.environ,XDG_CONFIG_HOME=str(config),XDG_DATA_HOME=str(state/'data'),XDG_CACHE_HOME=str(state/'cache'),GSK_RENDERER='cairo',WEBKIT_DISABLE_DMABUF_RENDERER='1',LIBGL_ALWAYS_SOFTWARE='1',GTK_USE_PORTAL='0')
 with open(state/f'{theme}.log','w') as log:
  process=subprocess.Popen([str(root/'bin/SADPTools-linux-amd64')],env=env,stdout=log,stderr=log)
  try:
   time.sleep(5)
   # Native app navigation: Sites. Screenshot coordinates documented with viewport.
   click(155,164)
   tree=subprocess.check_output(['xwininfo','-root','-tree'],text=True)
   match=re.search(r'(0x[0-9a-f]+) "SADPTools"',tree)
   if not match: raise RuntimeError('Native SADPTools window not found')
   subprocess.run(['import','-window',match[1],str(root/f'web/public/assets/app-{theme}.png')],check=True)
  finally:
   process.terminate()
   try: process.wait(timeout=8)
   except subprocess.TimeoutExpired: process.kill(); process.wait()
print('Captured actual app screenshots with synthetic data; no network actions.')

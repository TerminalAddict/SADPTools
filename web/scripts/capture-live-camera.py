#!/usr/bin/env python3
"""Read-only screenshots of an explicitly authorised RTSP camera.

Prompted credentials stay in process memory and are entered into the app's
session-only sign-in form. No credential arguments, environment vars, saved
profiles, HTTP camera requests, camera writes, or protocol logs. A temporary
local relay keeps the remote address out of the public screenshot. Run under
an isolated Xvfb/private DBus session; the normal released app renders the UI.
"""
import ctypes
import getpass
import json
import os
from pathlib import Path
import re
import resource
import selectors
import shutil
import socket
import subprocess
import tempfile
import threading
import time

resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
os.umask(0o077)
root = Path(__file__).resolve().parents[2]
raw = root/'web/.cache/live-camera-capture'
raw.mkdir(parents=True, exist_ok=True)
# XDG state lives in RAM and is removed after the session. It never contains a
# credential profile; media/transport bytes are not written by this script.
state = Path(tempfile.mkdtemp(prefix='sadptools-capture-',dir='/dev/shm'))
config = state/'config'
(config/'SADPTools').mkdir(parents=True)
x = ctypes.CDLL('libX11.so.6'); xt = ctypes.CDLL('libXtst.so.6')
x.XOpenDisplay.argtypes=[ctypes.c_char_p]; x.XOpenDisplay.restype=ctypes.c_void_p
D=x.XOpenDisplay(None)
x.XFlush.argtypes=[ctypes.c_void_p]
x.XStringToKeysym.argtypes=[ctypes.c_char_p];x.XStringToKeysym.restype=ctypes.c_ulong
x.XKeysymToKeycode.argtypes=[ctypes.c_void_p,ctypes.c_ulong];x.XKeysymToKeycode.restype=ctypes.c_uint
x.XResizeWindow.argtypes=[ctypes.c_void_p,ctypes.c_ulong,ctypes.c_uint,ctypes.c_uint]
x.XMoveWindow.argtypes=[ctypes.c_void_p,ctypes.c_ulong,ctypes.c_int,ctypes.c_int]
xt.XTestFakeMotionEvent.argtypes=[ctypes.c_void_p,ctypes.c_int,ctypes.c_int,ctypes.c_int,ctypes.c_ulong]
xt.XTestFakeButtonEvent.argtypes=[ctypes.c_void_p,ctypes.c_uint,ctypes.c_int,ctypes.c_ulong]
xt.XTestFakeKeyEvent.argtypes=[ctypes.c_void_p,ctypes.c_uint,ctypes.c_int,ctypes.c_ulong]

def window(viewer=True):
 tree=subprocess.check_output(['xwininfo','-root','-tree'],text=True)
 pattern=r'(0x[0-9a-f]+) "Live View[^"\n]*"' if viewer else r'(0x[0-9a-f]+) "SADPTools"'
 m=re.search(pattern,tree)
 if not m: raise RuntimeError('Expected application window is absent')
 info=subprocess.check_output(['xwininfo','-id',m[1]],text=True)
 a=int(re.search(r'Absolute upper-left X:\s+(-?\d+)',info)[1]); b=int(re.search(r'Absolute upper-left Y:\s+(-?\d+)',info)[1])
 return m[1],a,b

def click(a,b,viewer=True):
 _,ox,oy=window(viewer);xt.XTestFakeMotionEvent(D,0,ox+a,oy+b,0)
 xt.XTestFakeButtonEvent(D,1,1,0);xt.XTestFakeButtonEvent(D,1,0,0);x.XFlush(D);time.sleep(.4)

def key(name,down=None):
 code=x.XKeysymToKeycode(D,x.XStringToKeysym(name.encode()))
 if down is None:
  xt.XTestFakeKeyEvent(D,code,1,0);xt.XTestFakeKeyEvent(D,code,0,0)
 else: xt.XTestFakeKeyEvent(D,code,int(down),0)
 x.XFlush(D);time.sleep(.04)

def type_text(value):
 for char in value:
  shift=char.isupper()
  if shift:key('Shift_L',True)
  key(char.lower() if shift else char)
  if shift:key('Shift_L',False)

def scroll(steps):
 _,ox,oy=window();xt.XTestFakeMotionEvent(D,0,ox+700,oy+650,0)
 for _ in range(abs(steps)):
  button=5 if steps>0 else 4
  xt.XTestFakeButtonEvent(D,button,1,0);xt.XTestFakeButtonEvent(D,button,0,0)
 x.XFlush(D);time.sleep(.5)

def capture(name,theme):
 wid,ox,oy=window();xt.XTestFakeMotionEvent(D,0,ox+15,oy+10,0);x.XFlush(D);time.sleep(.3)
 subprocess.run(['import','-window',wid,str(raw/f'{name}-{theme}.png')],check=True)
 print('Captured',name,theme,flush=True)

stop=threading.Event(); peers=set(); guard=threading.Lock()
listener=socket.socket();listener.bind(('127.0.0.1',0));listener.listen(4);listener.settimeout(.5)

def relay(client, target):
 upstream=None
 try:
  upstream=socket.create_connection(target,timeout=5)
  client.settimeout(5);upstream.settimeout(5)
  with guard:peers.update((client,upstream))
  with selectors.DefaultSelector() as selection:
   selection.register(client,selectors.EVENT_READ,upstream)
   selection.register(upstream,selectors.EVENT_READ,client)
   while not stop.is_set():
    for event,_ in selection.select(.5):
     chunk=event.fileobj.recv(65536)
     if not chunk:return
     event.data.sendall(chunk)
 except OSError:pass
 finally:
  with guard:peers.discard(client);peers.discard(upstream)
  client.close()
  if upstream:upstream.close()

def accept(target):
 while not stop.is_set():
  try:client,_=listener.accept()
  except socket.timeout:continue
  except OSError:return
  threading.Thread(target=relay,args=(client,target),daemon=True).start()

app=None;password='';host='';username=''
try:
 host=getpass.getpass('Camera address (not saved): ')
 username=getpass.getpass('Username (not saved): ')
 password=getpass.getpass('Password (not saved): ')
 threading.Thread(target=accept,args=((host,554),),daemon=True).start()
 subprocess.run([str(root/'web/.cache/seed-live-capture'),str(config),str(listener.getsockname()[1])],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 env=dict(os.environ,XDG_CONFIG_HOME=str(config),XDG_DATA_HOME=str(state/'data'),XDG_CACHE_HOME=str(state/'cache'),GSK_RENDERER='cairo',WEBKIT_DISABLE_DMABUF_RENDERER='1',LIBGL_ALWAYS_SOFTWARE='1',GTK_USE_PORTAL='0')
 for theme in ('light','dark'):
  (config/'SADPTools/settings.json').write_text(json.dumps({'protocols':['sadp'],'theme':theme,'automatic':False,'checkUpdates':False,'timeoutSeconds':3,'intervalSeconds':30,'updateChannel':'beta'}))
  app=subprocess.Popen([str(root/'bin/SADPTools-linux-amd64')],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
  time.sleep(5);click(155,114,False);time.sleep(.7)
  # Saved table has a single manual entry. Inspect the native view before login.
  print('READY',theme,flush=True)
  while True:
   command=input('capture> ').strip().split()
   if not command:continue
   if command[0]=='click':click(int(command[1]),int(command[2]),len(command)<4 or command[3]!='main')
   elif command[0]=='shot':capture(command[1],theme)
   elif command[0]=='main':
    subprocess.run(['import','-window',window(False)[0],str(raw/f'main-{theme}.png')],check=True)
   elif command[0]=='login':
    # The native sign-in dialog focuses the password field.
    if username != 'admin':
     key('Shift_L',True);key('Tab');key('Shift_L',False)
     key('Control_L',True);key('a');key('Control_L',False);type_text(username);key('Tab')
    type_text(password);key('Return');time.sleep(8)
   elif command[0]=='resize':
    x.XResizeWindow(D,int(window()[0],16),int(command[1]),int(command[2]));x.XFlush(D);time.sleep(1)
   elif command[0]=='move':
    x.XMoveWindow(D,int(window()[0],16),int(command[1]),int(command[2]));x.XFlush(D);time.sleep(.3)
   elif command[0]=='scroll':scroll(int(command[1]))
   elif command[0]=='key':key(command[1])
   elif command[0]=='wait':time.sleep(min(int(command[1]),10))
   elif command[0]=='next':break
   elif command[0]=='quit':raise SystemExit()
  app.terminate()
  try:app.wait(timeout=5)
  except subprocess.TimeoutExpired:app.kill();app.wait()
  app=None
finally:
 password='';username='';host=''
 stop.set();listener.close()
 with guard:
  for peer in peers:
   try:peer.shutdown(socket.SHUT_RDWR)
   except OSError:pass
 if app:
  app.terminate()
  try:app.wait(timeout=5)
  except subprocess.TimeoutExpired:app.kill();app.wait()
 shutil.rmtree(state,ignore_errors=True)
 print('Closed camera connections and removed temporary session state.',flush=True)

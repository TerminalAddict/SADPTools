#!/usr/bin/env python3
"""Actual native window captures, isolated synthetic state, no physical device actions.
Run under xvfb-run + a private dbus-run-session. Raw captures stay ignored until reviewed.
"""
import ctypes,json,os,pathlib,re,subprocess,time
root=pathlib.Path(__file__).resolve().parents[2]
state=root/'web/.cache/feature-capture';state.mkdir(parents=True,exist_ok=True)
config=state/'config';(config/'SADPTools').mkdir(parents=True,exist_ok=True)
raw=state/'raw';raw.mkdir(exist_ok=True)
x=ctypes.CDLL('libX11.so.6');xt=ctypes.CDLL('libXtst.so.6')
x.XOpenDisplay.argtypes=[ctypes.c_char_p];x.XOpenDisplay.restype=ctypes.c_void_p;display=x.XOpenDisplay(None)
x.XFlush.argtypes=[ctypes.c_void_p]
x.XStringToKeysym.argtypes=[ctypes.c_char_p];x.XStringToKeysym.restype=ctypes.c_ulong
x.XKeysymToKeycode.argtypes=[ctypes.c_void_p,ctypes.c_ulong];x.XKeysymToKeycode.restype=ctypes.c_uint
xt.XTestFakeMotionEvent.argtypes=[ctypes.c_void_p,ctypes.c_int,ctypes.c_int,ctypes.c_int,ctypes.c_ulong]
xt.XTestFakeButtonEvent.argtypes=[ctypes.c_void_p,ctypes.c_uint,ctypes.c_int,ctypes.c_ulong]
xt.XTestFakeKeyEvent.argtypes=[ctypes.c_void_p,ctypes.c_uint,ctypes.c_int,ctypes.c_ulong]
def window(title='SADPTools'):
 tree=subprocess.check_output(['xwininfo','-root','-tree'],text=True)
 match=re.search(r'(0x[0-9a-f]+) "'+re.escape(title)+'"',tree)
 if not match: raise RuntimeError('Window not found: '+title)
 wid=match[1];info=subprocess.check_output(['xwininfo','-id',wid],text=True)
 a=int(re.search(r'Absolute upper-left X:\s+(-?\d+)',info)[1]);b=int(re.search(r'Absolute upper-left Y:\s+(-?\d+)',info)[1])
 return wid,a,b

def click(a,b,title='SADPTools'):
 _,ox,oy=window(title);xt.XTestFakeMotionEvent(display,0,a+ox,b+oy,0)
 xt.XTestFakeButtonEvent(display,1,1,0);xt.XTestFakeButtonEvent(display,1,0,0);x.XFlush(display);time.sleep(.5)
def key(name,hold=False):
 code=x.XKeysymToKeycode(display,x.XStringToKeysym(name.encode()));xt.XTestFakeKeyEvent(display,code,1,0)
 if not hold:xt.XTestFakeKeyEvent(display,code,0,0)
 x.XFlush(display);time.sleep(.06)
def up(name):
 code=x.XKeysymToKeycode(display,x.XStringToKeysym(name.encode()));xt.XTestFakeKeyEvent(display,code,0,0);x.XFlush(display)
def select(a,b,index):
 click(a,b);key('Home')
 for _ in range(index):key('Down')
 key('Return');time.sleep(.6)
def select_letter(a,b,letter):
 click(a,b);key(letter);key('Return');time.sleep(.6)
def scroll(steps,title='SADPTools',at=700):
 _,ox,oy=window(title);xt.XTestFakeMotionEvent(display,0,ox+at,oy+650,0)
 for _ in range(abs(steps)):
  xt.XTestFakeButtonEvent(display,5 if steps>0 else 4,1,0);xt.XTestFakeButtonEvent(display,5 if steps>0 else 4,0,0)
 x.XFlush(display);time.sleep(.6)
def capture(name,theme,title='SADPTools'):
 # Move the pointer away from controls so tooltips are not photographed.
 _,ox,oy=window(title);xt.XTestFakeMotionEvent(display,0,ox+20,oy+15,0);x.XFlush(display);time.sleep(.25)
 crop=['-crop','400x410+520+118','+repage'] if name=='settings' else []
 subprocess.run(['import','-window',window(title)[0],*crop,str(raw/f'{name}-{theme}.png')],check=True)
 print('Captured',name,theme,flush=True)
def nav(name):click({'discovery':80,'sites':155,'scanner':220,'provision':300,'audit':369,'switches':434,'commissioning':519}[name],114)
fixturelog=open(state/'fixture.log','w')
ready=state/'ready.json';ready.unlink(missing_ok=True)
fixture=subprocess.Popen([str(root/'web/.cache/feature-fixture'),str(config)],cwd=root,stdout=fixturelog,stderr=fixturelog)
try:
 for _ in range(100):
  if ready.exists():break
  if fixture.poll() is not None:raise RuntimeError('Fixture failed')
  time.sleep(.1)
 if not ready.exists():raise RuntimeError('Fixture did not become ready')
 for theme in os.environ.get('CAPTURE_THEMES','light,dark').split(','):
  (config/'SADPTools/settings.json').write_text(json.dumps({'protocols':['sadp'],'theme':theme,'timeoutSeconds':3,'intervalSeconds':30,'automatic':False,'checkUpdates':False,'updateChannel':'beta'}))
  env=dict(os.environ,XDG_CONFIG_HOME=str(config),XDG_DATA_HOME=str(state/'data'),XDG_CACHE_HOME=str(state/'cache'),GSK_RENDERER='cairo',WEBKIT_DISABLE_DMABUF_RENDERER='1',LIBGL_ALWAYS_SOFTWARE='1',GTK_USE_PORTAL='0')
  with open(state/f'{theme}.log','w') as log:
   app=subprocess.Popen([str(root/'web/.cache/SADPTools-capture')],env=env,stdout=log,stderr=log)
   try:
    time.sleep(5)
    if os.environ.get('CAPTURE_MODE')=='corrections':
     nav('sites');click(103,382);capture('add-ip',theme);key('Escape')
     nav('audit');capture('audit',theme);continue
    capture('discovery',theme)
    click(270,350);capture('discovery-details',theme)
    scroll(30,at=1250)
    click(1260,703);capture('network',theme);key('Escape')
    click(1260,740);time.sleep(.5);capture('recovery',theme);key('Escape')
    click(270,622);scroll(30,at=1250);click(1260,665);capture('activation',theme);key('Escape')
    nav('sites');capture('sites',theme)
    for name,a,b in [('add-ip',103,382),('credentials',592,280),('backup',757,280),('history',822,280),('device-details',1277,408)]:
     click(a,b);capture(name,theme);key('Escape');time.sleep(.25)
    for name in ['scanner','provision','audit','switches','commissioning']:
     nav(name);capture(name,theme)
    select_letter(260,388,'m');capture('nvr',theme)
    for name,a in [('events',340),('storage',458),('plans',559),('reports',667),('configuration',787),('remote',900)]:
     click(a,301)
     if name=='storage':click(1073,752);click(510,635)
     if name=='plans':click(227,464)
     capture(name,theme)
    click(95,301);capture('connectivity',theme)
    for index,name in enumerate(['connectivity','protocol','security','recordings','imaging','anpr','replacement','firmware','labels','support']):
     select(450,487,index)
     if name=='labels':click(87,765);scroll(7)
     capture(name,theme)
     if name=='labels':scroll(-30)
    nav('discovery');click(1380,45);capture('settings',theme);key('Escape')
    nav('sites');click(1210,408);time.sleep(6)
    (state/'windows.txt').write_text(subprocess.check_output(['xwininfo','-root','-tree'],text=True))
    matches=re.findall(r'(0x[0-9a-f]+) "([^"]*Live View[^"]*)"',(state/'windows.txt').read_text())
    if matches:
     title=matches[0][1];capture('live-view',theme,title)
     scroll(14,title);capture('live-tools',theme,title)
     click(150,702,title);scroll(15,title);capture('stream-info',theme,title)
     key('space');scroll(-100,title);scroll(14,title)
     click(150,742,title);scroll(18,title);capture('focus',theme,title)
     key('space');scroll(-100,title);scroll(14,title)
     click(150,782,title);scroll(18,title);capture('ptz',theme,title)
   finally:
    app.terminate()
    try:app.wait(timeout=10)
    except subprocess.TimeoutExpired:app.kill();app.wait()
finally:
 fixture.terminate()
 try:fixture.wait(timeout=10)
 except subprocess.TimeoutExpired:fixture.kill();fixture.wait()
 fixturelog.close()

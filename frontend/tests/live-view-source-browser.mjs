// Synthetic UI checks only. No camera URLs, credentials or hardware connections.
// Uses the website's pinned Playwright dependency, like site-transfer-browser.mjs.
import { chromium } from '../../web/node_modules/playwright/index.mjs';
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import assert from 'node:assert/strict';

const server = createServer(async (req, res) => {
  const path = new URL(req.url, 'http://localhost').pathname;
  res.setHeader('Content-Type', path === '/' ? 'text/html' : 'text/javascript');
  if (path === '/') return res.end('<!doctype html><html><head><title>Synthetic Live View</title><link rel="stylesheet" href="/bootstrap.css"><link rel="stylesheet" href="/toolkit.css"><link rel="stylesheet" href="/live-view.css"></head><body><script type="module">import {initLiveViewWindow} from "/live-view-window.js"; initLiveViewWindow();</script></body></html>');
  const css = { '/bootstrap.css':'../node_modules/bootstrap/dist/css/bootstrap.min.css', '/toolkit.css':'../src/toolkit.css', '/live-view.css':'../src/live-view.css' }[path];
  if (css) {res.setHeader('Content-Type','text/css');return res.end(await readFile(new URL(css,import.meta.url)));}
  if (path === '/api.js') return res.end(`
    export const Load=async()=>({siteID:'fixture-site',deviceID:'fixture-camera',theme:window.theme,detail:window.detail});
    export const Start=async(options)=>{window.calls.push(options);return{id:'fixture-ticket'}};
    export const Stop=async()=>{};
    export const Status=async()=>({state:'ready',message:'Synthetic waiting state',profiles:[],info:{},bytes:0,fragments:0});
    export const Heartbeat=async()=>{};
  `);
  if (path === '/vault.js') return res.end(`export const VaultStatus=async()=>({unlocked:!window.locked,automatic:false,mode:'passphrase'});export const UnlockVault=async()=>{window.locked=false};`);
  if (path === '/runtime.js') return res.end('export const Stream=()=>({close(){}});export const Window={Close:async()=>{}};');
  const allowed = ['live-view-window.js','live-view-source.js','live-view-player.js','live-view-tools.js','toolkit-ui.js','focus-math.js'];
  if (!allowed.includes(path.slice(1))) {res.writeHead(404);return res.end();}
  try {
    let source=await readFile(new URL(`../src/${path.slice(1)}`,import.meta.url),'utf8');
    if (path === '/live-view-window.js') source=source
      .replace('../bindings/github.com/TerminalAddict/SADPTools/liveviewservice.js','/api.js')
      .replace('../bindings/github.com/TerminalAddict/SADPTools/toolkitservice.js','/vault.js')
      .replace('@wailsio/runtime','/runtime.js').replace('import "./live-view.css";','');
    res.end(source);
  } catch {res.writeHead(500);res.end();}
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
const browser=await chromium.launch();
try {
  for (const theme of ['light','dark']) for (const scenario of ['saved','locked-saved','unknown','sadp','onvif']) {
    const page=await browser.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
    await page.addInitScript(({theme,scenario})=>{
      window.calls=[];window.theme=theme;window.locked=scenario==='locked-saved';
      window.detail={device:{ipv4:'192.0.2.41',vendor:'Synthetic vendor',protocols:scenario==='sadp'?['sadp']:[],xaddrs:scenario==='onvif'?['synthetic-advertisement']:[]},savedPreview:scenario.includes('saved'),credentials:scenario==='saved'?[{siteID:'fixture-site',recordID:'fixture-camera',label:'Synthetic assigned credential'}]:[]};
    },{theme,scenario});
    await page.goto(`http://127.0.0.1:${server.address().port}/`);
    await page.waitForSelector('#lv-method');
    if (scenario==='locked-saved') {
      await page.getByRole('heading',{name:'Unlock saved camera details'}).waitFor();
      assert.equal(await page.evaluate(()=>window.calls.length),0);
      await page.locator('input[name="passphrase"]').fill('SYNTHETIC-only');
      await page.getByRole('button',{name:'Unlock and view'}).click();
    }
    if (scenario==='unknown') {
      await page.waitForFunction(()=>document.querySelector('#lv-status').textContent.includes('Enter this camera'));
      assert.equal(await page.evaluate(()=>window.calls.length),0);
      assert.equal(await page.locator('#lv-advanced').evaluate(e=>e.open),true);
    } else {
      await page.waitForFunction(()=>window.calls.length===1);
      const options=await page.evaluate(()=>window.calls[0]);
      assert.equal(options.method,scenario.includes('saved')?'saved':scenario==='sadp'?'hikvision':'onvif');
      assert.equal(options.profile,'sub');
      if(scenario==='saved') assert.equal(options.credentialRecord,'fixture-camera');
      await page.locator('#lv-stop').click();
    }
    assert.deepEqual(errors,[]);
    await page.close();
  }
  console.log('Live View source, automatic start, locked endpoints and unknown-camera guidance passed in light/dark themes.');
} finally {await browser.close();await new Promise(resolve=>server.close(resolve));}

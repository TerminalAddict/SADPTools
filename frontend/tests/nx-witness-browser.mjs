// Synthetic NX metadata only. No actual camera/server/credential connections.
import { chromium } from '../../web/node_modules/playwright/index.mjs';
import AxeBuilder from '../../web/node_modules/@axe-core/playwright/dist/index.mjs';
import { createServer } from 'node:http';
import { readFile, mkdir } from 'node:fs/promises';
import assert from 'node:assert/strict';
const root=new URL('../../',import.meta.url);
const routes={'/nx-witness.js':'frontend/src/nx-witness.js','/toolkit-ui.js':'frontend/src/toolkit-ui.js','/bootstrap.css':'frontend/node_modules/bootstrap/dist/css/bootstrap.min.css','/style.css':'frontend/src/style.css','/toolkit.css':'frontend/src/toolkit.css'};
const server=createServer(async(req,res)=>{
 if(req.url==='/'){res.setHeader('Content-Type','text/html');res.end(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Synthetic NX Witness test</title><link rel="stylesheet" href="/bootstrap.css"><link rel="stylesheet" href="/style.css"><link rel="stylesheet" href="/toolkit.css"></head><body><main><h1>Synthetic site</h1><label>Site<select id="site"><option value="normal">Discovered site</option><option value="nx">Imported NX site</option></select></label><div id="actions"></div></main><script type="module">import {nxWitnessButton,openNXWitness} from '/nx-witness.js';const select=document.querySelector('#site');function render(){document.querySelector('#actions').innerHTML=nxWitnessButton({hasNXWitness:select.value==='nx'});const button=document.querySelector('#site-nx-witness');if(button)button.onclick=()=>openNXWitness(window.api,'fixture-site',fn=>{window.vaultListener=fn;return()=>window.vaultListener=null;});}select.onchange=render;render();</script></body></html>`);return;}
 const path=routes[req.url];if(!path){res.writeHead(404);res.end();return;}res.setHeader('Content-Type',path.endsWith('.css')?'text/css':'text/javascript');res.end(await readFile(new URL(path,root)));
});
await new Promise(r=>server.listen(0,'127.0.0.1',r));const browser=await chromium.launch();
try{
 for(const theme of ['light','dark'])for(const width of [1440,390]){
  const context=await browser.newContext({viewport:{width,height:960}});const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.addInitScript(({theme})=>{
   window.locked=false;window.reads=0;window.vaultListener=null;
   const info={version:1,importedAt:'2026-01-02T03:04:05Z',system:{name:'Synthetic campus',cloud_account:'fixture@example.invalid',properties:{auditTrailEnabled:'true'}},servers:[{name:'Recorder B',url:'http://192.0.2.80:7001',full_version:'synthetic',operating_system:'Synthetic OS',cpu:'Fixture CPU'},{name:'Recorder A',url:'http://192.0.2.81:7001'}],users:Array.from({length:55},(_,i)=>({account:'fixture-user-'+String(i).padStart(2,'0'),enabled:i!==0,role:'Viewer'})),storage:[{name:'Synthetic archive',server_name:'Recorder B',path:'/fixture/archive',recording_enabled:false,backup:true,stored_space_bytes:'0'}],licenses:[{name:'Synthetic licence',channels:'4',activations_remaining:null,deactivations_remaining:'0',key:'SYNTHETIC-KEY',license_block:'<script>fixture-only</script>'}]};
   window.api={VaultStatus:async()=>({unlocked:!window.locked,automatic:false,mode:'passphrase'}),UnlockVault:async()=>{window.locked=false;window.vaultListener?.()},NXWitness:async()=>{window.reads++;return structuredClone(info);}};
   document.addEventListener('DOMContentLoaded',()=>document.documentElement.dataset.bsTheme=theme);
  },{theme});
  await page.goto(`http://127.0.0.1:${server.address().port}/`,{waitUntil:'networkidle'});
  assert.equal(await page.locator('#site-nx-witness').count(),0);
  await page.locator('#site').selectOption('nx');await page.locator('#site-nx-witness').click();
  await page.getByRole('heading',{name:'NX Witness Server info'}).waitFor();
  assert.match(await page.locator('#nx-content').textContent(),/Synthetic campus/);
  await page.locator('[data-nx-section="servers"]').click();
  await page.locator('[data-nx-sort="name"]').click();
  assert.match(await page.locator('#nx-content tbody tr').first().textContent(),/Recorder A/);
  await page.locator('#nx-search').fill('Recorder B');assert.equal(await page.locator('#nx-content tbody tr').count(),1);
  await page.locator('[data-nx-section="users"]').click();assert.equal(await page.locator('#nx-content tbody tr').count(),50);
  await page.locator('#nx-next').click();assert.equal(await page.locator('#nx-content tbody tr').count(),5);
  await page.locator('#nx-search').fill('fixture-user-00');assert.equal(await page.locator('#nx-content tbody tr').count(),1);
  assert.match(await page.locator('#nx-content tbody').textContent(),/No/);
  await page.locator('[data-nx-section="storage"]').click();assert.match(await page.locator('#nx-content tbody').textContent(),/No/);
  await page.locator('[data-nx-section="licenses"]').click();
  await page.locator('#nx-content tbody td > details > summary').click();
  assert.match(await page.locator('#nx-content').textContent(),/Unknown/);
  assert.equal(await page.getByText('SYNTHETIC-KEY',{exact:true}).isVisible(),false);
  await page.getByText('Show licence key',{exact:true}).click();assert.equal(await page.getByText('SYNTHETIC-KEY',{exact:true}).isVisible(),true);
  assert.equal(await page.locator('#nx-content script').count(),0);
  const axe=await new AxeBuilder({page}).include('.nx-dialog').withTags(['wcag2a','wcag2aa']).analyze();
  assert.deepEqual(axe.violations.map(x=>({id:x.id,nodes:x.nodes.map(n=>n.target)})),[]);
  await mkdir(new URL('.cache/nx-witness-browser/',root),{recursive:true});
  await page.screenshot({path:new URL(`.cache/nx-witness-browser/${theme}-${width}.png`,root).pathname,fullPage:true});
  await page.evaluate(()=>{window.locked=true;window.vaultListener?.()});await page.waitForFunction(()=>!document.querySelector('.nx-dialog'));
  assert.equal(await page.getByText('SYNTHETIC-KEY',{exact:true}).count(),0);
  await page.locator('#site-nx-witness').click();await page.getByRole('heading',{name:'Unlock NX Witness information'}).waitFor();
  assert.equal(await page.evaluate(()=>window.reads),1);
  await page.locator('input[name="passphrase"]').fill('Synthetic passphrase');await page.getByRole('button',{name:'Unlock and view'}).click();
  await page.getByRole('heading',{name:'NX Witness Server info'}).waitFor();assert.equal(await page.evaluate(()=>window.reads),2);
  await page.locator('.nx-dialog button[type="submit"]').click();
  await page.locator('#site').selectOption('normal');assert.equal(await page.locator('#site-nx-witness').count(),0);
  assert.deepEqual(errors,[]);await context.close();
 }
 console.log('NX Witness UI passed in light/dark desktop/mobile: conditional visibility, sections, search, sorting, pagination, licence disclosure, vault locking and axe accessibility checks.');
}finally{await browser.close();await new Promise(r=>server.close(r));}

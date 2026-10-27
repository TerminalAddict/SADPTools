import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import assert from 'node:assert/strict';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const base=process.env.SITE_URL || 'http://127.0.0.1:8790';
const out=path.join(root,'screenshots'); await mkdir(out,{recursive:true});
const browser=await chromium.launch({headless:true});
const report=[];
async function featureChecks(page, theme, mobile = false) {
 const cards=page.locator('[data-feature]');
 assert.equal(await cards.count(),36);
 await page.locator('#feature-search').fill('retention');
 assert.equal(await page.locator('#feature-storage').isVisible(),true);
 assert.ok(await page.locator('[data-feature]:visible').count()<36);
 await page.locator('#feature-search').fill('no-such-feature-xyz');
 assert.equal(await page.locator('#feature-empty').isVisible(),true);
 await page.locator('#clear-feature-search').click();
 assert.equal(await page.locator('[data-feature]:visible').count(),36);
 await page.locator('[data-gallery]').first().click();
 await page.locator('#screen-viewer').waitFor({state:'visible'});
 assert.match(await page.locator('#viewer-image').getAttribute('src'),new RegExp(`-${theme}\\.png\\?v=[a-f0-9]+$`));
 await page.locator('[data-viewer-theme="dark"]').click();
 assert.match(await page.locator('#viewer-image').getAttribute('src'),/-dark\.png\?v=[a-f0-9]+$/);
 const title=await page.locator('#viewer-title').textContent();
 await page.locator('#viewer-next').click();
 assert.notEqual(await page.locator('#viewer-title').textContent(),title);
 await page.keyboard.press('ArrowLeft');
 assert.equal(await page.locator('#viewer-title').textContent(),title);
 await page.locator(`[data-viewer-theme="${theme}"]`).click();
 await page.locator('#viewer-image').evaluate(img=>img.decode());
 const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
 report.push({theme,route:`feature-viewer${mobile?'-mobile':''}`,violations:axe.violations.map(v=>({id:v.id,nodes:v.nodes.map(n=>({target:n.target,summary:n.failureSummary}))}))});
 assert.equal(await page.locator('#screen-viewer').evaluate(el=>el.scrollWidth<=el.clientWidth),true,'Viewer overflow');
 await page.screenshot({path:path.join(out,`feature-viewer-${mobile?'mobile-':''}${theme}.png`)});
 await page.keyboard.press('Escape');
 assert.equal(await page.locator('#screen-viewer').isVisible(),false);
 assert.equal(await page.locator('[data-gallery]').first().evaluate(el=>el===document.activeElement),true,'Viewer returns focus');
 assert.equal(await page.locator('[data-capture="Real camera"]').count(),4);
 for(const name of ['live-view','stream-info','focus','ptz']) {
  await page.locator(`#feature-${name} [data-gallery]`).click();
  assert.match(await page.locator('#viewer-counter').textContent(),/REAL CAMERA/);
  assert.match(await page.locator('#viewer-image').getAttribute('alt'),/real camera screen/);
  await page.locator('#viewer-image').evaluate(img=>img.decode());
  if(name==='focus') await page.screenshot({path:path.join(out,`real-camera-${mobile?'mobile-':''}${theme}.png`)});
  await page.keyboard.press('Escape');
 }
 // Every lazy thumbnail is loaded before capturing the complete catalogue.
 for(const img of await page.locator('[data-screen]').all()) {
  await img.scrollIntoViewIfNeeded(); await img.evaluate(el=>el.decode());
 }
 await page.evaluate(()=>scrollTo({top:0,behavior:'instant'}));
 await page.screenshot({path:path.join(out,`features-${mobile?'mobile-':''}${theme}.png`),fullPage:true});
 if(!mobile) await page.screenshot({path:path.join(out,`features-hero-${theme}.png`)});
}
try {
 for (const theme of ['light','dark']) {
  const context=await browser.newContext({viewport:{width:1440,height:1080},colorScheme:theme,deviceScaleFactor:1});
  const page=await context.newPage(); const errors=[]; const external=[];
  page.on('pageerror',error=>errors.push(error.message));
  page.on('request',r=>{if(new URL(r.url()).origin!==new URL(base).origin) external.push(r.url());});
  for (const route of ['/','/features.html','/windows-help.html','/mac-help.html','/privacy.html']) {
   await page.goto(base+route,{waitUntil:'networkidle'});
   await page.evaluate(()=>document.fonts.ready);
   assert.equal(await page.locator('html').getAttribute('data-theme'),theme);
   assert.equal(await page.locator('h1').count(),1);
   assert.match(await page.locator('.footer-bottom').textContent(),/Windows \+ Linux \+ Apple Mac/);
   assert.match(await page.locator('meta[property="og:image:alt"]').getAttribute('content'),/Windows, Linux and Apple Mac/);
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true,'Horizontal overflow');
   const broken=await page.locator('img').evaluateAll(images=>images.filter(i=>i.loading!=='lazy' && (!i.complete||i.naturalWidth===0)).map(i=>i.src));
   assert.deepEqual(broken,[],'Broken images');
   const json=await page.locator('script[type="application/ld+json"]').textContent(); JSON.parse(json);
   assert.equal(await page.locator('meta[property="og:image"]').count(),1);
   assert.equal(await page.locator('a[href*="github" i]').count(),0);
   const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
   report.push({theme,route,violations:axe.violations.map(v=>({id:v.id,description:v.description,nodes:v.nodes.map(n=>({target:n.target,summary:n.failureSummary}))}))});
   if(route==='/'){
    await page.locator('#screenshots').scrollIntoViewIfNeeded();
    await page.locator('[data-preview-theme="dark"]').click();
    assert.match(await page.locator('#preview-image').getAttribute('src'),/app-dark/);
    await page.locator(`[data-preview-theme="${theme}"]`).click();
    await page.evaluate(()=>scrollTo({top:0,behavior:'instant'}));
    await page.screenshot({path:path.join(out,`homepage-${theme}.png`),fullPage:true});
    await page.screenshot({path:path.join(out,`hero-${theme}.png`)});
    assert.equal(await page.locator('[data-download]').count(),7);
    assert.match(await page.locator('.mac-card').textContent(),/Apple Silicon \+ Intel/);
    assert.equal(await page.locator('.mac-card a[href="/download/macos.dmg"]').count(),1);
    assert.match(await page.locator('script[type="application/ld+json"]').textContent(),/macOS 12 or later/);
    await page.locator('#downloads').scrollIntoViewIfNeeded();
    await page.screenshot({path:path.join(out,`downloads-${theme}.png`)});
    const original=theme; await page.locator('.theme-toggle').click();
    assert.notEqual(await page.locator('html').getAttribute('data-theme'),original);
    await page.reload({waitUntil:'networkidle'});
    assert.notEqual(await page.locator('html').getAttribute('data-theme'),original,'Theme persists');
    await page.locator('.theme-toggle').click();
   } else if(route==='/features.html') await featureChecks(page,theme);
   else await page.screenshot({path:path.join(out,`${route.slice(1,-5)}-${theme}.png`),fullPage:true});
  }
  assert.deepEqual(errors,[],'Browser errors'); assert.deepEqual(external,[],'Unexpected third-party requests');
  await context.close();
 }
 for(const theme of ['light','dark']){
  const context=await browser.newContext({viewport:{width:390,height:844},colorScheme:theme,isMobile:true,deviceScaleFactor:1});
  const page=await context.newPage();
  await page.goto(base+'/',{waitUntil:'networkidle'});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true,'Mobile overflow');
  await page.locator('.menu-toggle').click(); assert.equal(await page.locator('.menu-toggle').getAttribute('aria-expanded'),'true');
  await page.locator('#navigation a[href="/#faq"]').click(); assert.equal(await page.locator('.menu-toggle').getAttribute('aria-expanded'),'false');
  await page.locator('#screenshots').scrollIntoViewIfNeeded();
  await page.locator(`[data-preview-theme="${theme}"]`).click();
  await page.evaluate(()=>scrollTo({top:0,behavior:'instant'}));
  await page.screenshot({path:path.join(out,`mobile-${theme}.png`),fullPage:true});
  const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
  report.push({theme,route:'mobile',violations:axe.violations.map(v=>({id:v.id,nodes:v.nodes.map(n=>({target:n.target,summary:n.failureSummary}))}))});
  await page.goto(base+'/features.html',{waitUntil:'networkidle'});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true,'Feature mobile overflow');
  await featureChecks(page,theme,true);
  const featureAxe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
  report.push({theme,route:'features-mobile',violations:featureAxe.violations.map(v=>({id:v.id,nodes:v.nodes.map(n=>({target:n.target,summary:n.failureSummary}))}))});
  await page.goto(base+'/mac-help.html',{waitUntil:'networkidle'});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true,'Mac help mobile overflow');
  const macAxe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
  report.push({theme,route:'mac-help-mobile',violations:macAxe.violations.map(v=>({id:v.id,nodes:v.nodes.map(n=>({target:n.target,summary:n.failureSummary}))}))});
  await page.screenshot({path:path.join(out,`mac-help-mobile-${theme}.png`),fullPage:true});
  await context.close();
 }
 await writeFile(path.join(out,'browser-report.json'),JSON.stringify({base,checkedAt:new Date().toISOString(),report},null,2)+'\n');
 const bad=report.filter(r=>r.violations.length); assert.deepEqual(bad,[],'Accessibility violations; see browser-report.json');
 console.log('Browser, theme, mobile, metadata, first-party requests and accessibility checks passed.');
} finally {await browser.close();}

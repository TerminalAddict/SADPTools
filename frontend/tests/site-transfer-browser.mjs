// Optional browser checks use the website's pinned Playwright/axe development
// dependencies. All service responses and passwords here are synthetic.
import { chromium } from "../../web/node_modules/playwright/index.mjs";
import AxeBuilder from "../../web/node_modules/@axe-core/playwright/dist/index.mjs";
import { createServer } from "node:http";
import { readFile, mkdir } from "node:fs/promises";
import assert from "node:assert/strict";

const root = new URL("../../", import.meta.url);
const routes = new Map([
  ["/site-transfer.js", "frontend/src/site-transfer.js"],
  ["/toolkit-ui.js", "frontend/src/toolkit-ui.js"],
  ["/bootstrap.css", "frontend/node_modules/bootstrap/dist/css/bootstrap.min.css"],
  ["/style.css", "frontend/src/style.css"], ["/toolkit.css", "frontend/src/toolkit.css"],
]);
const server = createServer(async (req, res) => {
  if (req.url === "/") {
    res.setHeader("Content-Type", "text/html");
    res.end(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Synthetic site transfer test</title><link rel="stylesheet" href="/bootstrap.css"><link rel="stylesheet" href="/style.css"><link rel="stylesheet" href="/toolkit.css"></head><body><main><h1>Synthetic test fixture</h1><button id="import">Import site</button><button id="export">Export site</button></main><script type="module">import {importSiteDialog,exportSiteDialog} from '/site-transfer.js';document.querySelector('#import').onclick=()=>importSiteDialog(window.api,async site=>{window.saved=site;});document.querySelector('#export').onclick=()=>exportSiteDialog(window.api,'synthetic-site');</script></body></html>`);
    return;
  }
  const file = routes.get(req.url);
  if (!file) { res.writeHead(404); res.end(); return; }
  try {
    res.setHeader("Content-Type", file.endsWith(".css") ? "text/css" : "text/javascript");
    res.end(await readFile(new URL(file, root)));
  } catch { res.writeHead(500); res.end(); }
});
await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
const browser = await chromium.launch();
const out = new URL(".cache/site-transfer-browser/", root);
await mkdir(out, { recursive: true });
try {
  for (const theme of ["light", "dark"]) for (const width of [1440, 390]) {
    const context = await browser.newContext({ viewport: { width, height: 960 } });
    await context.addInitScript(({theme}) => {
      window.calls = []; window.saved = null; window.pickCancelled = false;
      window.vault = { initialized:false, unlocked:false, automatic:false, mode:"passphrase" };
      window.api = {
        VaultStatus: async () => ({...window.vault}),
        SiteExportNeedsVault: async () => true,
        UnlockVault: async pass => { if (pass !== "Synthetic local passphrase") throw Error("Wrong synthetic passphrase"); window.vault.unlocked = true; window.vault.initialized = true; },
        PrepareSiteImport: async () => window.pickCancelled ? {} : ({token:"synthetic-token",format:"NX Camera Info JSON",name:"Synthetic workshop",customer:"Synthetic customer",devices:3,devicesWithCredentials:2,credentials:2,switches:1,documents:2,attachments:1,history:5,runs:2,needsVault:true,warnings:["One camera has no saved password."]}),
        CancelSiteImport: async token => { window.calls.push(["cancel", token]); },
        CommitSiteImport: async (token,name) => { if (!window.vault.unlocked) throw Error("Vault locked"); window.calls.push(["commit",token,name]); return {id:"synthetic-imported",name}; },
        BackupSite: async () => { window.calls.push(["export"]); return "/synthetic/fixture.sadpsite"; },
      };
      document.addEventListener("DOMContentLoaded", () => document.documentElement.dataset.bsTheme = theme);
    }, {theme});
    const page = await context.newPage();
    const errors = []; page.on("pageerror", error => errors.push(error.message));
    await page.goto(`http://127.0.0.1:${server.address().port}/`, {waitUntil:"networkidle"});
    const openReview = async () => {
      await page.locator("#import").click();
      await page.getByRole("button", {name:"Choose site file",exact:true}).click();
      await page.getByRole("heading",{name:"Review site import",exact:true}).waitFor();
    };
    await openReview();
    assert.equal(await page.locator('dialog [name="localRepeat"]').count(),1);
    assert.ok(!(await page.locator("dialog").textContent()).includes("SYNTHETIC-camera-password"));
    await page.locator('dialog [name="name"]').fill("Synthetic transferred site");
    await page.locator('dialog [name="localPassphrase"]').fill("Synthetic local passphrase");
    await page.locator('dialog [name="localRepeat"]').fill("Incorrect repeat");
    await page.getByRole("button",{name:"Import as new site",exact:true}).click();
    await page.locator(".dialog-error:not([hidden])").waitFor();
    assert.equal(await page.evaluate(() => window.calls.filter(c => c[0] === "commit").length),0);
    assert.equal(await page.locator('dialog [name="localPassphrase"]').inputValue(),"");
    const axe = await new AxeBuilder({page}).include("dialog").withTags(["wcag2a","wcag2aa","wcag21aa"]).analyze();
    assert.deepEqual(axe.violations.map(v => ({id:v.id,nodes:v.nodes.map(n=>n.failureSummary)})),[]);
    await page.screenshot({path:new URL(`review-${width}-${theme}.png`,out).pathname});
    await page.locator('dialog [name="localPassphrase"]').fill("Synthetic local passphrase");
    await page.locator('dialog [name="localRepeat"]').fill("Synthetic local passphrase");
    await page.getByRole("button",{name:"Import as new site",exact:true}).click();
    await page.waitForFunction(() => window.saved?.id === "synthetic-imported");
    assert.equal(await page.locator("dialog[open]").count(),0);
    await openReview(); await page.keyboard.press("Escape");
    await page.waitForFunction(() => window.calls.filter(c=>c[0]==="cancel").length===2);
    assert.equal(await page.evaluate(()=>window.calls.filter(c=>c[0]==="commit").length),1);
    await page.evaluate(()=>window.pickCancelled=true);
    await page.locator("#import").click();
    await page.getByRole("button",{name:"Choose site file",exact:true}).click();
    await page.waitForFunction(()=>!document.querySelector("dialog[open]"));
    await page.locator("#export").click();
    await page.locator('dialog [name="passphrase"]').fill("Synthetic file password");
    await page.locator('dialog [name="repeat"]').fill("Synthetic file password");
    await page.getByRole("button",{name:"Save site file",exact:true}).click();
    await page.getByRole("heading",{name:"Site exported",exact:true}).waitFor();
    assert.equal(await page.evaluate(()=>window.calls.filter(c=>c[0]==="export").length),1);
    assert.deepEqual(errors,[]);
    await context.close();
  }
  console.log("Site transfer dialogs: preview, local vault setup, password mismatch, import, cancel/Escape, cancelled file selection and export passed in four theme/viewport combinations; axe checks passed.");
} finally { await browser.close(); await new Promise(resolve=>server.close(resolve)); }

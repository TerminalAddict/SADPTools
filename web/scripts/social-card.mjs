import {chromium} from 'playwright';
import {fileURLToPath, pathToFileURL} from 'node:url';
import path from 'node:path';
const dir=path.dirname(fileURLToPath(import.meta.url));
const browser=await chromium.launch({headless:true});
try {const page=await browser.newPage({viewport:{width:1200,height:630},deviceScaleFactor:1});await page.goto(pathToFileURL(path.join(dir,'social-card.html')).href,{waitUntil:'networkidle'});await page.screenshot({path:path.join(dir,'../public/assets/social-card.png')});} finally {await browser.close();}

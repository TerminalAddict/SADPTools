// Only used against the isolated synthetic fixture from download-report.py.
import {chromium} from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import assert from 'node:assert/strict';
import {mkdir, readFile, writeFile} from 'node:fs/promises';
const url = process.env.REPORT_TEST_URL;
assert.match(url || '', /^http:\/\/127\.0\.0\.1:\d+\/assets\/downloads_report\.php$/);
const out = new URL('../screenshots/', import.meta.url);
await mkdir(out, {recursive: true});
const browser = await chromium.launch();
try {
  for (const theme of ['light', 'dark']) for (const width of [1440, 390]) {
    const context = await browser.newContext({viewport: {width, height: 900}, colorScheme: theme,
      httpCredentials: {username: 'fixture', password: 'synthetic-test-only'}});
    const page = await context.newPage(); const errors = [];
    page.on('pageerror', e => errors.push(e.message));
    page.on('response', response => {
      if (response.url().startsWith(url) && response.status() >= 400) errors.push(`Report HTTP ${response.status()}`);
    });
    page.on('dialog', () => { throw new Error('User-agent script executed'); });
    await page.goto(url, {waitUntil: 'networkidle'});
    assert.equal(await page.locator('html').getAttribute('data-theme'), theme);
    assert.equal(await page.locator('tbody tr').count(), 200);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
    const dangerous = page.locator('details').filter({hasText: '<script>alert(1)</script>'}).first();
    await dangerous.locator('summary').click();
    assert.match(await dangerous.textContent(), /<script>alert\(1\)<\/script>/);
    const navigate = async action => {
      await Promise.all([page.waitForEvent('framenavigated'), action()]);
      await page.waitForLoadState('networkidle');
      assert.deepEqual(errors, [], 'Report navigation should succeed');
    };
    await page.locator('#report-search').fill('EARLY-HISTORY safari');
    await navigate(() => page.getByRole('button', {name: 'Search', exact: true}).click());
    assert.equal(await page.locator('tbody tr').count(), 1);
    assert.match(await page.locator('tbody tr .date-cell').first().textContent(), /2026-01-02 01:30:00 NZDT/);
    assert.match(await page.locator('#report-count').textContent(), /1 matching requests/);
    assert.equal(await page.locator('#report-search').inputValue(), 'EARLY-HISTORY safari');
    assert.match(await page.locator('#export-csv').getAttribute('href'), /q=EARLY-HISTORY%20safari/);
    const [download] = await Promise.all([page.waitForEvent('download'), page.locator('#export-csv').click()]);
    assert.equal(download.suggestedFilename(), 'SADPTools-downloads.csv');
    const content = await readFile(await download.path(), 'utf8');
    assert.match(content, /early-history-fixture/);
    assert.ok(!content.includes('synthetic-extra-'));
    await navigate(() => page.getByRole('link', {name: 'Clear search'}).click());
    await navigate(() => page.locator('[data-sort="bytes"]').click());
    assert.equal(await page.locator('th:has([data-sort="bytes"])').getAttribute('aria-sort'), 'ascending');
    const byteValues = () => page.locator('tbody tr .bytes-cell').evaluateAll(cells => cells.map(c => Number(c.textContent.replaceAll(',', ''))));
    let values = await byteValues();
    assert.deepEqual(values, [...values].sort((a,b) => a-b));
    await navigate(() => page.locator('[data-sort="bytes"]').click());
    assert.equal(await page.locator('th:has([data-sort="bytes"])').getAttribute('aria-sort'), 'descending');
    values = await byteValues(); assert.deepEqual(values, [...values].sort((a,b) => b-a));
    await navigate(() => page.getByRole('link', {name: 'Next →'}).click());
    assert.ok(await page.locator('tbody tr').count() < 200);
    assert.match(await page.locator('.report-pagination').textContent(), /Page 2 of 2/);
    assert.equal(await page.locator('th:has([data-sort="bytes"])').getAttribute('aria-sort'), 'descending');
    await page.locator('#report-search').fill('no-such-record');
    await navigate(() => page.getByRole('button', {name: 'Search', exact: true}).click());
    assert.equal(await page.locator('tbody tr').count(), 0);
    assert.match(await page.locator('.report-empty').textContent(), /No download records match/);
    await page.goto(url, {waitUntil: 'networkidle'});
    assert.equal(await page.locator('#delete-selected').isDisabled(), true);
    await page.locator('#select-page').check();
    assert.equal(await page.locator('[data-record-checkbox]:checked').count(), 200);
    assert.match(await page.locator('#selection-status').textContent(), /200 selected on this page/);
    await page.locator('[data-record-checkbox]').first().uncheck();
    assert.equal(await page.locator('#select-page').evaluate(el => el.indeterminate), true);
    await page.locator('#clear-selection').click();
    await page.locator('#select-matching').check();
    const total = Number(await page.locator('#record-selection').getAttribute('data-matches'));
    assert.match(await page.locator('#selection-status').textContent(), new RegExp(`${total} selected across all pages`));
    await page.locator('[data-record-checkbox]').first().uncheck();
    assert.equal(await page.locator('#excluded-records input').count(), 1);
    await navigate(() => page.locator('#delete-selected').click());
    assert.match(await page.locator('h1').textContent(), new RegExp(`Delete ${total-1} download records`));
    const confirmAxe = await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
    assert.deepEqual(confirmAxe.violations.map(v => ({id: v.id, nodes: v.nodes.map(n => n.failureSummary)})), []);
    await page.screenshot({path: new URL(`delete-confirmation-${width}-${theme}.png`, out).pathname});
    if (theme === 'light' && width === 1440) await writeFile(new URL('delete-confirmation-fixture.html', out), await page.content());
    await navigate(() => page.getByRole('button', {name: 'Cancel', exact: true}).click());
    assert.equal(Number(await page.locator('#record-selection').getAttribute('data-matches')), total);
    assert.equal(await page.locator('[data-record-checkbox]:checked').count(), 0);
    // A row action always previews exactly one row, even with bulk selection active.
    await page.locator('#select-matching').check();
    await navigate(() => page.locator('.delete-row').first().click());
    assert.equal(await page.locator('h1').textContent(), 'Delete 1 download record?');
    await navigate(() => page.getByRole('button', {name: 'Cancel', exact: true}).click());
    assert.deepEqual(errors, []);
    const axe = await new AxeBuilder({page}).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']).analyze();
    assert.deepEqual(axe.violations.map(v => ({id: v.id, nodes: v.nodes.map(n => n.failureSummary)})), []);
    await page.screenshot({path: new URL(`download-report-${width}-${theme}.png`, out).pathname});
    if (theme === 'light' && width === 1440) {
      await writeFile(new URL('download-report-fixture.html', out), await page.content());
    }
    await context.close();
  }
  // Real deletions are exercised only against this temporary synthetic log.
  const context = await browser.newContext({httpCredentials: {username:'fixture',password:'synthetic-test-only'}});
  const page = await context.newPage();
  await page.goto(url+'?q=synthetic-extra-215', {waitUntil:'networkidle'});
  await page.locator('[data-record-checkbox]').check();
  await page.locator('#delete-selected').click(); await page.waitForLoadState('networkidle');
  assert.equal(await page.locator('h1').textContent(), 'Delete 1 download record?');
  await page.getByRole('button', {name:'Delete 1 record',exact:true}).click(); await page.waitForLoadState('networkidle');
  assert.match(await page.locator('.report-notice').textContent(), /Deleted 1 record/);
  assert.equal(await page.locator('tbody tr').count(), 0);
  await page.goto(url+'?q=synthetic-extra-214', {waitUntil:'networkidle'});
  await page.locator('.delete-row').click(); await page.waitForLoadState('networkidle');
  await page.getByRole('button', {name:'Delete 1 record',exact:true}).click(); await page.waitForLoadState('networkidle');
  assert.equal(await page.locator('tbody tr').count(), 0);
  await context.close();
  console.log('Private report: search/sort/export, selection, cancellation, individual deletion, themes, mobile and accessibility checks passed.');
} finally {await browser.close();}

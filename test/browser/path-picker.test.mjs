import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright';

test('shared picker selects paths, restores drafts, and preserves form values', async () => {
  const browser = await chromium.launch({ channel: process.env.BROWSER_CHANNEL || 'chrome', headless: true });
  try {
    const page = await browser.newPage();
    await page.setContent(`<span hidden data-path-option data-slug="applications/service" data-label="Applications / Service"></span>
      <form><input name="parent_path" data-path-input><input name="target" data-path-input hidden></form>`);
    const script = await readFile(new URL('../../web/dist/js/features/path-picker.js', import.meta.url), 'utf8');
    await page.addScriptTag({ content: script.replace('export function', 'function') + '\ninitPathPickers();' });
    const input = page.locator('[name=parent_path]');
    await input.fill('SERVICE');
    await input.press('Enter');
    assert.equal(await input.inputValue(), 'applications/service');
    assert.equal(await page.locator('form').evaluate(form => new FormData(form).get('parent_path')), 'applications/service');
    await input.evaluate(el => { el.value = 'restored/folder'; el.form.dispatchEvent(new Event('editor:restore-draft')); });
    await page.waitForFunction(() => document.querySelector('.shared-path-crumbs').textContent.includes('restored'));
    await page.locator('.path-crumb').first().click();
    assert.equal(await input.inputValue(), '');
    await input.press('Escape');
    assert.equal(await input.getAttribute('aria-expanded'), 'false');
    await page.locator('[name=target]').evaluate(el => { el.hidden = false; });
    await page.waitForFunction(() => !document.querySelectorAll('.shared-path-picker')[1].hidden);
  } finally { await browser.close(); }
});

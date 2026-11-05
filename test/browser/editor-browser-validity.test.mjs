import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";
import { pluginCatalog, pluginRoute, plugin } from "./plugin-fixture.mjs";

test("plugin editor validity gates Apply and preserves invalid drafts", async () => {
  const browser = await chromium.launch({channel: process.env.BROWSER_CHANNEL || "chrome", headless:true});
  try {
    const page = await browser.newPage();
    page.on("pageerror", error => console.error(error.message));
    page.on("console", message => {if(message.type() === "error") console.error(message.text());});
    const fakeJavaScript = `globalThis.kumbukaPlugin = { render() {}, edit(root, context) {
      const input = document.createElement('input'); input.setAttribute('aria-label','Plugin draft'); input.value=context.source;
      input.oninput = () => { context.onValidity({valid: input.value !== 'invalid', message: 'Fix this draft'}); context.onChange(input.value); };
      root.append(input); context.onValidity({valid:false,message:'Fix this draft'});
    }};`;
    await page.route("http://editor-validity.test/**", async route => {
      if (await pluginRoute(route,{fakeJavaScript})) return;
      const path = new URL(route.request().url()).pathname;
      if(path.startsWith('/assets/')) return route.fulfill({contentType:path.endsWith('.css')?'text/css':'text/javascript',body:await readFile(new URL('../../web/dist/'+path.slice(8),import.meta.url))});
      const widget = {plugin_id:plugin.plugin_id,id:'test',name:'Test',inline:false,syntax:{kind:'fence',name:'example'},attributes:[{name:'body',type:'string',required:true}],settings:[{type:'browser',label:'Draft',attribute:'body',browser_module_id:plugin.module_id}],preview:{kind:'card',card:{title:'Test'}}};
      await route.fulfill({contentType:'text/html',body:`<link rel="stylesheet" href="/assets/css/app.css">${pluginCatalog()}<button id="anchor">Edit</button><script type="module">
        import {createSettingsPopover} from '/assets/js/features/editor/visual-widget-form.js';
        import {initVisualWidgetPopovers} from '/assets/js/features/editor/visual-widget-popover.js';
        initVisualWidgetPopovers();
        document.body.append(createSettingsPopover('~~~example\\ninvalid\\n~~~', ${JSON.stringify(widget)}, document.querySelector('#anchor'), raw => {document.body.dataset.applied=raw},()=>{},()=>{},[]));
      </script>`});
    });
    await page.goto('http://editor-validity.test/');
    const input = page.frameLocator('iframe').getByLabel('Plugin draft');
    await input.waitFor();
    const dialog = page.getByRole('dialog');
    const before = await dialog.boundingBox();
    const viewport = page.viewportSize();
    assert.ok(Math.abs(before.x + before.width / 2 - viewport.width / 2) < 2);
    const title = await dialog.locator('.visual-widget-popover-title').boundingBox();
    await page.mouse.move(title.x + 10, title.y + 8);
    await page.mouse.down();
    await page.mouse.move(title.x + 60, title.y + 28);
    await page.mouse.up();
    const after = await dialog.boundingBox();
    assert.ok(after.x >= before.x + 49);
    const apply = page.getByRole('button',{name:'Apply',exact:true});
    assert.equal(await apply.isDisabled(),true);
    await input.fill('valid');
    await page.waitForFunction(()=>!document.querySelector('button[type=submit]').disabled);
    await input.fill('invalid');
    await page.waitForFunction(()=>document.querySelector('button[type=submit]').disabled);
    assert.equal(await input.inputValue(),'invalid');
    await input.fill('repaired');
    await apply.click();
    assert.match(await page.locator('body').getAttribute('data-applied'),/repaired/);
  } finally {await browser.close();}
});

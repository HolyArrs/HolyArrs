const { chromium } = require('../../../web/app/node_modules/playwright');
const fs = require('node:fs');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { execFileSync } = require('node:child_process');
const root = path.resolve(__dirname, '../../..');
const out = path.join(root, '.omx/artifacts/visual-ralph/intercom/typography-motion', process.env.MOTION_BATCH || 'before');
function sourceHash() {
  const hash = createHash('sha256');
  function walk(dir) {
    for (const name of fs.readdirSync(dir).sort()) {
      const file = path.join(dir, name);
      if (fs.statSync(file).isDirectory()) walk(file);
      else hash.update(path.relative(root, file)).update(fs.readFileSync(file));
    }
  }
  walk(path.join(root, 'web/app/src'));
  return hash.digest('hex');
}
async function motionSample(locator) {
  return locator.evaluate(async node => {
    const start = performance.now();
    const samples = [];
    do {
      const style = getComputedStyle(node);
      const box = node.getBoundingClientRect();
      samples.push({ ms: performance.now() - start, opacity: style.opacity, transform: style.transform, translate: style.translate, height: box.height,
        animations: node.getAnimations().map(a => ({ name: a.animationName || a.transitionProperty || 'svelte', duration: a.effect.getTiming().duration, easing: a.effect.getTiming().easing })) });
      await new Promise(requestAnimationFrame);
    } while (performance.now() - start < 300);
    return samples;
  });
}
(async () => {
  fs.mkdirSync(out, { recursive: true });
  const before = sourceHash();
  const checks = [];
  const browser = await chromium.launch({ headless: true });
  try {
    for (const width of (process.env.MOTION_WIDTHS || '390,834,1440').split(',').map(Number)) for (const reducedMotion of ['no-preference', 'reduce']) {
      const prefix = `${width}-${reducedMotion}`;
      const context = await browser.newContext({ viewport: { width, height: 900 }, serviceWorkers: 'block', reducedMotion, recordVideo: { dir: out, size: { width, height: 900 } } });
      await context.addInitScript(() => localStorage.setItem('holyarrs-theme', 'light'));
      await context.route('**/*', route => route.request().method() === 'GET' ? route.continue() : route.abort());
      const page = await context.newPage();
      const errors = [];
      page.on('pageerror', e => errors.push(e.message));
      await page.goto('http://127.0.0.1:18746/library?kind=movie', { waitUntil: 'networkidle' });
      await page.evaluate(() => document.fonts.ready);
      const typography = await page.locator('main').evaluate(node => {
        const elements = [document.body, node.querySelector('h1'), node.querySelector('input'), node.querySelector('.btn'), node.querySelector('.chip')].filter(Boolean);
        return elements.map(e => { const s = getComputedStyle(e); return { tag: e.tagName, text: e.textContent?.trim().slice(0, 40), font: s.fontFamily, size: s.fontSize, weight: s.fontWeight, leading: s.lineHeight, tracking: s.letterSpacing }; });
      });
      await page.screenshot({ path: path.join(out, prefix + '-library.png') });
      let navigation;
      if (width >= 1024) {
        const parent = page.locator('.context-sidebar .library-parent');
        await parent.click();
        await page.waitForTimeout(240);
        await parent.click();
        navigation = await motionSample(page.locator('.context-sidebar .library-reveal'));
        if (await parent.getAttribute('aria-expanded') !== 'true') throw Error('Library expansion state');
      } else {
        const open = width < 768 ? page.getByRole('button', { name: 'Library', exact: true }).first() : page.getByRole('button', { name: 'Library', exact: true }).first();
        await open.click();
        navigation = await motionSample(page.locator('dialog.navigation-dialog'));
        await page.keyboard.press('Escape');
        if (!await open.evaluate(n => n === document.activeElement)) throw Error('Navigation focus return');
        await page.waitForTimeout(250);
      }
      await page.goto('http://127.0.0.1:18746/library/1', { waitUntil: 'networkidle' });
      const edit = page.getByRole('button', { name: 'Edit', exact: true });
      await edit.click();
      const dialog = page.getByRole('dialog', { name: /^Edit / });
      const entry = await motionSample(dialog);
      await page.screenshot({ path: path.join(out, prefix + '-dialog.png') });
      await page.keyboard.press('Escape');
      const exit = await motionSample(page.locator('dialog[aria-labelledby="edit-title"]'));
      if (!await edit.evaluate(n => n === document.activeElement)) throw Error('Edit focus return');
      if (await page.locator('dialog[open]').count()) throw Error('Dialog did not close');
      const toast = await page.evaluate(async () => {
        const { toasts } = await import('/src/lib/toasts.svelte.ts');
        return toasts.push('Preview motion check', 'info');
      });
      const toastEntry = await motionSample(page.locator('.toast'));
      await page.getByRole('button', { name: 'Close message', exact: true }).click();
      await page.locator('.toast').waitFor({ state: 'detached' });
      let changedPreference, invalidToken;
      if (process.env.MOTION_ASSERT === '1') {
        const sequences = [navigation, entry, exit, toastEntry];
        if (reducedMotion === 'reduce' && sequences.some(s => s.some(f => f.animations.some(a => a.duration > 0)))) throw Error('Reduced motion still animated');
        if (reducedMotion !== 'reduce' && !entry.some(f => f.animations.length)) throw Error('Dialog entry has no animation');
        if (exit.some(f => f.height > 0)) throw Error('Closed dialog still occupies space');
        if (reducedMotion !== 'reduce' && !navigation.some(f => f.animations.length)) throw Error('Navigation has no motion');
        if (reducedMotion !== 'reduce' && !toastEntry.some(f => f.animations.length)) throw Error('Toast entry has no fade');
        const [body, heading, input, action, chip] = typography;
        if (typography.some(t => !t.font.includes('Geist'))) throw Error('Unexpected UI font family');
        if (body.size !== '15px' || body.leading !== '22.5px' || body.weight !== '400') throw Error('Unexpected body typography');
        if (heading.size !== '28px' || heading.weight !== '600') throw Error('Unexpected heading typography');
        if (Number.parseFloat(input.size) < (width < 768 ? 16 : 15)) throw Error('Input typography too small');
        if (action.size !== '14px' || action.weight !== '600') throw Error('Unexpected action typography');
        if (chip.size !== '12px' || chip.weight !== '500') throw Error('Unexpected chip typography');
        await page.emulateMedia({ reducedMotion: 'reduce' });
        await page.evaluate(async () => (await import('/src/lib/toasts.svelte.ts')).toasts.push('Reduced motion check'));
        changedPreference = await motionSample(page.locator('.toast'));
        if (changedPreference.some(f => f.animations.some(a => a.duration > 0))) throw Error('Live reduced-motion preference ignored');
        await page.getByRole('button', { name: 'Close message', exact: true }).click();
        await page.locator('.toast').waitFor({ state: 'detached' });
        await page.emulateMedia({ reducedMotion: 'no-preference' });
        await page.evaluate(async () => {
          document.documentElement.style.setProperty('--dur-fast', 'invalid');
          (await import('/src/lib/toasts.svelte.ts')).toasts.push('Invalid visual token check');
        });
        invalidToken = await motionSample(page.locator('.toast'));
        if (invalidToken.some(f => f.animations.some(a => a.duration > 0))) throw Error('Invalid visual token not handled safely');
        await page.getByRole('button', { name: 'Close message', exact: true }).click();
        await page.locator('.toast').waitFor({ state: 'detached' });
      }
      checks.push({ width, reducedMotion, typography, navigation, entry, exit, toastEntry, changedPreference, invalidToken, toastDiagnosticOnly: toast, errors });
      const video = page.video();
      await context.close();
      await video.saveAs(path.join(out, prefix + '.webm'));
      fs.unlinkSync(await video.path());
      fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify({ commit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim(), before, checks, after: sourceHash() }, null, 2));
      console.log(prefix, 'pass');
    }
  } finally { await browser.close(); }
  if (before !== sourceHash()) throw Error('Source drift');
  if (checks.some(c => c.errors.length)) throw Error('Browser errors');
})().catch(e => { console.error(e); process.exitCode = 1; });

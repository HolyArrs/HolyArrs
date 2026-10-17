const { chromium } = require('../../../web/app/node_modules/playwright');
const fs = require('node:fs');
const path = require('node:path');
const out = path.resolve(__dirname, '../../../.omx/artifacts/visual-ralph/intercom/full-audit/clearance');
(async () => {
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const checks = [];
  try {
    for (const width of [360, 390, 834]) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, serviceWorkers: 'block', reducedMotion: 'reduce' });
      await context.route('**/*', (route) => route.request().method() === 'GET' ? route.continue() : route.abort());
      const page = await context.newPage();
      await page.goto('http://127.0.0.1:18746/library/141', { waitUntil: 'networkidle' });
      const album = page.getByTestId('album').last();
      await album.evaluate((element) => element.scrollIntoView({ block: 'center' }));
      const albumBox = await album.boundingBox();
      const navBox = await page.locator('.tabbar').boundingBox();
      if (!albumBox || albumBox.y < 60 || (navBox && albumBox.y + albumBox.height > navBox.y)) throw Error('Album row obscured or outside viewport');
      const searchBox = await album.getByTestId('album-search').boundingBox();
      const switchBox = await album.locator('.switch').boundingBox();
      if (width <= 600 && (!searchBox || !switchBox || searchBox.height < 44 || switchBox.height < 44)) throw Error('Album action target below 44px');
      await page.screenshot({ path: path.join(out, width + '-music-album.png') });
      await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
      await page.screenshot({ path: path.join(out, width + '-music-end.png') });
      checks.push({ width, albumClearOfNavigation: true, mobileTargetsAtLeast44px: width <= 600 ? true : null });
      await page.goto('http://127.0.0.1:18746/wishlist', { waitUntil: 'networkidle' });
      const card = await page.locator('.messages').boundingBox();
      const heading = await page.locator('#h-msg').boundingBox();
      const firstMessage = await page.getByTestId('message').first().boundingBox();
      if (!card || !heading || !firstMessage || heading.x < card.x || heading.x + heading.width > card.x + card.width || Math.abs(heading.x - firstMessage.x) > 1) throw Error('Message heading not aligned inside card');
      await page.screenshot({ path: path.join(out, width + '-messages.png') });
      checks.push({ width, messageHeadingInsideCard: true, alignedWithRows: true, left: { card: card.x, heading: heading.x, row: firstMessage.x } });
      await context.close();
    }
  } finally {
    await browser.close();
    fs.writeFileSync(path.join(out, 'receipt.json'), JSON.stringify(checks, null, 2));
  }
  console.log(checks);
})().catch((error) => { console.error(error); process.exitCode = 1; });

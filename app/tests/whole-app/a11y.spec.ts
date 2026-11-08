import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { owner, signIn } from '../helpers/helpers.ts';

// Runs after the desktop tests (which make the owner and add a movie) and
// scans every page in light and dark with axe. Every WCAG violation fails.
const pages = [
	'/',
	'/welcome',
	'/library',
	'/library/1',
	'/library/held',
	'/library/turned-down',
	'/library/rename',
	'/library/import',
	'/search',
	'/downloads',
	'/downloads/history',
	'/calendar',
	'/settings',
	'/settings/account',
	'/settings/backups',
	'/settings/downloaders',
	'/settings/library-folders',
	'/settings/naming',
	'/settings/outside-lists',
	'/settings/sign-in',
	'/settings/search',
	'/settings/sources',
	'/settings/this-app',
	'/settings/tmdb',
	'/settings/music',
	'/settings/requests',
	'/settings/alerts',
	'/settings/media-servers',
	'/settings/pick-rules',
	// The first ready-made Pick rules, made when HolyArrs starts.
	'/settings/pick-rules/1',
	'/settings/title-defaults',
	'/explore',
	'/requests'
];

async function scan(page: Page, where: string) {
	// The sign-in card shows on the first frame; the wallpaper crossfades in.
	// Let entry fades finish so colours are measured at rest.
	await page.evaluate(() => Promise.all(document.getAnimations().filter((a) => a.effect?.getComputedTiming().iterations !== Infinity).map((a) => a.finished.catch(() => undefined))));
	const r = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	const msg = r.violations.map((v) => `${v.id}: ${v.help}\n  ${v.nodes.slice(0, 5).map((n) => n.target.join(' ')).join('\n  ')}`).join('\n');
	if (r.violations.length) console.log(`AXE ${where}\n${msg}`);
	expect.soft(r.violations.map((v) => v.id), where).toEqual([]);
}

for (const scheme of ['light', 'dark'] as const) {
	test(`no accessibility issues (${scheme})`, async ({ browser }) => {
		// About thirty pages, each scanned whole: more than the default 30 s.
		test.setTimeout(120_000);
		const ctx = await browser.newContext({ colorScheme: scheme });
		await ctx.addInitScript((t) => localStorage.setItem('holyarrs-theme', t), scheme);
		const page = await ctx.newPage();
		await page.goto('/login');
		await expect(page.locator('html')).toHaveAttribute('data-theme', scheme);
		await scan(page, `/login (${scheme})`);
		await signIn(page, owner());
		for (const p of pages) {
			await page.goto(p);
			await page.waitForLoadState('networkidle');
			await scan(page, `${p} (${scheme})`);
		}
		const first = page.locator('a[href^="/library/"]').first();
		await page.goto('/library');
		if (await first.count()) {
			await first.click();
			await page.waitForLoadState('networkidle');
			await scan(page, `title page (${scheme})`);
		}
		await ctx.close();
	});
}

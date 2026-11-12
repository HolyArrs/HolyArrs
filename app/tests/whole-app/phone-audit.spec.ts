import { expect, test, type Page } from '@playwright/test';
import { writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { setUpFresh } from '../helpers/helpers.ts';
import { dataDir } from '../helpers/data-folder.ts';

// Every page at 360 and 390 wide: no sideways scroll, the bottom menu shows,
// its tabs are big enough to tap, and the Manage sheet fits.
// Smaller tap targets inside pages are written to .tmp-phone/audit.json.
const tmp = dataDir('phone');
const pages = [
	'/', '/explore', '/popular', '/search', '/library', '/library/copies', '/library/held',
	'/library/import', '/library/missing-subtitles', '/library/rename', '/library/turned-down', '/calendar',
	'/requests', '/downloads', '/downloads/decide', '/downloads/history', '/downloads/turned-down', '/health', '/account',
	'/settings', '/settings/account', '/settings/alerts', '/settings/api', '/settings/appearance', '/settings/backups',
	'/settings/danger', '/settings/downloaders', '/settings/health', '/settings/library-folders', '/settings/logs',
	'/settings/media-servers', '/settings/move', '/settings/music', '/settings/naming', '/settings/outside-lists',
	'/settings/parts', '/settings/pick-rules', '/settings/schedules', '/settings/search', '/settings/sign-in',
	'/settings/sources', '/settings/subtitles', '/settings/this-app', '/settings/title-defaults', '/settings/tmdb',
	'/settings/users', '/settings/requests'
];

async function measure(page: Page) {
	return page.evaluate(() => {
		const cw = document.documentElement.clientWidth;
		const name = (el: Element) => `${el.tagName.toLowerCase()}${el.classList.length ? '.' + [...el.classList].filter((c) => !c.startsWith('svelte-')).join('.') : ''}`;
		const wide = [...document.querySelectorAll('body *')]
			.filter((el) => el.getBoundingClientRect().right > cw + 1 && ![...el.children].some((c) => c.getBoundingClientRect().right > cw + 1))
			.slice(0, 5)
			.map(name);
		const small = [...document.querySelectorAll('a[href], button, input:not([type=hidden]), select, textarea, [role=button], [role=tab], [role=radio], [role=checkbox], [role=switch], [role=menuitem]')]
			.filter((el) => {
				const r = el.getBoundingClientRect();
				if (!r.width || !r.height || getComputedStyle(el).visibility === 'hidden') return false;
				// Links inside running text are exempt, as WCAG 2.5.8 allows.
				if (el.tagName === 'A' && getComputedStyle(el).display === 'inline') return false;
				return r.width < 24 || r.height < 24;
			})
			.map((el) => {
				const r = el.getBoundingClientRect();
				return `${name(el)} "${(el.getAttribute('aria-label') ?? el.textContent ?? '').trim().slice(0, 30)}" ${Math.round(r.width)}x${Math.round(r.height)}`;
			});
		return { sw: document.documentElement.scrollWidth, cw, wide, small: [...new Set(small)] };
	});
}

test('phone audit: every page at 360 and 390', async ({ page }) => {
	test.setTimeout(600_000);
	// First run: the owner and one folder, with Music and Requests on so their pages open.
	const pw = `e2e-${Math.random().toString(36).slice(2)}-pw`;
	await setUpFresh(page, { name: 'luis', password: pw }, join(tmp, 'movies'), { music: true, invite: true });

	const report: Record<string, unknown> = {};
	const fails: string[] = [];
	for (const width of [360, 390]) {
		await page.setViewportSize({ width, height: 780 });
		for (const url of pages) {
			await page.goto(url);
			await page.waitForLoadState('networkidle');
			await page.waitForTimeout(300);
			const m = await measure(page);
			report[`${width} ${url}`] = m;
			if (m.sw > m.cw) fails.push(`${width} ${url}: ${m.sw}px wide (${m.wide.join(', ')})`);
			const tabs = page.locator('nav.tabbar');
			if (!(await tabs.isVisible())) { fails.push(`${width} ${url}: no bottom menu`); continue; }
			for (const box of await page.locator('nav.tabbar a, nav.tabbar button').evaluateAll((els) =>
				// A segmented control's buttons sit in a 44px track; the track is what you tap.
				els.filter((e) => (e as HTMLElement).offsetParent).map((e) => { const r = (e.closest('.ui-seg') ?? e).getBoundingClientRect(); return { n: (e.getAttribute('aria-label') ?? e.textContent ?? '').trim().slice(0, 20), w: r.width, h: r.height }; })
			)) if (box.w < 44 || box.h < 44) fails.push(`${width} ${url}: bottom menu tab "${box.n}" is ${Math.round(box.w)}x${Math.round(box.h)}`);
		}
		// The Manage sheet fits the screen and closes.
		await page.goto('/');
		await page.locator('nav.tabbar').getByRole('button', { name: /^Manage/ }).click();
		const sheet = page.getByRole('dialog', { name: 'Manage' });
		await expect(sheet).toBeVisible();
		// Wait for the rise-in animation to end before measuring.
		await sheet.evaluate((el) => Promise.all(el.getAnimations().map((a) => a.finished)));
		const b = (await sheet.boundingBox())!;
		if (b.x < 0 || b.x + b.width > width + 1 || b.y < 0 || b.y + b.height > 781) fails.push(`${width} Manage sheet off screen ${JSON.stringify(b)}`);
		report[`${width} manage-sheet`] = { ...b, small: (await measure(page)).small };
		await page.keyboard.press('Escape');
		await expect(sheet).toBeHidden();
	}
	writeFileSync(join(tmp, 'audit.json'), JSON.stringify({ fails, report }, null, 1));
	expect(fails, fails.join('\n')).toEqual([]);
});

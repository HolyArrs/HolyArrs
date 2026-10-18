import { expect, type Locator, type Page } from '@playwright/test';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const tmp = join(dirname(fileURLToPath(import.meta.url)), '.tmp');
export const moviesDir = join(tmp, 'movies');
const ownerFile = join(tmp, 'owner.json');

export interface Owner {
	name: string;
	password: string;
}

export function setupCode(): string {
	return readFileSync(join(tmp, 'config', 'setup-code.txt'), 'utf8').trim();
}

export function newOwner(): Owner {
	const o = { name: 'luis', password: `e2e-${Math.random().toString(36).slice(2)}-pw` };
	writeFileSync(ownerFile, JSON.stringify(o));
	return o;
}

export function owner(): Owner {
	if (!existsSync(ownerFile)) throw new Error('run the desktop tests first; they make the owner');
	return JSON.parse(readFileSync(ownerFile, 'utf8'));
}

export async function signIn(page: Page, o: Owner) {
	await page.goto('/login');
	await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
	await page.getByLabel('Name').fill(o.name);
	await page.getByLabel('Password', { exact: true }).fill(o.password);
	await page.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(page).toHaveURL(/\/$/);
}

/** libraryStatus picks a Library status filter from the Status chips (all, missing, not_out_yet, better, have). */
export async function libraryStatus(scope: Page | Locator, value: 'all' | 'missing' | 'not_out_yet' | 'better' | 'have') {
	const names = { all: /^All/, missing: /^Missing/, not_out_yet: /^Not out yet/, better: /^Better copy wanted/, have: /^In library/ };
	await scope.getByRole('group', { name: 'Status' }).getByRole('button', { name: names[value] }).click();
}

/** openApprovals reaches the Wishlist as an approver on a phone: Manage tab, then Approvals. */
export async function openApprovals(page: Page) {
	await page.locator('.ui-tabbar').getByRole('button', { name: /^Manage/ }).click();
	await page.getByRole('dialog', { name: 'Manage' }).getByRole('link', { name: /^Approvals/ }).click();
	await expect(page).toHaveURL(/wishlist/);
}

/** titleAction picks an item from a title page's hero menu: 'Library' (rename, delete, versions) or 'More actions'. */
export async function titleAction(page: Page, menu: 'Library' | 'More actions', item: string) {
	await page.getByRole('button', { name: menu, exact: true }).click();
	await page.getByRole('menuitem', { name: item, exact: true }).click();
}

/** libraryTool opens a Library tool from the Library page's Add menu. */
export async function libraryTool(page: Page, name: string) {
	await page.getByRole('main').getByRole('button', { name: 'Add', exact: true }).click();
	await page.getByRole('menu', { name: 'Add and Library tools' }).getByRole('menuitem', { name }).click();
}

/** openSeason shows one season on a show page (the picker holds them all) and returns its card. */
export async function openSeason(page: Page, index: number): Promise<Locator> {
	await expect(page.getByTestId('season')).toBeVisible();
	const seg = page.getByRole('radiogroup', { name: 'Season' });
	if (await seg.count()) {
		await seg.getByRole('radio').nth(index).click();
		await expect(seg.getByRole('radio').nth(index)).toHaveAttribute('aria-checked', 'true');
	} else {
		await page.getByRole('button', { name: 'Pick a season' }).click();
		await page.getByRole('menu', { name: 'Pick a season' }).getByRole('menuitemradio').nth(index).click();
	}
	return page.getByTestId('season');
}

/** openSeasonNamed shows the season with this name ("Season 1", "Specials") and returns its card. */
export async function openSeasonNamed(page: Page, name: string): Promise<Locator> {
	await expect(page.getByTestId('season')).toBeVisible();
	const seg = page.getByRole('radiogroup', { name: 'Season' });
	if (await seg.count()) {
		const radio = seg.getByRole('radio', { name, exact: true });
		await radio.click();
		await expect(radio).toHaveAttribute('aria-checked', 'true');
	} else {
		await page.getByRole('button', { name: 'Pick a season' }).click();
		await page.getByRole('menu', { name: 'Pick a season' }).getByRole('menuitemradio', { name, exact: true }).click();
	}
	return page.getByTestId('season');
}

/** seasonCount counts the seasons in a show page's season picker. */
export async function seasonCount(page: Page): Promise<number> {
	await expect(page.getByTestId('season')).toBeVisible();
	const seg = page.getByRole('radiogroup', { name: 'Season' });
	if (await seg.count()) return seg.getByRole('radio').count();
	await page.getByRole('button', { name: 'Pick a season' }).click();
	const n = await page.getByRole('menu', { name: 'Pick a season' }).getByRole('menuitemradio').count();
	await page.keyboard.press('Escape');
	return n;
}

/** episodeAction picks an item from an episode card's menu. */
export async function episodeAction(page: Page, code: string, item: string) {
	await page.getByRole('button', { name: `Actions for ${code}` }).click();
	await page.getByRole('menu', { name: `Actions for ${code}` }).locator('[role^="menuitem"]').filter({ has: page.getByText(item, { exact: true }) }).click();
}

/** episodeKept says whether an episode's Keep item is checked (opens and closes its menu). */
export async function episodeKept(page: Page, code: string): Promise<boolean> {
	await page.getByRole('button', { name: `Actions for ${code}` }).click();
	const v = await page.getByRole('menu', { name: `Actions for ${code}` }).getByRole('menuitemradio', { name: `Keep ${code} up to date` }).getAttribute('aria-checked');
	await page.keyboard.press('Escape');
	return v === 'true';
}

/** libraryKind picks a kind from the Library page's Kind chips. */
export async function libraryKind(page: Page, name: RegExp) {
	await page.getByRole('group', { name: 'Kind' }).getByRole('button', { name }).click();
}

/** signOut uses the account menu (sidebar) or, on a phone, the More sheet. */
export async function signOut(page: Page) {
	const account = page.getByRole('button', { name: 'Account and appearance' });
	if (await account.isVisible()) {
		await account.click();
		await page.getByRole('menuitem', { name: 'Sign out' }).click();
		return;
	}
	await page.locator('.ui-tabbar').getByRole('button', { name: /^(More|Manage)/ }).click();
	await page.getByRole('dialog').getByRole('button', { name: 'Sign out' }).click();
}

/** noSidewaysScroll fails when the page is wider than the screen. */
export async function noSidewaysScroll(page: Page) {
	const { sw, cw, wide } = await page.evaluate(() => {
		const cw = document.documentElement.clientWidth;
		// The deepest elements that stick out, to say what to fix.
		const wide = [...document.querySelectorAll('body *')]
			.filter((el) => el.getBoundingClientRect().right > cw + 1 && ![...el.children].some((c) => c.getBoundingClientRect().right > cw + 1))
			.slice(0, 3)
			.map((el) => `${el.tagName.toLowerCase()}.${[...el.classList].join('.')}`);
		return { sw: document.documentElement.scrollWidth, cw, wide };
	});
	expect(sw, `${page.url()} is ${sw}px wide on a ${cw}px screen (${wide.join(', ')})`).toBeLessThanOrEqual(cw);
}

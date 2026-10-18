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

/** libraryStatus picks a Library status filter from the Status menu (all, missing, not_out_yet, better, have). */
export async function libraryStatus(scope: Page | Locator, value: 'all' | 'missing' | 'not_out_yet' | 'better' | 'have') {
	const menu = scope.locator('details.status-filter:not(.requester-menu)');
	if (!(await menu.evaluate((d) => (d as HTMLDetailsElement).open))) await menu.locator('summary').click();
	await menu.getByLabel('Show titles matching').selectOption(value);
}

/** openApprovals reaches the Wishlist as an approver on a phone: Manage tab, then Approvals. */
export async function openApprovals(page: Page) {
	await page.locator('.ui-tabbar').getByRole('button', { name: /^Manage/ }).click();
	await page.getByRole('dialog', { name: 'Manage' }).getByRole('link', { name: /^Approvals/ }).click();
	await expect(page).toHaveURL(/wishlist/);
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

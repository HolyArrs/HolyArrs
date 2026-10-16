import { expect, type Page } from '@playwright/test';
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

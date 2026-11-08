import type { Browser, Page } from '@playwright/test';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { openFromSearch, type Owner } from '../helpers/helpers.ts';

// Shared by the Requests specs (asking, decisions, follows-library-artist).

export async function fresh(browser: Browser): Promise<Page> {
	return (await browser.newContext()).newPage();
}

// Search opens a movie's title page; Ask lives there. The main Ask only: a
// separate "Ask for … in 4K" can sit beside it.
export async function askButton(p: Page, words: string, name: string) {
	await openFromSearch(p, words, name);
	return p.getByTestId('title-detail').getByRole('button', { name: new RegExp(`^Ask for ${name}( too)?$`) });
}

// An approver's card on the Requests page (the cards list, every width).
export function card(p: Page, name: string) {
	return p.getByRole('list', { name: 'Requests' }).getByRole('listitem').filter({ hasText: name });
}

// kim joins in asking.spec.ts; decisions.spec.ts signs in as kim again. Her
// name and password are handed over in .tmp/requests.json, as owner.json is.
const kimFile = join(dirname(fileURLToPath(import.meta.url)), '..', '.tmp', 'requests.json');

export function saveKim(o: Owner) {
	mkdirSync(dirname(kimFile), { recursive: true });
	writeFileSync(kimFile, JSON.stringify(o));
}

export function savedKim(): Owner {
	if (!existsSync(kimFile)) throw new Error('run requests/asking.spec.ts first; it makes kim');
	return JSON.parse(readFileSync(kimFile, 'utf8'));
}

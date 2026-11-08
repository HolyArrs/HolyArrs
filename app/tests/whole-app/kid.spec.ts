import { expect, test, type Browser, type Page } from '@playwright/test';
import { owner, signIn } from '../helpers/helpers.ts';

// Kid accounts: an age-rating limit set at invite time. The fake movie
// database rates Paper Harbour PG and Ember Road R; every other fake movie
// has no rating, so a Kid never sees it.

test.describe.configure({ mode: 'serial' });

const kidPw = `e2e-${Math.random().toString(36).slice(2)}-kid`;
const allowed = ['Paper Harbour'];
const blocked = ['Ember Road', 'Glass Meridian', 'Driftwood Moon', 'Salt Choir'];

async function fresh(browser: Browser): Promise<Page> {
	return (await browser.newContext()).newPage();
}

let own: Page;
let kai: Page;

test.beforeAll(async ({ browser }) => {
	own = await fresh(browser);
	await signIn(own, owner());
});

test('the owner invites a Kid with an age rating', async ({ browser }) => {
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Invite someone' }).click();
	await own.getByLabel('Note for yourself').fill('Kai');
	await own.locator('#invite-card').getByRole('radio', { name: 'Kid' }).check();
	const rating = own.getByTestId('invite-preset-rating');
	await expect(rating.getByRole('radio', { name: 'PG', exact: true })).toHaveAttribute('aria-checked', 'true');
	await expect(own.getByText('Movies up to PG and shows up to TV-PG. Titles without a rating and music stay hidden.')).toBeVisible();
	const sent = own.waitForRequest((r) => r.url().endsWith('/api/v1/users/invites') && r.method() === 'POST');
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const body = (await sent).postDataJSON();
	expect(body.max_rating).toBe('PG');
	expect(body.perms).toEqual(['ask']);
	const link = (await own.locator('.fresh code').textContent())!.trim();

	kai = await fresh(browser);
	await kai.goto(new URL(link).pathname);
	await expect(kai.getByText('Invited as a Kid · up to PG')).toBeVisible();
	await kai.getByLabel('Name').fill('kai');
	await kai.getByLabel('Password', { exact: true }).fill(kidPw);
	await kai.getByLabel('Confirm password').fill(kidPw);
	await kai.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(kai).toHaveURL(/\/$/);

	await own.reload();
	await expect(own.getByTestId('kid-label').filter({ hasText: 'Kid · up to PG' })).toBeVisible();
});

test('a Kid sees only titles rated up to the limit, and no Music', async () => {
	const nav = kai.getByRole('navigation', { name: 'Main' }).first();
	await expect(nav.getByRole('link', { name: 'Movies', exact: true })).toBeVisible();
	await expect(nav.getByRole('link', { name: 'Music', exact: true })).toHaveCount(0);
	await kai.goto('/library?kind=movie');
	await expect(kai.getByRole('heading', { level: 1 })).toBeVisible();
	for (const name of blocked) await expect(kai.getByText(name, { exact: true })).toHaveCount(0);
	// Whatever is shown is on the allowed list.
	const shown = await kai.locator('main a[href*="/library/"]').allTextContents();
	for (const text of shown.map((t) => t.trim()).filter(Boolean)) expect(allowed.some((a) => text.includes(a))).toBe(true);
});

test('a blocked Library title is not available to a Kid, not missing', async () => {
	// The server answers 404 to a Kid for a Library title above their rating.
	const id = await own.evaluate(async (names) => {
		const r = await (await fetch('/api/v1/titles')).json();
		return (r.titles as { id: number; name: string }[]).find((t) => names.includes(t.name))?.id;
	}, blocked);
	expect(id).toBeDefined();
	await kai.goto(`/library/${id}`);
	await expect(kai.getByRole('heading', { name: 'This title is not available on this account.' })).toBeVisible();
	await expect(kai.getByRole('heading', { name: 'This title is not in your Library' })).toHaveCount(0);
});

test('an R title is not available, and asking for it is refused in plain words', async () => {
	await kai.goto('/title/movie/900006');
	await expect(kai.getByRole('heading', { name: 'This title is not available on this account.' })).toBeVisible();
	const answer = await kai.evaluate(async () => {
		const me = await (await fetch('/api/v1/auth/me')).json();
		const r = await fetch('/api/v1/asks', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': me.csrf }, body: JSON.stringify({ id: 'tmdb:900006' }) });
		return { status: r.status, body: await r.json() };
	});
	expect(answer.status).toBe(403);
	expect(answer.body.error).toBe('This account can ask only for titles rated up to PG.');
});

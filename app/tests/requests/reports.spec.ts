import { expect, test, type Browser, type Page } from '@playwright/test';
import { mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { addFromSearch, noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';
import { api } from '../helpers/api.ts';
import { tmp } from '../helpers/data-folder.ts';

// Reported problems: a member reports a problem with a movie from a
// phone (the sheet is a bottom sheet there), the owner finds it from
// Requests on Reports and marks it fixed with a reply, and the
// member sees it fixed, with the reply and a Fixed message. The spec makes
// its own data: the movie, a file for it and the member.
test.describe.configure({ mode: 'serial' });

const movie = 'Driftwood Moon';
const memberName = `rep${Math.random().toString(36).slice(2, 7)}`;
const memberPw = `e2e-${Math.random().toString(36).slice(2)}-rep`;
const note = 'The sound drifts after about 20 minutes';
const reply = 'Swapped it for a better copy, try again tonight';
const phone = { width: 390, height: 844 };

let own: Page;
let member: Page;
let titleId = 0;

async function fresh(browser: Browser, viewport = { width: 1280, height: 900 }): Promise<Page> {
	return (await browser.newContext({ viewport })).newPage();
}

test.beforeAll(async ({ browser }) => {
	own = await fresh(browser);
	await signIn(own, owner());
	// The movie: the desktop project adds it; add it here when it is not there.
	await own.goto('/library');
	await own.locator('[data-testid="library-card"], [data-testid="library-row"]').or(own.getByRole('heading', { name: 'Nothing here yet' })).first().waitFor();
	if (!(await own.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: movie }).count())) {
		const dialog = await addFromSearch(own, movie, movie);
		await dialog.getByLabel('Search when added').uncheck();
		await dialog.getByRole('button', { name: 'Add to Library' }).click();
		await expect(own).toHaveURL(/\/library\/\d+/);
	}
	const call = await api(own);
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: { id: number; name: string; kind: string }[] };
	titleId = titles.find((t) => t.kind === 'movie' && t.name === movie)?.id ?? 0;
	expect(titleId).toBeGreaterThan(0);

	// Only a title with a file can be reported: give it one by hand when it has none.
	const t = (await (await call('GET', `/titles/${titleId}`)).json()) as { slots: { file?: unknown }[] };
	if (!t.slots.some((s) => s.file)) {
		// Import by hand reads only from known folders; the Library folder is always one.
		const dir = join(tmp, 'movies', 'zz-reports-drop');
		rmSync(dir, { recursive: true, force: true });
		mkdirSync(dir, { recursive: true });
		const file = join(dir, 'Driftwood.Moon.2024.1080p.WEB-DL.H264-LAB.mkv');
		writeFileSync(file, 'stand-in video '.repeat(64));
		const r = await call('POST', '/manual-import', { items: [{ path: file, title_id: titleId, mode: 'move' }] });
		expect(r.status(), await r.text()).toBe(200);
		const [res] = ((await r.json()) as { results: { outcome: string; error?: string }[] }).results;
		expect(res.outcome, res.error).toBe('placed');
	}

	// The member, invited by the owner.
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Invite someone' }).click();
	await own.getByLabel('Note for yourself').fill('Reports');
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	member = await fresh(browser, phone);
	await member.goto(new URL(link).pathname);
	await member.getByLabel('Name', { exact: true }).fill(memberName);
	await member.getByLabel('Password', { exact: true }).fill(memberPw);
	await member.getByLabel('Confirm password').fill(memberPw);
	await member.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(member).toHaveURL(/\/$/);
});

test('a member reports a problem from a phone, in a bottom sheet', async () => {
	await member.goto(`/library/${titleId}`);
	await expect(member.getByRole('heading', { level: 1 })).toContainText(movie);
	await noSidewaysScroll(member);
	// Members get More with one item: Report a problem.
	await member.getByRole('button', { name: 'More actions', exact: true }).click();
	const items = member.getByRole('menuitem');
	await expect(items).toHaveCount(1);
	await items.filter({ hasText: 'Report a problem' }).click();

	const sheet = member.getByRole('dialog', { name: 'Report a problem' });
	await expect(sheet).toBeVisible();
	await sheet.evaluate((el) => Promise.all(el.getAnimations().map((a) => a.finished.catch(() => undefined))));
	// At phone width the sheet sits on the bottom edge, as wide as the screen.
	const box = (await sheet.boundingBox())!;
	expect(Math.round(box.width)).toBe(phone.width);
	expect(Math.round(box.y + box.height)).toBeGreaterThanOrEqual(phone.height - 2);
	await noSidewaysScroll(member);

	await expect(sheet.getByRole('radio')).toHaveCount(5);
	await expect(sheet.getByRole('button', { name: 'Send report' })).toBeDisabled();
	await sheet.getByRole('radio', { name: 'Sound' }).check();
	await sheet.getByLabel(/Anything else/).fill(note);
	await sheet.getByRole('button', { name: 'Send report' }).click();
	await expect(member.getByTestId('report-sent')).toContainText('Sound');

	await member.getByRole('link', { name: 'See your reports' }).click();
	await expect(member).toHaveURL(/\/requests\/reports/);
	await expect(member.getByRole('heading', { level: 1, name: 'Your reports' })).toBeVisible();
	const card = member.getByTestId('report').filter({ hasText: movie });
	await expect(card).toHaveCount(1);
	await expect(card.getByTestId('report-state')).toContainText('Open');
	await expect(card.getByTestId('report-kind')).toHaveText('Sound');
	await expect(card).toContainText(note);
	// Members cannot close reports.
	await expect(member.getByRole('button', { name: 'Mark fixed' })).toHaveCount(0);
	await noSidewaysScroll(member);
});

test('the owner finds it from Requests and marks it fixed with a reply', async () => {
	await own.goto('/requests');
	const link = own.getByTestId('reports-link');
	await expect(link).toContainText(/Reports/);
	await expect(link).toContainText(/\d+/);
	await link.click();
	await expect(own).toHaveURL(/\/requests\/reports/);
	await expect(own.getByRole('heading', { level: 1, name: 'Reports' })).toBeVisible();

	const card = own.getByTestId('report').filter({ hasText: memberName });
	await expect(card).toHaveCount(1);
	await expect(card).toContainText(movie);
	await expect(card.getByTestId('report-state')).toContainText('Open');
	await card.getByRole('button', { name: 'Mark fixed' }).click();

	// Mark fixed opens the report's window (named after the title), ready for a reply.
	const sheet = own.getByRole('dialog', { name: new RegExp(movie) });
	await expect(sheet).toBeVisible();
	await sheet.getByLabel(new RegExp(`^Reply to ${memberName}`)).fill(reply);
	await sheet.getByRole('button', { name: 'Mark fixed' }).click();
	await expect(sheet).toBeHidden();
	await expect(own.getByText(`${memberName} gets a message`)).toBeVisible();

	// Off the Open list, on the Fixed list with the reply.
	await expect(card).toHaveCount(0);
	await own.getByRole('button', { name: /^Fixed/ }).click();
	await expect(card.getByTestId('report-state')).toContainText('Fixed');
	await expect(card.getByTestId('report-reply')).toHaveText(reply);
	await expect(card.getByRole('button', { name: 'Reopen' })).toBeVisible();
});

test('the member sees it fixed, with the reply and a Fixed message', async () => {
	await member.goto('/requests/reports');
	const card = member.getByTestId('report').filter({ hasText: movie });
	await expect(card.getByTestId('report-state')).toContainText('Fixed');
	await expect(card.getByTestId('report-reply')).toHaveText(reply);
	await noSidewaysScroll(member);

	// The Fixed news pops once as a pill wherever they are (maybe already gone by now) and
	// stays in Recent alerts behind the bell.
	await member.goto('/requests');
	// On a phone the bell is in the More window.
	await member.getByRole('button', { name: /^More/ }).click();
	await member.getByRole('link', { name: /^Recent alerts/ }).click();
	const recent = member.getByRole('dialog', { name: 'Recent alerts' });
	await expect(recent).toContainText('Problem fixed');
	await expect(recent).toContainText(movie);
	await member.keyboard.press('Escape');
	await noSidewaysScroll(member);
});

test.afterAll(async () => {
	await own?.context().close();
	await member?.context().close();
});

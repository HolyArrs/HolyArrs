import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { noSidewaysScroll, openAdvanced, owner, signIn } from '../helpers/helpers.ts';

// Pick rules: copy a ready-made set, change a step and a trait, save, see it
// after a reload, change it again, remove the copy (asked first); a
// ready-made set keeps its name and cannot be removed; a phone and
// accessibility.
test.describe.configure({ mode: 'serial' });

let page: Page;
test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
});
test.afterAll(async () => {
	await page.context().close();
});

/** row is one set on the Pick rules list, found by its exact name. */
const row = (name: string) => page.getByTestId('pick-rule').filter({ has: page.getByText(name, { exact: true }) });

async function more(name: string, item: string) {
	await row(name).getByRole('button', { name: `More for ${name}` }).click();
	await page.getByRole('menu', { name: `More for ${name}` }).getByRole('menuitem', { name: item }).click();
}

test('copy Standard, change a step and a trait, save, and the changes stay after a reload', async () => {
	await page.goto('/settings/pick-rules');
	await expect(row('Standard')).toContainText('Ready-made');
	await more('Standard', 'Make a copy');
	await expect(page).toHaveURL(/\/settings\/pick-rules\/new\?from=\d+$/);
	await expect(page.getByRole('heading', { name: 'Add a quality set', level: 1 })).toBeVisible();
	await expect(page.getByLabel('Name', { exact: true })).toHaveValue('Standard copy');

	// A step: 4K web on.
	const fourK = page.getByRole('switch', { name: '4K web' });
	await expect(fourK).not.toBeChecked();
	await fourK.check();
	// A trait: HEVC is worth 25 points instead of 10.
	const hevc = page.getByLabel('Points for HEVC (smaller files)');
	await expect(hevc).toHaveValue('10');
	await hevc.fill('25');

	const bar = page.getByTestId('savebar');
	await bar.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Standard copy made')).toBeVisible();
	await expect(page).toHaveURL(/\/settings\/pick-rules\/\d+$/);
	await expect(bar).toHaveCount(0);

	await page.reload();
	await expect(page.getByRole('heading', { name: 'Standard copy', level: 1 })).toBeVisible();
	await expect(page.getByRole('switch', { name: '4K web' })).toBeChecked();
	await expect(page.getByRole('switch', { name: '720p web' })).toBeChecked();
	await expect(page.getByLabel('Points for HEVC (smaller files)')).toHaveValue('25');
	await expect(page.getByLabel('Good enough at')).toHaveValue('1080p_web');
});

test('a mistake is named on its field before anything is sent, and Save keeps a change', async () => {
	// Turning off the Good enough step is caught on the page.
	await page.getByRole('switch', { name: '1080p web' }).uncheck();
	const bar = page.getByTestId('savebar');
	await bar.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Good enough must be a step that is on.')).toBeVisible();
	await bar.getByRole('button', { name: 'Discard' }).click();
	await expect(page.getByRole('switch', { name: '1080p web' })).toBeChecked();
	await expect(page.getByText('Good enough must be a step that is on.')).toHaveCount(0);

	await page.getByLabel('Wait for Usenet (minutes)').fill('30');
	await bar.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Quality set saved')).toBeVisible();
	await expect(bar).toHaveCount(0);
	await page.reload();
	await expect(page.getByLabel('Wait for Usenet (minutes)')).toHaveValue('30');
	await expect(page.getByLabel('Points for HEVC (smaller files)')).toHaveValue('25');
});

test('removing asks first, and then the copy is gone', async () => {
	await page.goto('/settings/pick-rules');
	await expect(row('Standard copy')).toContainText('Not used yet');
	await more('Standard copy', 'Remove');
	const sheet = page.getByRole('dialog', { name: 'Remove this quality set?' });
	await expect(sheet).toContainText('Standard copy');
	await sheet.getByRole('button', { name: 'Cancel' }).click();
	await expect(sheet).toBeHidden();
	await expect(row('Standard copy')).toHaveCount(1);

	await more('Standard copy', 'Remove');
	await sheet.getByRole('button', { name: 'Remove', exact: true }).click();
	await expect(page.getByText('Standard copy removed')).toBeVisible();
	await expect(row('Standard copy')).toHaveCount(0);
	await page.reload();
	await expect(row('Standard')).toHaveCount(1);
	await expect(row('Standard copy')).toHaveCount(0);
});

test('a ready-made set cannot be removed or renamed, but can be copied and changed', async () => {
	await page.goto('/settings/pick-rules');
	await row('Standard').getByRole('button', { name: 'More for Standard' }).click();
	const menu = page.getByRole('menu', { name: 'More for Standard' });
	await expect(menu.getByRole('menuitem', { name: 'Make a copy' })).toBeVisible();
	await expect(menu.getByRole('menuitem', { name: /^Remove/ })).toHaveCount(0);
	await page.keyboard.press('Escape');

	await row('Standard').getByRole('link', { name: 'Change' }).click();
	await expect(page.getByRole('heading', { name: 'Standard', level: 1 })).toBeVisible();
	await expect(page.getByTestId('preset-name')).toContainText('Standard');
	await expect(page.getByLabel('Name', { exact: true })).toHaveCount(0);
	await expect(page.getByRole('switch', { name: '1080p web' })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Make a copy' })).toBeVisible();
});

test('a new set starts for movies and TV shows; ticking Music switches to the music steps', async () => {
	await page.goto('/settings/pick-rules/new');
	await expect(page.getByLabel('Movies', { exact: true })).toBeChecked();
	await expect(page.getByLabel('TV shows', { exact: true })).toBeChecked();
	await expect(page.getByRole('switch', { name: '1080p web' })).toBeVisible();
	await page.getByLabel('Music', { exact: true }).check();
	await expect(page.getByLabel('Movies', { exact: true })).not.toBeChecked();
	await expect(page.getByRole('switch', { name: 'Lossless (CD quality)' })).toBeChecked();
	await expect(page.getByRole('switch', { name: '1080p web' })).toHaveCount(0);
	// Nothing was made: leaving asks first.
	// The page header has no back link; leave through the Settings menu.
	await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'Quality', exact: true }).click();
	await page.getByRole('dialog', { name: 'Leave without saving?' }).getByRole('button', { name: 'Leave without saving' }).click();
	await expect(page).toHaveURL(/\/settings\/pick-rules$/);
});

// Your own traits: made on the list page (mistakes named first), used
// in a copy's Never, and taken out of it again when removed. Whatever the
// test made is removed at the end, pass or fail.
test('your own trait is made, used in Never and taken out of the rules when removed', async () => {
	test.setTimeout(90_000);
	const me = (await (await page.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	const call = (method: string, path: string, data?: unknown) => page.request.fetch(`/api/v1${path}`, { method, data, headers: { 'X-CSRF-Token': me.csrf } });
	try {
		await page.goto('/settings/pick-rules');
		const card = page.locator('#own-traits');
		await expect(card).toContainText('No traits of your own');
		await card.getByRole('button', { name: 'Make a trait' }).click();
		const sheet = page.getByRole('dialog', { name: 'Make a trait' });
		await sheet.getByLabel('Name').fill('Dutch subs');
		await sheet.getByRole('button', { name: 'Make trait' }).click();
		await expect(sheet.getByText('List at least one word or pattern.')).toBeVisible();
		await sheet.getByLabel('Words').fill('NLSubs\n/(open/');
		await sheet.getByRole('button', { name: 'Make trait' }).click();
		await expect(sheet.getByText('/(open/ is not a pattern HolyArrs can read (missing closing )).')).toBeVisible();
		await sheet.evaluate((el) => Promise.all(el.getAnimations().map((x) => x.finished)));
		expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
		await sheet.getByLabel('Words').fill('NLSubs\n/\\bdutch\\b/');
		await sheet.getByRole('button', { name: 'Make trait' }).click();
		await expect(page.getByText('Dutch subs made')).toBeVisible();
		await expect(page.getByTestId('own-trait')).toContainText('Name has: NLSubs, /\\bdutch\\b/');

		// A copy of Standard never takes it.
		await more('Standard', 'Make a copy');
		const never = page.getByLabel('Add to Never');
		await expect(never.locator('optgroup[label="Your traits"] option', { hasText: 'Dutch subs' })).toHaveCount(1);
		await never.selectOption({ label: 'Dutch subs' });
		await expect(page.getByTestId('never').getByText('Dutch subs')).toBeVisible();
		await page.getByTestId('savebar').getByRole('button', { name: 'Save' }).click();
		await expect(page).toHaveURL(/\/settings\/pick-rules\/\d+$/);
		const copy = page.url();
		await page.reload();
		await expect(page.getByTestId('never').getByText('Dutch subs')).toBeVisible();

		// Removing the trait takes it out of the copy.
		await page.goto('/settings/pick-rules');
		await page.getByTestId('own-trait').getByRole('button', { name: 'More for Dutch subs' }).click();
		await page.getByRole('menu', { name: 'More for Dutch subs' }).getByRole('menuitem', { name: 'Remove' }).click();
		await page.getByRole('dialog', { name: 'Remove this trait?' }).getByRole('button', { name: 'Remove', exact: true }).click();
		await expect(page.getByText('Dutch subs removed')).toBeVisible();
		await expect(page.getByTestId('own-trait')).toHaveCount(0);
		await page.goto(copy);
		await expect(page.getByTestId('never')).not.toContainText('Dutch subs');
	} finally {
		const ts = ((await (await call('GET', '/own-traits')).json()) as { own_traits: { id: number }[] }).own_traits;
		for (const t of ts) await call('DELETE', `/own-traits/${t.id}`);
		const rs = ((await (await call('GET', '/pick-rules')).json()) as { pick_rules: { id: number; name: string }[] }).pick_rules;
		for (const r of rs.filter((x) => x.name === 'Standard copy')) await call('DELETE', `/pick-rules/${r.id}`);
	}
});

test('a phone: the list and the editor fit without sideways scroll', async () => {
	await page.setViewportSize({ width: 320, height: 720 });
	await page.goto('/settings/pick-rules');
	await expect(row('Standard')).toBeVisible();
	await noSidewaysScroll(page);
	await row('Standard').getByRole('link', { name: 'Change' }).click();
	await expect(page.getByRole('switch', { name: '1080p web' })).toBeVisible();
	await openAdvanced(page);
	await noSidewaysScroll(page);
	await page.setViewportSize({ width: 1280, height: 720 });
});

test('accessibility: the list, the remove sheet and a copy in the editor with Advanced open', async () => {
	await page.goto('/settings/pick-rules');
	await expect(row('Standard')).toBeVisible();
	expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
	// The remove sheet, on a set nothing uses (a fresh copy, made and then removed).
	await more('Standard', 'Make a copy');
	await page.getByTestId('savebar').getByRole('button', { name: 'Save' }).click();
	await expect(page).toHaveURL(/\/settings\/pick-rules\/\d+$/);
	await page.goto('/settings/pick-rules');
	await more('Standard copy', 'Remove');
	const sheet = page.getByRole('dialog', { name: 'Remove this quality set?' });
	await expect(sheet).toBeVisible();
	await sheet.evaluate((el) => Promise.all(el.getAnimations().map((x) => x.finished)));
	expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
	await sheet.getByRole('button', { name: 'Remove', exact: true }).click();
	await expect(row('Standard copy')).toHaveCount(0);
	await more('Standard', 'Make a copy');
	await expect(page.getByLabel('Name', { exact: true })).toHaveValue('Standard copy');
	await openAdvanced(page);
	// The save bar rises in (opacity from 0); check its colours once it has settled.
	const bar = page.getByTestId('savebar');
	await bar.evaluate((el) => Promise.all(el.getAnimations({ subtree: true }).map((x) => x.finished)));
	expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

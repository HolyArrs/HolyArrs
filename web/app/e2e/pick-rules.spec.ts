import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { noSidewaysScroll, openAdvanced, owner, signIn } from './helpers.ts';

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

test('copy Good everyday, change a step and a trait, save, and the changes stay after a reload', async () => {
	await page.goto('/settings/pick-rules');
	await expect(row('Good everyday')).toContainText('Ready-made');
	await more('Good everyday', 'Make a copy');
	await expect(page).toHaveURL(/\/settings\/pick-rules\/new\?from=\d+$/);
	await expect(page.getByRole('heading', { name: 'New Pick rules', level: 1 })).toBeVisible();
	await expect(page.getByLabel('Name', { exact: true })).toHaveValue('Good everyday copy');

	// A step: 4K web on.
	const fourK = page.getByRole('switch', { name: '4K web' });
	await expect(fourK).not.toBeChecked();
	await fourK.check();
	// A trait: HEVC is worth 25 points instead of 10.
	const hevc = page.getByLabel('Points for HEVC (smaller files)');
	await expect(hevc).toHaveValue('10');
	await hevc.fill('25');

	const bar = page.getByTestId('savebar');
	await bar.getByRole('button', { name: 'Make these Pick rules' }).click();
	await expect(page.getByText('Good everyday copy made.')).toBeVisible();
	await expect(page).toHaveURL(/\/settings\/pick-rules\/\d+$/);
	await expect(bar).toHaveCount(0);

	await page.reload();
	await expect(page.getByRole('heading', { name: 'Good everyday copy', level: 1 })).toBeVisible();
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
	await expect(page.getByText('Pick rules saved.')).toBeVisible();
	await expect(bar).toHaveCount(0);
	await page.reload();
	await expect(page.getByLabel('Wait for Usenet (minutes)')).toHaveValue('30');
	await expect(page.getByLabel('Points for HEVC (smaller files)')).toHaveValue('25');
});

test('removing asks first, and then the copy is gone', async () => {
	await page.goto('/settings/pick-rules');
	await expect(row('Good everyday copy')).toContainText('Not used yet');
	await more('Good everyday copy', 'Remove…');
	const sheet = page.getByRole('dialog', { name: 'Remove these Pick rules?' });
	await expect(sheet).toContainText('Good everyday copy');
	await sheet.getByRole('button', { name: 'Cancel' }).click();
	await expect(sheet).toBeHidden();
	await expect(row('Good everyday copy')).toHaveCount(1);

	await more('Good everyday copy', 'Remove…');
	await sheet.getByRole('button', { name: 'Remove', exact: true }).click();
	await expect(page.getByText('Good everyday copy removed.')).toBeVisible();
	await expect(row('Good everyday copy')).toHaveCount(0);
	await page.reload();
	await expect(row('Good everyday')).toHaveCount(1);
	await expect(row('Good everyday copy')).toHaveCount(0);
});

test('a ready-made set cannot be removed or renamed, but can be copied and changed', async () => {
	await page.goto('/settings/pick-rules');
	await row('Good everyday').getByRole('button', { name: 'More for Good everyday' }).click();
	const menu = page.getByRole('menu', { name: 'More for Good everyday' });
	await expect(menu.getByRole('menuitem', { name: 'Make a copy' })).toBeVisible();
	await expect(menu.getByRole('menuitem', { name: /^Remove/ })).toHaveCount(0);
	await page.keyboard.press('Escape');

	await row('Good everyday').getByRole('link', { name: 'Change' }).click();
	await expect(page.getByRole('heading', { name: 'Good everyday', level: 1 })).toBeVisible();
	await expect(page.getByTestId('preset-name')).toContainText('Good everyday');
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
	await page.locator('header.ui-head a.back').click();
	await page.getByRole('dialog', { name: 'Leave without saving?' }).getByRole('button', { name: 'Leave without saving' }).click();
	await expect(page).toHaveURL(/\/settings\/pick-rules$/);
});

test('a phone: the list and the editor fit without sideways scroll', async () => {
	await page.setViewportSize({ width: 320, height: 720 });
	await page.goto('/settings/pick-rules');
	await expect(row('Good everyday')).toBeVisible();
	await noSidewaysScroll(page);
	await row('Good everyday').getByRole('link', { name: 'Change' }).click();
	await expect(page.getByRole('switch', { name: '1080p web' })).toBeVisible();
	await openAdvanced(page);
	await noSidewaysScroll(page);
	await page.setViewportSize({ width: 1280, height: 720 });
});

test('accessibility: the list, the remove sheet and a copy in the editor with Advanced open', async () => {
	await page.goto('/settings/pick-rules');
	await expect(row('Good everyday')).toBeVisible();
	expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
	// The remove sheet, on a set nothing uses (a fresh copy, made and then removed).
	await more('Good everyday', 'Make a copy');
	await page.getByTestId('savebar').getByRole('button', { name: 'Make these Pick rules' }).click();
	await expect(page).toHaveURL(/\/settings\/pick-rules\/\d+$/);
	await page.goto('/settings/pick-rules');
	await more('Good everyday copy', 'Remove…');
	const sheet = page.getByRole('dialog', { name: 'Remove these Pick rules?' });
	await expect(sheet).toBeVisible();
	await sheet.evaluate((el) => Promise.all(el.getAnimations().map((x) => x.finished)));
	expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
	await sheet.getByRole('button', { name: 'Remove', exact: true }).click();
	await expect(row('Good everyday copy')).toHaveCount(0);
	await more('Good everyday', 'Make a copy');
	await expect(page.getByLabel('Name', { exact: true })).toHaveValue('Good everyday copy');
	await openAdvanced(page);
	expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

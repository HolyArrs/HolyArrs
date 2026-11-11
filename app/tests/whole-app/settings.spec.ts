import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn, addFromSearch } from '../helpers/helpers.ts';

// Settings: My account on top, then the six groups, setting search with a deep link that scrolls to
// and highlights the field, the save bar and its leave guard, Advanced
// disclosures that stay as each person left them, and Movies and TV defaults
// that the add dialog starts from.
test.describe.configure({ mode: 'serial' });

let page: Page;
test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
});
test.afterAll(async () => {
	await page.context().close();
});

test('Settings home: five group cards; a card opens its group with a row of capsules, Back returns', async () => {
	await page.goto('/settings');
	await expect(page.getByRole('heading', { name: 'Settings', level: 1 })).toBeVisible();
	await expect(page.getByTestId('settings-group').getByRole('heading', { level: 2 })).toHaveText([
		'Library', 'Downloads', 'People', 'Linked apps', 'Server'
	]);
	await expect(page.getByTestId('settings-group').filter({ hasText: 'Downloads' })).toContainText('How titles arrive');
	await page.getByTestId('settings-group').filter({ hasText: 'Downloads' }).click();
	await expect(page).toHaveURL(/\/settings\/downloaders$/);
	await expect(page.getByTestId('settings-group-name')).toHaveText('Downloads');
	const nav = page.getByRole('navigation', { name: 'Settings sections' });
	await expect(nav.getByRole('link', { name: 'Quality', exact: true })).toBeVisible();
	await expect(nav.getByRole('link', { name: 'Library folders' })).toHaveCount(0);
	await page.getByTestId('settings-back').click();
	await expect(page).toHaveURL(/\/settings$/);
	await expect(page.locator('.sx-ghost')).toHaveCount(0);
});

test('search finds one setting, and its link scrolls to it, opens Advanced and highlights it', async () => {
	await page.goto('/settings');
	await page.getByTestId('search-box').fill('free space');
	const result = page.getByTestId('setting-result').filter({ hasText: 'Always leave this much free space' });
	await expect(result).toContainText('Library › Naming and files › Advanced');
	await result.click();
	await expect(page).toHaveURL(/\/settings\/naming#min-free$/);
	const field = page.locator('#min-free');
	await expect(field).toBeVisible();
	await expect(field).toBeInViewport();
	await expect(field).toBeFocused();
	await expect(page.locator('.flash-target')).toContainText('Always leave this much free space');
	// The Advanced it sits in is open now.
	await expect(page.locator('#where-files-go details.advanced')).toHaveAttribute('open', '');
});

test('a direct link to a field inside More options opens it, scrolls there and focuses it', async () => {
	await page.goto('/settings/sign-in#hosts-card');
	await expect(page.locator('details[data-testid="more-options"]').filter({ has: page.locator('#hosts-card') })).toHaveAttribute('open', '');
	await expect(page.locator('#hosts-card')).toBeInViewport();
	await page.goto('/settings/users#inv-note');
	await expect(page.locator('#inv-note')).toBeVisible();
	await expect(page.locator('#inv-note')).toBeFocused();
	await page.goto('/settings/account#pw-current');
	await expect(page.locator('#pw-current')).toBeVisible();
});

test('a failed trailers save puts the switch back and says why, in plain words', async () => {
	await page.goto('/settings/appearance');
	const box = page.locator('#autoplay-trailers');
	const was = await box.isChecked();
	await page.route('**/api/v1/settings/ui', (r) => (r.request().method() === 'PUT' ? r.fulfill({ status: 500, json: { error: 'disk is read-only' } }) : r.fallback()));
	await box.click();
	await expect(page.getByTestId('autoplay-error')).toHaveText('Not saved. Disk is read-only.');
	await expect(box).toBeChecked({ checked: was });
	await page.unroute('**/api/v1/settings/ui');
});

test('the minimal look is saved in this browser, lasts a reload, and Full brings the full look back', async () => {
	await page.goto('/settings/appearance');
	const looks = page.locator('#look');
	await looks.getByRole('radio', { name: 'Minimal' }).click();
	await expect(page.locator('html')).toHaveAttribute('data-look', 'minimal');
	await expect(page.getByTestId('wall-off-minimal')).toBeVisible();
	await page.reload();
	await expect(page.locator('html')).toHaveAttribute('data-look', 'minimal');
	await expect(looks.getByRole('radio', { name: 'Minimal' })).toHaveAttribute('aria-checked', 'true');
	await looks.getByRole('radio', { name: 'Full' }).click();
	await expect(page.locator('html')).not.toHaveAttribute('data-look', /./);
	await expect(page.getByTestId('wall-off-minimal')).toHaveCount(0);
});

test('search by another word, and the keyboard reaches the result', async () => {
	await page.goto('/settings');
	const box = page.getByTestId('search-box');
	await box.fill('dark mode');
	await expect(page.getByTestId('setting-result').first()).toContainText('Theme');
	await box.press('Tab');
	await expect(page.getByTestId('setting-result').first()).toBeFocused();
	await page.keyboard.press('Enter');
	await expect(page).toHaveURL(/\/settings\/appearance#theme$/);
	await expect(page.getByRole('radiogroup', { name: 'Theme' })).toBeVisible();
	await page.goto('/settings');
	await page.getByTestId('search-box').fill('zebra crossing');
	await expect(page.getByText('No matching settings')).toBeVisible();
});

test('the save bar shows only with a change; Discard puts it back', async () => {
	await page.goto('/settings/music');
	await expect(page.getByRole('heading', { name: 'Music defaults', level: 1 })).toBeVisible();
	const bar = page.getByTestId('savebar');
	await expect(bar).toHaveCount(0);
	const before = await page.getByLabel('New albums').inputValue();
	const other = await page.getByLabel('New albums').locator('option').evaluateAll((os, b) => (os as HTMLOptionElement[]).map((o) => o.value).find((v) => v !== b)!, before);
	await page.getByLabel('New albums').selectOption(other);
	await expect(bar).toContainText('Unsaved changes');
	await bar.getByRole('button', { name: 'Discard' }).click();
	await expect(bar).toHaveCount(0);
	await expect(page.getByLabel('New albums')).toHaveValue(before);
});

test('leaving with unsaved changes asks first; Stay keeps them, Leave goes', async () => {
	await page.goto('/settings/requests');
	await page.getByLabel('Movie requests', { exact: true }).fill('7');
	await expect(page.getByTestId('savebar')).toBeVisible();
	// Another page of the People group, from its capsule row.
	await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'Sign-in and security' }).click();
	const ask = page.getByRole('dialog', { name: 'Leave without saving?' });
	await expect(ask).toBeVisible();
	await ask.getByRole('button', { name: 'Stay' }).click();
	await expect(ask).toBeHidden();
	await expect(page).toHaveURL(/\/settings\/requests$/);
	await expect(page.getByLabel('Movie requests', { exact: true })).toHaveValue('7');

	// Another page of the People group, from its capsule row.
	await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'Sign-in and security' }).click();
	await page.getByRole('dialog', { name: 'Leave without saving?' }).getByRole('button', { name: 'Leave without saving' }).click();
	await expect(page.getByRole('heading', { name: 'Sign-in and security', level: 1 })).toBeVisible();
	await page.goto('/settings/requests');
	await expect(page.getByLabel('Movie requests', { exact: true })).not.toHaveValue('7');
});

test('Save in the bar saves through the page form, with a toast', async () => {
	await page.goto('/settings/requests');
	const field = page.getByLabel('Movie requests', { exact: true });
	const before = await field.inputValue();
	await field.fill(String(Number(before) + 1));
	await page.getByTestId('savebar').getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Request settings saved')).toBeVisible();
	await expect(page.getByTestId('savebar')).toHaveCount(0);
	await page.reload();
	await expect(field).toHaveValue(String(Number(before) + 1));
	await field.fill(before);
	await page.getByTestId('savebar').getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Request settings saved').first()).toBeVisible();
});

test('Advanced starts closed and stays as the person left it', async () => {
	await page.goto('/settings/search');
	const card = page.locator('#early-fake');
	const advanced = card.locator('details.advanced');
	await expect(advanced).not.toHaveAttribute('open', '');
	await expect(page.getByLabel('Hours a Release may be early')).toBeHidden();
	const kept = () => page.evaluate(() => Object.keys(localStorage).filter((k) => k.startsWith('holyarrs.settings.advanced.') && k.endsWith('.early-fake')).length);
	await advanced.getByText('Advanced').click();
	await expect(page.getByLabel('Hours a Release may be early')).toBeVisible();
	await expect.poll(kept).toBe(1);
	await page.reload();
	await expect(page.getByLabel('Hours a Release may be early')).toBeVisible();
	// Other cards keep their own state.
	await expect(page.locator('#season-packs details.advanced')).not.toHaveAttribute('open', '');
	await advanced.getByText('Advanced').click();
	await expect.poll(kept).toBe(0);
	await page.reload();
	await expect(page.getByLabel('Hours a Release may be early')).toBeHidden();
});

test('Movies and TV defaults: saved choices are where the add movie dialog starts', async () => {
	await page.goto('/settings/title-defaults');
	await expect(page.getByRole('heading', { name: 'Movies and TV defaults', level: 1 })).toBeVisible();
	const movies = page.locator('#movie-defaults');
	const folder = movies.getByLabel('Library folder');
	await expect(folder).toHaveValue('0');
	await expect(movies.getByLabel('Search when added')).toBeChecked();
	await expect(page.locator('#show-defaults').getByLabel('Search when added')).not.toBeChecked();

	// A folder and Pick rules of its own, Keep up to date and Search when added off.
	const first = (await folder.locator('option').nth(1).getAttribute('value'))!;
	await folder.selectOption(first);
	await movies.getByLabel('Quality', { exact: true }).selectOption({ label: '4K' });
	await movies.getByLabel('Keep up to date').uncheck();
	await movies.getByLabel('Search when added').uncheck();
	const bar = page.getByTestId('savebar');
	await bar.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Movies and TV defaults saved')).toBeVisible();
	await expect(bar).toHaveCount(0);
	await page.reload();
	await expect(movies.getByLabel('Library folder')).toHaveValue(first);
	await expect(movies.getByLabel('Quality', { exact: true }).locator('option:checked')).toHaveText('4K');
	await expect(movies.getByLabel('Keep up to date')).not.toBeChecked();
	await expect(movies.getByLabel('Search when added')).not.toBeChecked();

	// The add dialog starts from them (Salt Choir is never added by the earlier tests).
	const dialog = await addFromSearch(page, 'salt choir', 'Salt Choir');
	await expect(dialog.getByLabel('Library folder')).toHaveValue(first);
	await expect(dialog.getByLabel('Pick rules').locator('option:checked')).toHaveText('4K');
	await expect(dialog.getByLabel('Keep up to date')).not.toBeChecked();
	await expect(dialog.getByLabel('Search when added')).not.toBeChecked();
	// A folder chosen in one add is not where the next add starts.
	const values = await dialog.getByLabel('Library folder').locator('option').evaluateAll((os) => os.map((o) => (o as HTMLOptionElement).value));
	const other = values.find((v) => v !== first);
	if (other) {
		await dialog.getByLabel('Library folder').selectOption(other);
		await dialog.getByRole('button', { name: 'Cancel' }).click();
		await expect(dialog).toBeHidden();
		await page.getByRole('button', { name: 'Add Salt Choir' }).click();
		await expect(dialog.getByLabel('Library folder')).toHaveValue(first);
		await expect(dialog.getByLabel('Library folder')).not.toHaveValue(other);
	} else {
		test.info().annotations.push({ type: 'note', description: 'Only one movie folder: the second-add folder check needs two.' });
	}
	await dialog.getByRole('button', { name: 'Cancel' }).click();
	await expect(dialog).toBeHidden();

	// Back to the built-in choices for the tests after this one.
	await page.goto('/settings/title-defaults');
	await movies.getByLabel('Library folder').selectOption('0');
	await movies.getByLabel('Quality', { exact: true }).selectOption('0');
	await movies.getByLabel('Keep up to date').check();
	await movies.getByLabel('Search when added').check();
	await bar.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Movies and TV defaults saved').first()).toBeVisible();
	await expect(bar).toHaveCount(0);
});

test('Danger zone is last, outlined, and asks before a restart', async () => {
	await page.goto('/settings/danger');
	await page.getByTestId('control-app').getByRole('button', { name: 'Restart HolyArrs' }).click();
	const box = page.getByTestId('confirm');
	await expect(box).toContainText('restart HolyArrs');
	await box.getByRole('button', { name: 'Cancel' }).click();
	await expect(box).toHaveCount(0);
});

test('a phone: Settings home is the cards, a page opens alone with a back link, no sideways scroll', async () => {
	await page.setViewportSize({ width: 320, height: 720 });
	await page.goto('/settings');
	await noSidewaysScroll(page);
	await page.getByTestId('settings-group').first().click();
	const back = page.getByRole('link', { name: 'Settings', exact: true });
	await expect(back).toBeVisible();
	await noSidewaysScroll(page);
	await back.click();
	await expect(page).toHaveURL(/\/settings$/);
	await page.setViewportSize({ width: 1280, height: 720 });
});

test('accessibility: Settings home, a search and a page with the save bar', async () => {
	await page.goto('/settings');
	expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
	await page.getByTestId('search-box').fill('password');
	expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
	await page.goto('/settings/naming');
	await page.getByLabel('Keep deleted files for (days)').fill('8');
	await expect(page.getByTestId('savebar')).toBeVisible();
	// Scan the bar once it has finished rising in, not halfway through its fade.
	await page.getByTestId('savebar').evaluate((el) => Promise.all(el.getAnimations({ subtree: true }).map((x) => x.finished)));
	expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
	await page.getByTestId('savebar').getByRole('button', { name: 'Discard' }).click();
});

test('URL base: a saved path is cleaned, asks for a restart, and can be cleared', async () => {
	await page.goto('/settings/this-app');
	// URL base is a rare setting, so it sits under More options (closed until opened).
	await page.getByTestId('more-options').getByText('More options').click();
	const card = page.getByTestId('url-base-form');
	await expect(card).toBeVisible();
	await card.getByLabel('URL base').fill('holyarrs/');
	await card.getByRole('button', { name: 'Save' }).click();
	await expect(card.getByLabel('URL base')).toHaveValue('/holyarrs');
	await expect(card.getByRole('status')).toContainText('Needs a restart');
	// Back to none so the tests after this one start the same way.
	await card.getByLabel('URL base').fill('');
	await card.getByRole('button', { name: 'Save' }).click();
	await expect(card.getByLabel('URL base')).toHaveValue('');
	await expect(card.getByRole('status')).toHaveCount(0);
});

test('Requests from Seerr and Overseerr: switch on, make a key shown once, remove it (#85)', async () => {
	await page.goto('/settings/api');
	const card = page.locator('#seerr-card');
	await expect(card).toBeVisible();
	const on = card.getByTestId('seerr-on');
	await expect(on).not.toBeChecked();
	await on.check();
	await expect(on).toBeChecked();
	const shows = card.getByTestId('seerr-side-sonarr');
	await expect(shows).toContainText('/sonarr');
	await expect(card.getByTestId('seerr-side-radarr')).toContainText('/radarr');
	await expect(card.getByTestId('seerr-delete')).not.toBeChecked();

	await shows.getByRole('button', { name: 'Make key' }).click();
	const fresh = card.getByTestId('seerr-fresh');
	await expect(fresh).toContainText('shown only once');
	const key = (await fresh.locator('code').textContent())!.trim();
	expect(key).toMatch(/^[0-9a-f]{32}$/);
	await expect(card.getByTestId('seerr-key-sonarr')).toContainText(`Key ending in ${key.slice(-4)},`);
	await fresh.getByRole('button', { name: 'Close', exact: true }).click();
	await expect(fresh).toHaveCount(0);
	// Shown once: after a reload only the last four characters are left.
	await page.reload();
	await expect(card.getByTestId('seerr-key-sonarr')).toContainText(key.slice(-4));
	await expect(card).not.toContainText(key);

	page.once('dialog', (d) => void d.accept());
	await card.getByTestId('seerr-side-sonarr').getByRole('button', { name: 'Remove key' }).click();
	await expect(card.getByTestId('seerr-side-sonarr')).toContainText('No key');
	await card.getByTestId('seerr-on').uncheck();
	await expect(card.getByTestId('seerr-side-sonarr')).toHaveCount(0);
});

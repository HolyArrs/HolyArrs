import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';

// Parts and controls (3.7.7) and the first-run "What do you use?" step
// (3.7.8): a part turned off and on with the two-step confirm, a refused
// switch says what it needs, a stage paused and resumed, a linked app
// reconnected, the whole-app restart asks first (and is cancelled here),
// the Events of the controls used, "Just movies" in one click, a phone and
// accessibility.
test.describe.configure({ mode: 'serial' });

let page: Page;

test.beforeAll(async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
	page = await ctx.newPage();
	await signIn(page, owner());
});

const ids: Record<string, string> = { Movies: 'movies', 'TV shows': 'shows', Music: 'music', Subtitles: 'subtitles', 'Requests': 'wishlist', Sources: 'sources' };
const part = (name: string) => page.locator(`[data-testid="part"][data-part="${ids[name]}"]`);

async function confirm(label: RegExp) {
	const box = page.getByTestId('confirm');
	await expect(box).toBeVisible();
	await box.getByRole('button', { name: label }).click();
	await expect(box).toHaveCount(0);
}

test('turn a part off and on, a refused switch, and the Events', async () => {
	await page.goto('/settings');
	// Settings home: open the Library card, then its Parts capsule.
	await page.getByTestId('settings-group').filter({ hasText: 'Library' }).click();
	await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'Parts', exact: true }).click();
	await expect(page.getByRole('heading', { name: 'Parts', level: 1 })).toBeVisible();
	await expect(page.getByTestId('part')).toHaveCount(6);
	await expect(part('Sources')).toContainText('When off: Automatic searches stop');
	await expect(part('Music')).toContainText('When off: Artist and album pages');

	// Turning off asks first, in plain words.
	await part('Music').getByRole('button', { name: 'Turn Music off' }).click();
	await expect(page.getByTestId('confirm')).toContainText('its data is kept');
	await confirm(/^Yes, turn music off/);
	await expect(part('Music')).toHaveAttribute('data-state', 'off');
	await expect(page.getByRole('navigation', { name: 'Main' }).first()).toBeVisible();

	// With Music off, turning Movies and then Shows off would leave
	// Subtitles nothing: the server refuses and says why.
	await part('Movies').getByRole('button', { name: 'Turn Movies off' }).click();
	await confirm(/^Yes, turn movies off/);
	await expect(part('Movies')).toHaveAttribute('data-state', 'off');
	await part('TV shows').getByRole('button', { name: 'Turn TV shows off' }).click();
	await confirm(/^Yes, turn tv shows off/);
	await expect(page.getByRole('status').filter({ hasText: 'Subtitles need Movies or TV shows' })).toBeVisible();
	await expect(part('TV shows')).not.toHaveAttribute('data-state', 'off');

	// Back on.
	await part('Movies').getByRole('button', { name: 'Turn Movies on' }).click();
	await expect(part('Movies')).not.toHaveAttribute('data-state', 'off');
	await part('Music').getByRole('button', { name: 'Turn Music on' }).click();
	await expect(part('Music')).not.toHaveAttribute('data-state', 'off');

	// Restarting one part asks first too, and leaves the others alone.
	await part('Subtitles').getByRole('button', { name: 'Restart Subtitles' }).click();
	await confirm(/^Yes, restart subtitles/);
	await expect(page.getByRole('status').filter({ hasText: 'Subtitles restarted' })).toBeVisible();

	await page.reload();
	const used = page.getByTestId('control-events');
	await expect(used).toContainText('turned Music off');
	await expect(used).toContainText('turned Music on');
	await expect(used).toContainText('restarted Subtitles');
});

test('pause and resume a stage, reconnect a linked app', async () => {
	const stage = page.getByTestId('control-stage').filter({ has: page.getByRole('button', { name: /^Pause / }) }).first();
	await expect(stage).toBeVisible();
	const name = (await stage.locator('.name').innerText()).split('\n')[0].trim();
	await stage.getByRole('button', { name: /^Pause / }).click();
	await expect(page.getByTestId('control-stage').filter({ hasText: name }).getByRole('button', { name: /^Resume / })).toBeVisible();
	await page.getByTestId('control-stage').filter({ hasText: name }).getByRole('button', { name: /^Resume / }).click();
	await expect(page.getByTestId('control-stage').filter({ hasText: name }).getByRole('button', { name: /^Pause / })).toBeVisible();

	const link = page.getByTestId('control-link').filter({ has: page.getByRole('button', { name: /^(Reconnect|Test now) / }) }).first();
	if ((await link.count()) > 0) {
		await link.getByRole('button', { name: /^(Reconnect|Test now) / }).click();
		await expect(page.locator('.toasts')).not.toBeEmpty();
	}
	await page.reload();
	await expect(page.getByTestId('control-events')).toContainText(`paused the stage`);
	await expect(page.getByTestId('control-events')).toContainText(`resumed the stage`);
});

test('the whole-app restart asks first and can be cancelled', async () => {
	// It lives on System > Danger zone; Parts links there.
	await page.getByTestId('settings-links').getByRole('link', { name: /Danger zone/ }).click();
	await expect(page.getByRole('heading', { name: 'Danger zone', level: 1 })).toBeVisible();
	const app = page.getByTestId('control-app');
	await expect(app).toContainText("Docker's restart policy");
	await app.getByRole('button', { name: 'Restart HolyArrs' }).click();
	const box = page.getByTestId('confirm');
	await expect(box).toContainText('restart HolyArrs');
	await expect(box.getByRole('button', { name: /^Yes, restart holyarrs/ })).toBeFocused();
	await box.getByRole('button', { name: 'Cancel' }).click();
	await expect(box).toHaveCount(0);
	// Still running.
	await page.reload();
	await expect(page.getByRole('heading', { name: 'Danger zone', level: 1 })).toBeVisible();
});

test('the parts step turns parts off, and everything comes back', async () => {
	await page.goto('/welcome');
	await expect(page.getByRole('heading', { name: 'What do you use?', level: 1 })).toBeVisible();
	await expect(page.getByLabel('Movies', { exact: true })).toBeChecked();
	await expect(page.getByLabel('TV shows', { exact: true })).toBeChecked();
	await expect(page.getByLabel('Music', { exact: true })).not.toBeChecked();
	await page.getByLabel('TV shows', { exact: true }).uncheck();
	await page.getByLabel('Subtitles', { exact: true }).uncheck();
	await page.getByRole('button', { name: 'Continue' }).click();
	await expect(page).toHaveURL(/\/$/);
	await page.goto('/settings/parts');
	for (const off of ['TV shows', 'Music', 'Subtitles', 'Requests']) await expect(part(off)).toHaveAttribute('data-state', 'off');
	// The step does not ask about Sources, so it stays as it was.
	for (const on of ['Movies', 'Sources']) await expect(part(on)).not.toHaveAttribute('data-state', 'off');

	// The same step turns everything back on.
	await page.goto('/welcome');
	await page.getByLabel('Music', { exact: true }).check();
	await page.getByLabel('Yes', { exact: true }).check();
	await page.getByRole('button', { name: 'Continue' }).click();
	await expect(page).toHaveURL(/\/$/);
	await page.goto('/settings/parts');
	for (const on of ['Movies', 'TV shows', 'Music', 'Subtitles', 'Requests']) await expect(part(on)).not.toHaveAttribute('data-state', 'off');
});

test('both screens at phone width, and accessibility', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 412, height: 915 }, isMobile: true, hasTouch: true });
	const phone = await ctx.newPage();
	await signIn(phone, owner());
	for (const [path, heading] of [
		['/settings/parts', 'Parts'],
		['/welcome', 'What do you use?']
	]) {
		await phone.goto(path);
		await expect(phone.getByRole('heading', { name: heading, level: 1 })).toBeVisible();
		await noSidewaysScroll(phone);
		const a11y = await new AxeBuilder({ page: phone }).include('main').analyze();
		expect(a11y.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target).join(' ')}`)).toEqual([]);
	}
	await phone.goto('/settings/parts');
	await phone.getByTestId('part').first().getByRole('button', { name: /^Turn .* off$/ }).click();
	await expect(phone.getByTestId('confirm')).toBeVisible();
	await noSidewaysScroll(phone);
	await phone.getByTestId('confirm').getByRole('button', { name: 'Cancel' }).click();
	const desktop = await new AxeBuilder({ page }).include('main').analyze();
	expect(desktop.violations.map((v) => v.id)).toEqual([]);
	await ctx.close();
});

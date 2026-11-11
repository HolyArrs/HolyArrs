import { expect, test } from '@playwright/test';
import { moviesDir, newOwner, noSidewaysScroll, owner, signIn, addFromSearch, setUpFresh } from '../helpers/helpers.ts';

test.describe.configure({ mode: 'serial' });

async function enterApp(page: import('@playwright/test').Page) {
	await page.goto('/login');
	const setup = page.getByRole('heading', { name: 'Set up HolyArrs' });
	const login = page.getByRole('heading', { name: 'Sign in' });
	await expect(setup.or(login)).toBeVisible();
	if (await login.isVisible()) {
		await signIn(page, owner());
		return;
	}
	// Run alone on a fresh app: the same first run as the setup project.
	await setUpFresh(page, newOwner(), moviesDir, { tv: true, music: true, invite: true });
}

async function paperHarbour(page: import('@playwright/test').Page): Promise<string> {
	await page.goto('/library');
	// Wait for the Library to load (its title count shows) before asking whether the movie is there.
	await expect(page.getByText(/^\d+ (movies?|TV shows?|artists?|titles?)$/)).toBeVisible();
	const movie = page.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: 'Paper Harbour' }).first();
	if ((await movie.count()) === 0) {
		const dialog = await addFromSearch(page, 'paper harbour', 'Paper Harbour');
		await dialog.getByLabel('Search when added').uncheck();
		await dialog.getByRole('button', { name: 'Add to Library' }).click();
		await expect(page).toHaveURL(/\/library\/\d+/);
		return new URL(page.url()).pathname;
	}
	const id = await movie.getAttribute('data-title-id');
	expect(id).toBeTruthy();
	return `/library/${id}`;
}

test("a title's Library tab lists its Versions on the File cards, with no Copies section", async ({ page }) => {
	await enterApp(page);
	const href = await paperHarbour(page);
	await page.goto(`${href!}?tab=library`);
	await expect(page.getByTestId('lib-pane')).toBeVisible();
	// Protect a copy, keep a copy and add a Version, and the Versions list are out of the title page
	// (docs/library/audio-versions.md); every Version shows as a File card, a Version without a file says so.
	await expect(page.getByTestId('version-card').first()).toBeVisible();
	await expect(page.getByTestId('copy-controls')).toHaveCount(0);
	await expect(page.getByTestId('movie-versions')).toHaveCount(0);
	await page.setViewportSize({ width: 412, height: 915 });
	await noSidewaysScroll(page);
});

test('edition decision requires the check and retries with the confirmed edition', async ({ page }) => {
	await enterApp(page);
	let decided = false;
	let body: Record<string, unknown> | undefined;
	await page.route('**/api/v1/needs-decision', (route) => route.fulfill({ json: decided ? { items: [], count: 0 } : {
		items: [{ slot_id: 900, title_id: 1, title: 'Paper Harbour', year: 2023, version: 'HD', versions: 1, why: 'conflict', reason: 'The edition needs checking.', has_copy: false, since: '2026-10-15T12:00:00Z', kind: 'movie', download_id: 800 }],
		count: 1
	} }));
	await page.route('**/api/v1/downloads/800/edition', async (route) => {
		body = route.request().postDataJSON() as Record<string, unknown>;
		decided = true;
		await route.fulfill({ json: { download_id: 800, edition: body.edition, confirmed: true, job_id: 901 } });
	});

	// Home's link for a decision opens that row's panel on Downloads.
	await page.goto('/downloads?show=problems.decide&slot=900');
	// Set edition opens the form; it saves only once an edition is typed in.
	await page.getByTestId('dl-panel').getByRole('button', { name: 'Set edition' }).click();
	const form = page.getByTestId('dl-panel').getByTestId('edition-decision');
	const save = form.getByRole('button', { name: 'Save and try again' });
	await expect(save).toBeDisabled();
	await form.getByLabel('Edition', { exact: true }).fill('Director cut');
	await save.click();
	await expect.poll(() => body).toEqual({ edition: 'Director cut', confirm: true, retry: true });
	await expect(page.getByTestId('decision-panel')).toHaveCount(0);
});

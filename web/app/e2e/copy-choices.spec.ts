import { expect, test } from '@playwright/test';
import { moviesDir, newOwner, noSidewaysScroll, owner, setupCode, signIn, addFromSearch } from './helpers.ts';

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
	const o = newOwner();
	await page.getByLabel('Setup code').fill(setupCode());
	await page.getByLabel('Name').fill(o.name);
	await page.getByLabel('Password', { exact: true }).fill(o.password);
	await page.getByLabel('Password again').fill(o.password);
	await page.getByRole('button', { name: 'Make the owner and sign in' }).click();
	await page.getByLabel('Music', { exact: true }).check();
	await page.getByLabel('Subtitles', { exact: true }).check();
	await page.getByLabel('Yes', { exact: true }).check();
	await page.getByRole('button', { name: 'Continue' }).click();
	await page.getByLabel('Folder path').fill(moviesDir);
	await page.getByLabel(/^Name/).fill('Movies');
	await page.getByRole('button', { name: 'Add folder and open Home' }).click();
}

async function paperHarbour(page: import('@playwright/test').Page): Promise<string> {
	await page.goto('/library');
	// Wait for the Library to load (its title count shows) before asking whether the movie is there.
	await expect(page.getByText(/^\d+ titles?$/)).toBeVisible();
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

test('copy controls show inspected facts and require explicit choices', async ({ page }) => {
	await enterApp(page);
	const href = await paperHarbour(page);

	let files = [
		{
			ID: 701,
			VersionID: 501,
			RelativePath: 'Paper Harbour (2023)/Paper Harbour (2023).mkv',
			Size: 12_345,
			AddedAt: '2026-10-15T12:00:00Z',
			State: 'present',
			Protected: false,
			MediaInfo: { audio_languages: ['fr', 'en'], edition: 'Theatrical', video_codec: 'hevc' }
		}
	];
	let versions = [
		{ id: 501, label: 'HD', main: true, pick_rules_id: 1, pick_rules_name: 'HD', library_folder_id: 1, folder_path: 'C:/Movies/Paper Harbour (2023)', keep_up_to_date: true, edition: 'Theatrical', audio_languages: ['en'] },
		{ id: 502, label: '4K', main: false, pick_rules_id: 1, pick_rules_name: '4K', library_folder_id: 1, folder_path: 'C:/Movies/Paper Harbour (2023) [4K]', keep_up_to_date: true, edition: 'Director cut', audio_languages: ['en', 'fr'] }
	];
	const calls: Array<{ path: string; body: Record<string, unknown> }> = [];

	await page.route('**/api/v1/titles/*/copies', (route) => route.fulfill({ json: { files, versions } }));
	await page.route('**/api/v1/titles/*/files/*/choices', async (route) => {
		const body = route.request().postDataJSON() as Record<string, unknown>;
		calls.push({ path: new URL(route.request().url()).pathname, body });
		files = files.map((file) => file.ID === 701
			? {
				...file,
				...(typeof body.protected === 'boolean' ? { Protected: body.protected } : {}),
				...(typeof body.edition === 'string' ? { MediaInfo: { ...file.MediaInfo, edition: body.edition } } : {})
			}
			: file);
		await route.fulfill({ json: files[0] });
	});
	await page.route('**/api/v1/titles/*/files/*/keep-and-add', async (route) => {
		const body = route.request().postDataJSON() as Record<string, unknown>;
		calls.push({ path: new URL(route.request().url()).pathname, body });
		files = files.map((file) => ({ ...file, Protected: true }));
		versions = [...versions, { id: 503, label: String(body.label), main: false, pick_rules_id: Number(body.pick_rules_id), pick_rules_name: 'HD', library_folder_id: Number(body.library_folder_id), folder_path: 'C:/Movies/Paper Harbour Archive', keep_up_to_date: true, edition: String(body.edition), audio_languages: body.audio_languages as string[] }];
		await route.fulfill({ status: 201, json: versions.at(-1) });
	});
	await page.route('**/api/v1/titles/*/versions/*/adopt', async (route) => {
		const body = route.request().postDataJSON() as Record<string, unknown>;
		calls.push({ path: new URL(route.request().url()).pathname, body });
		files = [...files, { ID: 702, VersionID: 502, RelativePath: String(body.relative_path), Size: 99, AddedAt: '2026-10-15T12:00:00Z', State: 'present', Protected: false, MediaInfo: { audio_languages: ['en', 'fr'], edition: String(body.confirmed_edition), video_codec: 'hevc' } }];
		await route.fulfill({ status: 201, json: files.at(-1) });
	});

	await page.goto(href!);
	const controls = page.getByTestId('copy-controls');
	await expect(controls).toContainText('Required audio: en');
	await expect(controls).toContainText('Inspected audio: fr, en');
	await expect(controls).not.toContainText('subtitle');

	await controls.getByRole('button', { name: 'Protect this copy' }).click();
	await expect(controls.getByText('Protected', { exact: true })).toBeVisible();
	await expect(controls.getByRole('button', { name: 'Remove protection' })).toBeDisabled();
	await controls.getByLabel("I understand this removes the copy's protection").check();
	await controls.getByRole('button', { name: 'Remove protection' }).click();
	expect(calls.at(-1)?.body).toEqual({ protected: false, acknowledge_protection: true });

	const checkedEdition = controls.getByLabel('Checked edition');
	await checkedEdition.fill('Director cut');
	await expect(checkedEdition).toHaveValue('Director cut');
	await expect(controls.getByRole('button', { name: 'Save edition' })).toBeDisabled();
	await controls.getByLabel("I checked this file's edition").check();
	await controls.getByRole('button', { name: 'Save edition' }).click();
	expect(calls.at(-1)?.body).toEqual({ edition: 'Director cut' });

	await controls.getByRole('button', { name: 'Keep this copy and add another Version' }).click();
	const add = page.getByRole('dialog', { name: 'Keep this copy and add another Version' });
	await add.getByLabel('Version name').fill('Archive');
	await add.getByLabel(/Edition wanted/).fill('Theatrical');
	await add.getByLabel(/Audio languages wanted/).fill('EN, fr, en');
	await add.getByRole('button', { name: 'Protect and add Version' }).click();
	expect(calls.at(-1)?.body.audio_languages).toEqual(['en', 'fr']);
	await expect(controls).toContainText('Archive');

	await controls.getByRole('button', { name: 'Use an existing video in this Version folder' }).first().click();
	const adopt = page.getByRole('dialog', { name: /Use an existing video/ });
	await adopt.getByLabel('Path relative to the Library folder').fill('Paper Harbour (2023) [4K]/Paper Harbour.mkv');
	await adopt.getByLabel(/Checked edition/).fill('Director cut');
	await adopt.getByLabel(/I checked the file/).check();
	await adopt.getByRole('button', { name: 'Check and use file' }).click();
	expect(calls.at(-1)?.body).toEqual({ relative_path: 'Paper Harbour (2023) [4K]/Paper Harbour.mkv', confirmed_edition: 'Director cut' });
	await expect(controls).toContainText('Paper Harbour.mkv');

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

	await page.goto('/downloads/decide');
	const form = page.getByTestId('edition-decision');
	await form.getByLabel('Edition', { exact: true }).fill('Director cut');
	await expect(form.getByRole('button', { name: 'Save edition and try import again' })).toBeDisabled();
	await form.getByLabel(/I checked this download/).check();
	await form.getByRole('button', { name: 'Save edition and try import again' }).click();
	expect(body).toEqual({ edition: 'Director cut', confirm: true, retry: true });
	await expect(page.getByText('Nothing needs you')).toBeVisible();
});

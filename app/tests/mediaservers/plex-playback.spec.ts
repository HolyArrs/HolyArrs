import { expect, test } from '@playwright/test';
import { plexPort, plexServerToken } from '../fakes/index.mjs';
import { noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';

test('Plex playback is opt-in and a resumed play appears once in history', async ({ page }) => {
	test.setTimeout(75_000);
	await signIn(page, owner());
	const me = await page.request.get('/api/v1/auth/me');
	const { csrf } = await me.json();
	const movie = await page.request.post('/api/v1/titles', {
		headers: { 'X-CSRF-Token': csrf },
		data: { id: 'tmdb:900415', search_on_add: false, keep_up_to_date: false, follow: false }
	});
	expect(movie.status()).toBe(201);
	const { title: { id: titleId } } = await movie.json();
	const plex = `http://127.0.0.1:${plexPort}`;
	const added = await page.request.post('/api/v1/media-servers', {
		headers: { 'X-CSRF-Token': csrf },
		data: { name: 'Playback Lab Plex', kind: 'plex', url: plex, token: plexServerToken, allow_local_network: true }
	});
	expect(added.status()).toBe(201);
	const { id } = await added.json();
	const setState = async (state: 'playing' | 'paused' | 'buffering' | 'stopped') => {
		const data = state === 'stopped' ? '<MediaContainer size="0"/>' : `<MediaContainer size="1">
			<Video type="movie" ratingKey="100" title="Moonlit Station" viewOffset="5000" duration="60000" thumb="/library/metadata/100/thumb/1">
				<User id="12" title="Demo Viewer"/><Player title="Demo TV" state="${state}"/><Session id="test-session" bandwidth="6200" location="lan"/>
				<Guid id="tmdb://900415"/><Media videoCodec="hevc"><Part decision="directplay"/></Media>
			</Video></MediaContainer>`;
		expect((await page.request.put(`${plex}/lab/sessions`, { data })).status()).toBe(204);
	};

	try {
		await setState('playing');
		await page.goto('/settings/media-servers');
		const nav = page.locator('#sidebar').getByRole('link', { name: 'Playback', exact: true });
		const toggle = page.getByTestId('playback-monitoring').getByRole('checkbox', { name: 'Playback Lab Plex' });
		await expect(toggle).not.toBeChecked();
		await expect(nav).toHaveCount(0);
		await toggle.check();
		await nav.click();
		await expect(page.getByRole('heading', { name: 'Playback', exact: true })).toBeVisible();

		const panel = page.getByRole('region', { name: 'Playback', exact: true });
		await expect(panel.getByText('Moonlit Station')).toBeVisible({ timeout: 15_000 });
		await expect(panel.getByText('Demo Viewer · Demo TV')).toBeVisible();
		await expect(panel.getByText('Direct play')).toBeVisible();
		await expect.poll(() => panel.locator('.streams img').evaluate((img) => img instanceof HTMLImageElement && img.complete && img.naturalWidth > 0)).toBe(true);
		const poster = panel.getByRole('link', { name: 'Open in Library · Moonlit Station', exact: true });
		await poster.click();
		await expect(page).toHaveURL(new RegExp(`/library/${titleId}$`));
		await page.goBack();
		const position = panel.getByTestId('playback-position');
		await expect(position).toHaveText(/^0:0[6-9]$/, { timeout: 5000 });
		await setState('buffering');
		await expect(panel.locator('.stream-heading').getByText('Buffering', { exact: true })).toBeVisible({ timeout: 15_000 });
		await expect(position).toHaveText('0:05');
		await panel.getByText('Details', { exact: true }).click();
		await expect(panel.getByText('LAN · ≈ 6.2 Mb/s')).toBeVisible();
		await expect(panel.getByTestId('buffering-observed')).toContainText('1×');
		await panel.getByText('Details', { exact: true }).click();
		await setState('paused');
		await expect(panel.getByText('Paused', { exact: true })).toBeVisible({ timeout: 15_000 });
		// Wait long enough to catch the normal playing clock's next tick.
		await page.waitForTimeout(1250);
		await expect(position).toHaveText('0:05');
		await setState('playing');
		await expect(panel.getByText('Playing', { exact: true })).toBeVisible({ timeout: 15_000 });
		await setState('stopped');
		await expect(panel.getByTestId('playback-metrics').getByText('1 play', { exact: true })).toBeVisible({ timeout: 15_000 });
		await expect(panel.locator('.history').getByText('Moonlit Station')).toHaveCount(1);

		await page.setViewportSize({ width: 390, height: 844 });
		await noSidewaysScroll(page);
		await panel.getByRole('searchbox', { name: 'Title' }).fill('Unplayed title');
		await panel.getByRole('button', { name: 'Filter', exact: true }).click();
		await expect(panel.getByText('Nothing matches.')).toBeVisible();
		await page.setViewportSize({ width: 1280, height: 800 });
		await page.getByRole('link', { name: 'Media servers', exact: true }).click();
		await toggle.uncheck();
		await expect(nav).toHaveCount(0);
		expect((await (await page.request.get(`/api/v1/media-servers/${id}/playback`)).json()).plays).toBe(1);
	} finally {
		await setState('stopped');
		await page.request.delete(`/api/v1/media-servers/${id}`, { headers: { 'X-CSRF-Token': csrf } });
		await page.request.post(`/api/v1/titles/${titleId}/delete`, {
			headers: { 'X-CSRF-Token': csrf },
			data: { delete_files: false, never_add: false, apply: true }
		});
	}
});

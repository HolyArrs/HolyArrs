import { expect, test } from '@playwright/test';
import { plexPort, plexServerToken } from '../fakes/index.mjs';
import { noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';

test('Plex playback is opt-in and a resumed play appears once in history', async ({ page }) => {
	test.setTimeout(60_000);
	await signIn(page, owner());
	const me = await page.request.get('/api/v1/auth/me');
	const { csrf } = await me.json();
	const plex = `http://127.0.0.1:${plexPort}`;
	const added = await page.request.post('/api/v1/media-servers', {
		headers: { 'X-CSRF-Token': csrf },
		data: { name: 'Playback Lab Plex', kind: 'plex', url: plex, token: plexServerToken, allow_local_network: true }
	});
	expect(added.status()).toBe(201);
	const { id } = await added.json();
	const setState = async (state: 'playing' | 'paused' | 'stopped') => {
		const data = state === 'stopped' ? '<MediaContainer size="0"/>' : `<MediaContainer size="1">
			<Video type="movie" ratingKey="100" title="Moonlit Station" viewOffset="5000" duration="60000">
				<User id="12" title="Demo Viewer"/><Player title="Demo TV" state="${state}"/><Session id="test-session"/>
				<Media><Part decision="directplay"/></Media>
			</Video></MediaContainer>`;
		expect((await page.request.put(`${plex}/lab/sessions`, { data })).status()).toBe(204);
	};

	try {
		await setState('playing');
		await page.goto('/settings/media-servers');
		const card = page.getByTestId('media-server').filter({ hasText: 'Playback Lab Plex' });
		await card.getByRole('button', { name: 'Playback', exact: true }).click();
		await expect(card.getByText('Monitor playback · Off', { exact: true })).toBeVisible();
		await card.getByRole('button', { name: 'Change', exact: true }).click();
		const toggle = page.getByRole('checkbox', { name: /Monitor playback/ });
		await expect(toggle).not.toBeChecked();
		await toggle.check();
		await page.getByRole('button', { name: 'Save', exact: true }).first().click();

		const panel = card.getByRole('region', { name: 'Playback', exact: true });
		await expect(panel.getByText('Moonlit Station')).toBeVisible({ timeout: 15_000 });
		await expect(panel.getByText('Demo Viewer · Demo TV · Direct play')).toBeVisible();
		await setState('paused');
		await expect(panel.getByText('Paused', { exact: true })).toBeVisible({ timeout: 15_000 });
		await setState('playing');
		await expect(panel.getByText('Playing', { exact: true })).toBeVisible({ timeout: 15_000 });
		await setState('stopped');
		await expect(panel.getByText('1 play · Observed watch time', { exact: false })).toBeVisible({ timeout: 15_000 });
		await expect(panel.getByText('Moonlit Station')).toHaveCount(1);

		await page.setViewportSize({ width: 390, height: 844 });
		await noSidewaysScroll(page);
		await panel.getByRole('searchbox', { name: 'Title' }).fill('Unplayed title');
		await panel.getByRole('button', { name: 'Filter', exact: true }).click();
		await expect(panel.getByText('Nothing matches.')).toBeVisible();
	} finally {
		await setState('stopped');
		await page.request.delete(`/api/v1/media-servers/${id}`, { headers: { 'X-CSRF-Token': csrf } });
	}
});

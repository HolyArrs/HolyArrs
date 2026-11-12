import { expect, test } from '@playwright/test';
import { owner, signIn } from '../helpers/helpers.ts';
import { api } from '../helpers/api.ts';

// Episode orders from TMDB on the show page. The fake TMDB lists no orders,
// so the order list and the save answer are stood in for here; the show is
// any show the earlier projects added.

const group = { id: 'a1b2c3d4e5f6a1b2c3d4e5f6', name: 'Disney+ order', type: 4, seasons: 19, episodes: 410 };

test('Episode order: a TMDB order is listed, picking it saves its id and offers the rename preview', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: { id: number; kind: string }[] };
	const show = titles.find((x) => x.kind === 'show');
	expect(show, 'an earlier project adds a show').toBeTruthy();
	const id = show!.id;

	await page.route(`**/api/v1/titles/${id}/episode-groups`, (r) => r.fulfill({ json: { groups: [group], chosen: null } }));
	let sent: Record<string, unknown> | null = null;
	await page.route(`**/api/v1/titles/${id}`, async (r) => {
		if (r.request().method() !== 'PUT') return r.fallback();
		sent = r.request().postDataJSON() as Record<string, unknown>;
		const real = (await (await call('GET', `/titles/${id}`)).json()) as { show: Record<string, unknown> };
		await r.fulfill({ json: { title: { ...real, show: { ...real.show, episode_group: { id: group.id, name: group.name, type: group.type } } }, changes: {}, rename_suggested: true } });
	});

	await page.goto(`/library/${id}?tab=library`);
	await page.getByRole('button', { name: 'Episode order' }).click();
	await expect(page.getByRole('menu')).toContainText('Orders from TMDB for this show');
	await expect(page.getByRole('menu')).toContainText('19 seasons · 410 episodes');
	await page.getByRole('menuitemradio', { name: /Disney\+ order/ }).or(page.getByRole('menuitem', { name: /Disney\+ order/ })).click();

	await expect.poll(() => sent).toEqual({ episode_group_id: group.id });
	await expect(page.getByTestId('order-chip')).toHaveText(group.name);
	await expect(page.getByRole('button', { name: 'Preview rename' })).toBeVisible();
});

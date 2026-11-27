import { expect, test } from '@playwright/test';
import { openLibraryTab, owner, signIn } from '../helpers/helpers.ts';
import { api, type Call } from '../helpers/api.ts';

// Title tags: the Tags row on a movie's Library tab puts tags on the title,
// so tagged Sources and Downloaders serve it, and takes them off again.

const tmdb = 900035;
const name = 'Marigold Bellows';

/** freshMovie puts the fake-TMDB movie in the Library with no file and no tags, and returns its id. */
async function freshMovie(call: Call): Promise<number> {
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: { id: number; name: string; kind: string }[] };
	for (const t of titles.filter((x) => x.kind === 'movie' && x.name === name)) {
		const gone = await call('POST', `/titles/${t.id}/delete`, { delete_files: true, never_add: false, apply: true });
		expect(gone.ok(), await gone.text()).toBe(true);
	}
	const added = await call('POST', '/titles', { id: `tmdb:${tmdb}`, search_on_add: false, keep_up_to_date: false, follow: false });
	expect(added.status(), await added.text()).toBe(201);
	return ((await added.json()) as { title: { id: number } }).title.id;
}

async function tagsOf(call: Call, id: number): Promise<string[]> {
	return ((await (await call('GET', `/titles/${id}`)).json()) as { tags?: string[] }).tags ?? [];
}

test('Tags typed on a movie’s Library tab are saved on the title, and a tag taken off is gone', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const id = await freshMovie(call);
	expect(await tagsOf(call, id)).toEqual([]);

	await page.goto(`/library/${id}`);
	await openLibraryTab(page);
	const row = page.getByTestId('tags-row');
	await expect(row).toBeVisible();

	// Enter and a comma both commit a tag; typed tags are kept as the server cleans them (lower case, sorted).
	const input = row.getByTestId('title-tags-input');
	await input.fill('UA Dub');
	await input.press('Enter');
	await input.fill('toloka,');
	await expect.poll(() => tagsOf(call, id)).toEqual(['toloka', 'ua-dub']);

	// The row shows what was saved after a reload.
	await page.reload();
	await openLibraryTab(page);
	await expect(page.getByTestId('tags-row')).toContainText('ua-dub');
	await expect(page.getByTestId('tags-row')).toContainText('toloka');

	// Taking a tag off saves at once.
	await page.getByTestId('tags-row').getByRole('button', { name: 'Remove toloka' }).click();
	await expect.poll(() => tagsOf(call, id)).toEqual(['ua-dub']);
});

test('Tags sent when a movie is added are put on it', async ({ page }) => {
	await signIn(page, owner());
	const call = await api(page);
	const id = await freshMovie(call);
	const gone = await call('POST', `/titles/${id}/delete`, { delete_files: true, never_add: false, apply: true });
	expect(gone.ok(), await gone.text()).toBe(true);
	const added = await call('POST', '/titles', { id: `tmdb:${tmdb}`, search_on_add: false, keep_up_to_date: false, follow: false, tags: ['Toloka', 'UA Dub'] });
	expect(added.status(), await added.text()).toBe(201);
	const again = ((await added.json()) as { title: { id: number } }).title.id;
	expect(await tagsOf(call, again)).toEqual(['toloka', 'ua-dub']);
});

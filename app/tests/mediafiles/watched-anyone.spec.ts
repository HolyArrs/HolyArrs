import { expect, test } from '@playwright/test';
import { owner, signIn } from '../helpers/helpers.ts';

// The owner's "Watched by anyone" filter keeps only the titles some account watched; the list itself comes from the media servers' watch history.
test('the owner filters the Library to titles anybody watched', async ({ page }) => {
	await signIn(page, owner());
	const { titles } = (await (await page.request.get('/api/v1/titles')).json()) as { titles: { id: number; name: string }[] };
	const seen = titles.slice(0, 1);
	await page.route('**/api/v1/library/watched-by-anyone', (r) => r.fulfill({ json: { enabled: true, titles: seen.map((t) => t.id) } }));
	await page.goto('/library?watched=anyone&layout=table');
	const chip = page.getByRole('button', { name: 'Watched by anyone', exact: true });
	await expect(chip).toBeVisible();
	await expect(page.locator('[data-testid="library-card"], [data-testid="library-row"]')).toHaveCount(seen.length);
	await chip.click();
	await expect(page).not.toHaveURL(/watched=anyone/);
});

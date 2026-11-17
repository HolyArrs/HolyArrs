import { expect, test } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';

// A router or VPN in fake-IP mode (Clash, Surge) sends the metadata server's
// name to 198.18.x.x, which HolyArrs refuses. The pages name that cause and
// the host to add, instead of only saying the server is down.
const words = 'Your router or VPN is sending HolyArrs to a fake address (198.18.x.x). This usually comes from Clash or Surge. Add api.holyarrs.com to its direct list.';

test('a fake 198.18.x.x address is named on Settings > Metadata and in the toast', async ({ page }) => {
	await page.route('**/api/v1/metadata/health', (r) => r.fulfill({ json: { up: false, fakeIP: 'api.holyarrs.com', message: words } }));
	await signIn(page, owner());
	for (const width of [1280, 390]) {
		await page.setViewportSize({ width, height: width > 500 ? 900 : 844 });
		await page.goto('/settings/tmdb');
		await expect(page.getByRole('status').filter({ hasText: words })).toBeVisible();
		await noSidewaysScroll(page);
	}
	await expect(page.getByText(words).first()).toBeVisible();
});

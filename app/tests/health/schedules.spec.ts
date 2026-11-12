import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
import { moviesDir, newOwner, noSidewaysScroll, owner, signIn, setUpFresh } from '../helpers/helpers.ts';

test.describe.configure({ mode: 'serial' });

async function enterFreshApp(page: import('@playwright/test').Page) {
	await page.goto('/login');
	const setupHeading = page.getByRole('heading', { name: 'Set up HolyArrs' });
	const signInHeading = page.getByRole('heading', { name: 'Sign in' });
	await expect(setupHeading.or(signInHeading)).toBeVisible();
	if (await signInHeading.isVisible()) {
		await signIn(page, owner());
		return;
	}
	// Run alone on a fresh app: the same first run as the setup project.
	await setUpFresh(page, newOwner(), moviesDir, { tv: true, music: true, invite: true });
}

test('movie detail refresh can be turned off, survives reload, and can be run by hand', async ({ page }) => {
	await enterFreshApp(page);
	await page.goto('/settings/schedules');
	const movie = page.getByTestId('schedule').filter({ has: page.getByText('Refresh movie details', { exact: true }) });
	await expect(movie).toBeVisible();
	// Turning it off lives in the interval editor, behind Change.
	await movie.getByRole('button', { name: 'Change how often Refresh movie details runs' }).click();
	await movie.getByRole('button', { name: 'Turn off automatic refresh' }).click();
	await expect(movie).toHaveAttribute('data-state', 'off');
	await expect(movie.getByTestId('movie-refresh-off-note')).toContainText('can become stale');
	await expect(movie.getByRole('button', { name: 'Run Refresh movie details now' })).toBeVisible();

	await page.reload();
	await expect(movie).toHaveAttribute('data-state', 'off');
	await movie.getByRole('button', { name: 'Change how often Refresh movie details runs' }).click();
	await movie.getByRole('button', { name: 'Turn on automatic refresh' }).click();
	await expect(movie).toHaveAttribute('data-state', 'waiting');
	await expect(movie.getByTestId('next')).not.toContainText('Turned off');
});

test('movie refresh control works at phone width and is accessible', async ({ page }) => {
	await page.setViewportSize({ width: 412, height: 915 });
	await signIn(page, owner());
	await page.goto('/settings/schedules');
	await page.getByRole('button', { name: 'Change how often Refresh movie details runs' }).click();
	await expect(page.getByRole('button', { name: 'Turn off automatic refresh' })).toBeVisible();
	await noSidewaysScroll(page);
	const a11y = await new AxeBuilder({ page }).include('main').analyze();
	expect(a11y.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target).join(' ')}`)).toEqual([]);
});

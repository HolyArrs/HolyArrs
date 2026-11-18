import { expect, test, type Locator } from '@playwright/test';
import { owner, signIn } from '../helpers/helpers.ts';

// Invite someone: the Role and Preset bars never cut a name off, even when
// all five presets (Custom included) must share a narrow phone, the bar stays
// short, and nothing moves under the pointer when a switch is toggled.

// The sheet slides in: wait until a part of it has stopped moving before measuring.
async function settled(part: Locator) {
	let last = -1;
	await expect
		.poll(async () => {
			const y = (await part.boundingBox())!.y;
			const same = y === last;
			last = y;
			return same;
		})
		.toBe(true);
}

test('no preset name is cut off on a narrow screen', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 320, height: 700 } })).newPage();
	await signIn(page, owner());
	await page.goto('/settings/users');
	await page.getByRole('button', { name: 'Invite someone' }).click();
	await page.locator('#invite-card input[type=checkbox]').nth(1).click();
	const bar = page.getByTestId('invite-preset');
	await expect(bar.getByRole('radio', { name: 'Custom' })).toHaveAttribute('aria-checked', 'true');
	const cut = await bar.locator('button span').evaluateAll((l) => l.filter((s) => s.scrollWidth > s.clientWidth + 1).length);
	expect(cut).toBe(0);
});

test('the Preset bar keeps its shape and the switches stay put when one is toggled', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 390, height: 844 } })).newPage();
	await signIn(page, owner());
	await page.goto('/settings/users');
	await page.getByRole('button', { name: 'Invite someone' }).click();
	const box = page.locator('#invite-card input[type=checkbox]').nth(1);
	const bar = page.getByTestId('invite-preset');
	await settled(box);
	const before = [(await bar.boundingBox())!.height, (await box.boundingBox())!.y];
	await box.click();
	await expect(bar.getByRole('radio', { name: 'Custom' })).toHaveAttribute('aria-checked', 'true');
	await expect.poll(async () => [(await bar.boundingBox())!.height, (await box.boundingBox())!.y]).toEqual(before);
	await box.click();
	await expect(bar.getByRole('radio', { name: 'Member' })).toHaveAttribute('aria-checked', 'true');
	await expect.poll(async () => [(await bar.boundingBox())!.height, (await box.boundingBox())!.y]).toEqual(before);
});

test('the Preset bar stays short on a phone and the main button is on screen', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 390, height: 844 } })).newPage();
	await signIn(page, owner());
	await page.goto('/settings/users');
	await page.getByRole('button', { name: 'Invite someone' }).click();
	const bar = page.getByTestId('invite-preset');
	await settled(bar);
	expect((await bar.boundingBox())!.height).toBeLessThanOrEqual(100);
	const cut = await bar.locator('button span').evaluateAll((l) => l.filter((s) => s.scrollWidth > s.clientWidth + 1).length);
	expect(cut).toBe(0);
	await expect(page.getByRole('button', { name: 'Make an invite link' })).toBeInViewport({ ratio: 1 });
});

test('the picked Preset segment shows the focus ring when reached by keyboard and still looks picked', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 1280, height: 900 } })).newPage();
	await signIn(page, owner());
	await page.goto('/settings/users');
	await page.getByRole('button', { name: 'Invite someone' }).click();
	const bar = page.getByTestId('invite-preset');
	await bar.getByRole('radio', { name: 'Kid' }).click();
	await page.keyboard.press('ArrowLeft');
	const member = bar.getByRole('radio', { name: 'Member' });
	await expect(member).toBeFocused();
	await expect(member).toHaveAttribute('aria-checked', 'true');
	const look = await member.evaluate((el) => ({ ring: el.matches(':focus-visible'), shadow: getComputedStyle(el).boxShadow, fill: getComputedStyle(el).backgroundColor }));
	expect(look.ring).toBe(true);
	expect(look.shadow).toContain('0px 0px 0px 4px');
	expect(look.fill).not.toBe('rgba(0, 0, 0, 0)');
});

test('a switch that turns the Preset bar to Custom is said once', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 1280, height: 900 } })).newPage();
	await signIn(page, owner());
	await page.goto('/settings/users');
	await page.getByRole('button', { name: 'Invite someone' }).click();
	const bar = page.getByTestId('invite-preset');
	const said = page.getByTestId('invite-preset-said');
	const boxes = page.locator('#invite-card input[type=checkbox]');
	await expect(said).toHaveAttribute('aria-live', 'polite');
	// Count each time the region gets words to say.
	await said.evaluate((el) => {
		(window as unknown as { saidTimes: number }).saidTimes = 0;
		new MutationObserver(() => {
			if (el.textContent?.trim()) (window as unknown as { saidTimes: number }).saidTimes++;
		}).observe(el, { childList: true, characterData: true, subtree: true });
	});
	const times = () => page.evaluate(() => (window as unknown as { saidTimes: number }).saidTimes);
	await boxes.nth(1).click();
	await expect(bar.getByRole('radio', { name: 'Custom' })).toHaveAttribute('aria-checked', 'true');
	await expect(said).toHaveText('Custom');
	// Another switch while already Custom: nothing new to say.
	await boxes.nth(2).click();
	await boxes.nth(2).click();
	expect(await times()).toBe(1);
	// Picking from the bar is heard on the bar itself, not again here.
	await bar.getByRole('radio', { name: 'Manager' }).click();
	await expect(said).toHaveText('');
	await bar.getByRole('radio', { name: 'Custom' }).click();
	await expect(bar.getByRole('radio', { name: 'Custom' })).toHaveAttribute('aria-checked', 'true');
	await expect(said).toHaveText('');
	expect(await times()).toBe(1);
});

test('an admin who is not the owner sees the one role as plain text, not a bar', async ({ browser }) => {
	const own = await (await browser.newContext({ viewport: { width: 1280, height: 900 } })).newPage();
	await signIn(own, owner());
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Invite someone' }).click();
	await own.getByTestId('invite-role').getByRole('radio', { name: 'Admin' }).click();
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	const name = `adm${Math.random().toString(36).slice(2, 8)}`;
	const pw = `e2e-${Math.random().toString(36).slice(2)}-pw`;
	const page = await (await browser.newContext({ viewport: { width: 390, height: 844 } })).newPage();
	await page.goto(new URL(link).pathname);
	await page.getByLabel('Name', { exact: true }).fill(name);
	await page.getByLabel('Password', { exact: true }).fill(pw);
	await page.getByLabel('Confirm password').fill(pw);
	await page.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(page).toHaveURL(/\/$/);
	await page.goto('/settings/users');
	await page.getByRole('button', { name: 'Invite someone' }).click();
	const role = page.getByTestId('invite-role');
	await expect(role).toHaveText('Member');
	await expect(role.getByRole('radio')).toHaveCount(0);
	await expect(page.locator('#invite-card [role=radiogroup]')).toHaveCount(1);
	await expect(page.getByTestId('invite-preset')).toBeVisible();
});

test('arrow keys step over Custom while it cannot be picked', async ({ browser }) => {
	const page = await (await browser.newContext({ viewport: { width: 1280, height: 900 } })).newPage();
	await signIn(page, owner());
	await page.goto('/settings/users');
	await page.getByRole('button', { name: 'Invite someone' }).click();
	const bar = page.getByTestId('invite-preset');
	await bar.getByRole('radio', { name: 'Kid' }).click();
	await page.keyboard.press('ArrowRight');
	await expect(bar.getByRole('radio', { name: 'Admin' })).toHaveAttribute('aria-checked', 'true');
	await page.keyboard.press('End');
	await expect(bar.getByRole('radio', { name: 'Kid' })).toHaveAttribute('aria-checked', 'true');
	await page.keyboard.press('ArrowLeft');
	await expect(bar.getByRole('radio', { name: 'Member' })).toHaveAttribute('aria-checked', 'true');
});

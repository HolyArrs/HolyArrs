import { expect, test, type Browser, type Page } from '@playwright/test';
import { discordPort } from './fakes.mjs';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';

// The Wishlist at phone width (Pixel 7): a member asks for movies, the owner
// approves and turns down from a phone, a Discord Alert (the fake one in
// fakes.mjs) names the member, and the weekly limit says when to try again.
// Runs after the other projects: the owner, the Movies folder and the TMDB
// key exist.
test.describe.configure({ mode: 'serial' });

let own: Page;
let kim: Page;
const kimPw = `e2e-${Math.random().toString(36).slice(2)}-kim`;
const kimDiscord = '123456789012345678';

async function fresh(browser: Browser): Promise<Page> {
	return (await browser.newContext()).newPage();
}

async function posts(): Promise<string[]> {
	const r = await fetch(`http://127.0.0.1:${discordPort}/posts`);
	return ((await r.json()) as { content: string }[]).map((p) => p.content);
}

async function search(p: Page, name: string) {
	await p.goto('/search');
	await p.getByLabel('Movie name or id').fill(name);
	await p.getByRole('button', { name: 'Search', exact: true }).click();
}

test.beforeAll(async ({ browser }) => {
	own = await fresh(browser);
	await signIn(own, owner());
});

test('the owner adds a Discord Alert and sends a test', async () => {
	await own.goto('/settings/alerts');
	await expect(own.getByRole('heading', { level: 1, name: 'Alerts' })).toBeVisible();
	await expect(own.getByText('No Alerts yet')).toBeVisible();
	await own.getByRole('button', { name: 'Add an Alert' }).click();
	await own.getByLabel('Name', { exact: true }).fill('Family Discord');
	await own.getByLabel('Discord link').fill(`http://127.0.0.1:${discordPort}/api/webhooks/1/stand-in`);
	await own.getByLabel('May reach the local network').check();
	await own.getByRole('button', { name: 'Save Alert' }).click();
	const row = own.getByTestId('alert');
	await expect(row).toContainText('Family Discord');
	await expect(row).toContainText('Someone asks · Approved · Turned down · Ready');
	await expect(own.locator('body')).not.toContainText('stand-in');
	await row.getByRole('button', { name: 'Send a test' }).click();
	await expect(row.getByTestId('alert-result')).toContainText('Last sent');
	expect((await posts()).some((c) => c.includes('HolyArrs test'))).toBe(true);
	await noSidewaysScroll(own);
});

test('a member asks for a movie from a phone and sees it on the Wishlist', async ({ browser }) => {
	await own.goto('/settings/users');
	await own.getByLabel('Note for yourself').fill('Kim');
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	kim = await fresh(browser);
	await kim.goto(new URL(link).pathname);
	await kim.getByLabel('Name', { exact: true }).fill('kim');
	await kim.getByLabel('Password', { exact: true }).fill(kimPw);
	await kim.getByLabel('Password again').fill(kimPw);
	await kim.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(kim).toHaveURL(/\/$/);

	await kim.goto('/account');
	await kim.getByLabel('Discord user id').fill(kimDiscord);
	await kim.locator('section', { has: kim.getByRole('heading', { name: 'Alerts for you' }) }).getByRole('button', { name: 'Save' }).click();
	await expect(kim.getByText('Saved where Alerts reach you.')).toBeVisible();

	await search(kim, 'copper');
	const hit = kim.locator('li.hit', { hasText: 'Copper Lighthouse' });
	await expect(kim.getByTestId('ask-limit')).toContainText('You have 10 movie asks left this week');
	await hit.getByRole('button', { name: 'Ask', exact: true }).click();
	// Ask never fires in one click: a confirm sheet opens with focus inside.
	const confirmAsk = kim.getByRole('dialog', { name: /^Ask for Copper Lighthouse/ });
	await expect(confirmAsk).toBeVisible();
	await expect(confirmAsk.locator(':focus')).toHaveCount(1);
	await kim.keyboard.press('Escape');
	await expect(confirmAsk).toBeHidden();
	await expect(hit.getByRole('button', { name: 'Ask', exact: true })).toBeFocused();
	await hit.getByRole('button', { name: 'Ask', exact: true }).click();
	await confirmAsk.getByRole('button', { name: 'Ask for Copper Lighthouse' }).click();
	await expect(hit.getByRole('link', { name: /^You asked · Requested on your Wishlist$/ })).toBeVisible();
	await expect(kim.getByTestId('ask-limit')).toContainText('You have 9 movie asks left this week');
	await noSidewaysScroll(kim);
	// A movie already in the Library still says so; there is nothing to ask.
	await search(kim, 'orchard');
	await expect(kim.locator('li.hit', { hasText: 'Quiet Orchard' }).getByRole('link', { name: 'In your Library' })).toBeVisible();

	await kim.locator('nav.tabbar').getByRole('link', { name: 'Wishlist' }).click();
	const ask = kim.getByTestId('ask').filter({ hasText: 'Copper Lighthouse' });
	await expect(ask.getByTestId('ask-state')).toHaveText('Requested');
	await expect(ask.getByTestId('asked-by')).toContainText('Asked by kim');
	// Take back lives only in the row's ⋯ menu, as a destructive item with a confirm sheet.
	await expect(ask.getByRole('button', { name: 'Take back' })).toHaveCount(0);
	await ask.getByRole('button', { name: 'More for Copper Lighthouse' }).click();
	await expect(ask.getByRole('menuitem', { name: 'Take back' })).toBeVisible();
	await ask.getByRole('menuitem', { name: 'Take back' }).click();
	const confirmTakeBack = kim.getByRole('dialog', { name: /^Take back Copper Lighthouse/ });
	await expect(confirmTakeBack.getByRole('button', { name: 'Take back' })).toBeVisible();
	await confirmTakeBack.getByRole('button', { name: 'Keep it' }).click();
	await expect(confirmTakeBack).toBeHidden();
	await expect(ask.getByTestId('ask-state')).toHaveText('Requested');
	await expect(ask.getByRole('button', { name: 'Approve' })).toHaveCount(0);
	// Take back through to removal: the ask leaves the Wishlist and, with no
	// row left, focus lands on the page heading.
	await ask.getByRole('button', { name: 'More for Copper Lighthouse' }).click();
	await ask.getByRole('menuitem', { name: 'Take back' }).click();
	await confirmTakeBack.getByRole('button', { name: 'Take back' }).click();
	await expect(confirmTakeBack).toBeHidden();
	await expect(confirmTakeBack.getByTestId('take-back-error')).toHaveCount(0);
	await expect(ask).toHaveCount(0);
	await expect(kim.getByText('Took back the ask for Copper Lighthouse.')).toBeVisible();
	await expect(kim.getByRole('heading', { level: 1 })).toBeFocused();
	// Asked again, it is back on the Wishlist for the owner to approve.
	await search(kim, 'copper');
	const again = kim.locator('li.hit', { hasText: 'Copper Lighthouse' });
	await again.getByRole('button', { name: 'Ask', exact: true }).click();
	await kim.getByRole('dialog', { name: /^Ask for Copper Lighthouse/ }).getByRole('button', { name: 'Ask for Copper Lighthouse' }).click();
	await expect(again.getByRole('link', { name: /^You asked · Requested on your Wishlist$/ })).toBeVisible();
	await kim.locator('nav.tabbar').getByRole('link', { name: 'Wishlist' }).click();
	await expect(ask.getByTestId('ask-state')).toHaveText('Requested');
	await noSidewaysScroll(kim);
	expect((await posts()).some((c) => c.includes('kim asked for Copper Lighthouse'))).toBe(true);
});

test('the owner approves from a phone: the movie is added and kim is told', async () => {
	await own.goto('/library');
	// Approvers find the Wishlist as Approvals in the Manage sheet; the tab shows a dot.
	const tab = own.locator('nav.tabbar').getByRole('button', { name: /^Manage/ });
	await expect(tab).toHaveAccessibleName(/items need attention/);
	await tab.click();
	await own.getByRole('dialog', { name: 'Manage' }).getByRole('link', { name: 'Approvals, 1 waiting' }).click();
	await expect(own.getByRole('button', { name: /^Waiting/ })).toHaveAttribute('aria-pressed', 'true');
	const ask = own.getByTestId('ask').filter({ hasText: 'Copper Lighthouse' });
	await expect(ask.getByTestId('asked-by')).toContainText('Asked by kim');
	await noSidewaysScroll(own);
	await ask.getByRole('button', { name: 'Approve' }).click();
	const dialog = own.getByRole('dialog');
	await expect(dialog.getByLabel('Library folder')).toBeVisible();
	await dialog.getByLabel('Note (optional)').fill('Friday night pick');
	await noSidewaysScroll(own);
	await dialog.getByRole('button', { name: 'Approve and add' }).click();
	await expect(dialog).toBeHidden();

	await own.getByRole('button', { name: /^Everyone's/ }).click();
	await expect(ask.getByTestId('ask-state')).toHaveText('Approved');
	await expect(ask.getByTestId('decided-by')).toContainText('Approved by luis');
	await expect.poll(async () => (await posts()).some((c) => c.startsWith(`<@${kimDiscord}> **Approved: Copper Lighthouse`))).toBe(true);

	// The Movie page names who asked and who approved.
	await ask.getByRole('link', { name: /Copper Lighthouse/ }).click();
	const asks = own.getByTestId('title-asks');
	await expect(asks).toContainText('kim');
	await expect(asks).toContainText('Approved by luis');
	// History on Home names them too.
	await own.goto('/');
	await expect(own.getByText('kim asked for Copper Lighthouse (2017)')).toBeVisible();

	await kim.goto('/wishlist');
	await expect(kim.getByTestId('message').first()).toContainText('Copper Lighthouse (2017) was approved by luis');
	await expect(kim.getByTestId('message').first()).toContainText('Friday night pick');
	await expect(kim.getByTestId('ask').filter({ hasText: 'Copper Lighthouse' }).getByTestId('ask-state')).toHaveText('Approved');
	await kim.getByRole('button', { name: 'Mark all read' }).click();
	await expect(kim.getByRole('button', { name: 'Mark all read' })).toHaveCount(0);
});

test('turned down with a note; over the weekly limit kim is told when to try again', async () => {
	await search(kim, 'velvet');
	await kim.locator('li.hit', { hasText: 'Velvet Tide' }).getByRole('button', { name: 'Ask', exact: true }).click();
	await kim.getByRole('dialog', { name: /^Ask for Velvet Tide/ }).getByRole('button', { name: 'Ask for Velvet Tide' }).click();
	await expect(kim.locator('li.hit', { hasText: 'Velvet Tide' }).getByRole('link', { name: /^You asked · Requested on your Wishlist$/ })).toBeVisible();

	await own.goto('/wishlist');
	const ask = own.getByTestId('ask').filter({ hasText: 'Velvet Tide' });
	await ask.getByRole('button', { name: 'Turn down' }).click();
	await own.getByRole('dialog').getByLabel(/^Why/).fill('We have it on disc');
	await own.getByRole('dialog').getByRole('button', { name: 'Turn down' }).click();
	await own.getByRole('button', { name: /^Everyone's/ }).click();
	await expect(ask.getByTestId('ask-state')).toHaveText('Turned down');

	await kim.goto('/wishlist');
	const mine = kim.getByTestId('ask').filter({ hasText: 'Velvet Tide' });
	await expect(mine.getByTestId('ask-state')).toHaveText('Turned down');
	await expect(mine).toContainText('We have it on disc');

	// One movie a week for kim, set on the Users page: Copper Lighthouse used
	// it (the turned down one does not count). Shows and music keep the default.
	await own.goto('/settings/users');
	const row = own.locator('li.person', { hasText: 'kim' });
	await expect(row.getByTestId('user-limits')).toContainText('1 movie ask of 10');
	await row.getByRole('button', { name: 'Change kim' }).click();
	const dlg = own.getByRole('dialog');
	await expect(dlg.getByLabel('Show asks')).toHaveAttribute('placeholder', 'Default (10)');
	await dlg.getByLabel('Movie asks').fill('1');
	await dlg.getByRole('button', { name: 'Save' }).click();
	await expect(row.getByTestId('user-limits')).toContainText('1 movie ask of 1 (own)');
	await expect(row.getByTestId('user-limits')).toContainText('0 show asks of 10');
	await noSidewaysScroll(own);

	await search(kim, 'salt choir');
	await expect(kim.getByTestId('ask-limit')).toContainText('Try again in 7 days');
	await kim.locator('li.hit', { hasText: 'Salt Choir' }).getByRole('button', { name: 'Ask', exact: true }).click();
	await kim.getByRole('dialog', { name: /^Ask for Salt Choir/ }).getByRole('button', { name: 'Ask for Salt Choir' }).click();
	await expect(kim.getByRole('alert')).toContainText('try again in 7 days');
	await noSidewaysScroll(kim);
});

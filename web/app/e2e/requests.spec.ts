import { expect, test, type Browser, type Page } from '@playwright/test';
import { discordPort } from './fakes.mjs';
import { ensureInLibrary, noSidewaysScroll, openFromSearch, owner, signIn } from './helpers.ts';

// Requests at phone width (Pixel 7): a member asks for movies, the owner
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

// Search opens a movie's title page; Ask lives there.
async function askButton(p: Page, words: string, name: string) {
	await openFromSearch(p, words, name);
	return p.getByTestId('title-detail').getByRole('button', { name: new RegExp(`^Ask for ${name}`) });
}

test.beforeAll(async ({ browser }) => {
	// Run on setup alone, Quiet Orchard (added by movies) is not there yet.
	await ensureInLibrary(browser, 'imdb:tt9000030', 'Quiet Orchard');
	own = await fresh(browser);
	await signIn(own, owner());
});

test('the owner adds a Discord Alert and sends a test', async () => {
	await own.goto('/settings/alerts');
	await expect(own.getByRole('heading', { level: 1, name: 'Alerts' })).toBeVisible();
	await expect(own.getByText('No Alerts yet')).toBeVisible();
	await own.getByRole('button', { name: 'Add an Alert' }).first().click();
	await own.getByLabel('Name', { exact: true }).fill('Family Discord');
	await own.getByLabel('Discord link').fill(`http://127.0.0.1:${discordPort}/api/webhooks/1/stand-in`);
	await own.getByLabel('May reach the local network').check();
	await own.locator('form.sc-form').getByRole('button', { name: 'Save', exact: true }).click();
	const row = own.getByTestId('alert');
	await expect(row).toContainText('Family Discord');
	await expect(row).toContainText('Someone asks · Approved · Declined · Ready');
	await expect(own.locator('body')).not.toContainText('stand-in');
	await row.getByRole('button', { name: 'Send a test' }).click();
	await expect(row.getByTestId('alert-result')).toContainText('Last sent');
	expect((await posts()).some((c) => c.includes('HolyArrs test'))).toBe(true);
	await noSidewaysScroll(own);
});

test('a member asks for a movie from a phone and sees it on the Requests page', async ({ browser }) => {
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
	await kim.getByLabel('Discord user ID').fill(kimDiscord);
	await kim.getByTestId('savebar').getByRole('button', { name: 'Save' }).click();
	await expect(kim.getByText('Saved where Alerts reach you.')).toBeVisible();

	await kim.goto('/requests');
	await expect(kim.getByTestId('ask-limit')).toContainText('You have 10 movie asks left this week');
	const askCopper = await askButton(kim, 'copper', 'Copper Lighthouse');
	await askCopper.click();
	// Ask never fires in one click: a confirm sheet opens with focus inside.
	const confirmAsk = kim.getByRole('dialog', { name: /^Ask for Copper Lighthouse/ });
	await expect(confirmAsk).toBeVisible();
	await expect(confirmAsk.locator(':focus')).toHaveCount(1);
	await kim.keyboard.press('Escape');
	await expect(confirmAsk).toBeHidden();
	await expect(askCopper).toBeFocused();
	await askCopper.click();
	await confirmAsk.getByRole('button', { name: 'Ask for Copper Lighthouse' }).click();
	await expect(askCopper).toBeDisabled();
	await noSidewaysScroll(kim);
	// A movie already in the Library opens its Library page; there is nothing to ask.
	await openFromSearch(kim, 'orchard', 'Quiet Orchard');
	await expect(kim).toHaveURL(/\/library\/\d+/);
	await expect(kim.getByRole('button', { name: /^Ask for/ })).toHaveCount(0);

	await kim.locator('nav.tabbar').getByRole('link', { name: 'Requests' }).click();
	await expect(kim.getByTestId('ask-limit')).toContainText('You have 9 movie asks left this week');
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
	// Take back through to removal: the ask leaves Requests and, with no
	// row left, focus lands on the page heading.
	await ask.getByRole('button', { name: 'More for Copper Lighthouse' }).click();
	await ask.getByRole('menuitem', { name: 'Take back' }).click();
	await confirmTakeBack.getByRole('button', { name: 'Take back' }).click();
	await expect(confirmTakeBack).toBeHidden();
	await expect(confirmTakeBack.getByTestId('take-back-error')).toHaveCount(0);
	await expect(ask).toHaveCount(0);
	await expect(kim.getByText('Took back Copper Lighthouse.')).toBeVisible();
	await expect(kim.getByRole('heading', { level: 1 })).toBeFocused();
	// Asked again, it is back on the Requests page for the owner to approve.
	const again = await askButton(kim, 'copper', 'Copper Lighthouse');
	await again.click();
	await kim.getByRole('dialog', { name: /^Ask for Copper Lighthouse/ }).getByRole('button', { name: 'Ask for Copper Lighthouse' }).click();
	await expect(again).toBeDisabled();
	await kim.locator('nav.tabbar').getByRole('link', { name: 'Requests' }).click();
	await expect(ask.getByTestId('ask-state')).toHaveText('Requested');
	await noSidewaysScroll(kim);
	expect((await posts()).some((c) => c.includes('kim asked for Copper Lighthouse'))).toBe(true);
});

// An approver's card on the Requests page (the cards list, every width).
function card(p: Page, name: string) {
	return p.getByRole('list', { name: 'Requests' }).getByRole('listitem').filter({ hasText: name });
}

test('the owner approves from a phone: the movie is added and kim is told', async () => {
	await own.goto('/library');
	// Approvers find Requests in the Manage sheet, under Your stuff; the tab shows a dot.
	const tab = own.locator('nav.tabbar').getByRole('button', { name: /^Manage/ });
	await expect(tab).toHaveAccessibleName(/items need attention/);
	await tab.click();
	await own.getByRole('dialog', { name: 'Manage' }).getByRole('link', { name: 'Requests, 1 waiting' }).click();
	// Approvers open on Pending, with the count on the tab.
	await expect(own.getByRole('button', { name: /^Pending 1/ })).toHaveAttribute('aria-pressed', 'true');
	const ask = card(own, 'Copper Lighthouse');
	await expect(ask).toContainText('kim');
	await expect(ask.getByRole('button', { name: 'Open Copper Lighthouse, asked by kim' })).toBeVisible();
	await noSidewaysScroll(own);
	// The card opens its window: the banner says who asked, and the window opens
	// straight into What to add, which holds the Approve.
	await ask.getByRole('button', { name: 'Open Copper Lighthouse, asked by kim' }).click();
	const win = own.getByRole('dialog', { name: 'Copper Lighthouse' });
	await expect(win).toBeVisible();
	await expect(win.getByTestId('ask-banner')).toContainText('kim asked');
	const sheet = win.getByRole('region', { name: 'What to add' });
	await expect(sheet).toBeVisible();
	await expect(sheet).toContainText('Approving adds the movie');
	await noSidewaysScroll(own);
	await sheet.getByRole('button', { name: 'Approve', exact: true }).click();
	await expect(win).toBeHidden();
	// Undo waits at the bottom for a few seconds, then the answer is sent.
	await expect(own.getByRole('status').filter({ hasText: 'Approved Copper Lighthouse.' }).getByRole('button', { name: 'Undo' })).toBeVisible();
	await expect(own.getByRole('button', { name: /^Pending 0/ })).toBeVisible();
	await expect.poll(async () => (await posts()).some((c) => c.startsWith(`<@${kimDiscord}> **Approved: Copper Lighthouse`)), { timeout: 20_000 }).toBe(true);

	// Processing holds it; its window says it is being got for kim.
	await own.getByRole('button', { name: /^Processing/ }).click();
	await ask.getByRole('button', { name: /^Open Copper Lighthouse/ }).click();
	await expect(win.getByTestId('ask-status')).toContainText('Getting it for kim');
	// The Movie page names who asked (the one title page has no Requests section, so
	// who approved is checked in the Events list below).
	await win.getByTestId('ask-title-page').click();
	await expect(own).toHaveURL(/\/library\/\d+/);
	await expect(own.getByTestId('asked-by-line')).toContainText('Asked by kim');
	// The Events list names them (Home no longer has a Latest Events row).
	// kim asked, took it back and asked again: two ask events, the take-back between them.
	const events = ((await (await own.request.get('/api/v1/events?limit=200')).json()) as { events: { Summary: string }[] }).events.map((e) => e.Summary);
	expect(events.filter((e) => /asked for Copper Lighthouse/.test(e))).toHaveLength(2);
	expect(events.filter((e) => /took back the ask for Copper Lighthouse/.test(e))).toHaveLength(1);
	expect(events.filter((e) => /Copper Lighthouse.*: approved by luis/.test(e))).toHaveLength(1);

	await kim.goto('/requests');
	await kim.getByText(/^Show the latest/).click();
	await expect(kim.getByTestId('message').first()).toContainText('Copper Lighthouse (2017) was approved by luis');
	await expect(kim.getByTestId('ask').filter({ hasText: 'Copper Lighthouse' }).getByTestId('ask-state')).toHaveText('Approved');
	await kim.getByRole('button', { name: 'Mark all read' }).click();
	await expect(kim.getByRole('button', { name: 'Mark all read' })).toHaveCount(0);
});

test('turned down with a note; over the weekly limit kim is told when to try again', async () => {
	const velvet = await askButton(kim, 'velvet', 'Velvet Tide');
	await velvet.click();
	await kim.getByRole('dialog', { name: /^Ask for Velvet Tide/ }).getByRole('button', { name: 'Ask for Velvet Tide' }).click();
	await expect(velvet).toBeDisabled();

	await own.goto('/requests');
	await card(own, 'Velvet Tide').getByRole('button', { name: 'Decline Velvet Tide' }).click();
	const why = own.getByRole('dialog', { name: /^Decline Velvet Tide/ });
	await expect(why.getByLabel(/^Reason/)).toBeFocused();
	await why.getByLabel(/^Reason/).fill('We have it on disc');
	await why.getByRole('button', { name: 'Decline', exact: true }).click();
	await expect(why).toBeHidden();
	await own.getByRole('button', { name: /^Done/ }).click();
	const ask = card(own, 'Velvet Tide');
	// A turn down is sent after its few seconds of Undo (the card says Sending… till then).
	await expect(ask.getByText('Sending…')).toHaveCount(0, { timeout: 15_000 });
	await expect(ask).toContainText('Declined');

	await kim.goto('/requests');
	await kim.getByRole('button', { name: /^Declined/ }).click();
	const mine = kim.getByTestId('ask').filter({ hasText: 'Velvet Tide' });
	await expect(mine.getByTestId('ask-state')).toHaveText('Declined');
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

	await kim.goto('/requests');
	await expect(kim.getByTestId('ask-limit')).toContainText('Try again in 7 days');
	await (await askButton(kim, 'salt choir', 'Salt Choir')).click();
	await kim.getByRole('dialog', { name: /^Ask for Salt Choir/ }).getByRole('button', { name: 'Ask for Salt Choir' }).click();
	await expect(kim.getByRole('alert')).toContainText('try again in 7 days');
	await noSidewaysScroll(kim);
});

import { expect, test, type Browser, type Page } from '@playwright/test';
import { createHmac } from 'node:crypto';
import { owner, signIn, signOut } from './helpers.ts';
import { embyUser, jellyfinPort, mediaUser } from './fakes.mjs';

test.describe.configure({ mode: 'serial' });

const memberPw = `e2e-${Math.random().toString(36).slice(2)}-member`;

/** totp is the six-digit code for a base32 secret, like an authenticator app. */
function totp(secret: string, at = Date.now()): string {
	const alpha = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
	let bits = '';
	for (const ch of secret.replace(/\s/g, '')) bits += alpha.indexOf(ch).toString(2).padStart(5, '0');
	const key = Buffer.from(bits.match(/.{8}/g)!.map((b) => parseInt(b, 2)));
	const msg = Buffer.alloc(8);
	msg.writeBigUInt64BE(BigInt(Math.floor(at / 1000 / 30)));
	const h = createHmac('sha1', key).update(msg).digest();
	const off = h[h.length - 1] & 15;
	return String((h.readUInt32BE(off) & 0x7fffffff) % 1_000_000).padStart(6, '0');
}

async function fresh(browser: Browser): Promise<Page> {
	return (await browser.newContext()).newPage();
}

let own: Page;
let ana: Page;
let backup: string[] = [];

test.beforeAll(async ({ browser }) => {
	own = await fresh(browser);
	await signIn(own, owner());
});

test('the owner invites a member with a one-time link', async ({ browser }) => {
	await own.goto('/settings/users');
	await expect(own.getByRole('heading', { level: 1, name: 'Users' })).toBeVisible();
	await own.getByLabel('Note for yourself').fill('Ana');
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	expect(link).toContain('/invite/');

	ana = await fresh(browser);
	await ana.goto(new URL(link).pathname);
	await expect(ana.getByRole('heading', { name: 'Join HolyArrs' })).toBeVisible();
	await ana.getByLabel('Name').fill('ana');
	await ana.getByLabel('Password', { exact: true }).fill(memberPw);
	await ana.getByLabel('Password again').fill(memberPw);
	await ana.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(ana).toHaveURL(/\/$/);

	// The link works once.
	const again = await fresh(browser);
	await again.goto(new URL(link).pathname);
	await expect(again.getByRole('alert')).toContainText('used up');
	await own.reload();
	await expect(own.getByText('Used by ana')).toBeVisible();
});

test('a member sees Home, the Library, Search, Requests and My account only', async () => {
	const nav = ana.getByRole('navigation', { name: 'Main' }).first();
	for (const label of ['Home', 'Explore', 'Movies', 'Requests']) await expect(nav.getByRole('link', { name: label, exact: true })).toBeVisible();
	await expect(ana.getByTestId('search-box')).toBeVisible();
	// A member follows the downloads and dates of what they asked for, but not the manager pages.
	for (const label of ['Downloads', 'Calendar']) await expect(nav.getByRole('link', { name: label, exact: true })).toBeVisible();
	for (const label of ['History', 'Library tools', 'Status']) await expect(nav.getByRole('link', { name: label })).toHaveCount(0);
	await expect(ana.getByRole('link', { name: 'Settings' })).toHaveCount(0);
	await ana.getByRole('button', { name: 'Account and appearance' }).click();
	await expect(ana.getByRole('menuitem', { name: 'Account', exact: true })).toBeVisible();
	await ana.keyboard.press('Escape');
	// Manager pages send a member to their own Home.
	for (const p of ['/settings', '/settings/backups', '/downloads/history', '/']) {
		await ana.goto(p);
		await expect(ana).toHaveURL(/\/$/);
	}
	await expect(ana.getByRole('heading', { level: 1, name: 'Home' })).toBeVisible();
	await ana.goto('/requests');
	await expect(ana.getByRole('heading', { name: 'Nothing on the Requests page yet' })).toBeVisible();
	await ana.goto('/search?q=Dune');
	await expect(ana.getByRole('heading', { level: 2 }).first()).toBeVisible();
	await expect(ana.getByRole('button', { name: 'Add' })).toHaveCount(0);
});

test('a member turns on two-factor sign-in and needs a code to sign in', async () => {
	await ana.goto('/account');
	await ana.getByTestId('two-factor-open').click();
	// A code to scan, drawn square, next to the key to type.
	const qr = ana.getByRole('img', { name: 'QR code for your authenticator app' });
	await expect(qr).toBeVisible();
	const box = (await qr.boundingBox())!;
	expect(Math.abs(box.width - box.height)).toBeLessThan(2);
	const secret = (await ana.locator('code.key').textContent())!.replace(/\s/g, '');
	await ana.getByLabel('Six-digit code').fill(totp(secret));
	await ana.getByRole('button', { name: 'Turn on' }).click();
	await expect(ana.getByText('Backup codes.')).toBeVisible();
	backup = (await ana.locator('ul.codes li').allTextContents()).map((s) => s.trim());
	expect(backup).toHaveLength(10);
	await ana.getByRole('button', { name: 'Done' }).click();
	await expect(ana.locator('.chip.ok', { hasText: 'On' })).toBeVisible();

	await signOut(ana);
	await expect(ana).toHaveURL(/\/login/);
	await ana.getByLabel('Name').fill('ana');
	await ana.getByLabel('Password', { exact: true }).fill(memberPw);
	await ana.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(ana.getByRole('heading', { name: 'Two-factor sign-in' })).toBeVisible();
	await ana.getByLabel('Code').fill('000000');
	await ana.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(ana.getByRole('alert')).toContainText('wrong code');
	// The app code was just used to turn it on, so sign in with a backup code.
	await ana.getByLabel('Code').fill(backup[0]);
	await ana.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(ana).toHaveURL(/\/account$/); // back where they were
});

test('the owner changes what a member may do, and can turn them off', async () => {
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Change ana' }).click();
	const dlg = own.getByRole('dialog');
	await dlg.getByLabel('Run the Library').check();
	await dlg.getByRole('button', { name: 'Save' }).click();
	await expect(own.getByText('ana saved.')).toBeVisible();
	await ana.reload();
	await expect(ana.getByRole('navigation', { name: 'Main' }).first().getByRole('link', { name: 'Downloads' })).toBeVisible();

	await own.getByRole('button', { name: 'Reset two-factor sign-in for ana' }).click({ trial: true });
	own.once('dialog', (d) => d.accept());
	await own.getByRole('button', { name: 'Turn off ana' }).click();
	await expect(own.getByText('ana turned off.')).toBeVisible();
	await ana.goto('/library');
	await expect(ana).toHaveURL(/\/login/);
	own.once('dialog', (d) => d.accept());
	await own.getByRole('button', { name: 'Turn on ana' }).click();
	await expect(own.getByText('ana turned on.')).toBeVisible();
});

test('the owner sets a forgotten password, and ana switches account on a shared screen', async () => {
	const newPw = `${memberPw}-new`;
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Set a new password for ana' }).click();
	const dlg = own.getByRole('dialog', { name: 'New password for ana' });
	await dlg.getByLabel('New password').fill(newPw);
	await dlg.getByRole('button', { name: 'Set password' }).click();
	await expect(own.getByText('New password set for ana.')).toBeVisible();
	await ana.goto('/library');
	await expect(ana).toHaveURL(/\/login/);
	await ana.getByLabel('Name').fill('ana');
	await ana.getByLabel('Password', { exact: true }).fill(newPw);
	await ana.getByRole('button', { name: 'Sign in', exact: true }).click();
	await ana.getByLabel('Code').fill(backup[1]);
	await ana.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(ana.getByRole('heading', { name: 'Two-factor sign-in' })).toHaveCount(0);
	await expect(ana).not.toHaveURL(/\/login/);

	// Switch account: signs out, and ana is one tap away on this device.
	await ana.getByRole('button', { name: 'Account and appearance' }).click();
	await ana.getByRole('menuitem', { name: 'Switch account' }).click();
	await expect(ana).toHaveURL(/\/login/);
	const people = ana.getByTestId('recent-people');
	await people.getByRole('button', { name: 'ana', exact: true }).click();
	await expect(ana.getByLabel('Name')).toHaveValue('ana');
	await people.getByRole('button', { name: 'Forget ana on this device' }).click();
	await expect(people).toHaveCount(0);
});

test('sign in with Jellyfin makes a member', async ({ browser }) => {
	await own.goto('/settings/sign-in');
	const card = own.locator('section', { has: own.getByRole('heading', { name: 'Sign in with Jellyfin' }) });
	await card.getByText('Show the Jellyfin button on the sign-in page').click();
	await card.getByLabel('Jellyfin server address').fill(`http://127.0.0.1:${jellyfinPort}`);
	await card.getByLabel('Who may join the first time').selectOption('everyone');
	await card.getByRole('button', { name: 'Save' }).click();
	// Letting everyone join asks once before it is saved.
	await own.getByRole('button', { name: 'Let everyone join' }).click();
	await expect(own.getByText('Jellyfin sign-in saved.')).toBeVisible();

	const mum = await fresh(browser);
	await mum.goto('/login');
	await mum.getByRole('button', { name: 'Continue with Jellyfin' }).click();
	await mum.getByLabel('Name').fill(mediaUser.name);
	await mum.getByLabel('Password', { exact: true }).fill('not it');
	await mum.getByRole('button', { name: 'Sign in with Jellyfin' }).click();
	await expect(mum.getByRole('alert')).toContainText('wrong name or password');
	await mum.getByLabel('Password', { exact: true }).fill(mediaUser.password);
	await mum.getByRole('button', { name: 'Sign in with Jellyfin' }).click();
	await expect(mum).toHaveURL(/\/$/);
	await expect(mum.getByRole('button', { name: 'Account and appearance' })).toBeVisible();

	// A first HolyArrs password, for when Jellyfin is down.
	const first = `${mediaUser.password}-holyarrs`;
	await mum.goto('/account');
	await mum.getByRole('button', { name: 'Set a password' }).click();
	await expect(mum.getByLabel('Current password')).toHaveCount(0);
	await mum.getByLabel('New password', { exact: true }).fill(first);
	await mum.getByLabel('Confirm new password').fill(first);
	await mum.getByRole('button', { name: 'Set password' }).click();
	await expect(mum.getByText('Password set.', { exact: false })).toBeVisible();
	const back = await fresh(browser);
	await back.goto('/login');
	await back.getByLabel('Name').fill('Mum');
	await back.getByLabel('Password', { exact: true }).fill(first);
	await back.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(back).toHaveURL(/\/$/);
});

test('sign in with Emby makes a member, next to Jellyfin', async ({ browser }) => {
	await own.goto('/settings/sign-in');
	const card = own.locator('section', { has: own.getByRole('heading', { name: 'Sign in with Emby' }) });
	await card.getByText('Show the Emby button on the sign-in page').click();
	await card.getByLabel('Emby server address').fill(`http://127.0.0.1:${jellyfinPort}/emby`);
	await card.getByLabel('Who may join the first time').selectOption('everyone');
	await card.getByRole('button', { name: 'Save' }).click();
	await own.getByRole('button', { name: 'Let everyone join' }).click();
	await expect(own.getByText('Emby sign-in saved.')).toBeVisible();

	const gran = await fresh(browser);
	await gran.goto('/login');
	// With two server ways on, each is a tile of its own.
	await expect(gran.getByRole('button', { name: 'Jellyfin', exact: true })).toBeVisible();
	await gran.getByRole('button', { name: 'Emby', exact: true }).click();
	await gran.getByLabel('Name').fill(embyUser.name);
	// Mum's Jellyfin password is not an Emby password.
	await gran.getByLabel('Password', { exact: true }).fill(mediaUser.password);
	await gran.getByRole('button', { name: 'Sign in with Emby' }).click();
	await expect(gran.getByRole('alert')).toContainText('wrong name or password');
	await gran.getByLabel('Password', { exact: true }).fill(embyUser.password);
	await gran.getByRole('button', { name: 'Sign in with Emby' }).click();
	await expect(gran).toHaveURL(/\/$/);
	await expect(gran.getByRole('button', { name: 'Account and appearance' })).toBeVisible();
});

test('sign in with Plex makes a member (fake Plex)', async ({ browser }) => {
	await own.goto('/settings/sign-in');
	const card = own.locator('section', { has: own.getByRole('heading', { name: 'Sign in with Plex' }) });
	await card.getByText('Show the Plex button on the sign-in page').click();
	await card.getByText('Advanced: type the server id').click();
	await card.getByLabel('Server id people must share (optional)').fill('fake-server');
	await card.getByLabel('Who may join the first time').selectOption('everyone');
	await card.getByRole('button', { name: 'Save' }).click();
	await own.getByRole('button', { name: 'Let everyone join' }).click();
	await expect(own.getByText('Plex sign-in saved.')).toBeVisible();

	const kid = await fresh(browser);
	await kid.goto('/login');
	const popup = kid.waitForEvent('popup');
	await kid.getByRole('button', { name: 'Plex', exact: true }).click();
	await (await popup).waitForLoadState().catch(() => {});
	await expect(kid).toHaveURL(/\/$/, { timeout: 15_000 });
	await expect(kid.getByRole('button', { name: 'Account and appearance' })).toBeVisible();

	await own.goto('/settings/users');
	await expect(own.getByText('Jellyfin: Mum')).toBeVisible();
	await expect(own.getByText('Emby: Gran')).toBeVisible();
	await expect(own.getByText('Plex: plexkid')).toBeVisible();
});

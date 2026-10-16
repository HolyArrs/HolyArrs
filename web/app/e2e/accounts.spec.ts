import { expect, test, type Browser, type Page } from '@playwright/test';
import { createHmac } from 'node:crypto';
import { owner, signIn } from './helpers.ts';
import { jellyfinPort, mediaUser } from './fakes.mjs';

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
	await expect(ana).toHaveURL(/\/library$/);

	// The link works once.
	const again = await fresh(browser);
	await again.goto(new URL(link).pathname);
	await expect(again.getByRole('alert')).toContainText('used up');
	await own.reload();
	await expect(own.getByText('Used by ana')).toBeVisible();
});

test('a member sees Library, Search, Wishlist and My account only', async () => {
	const nav = ana.getByRole('navigation', { name: 'Main' }).first();
	for (const label of ['Library', 'Search', 'Wishlist']) await expect(nav.getByRole('link', { name: label })).toBeVisible();
	for (const label of ['Home', 'Downloads', 'Calendar']) await expect(nav.getByRole('link', { name: label })).toHaveCount(0);
	await expect(ana.getByRole('link', { name: 'Settings' })).toHaveCount(0);
	await expect(ana.getByRole('link', { name: 'My account' }).first()).toBeVisible();
	for (const p of ['/settings', '/settings/backups', '/downloads', '/', '/calendar']) {
		await ana.goto(p);
		await expect(ana).toHaveURL(/\/library$/);
	}
	await ana.goto('/wishlist');
	await expect(ana.getByRole('heading', { name: 'Nothing on the Wishlist yet' })).toBeVisible();
	await ana.goto('/search?q=Dune');
	await expect(ana.getByRole('heading', { level: 2 }).first()).toBeVisible();
	await expect(ana.getByRole('button', { name: 'Add' })).toHaveCount(0);
});

test('a member turns on two-factor sign-in and needs a code to sign in', async () => {
	await ana.goto('/account');
	await ana.getByRole('button', { name: 'Set up two-factor sign-in' }).click();
	const secret = (await ana.locator('code.key').textContent())!.replace(/\s/g, '');
	await ana.getByLabel('Six-digit code').fill(totp(secret));
	await ana.getByRole('button', { name: 'Turn on' }).click();
	await expect(ana.getByText('Backup codes.')).toBeVisible();
	backup = (await ana.locator('ul.codes li').allTextContents()).map((s) => s.trim());
	expect(backup).toHaveLength(10);
	await ana.getByRole('button', { name: 'Done' }).click();
	await expect(ana.locator('.chip.ok', { hasText: 'On' })).toBeVisible();

	await ana.getByRole('button', { name: 'Sign out' }).click();
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

test('sign in with Jellyfin makes a member', async ({ browser }) => {
	await own.goto('/settings/sign-in');
	const card = own.locator('section', { has: own.getByRole('heading', { name: 'Sign in with Jellyfin' }) });
	await card.getByText('Show the Jellyfin button on the sign-in page').click();
	await card.getByLabel('Jellyfin server address').fill(`http://127.0.0.1:${jellyfinPort}`);
	await card.getByLabel('Who may join the first time').selectOption('everyone');
	await card.getByRole('button', { name: 'Save' }).click();
	await expect(own.getByText('Jellyfin sign-in saved.')).toBeVisible();

	const mum = await fresh(browser);
	await mum.goto('/login');
	await mum.getByRole('button', { name: 'Sign in with Jellyfin' }).click();
	await mum.getByLabel('Name').fill(mediaUser.name);
	await mum.getByLabel('Password', { exact: true }).fill('not it');
	await mum.getByRole('button', { name: 'Sign in with Jellyfin' }).click();
	await expect(mum.getByRole('alert')).toContainText('wrong name or password');
	await mum.getByLabel('Password', { exact: true }).fill(mediaUser.password);
	await mum.getByRole('button', { name: 'Sign in with Jellyfin' }).click();
	await expect(mum).toHaveURL(/\/library$/);
	await expect(mum.getByRole('link', { name: 'My account' }).first()).toBeVisible();
});

test('sign in with Plex makes a member (fake Plex)', async ({ browser }) => {
	await own.goto('/settings/sign-in');
	const card = own.locator('section', { has: own.getByRole('heading', { name: 'Sign in with Plex' }) });
	await card.getByText('Show the Plex button on the sign-in page').click();
	await card.getByLabel('Server id people must share (optional)').fill('fake-server');
	await card.getByLabel('Who may join the first time').selectOption('everyone');
	await card.getByRole('button', { name: 'Save' }).click();
	await expect(own.getByText('Plex sign-in saved.')).toBeVisible();

	const kid = await fresh(browser);
	await kid.goto('/login');
	const popup = kid.waitForEvent('popup');
	await kid.getByRole('button', { name: 'Sign in with Plex' }).click();
	await (await popup).waitForLoadState().catch(() => {});
	await expect(kid).toHaveURL(/\/library$/, { timeout: 15_000 });

	await own.goto('/settings/users');
	await expect(own.getByText('Jellyfin: Mum')).toBeVisible();
	await expect(own.getByText('Plex: plexkid')).toBeVisible();
});

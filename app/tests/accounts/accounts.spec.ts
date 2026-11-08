import { expect, test, type Browser, type Dialog, type Locator, type Page } from '@playwright/test';
import { createHmac } from 'node:crypto';
import { owner, signIn, signOut } from '../helpers/helpers.ts';
import { embyUser, jellyfinPort, mediaUser } from '../fakes/index.mjs';

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
	await own.getByRole('button', { name: 'Invite someone' }).click();
	await expect(own.getByRole('heading', { level: 1, name: 'Users' })).toBeVisible();
	await own.getByLabel('Note for yourself').fill('Ana');
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	expect(link).toContain('/invite/');

	// Opened while signed in: it says who, and offers Home instead of a second account.
	const mine = await own.context().newPage();
	await mine.goto(new URL(link).pathname);
	await expect(mine.getByTestId('invite-signed-in')).toContainText("You're signed in as");
	await expect(mine.getByRole('link', { name: 'Go home' })).toBeVisible();
	await expect(mine.getByRole('button', { name: 'Sign out and join' })).toBeVisible();
	await mine.close();

	ana = await fresh(browser);
	await ana.goto(new URL(link).pathname);
	await expect(ana.getByRole('heading', { name: 'Join HolyArrs' })).toBeVisible();
	await ana.getByLabel('Name').fill('ana');
	await ana.getByLabel('Password', { exact: true }).fill(memberPw);
	await ana.getByLabel('Confirm password').fill(memberPw);
	await ana.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(ana).toHaveURL(/\/$/);

	// The link works once.
	const again = await fresh(browser);
	await again.goto(new URL(link).pathname);
	await expect(again.getByRole('alert')).toContainText('used up');
	await own.reload();
	await expect(own.getByText('Used by ana')).toBeVisible();
});

test('a member links their Plex account from My account for the watchlist, and unlinks it', async () => {
	await ana.goto('/settings/account');
	const row = ana.locator('#linked-me');
	await row.getByTestId('link-sign-in').click();
	const box = row.getByTestId('confirm-sign-in');
	if (await box.isVisible().catch(() => false)) {
		await box.getByLabel('HolyArrs password').fill(memberPw);
		await box.getByRole('button', { name: 'Confirm' }).click();
	}
	// Plex sign-in is off here: linking works anyway, and the Plex page opens in its own window.
	const popup = ana.waitForEvent('popup');
	await row.getByRole('button', { name: 'Plex', exact: true }).click();
	await (await popup).waitForLoadState().catch(() => {});
	await expect(ana.getByText('Plex plexkid linked')).toBeVisible({ timeout: 15_000 });
	const mine = row.getByTestId('my-plex-link');
	await expect(mine).toContainText('Plex, as plexkid');
	await mine.getByRole('button', { name: 'Unlink Plex' }).click();
	const again = mine.getByTestId('confirm-sign-in');
	if (await again.isVisible().catch(() => false)) {
		await again.getByLabel('HolyArrs password').fill(memberPw);
		await again.getByRole('button', { name: 'Confirm' }).click();
	}
	await mine.getByRole('button', { name: 'Unlink', exact: true }).click();
	await expect(ana.getByText('Plex unlinked', { exact: false })).toBeVisible();
	await expect(mine).toHaveCount(0);
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
	// Manager pages send a member to their own Home; a Settings page they may not open says so in place.
	await ana.goto('/settings/backups');
	await expect(ana.locator('#settings-blocked')).toContainText("You can't change this");
	for (const p of ['/settings', '/downloads/history', '/']) {
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
	await ana.getByRole('button', { name: 'Close', exact: true }).click();
	await expect(ana.locator('.chip.ok', { hasText: 'On' })).toBeVisible();

	await signOut(ana);
	await expect(ana).toHaveURL(/\/login/);
	await ana.getByLabel('Name').fill('ana');
	await ana.getByLabel('Password', { exact: true }).fill(memberPw);
	await ana.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(ana.getByRole('heading', { name: 'Two-step check' })).toBeVisible();
	// Six digits in the first box spread over all six and send at once.
	await ana.getByLabel('Digit 1').fill('000000');
	await expect(ana.getByRole('alert')).toContainText(/wrong code/i);
	// The app code was just used to turn it on, so sign in with a backup code.
	await ana.getByRole('button', { name: 'Use a backup code' }).click();
	await ana.getByLabel('Backup code').fill(backup[0]);
	await ana.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(ana).toHaveURL(/\/account$/); // back where they were
});

test('the owner changes what a member may do, and can turn them off', async () => {
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Change ana' }).click();
	const dlg = own.getByRole('dialog');
	await dlg.getByLabel('Run the Library').check();
	await dlg.getByRole('button', { name: 'Save' }).click();
	await expect(own.getByText('ana saved')).toBeVisible();
	await ana.reload();
	await expect(ana.getByRole('navigation', { name: 'Main' }).first().getByRole('link', { name: 'Downloads' })).toBeVisible();

	await own.getByRole('button', { name: 'Reset two-factor sign-in for ana' }).click({ trial: true });
	own.once('dialog', (d) => d.accept());
	await own.getByRole('button', { name: 'Turn off ana' }).click();
	await expect(own.getByText('ana turned off')).toBeVisible();
	await ana.goto('/library');
	await expect(ana).toHaveURL(/\/login/);
	own.once('dialog', (d) => d.accept());
	await own.getByRole('button', { name: 'Turn on ana' }).click();
	await expect(own.getByText('ana turned on')).toBeVisible();
});

test('the owner sets a forgotten password, and ana switches account on a shared screen', async () => {
	const newPw = `${memberPw}-new`;
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Set a new password for ana' }).click();
	const dlg = own.getByRole('dialog', { name: 'New password for ana' });
	await dlg.getByLabel('New password').fill(newPw);
	await dlg.getByRole('button', { name: 'Set password' }).click();
	await expect(own.getByRole('status').filter({ hasText: 'Password set for ana' })).toBeVisible();
	await ana.goto('/library');
	await expect(ana).toHaveURL(/\/login/);
	// ana signed in here before: the page opens on her own step, password only.
	await expect(ana.getByTestId('signin-person')).toContainText('ana');
	await ana.getByLabel('Password', { exact: true }).fill(newPw);
	await ana.getByRole('button', { name: 'Sign in', exact: true }).click();
	await ana.getByRole('button', { name: 'Use a backup code' }).click();
	await ana.getByLabel('Backup code').fill(backup[1]);
	await ana.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(ana.getByRole('heading', { name: 'Two-step check' })).toHaveCount(0);
	await expect(ana).not.toHaveURL(/\/login/);

	// Switch account: signs out, and ana is one tap away on this device.
	await ana.getByRole('button', { name: 'Account and appearance' }).click();
	await ana.getByRole('menuitem', { name: 'Switch account' }).click();
	await expect(ana).toHaveURL(/\/login/);
	await expect(ana.getByTestId('signin-person')).toContainText('ana');
	await ana.getByRole('button', { name: 'Switch account' }).click();
	const people = ana.getByTestId('recent-people');
	await expect(people.getByRole('button', { name: /^ana/ })).toBeVisible();
	await ana.getByRole('button', { name: 'Edit list' }).click();
	await people.getByRole('button', { name: 'Forget ana on this device' }).click();
	await expect(people).toHaveCount(0);
	await expect(ana.getByLabel('Name')).toBeVisible();
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
	await expect(own.getByText('Jellyfin sign-in saved')).toBeVisible();

	const mum = await fresh(browser);
	await mum.goto('/login');
	await mum.getByRole('button', { name: 'Continue with Jellyfin' }).click();
	await mum.getByLabel('Name').fill(mediaUser.name);
	await mum.getByLabel('Password', { exact: true }).fill('not it');
	await mum.getByRole('button', { name: 'Sign in with Jellyfin' }).click();
	await expect(mum.getByRole('alert')).toContainText(/wrong name or password/i);
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
	await expect(mum.getByText('Password set', { exact: false })).toBeVisible();
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
	await expect(own.getByText('Emby sign-in saved')).toBeVisible();

	const gran = await fresh(browser);
	await gran.goto('/login');
	// With two server ways on, each is a tile of its own.
	await expect(gran.getByRole('button', { name: 'Jellyfin', exact: true })).toBeVisible();
	await gran.getByRole('button', { name: 'Emby', exact: true }).click();
	await gran.getByLabel('Name').fill(embyUser.name);
	// Mum's Jellyfin password is not an Emby password.
	await gran.getByLabel('Password', { exact: true }).fill(mediaUser.password);
	await gran.getByRole('button', { name: 'Sign in with Emby' }).click();
	await expect(gran.getByRole('alert')).toContainText(/wrong name or password/i);
	await gran.getByLabel('Password', { exact: true }).fill(embyUser.password);
	await gran.getByRole('button', { name: 'Sign in with Emby' }).click();
	await expect(gran).toHaveURL(/\/$/);
	await expect(gran.getByRole('button', { name: 'Account and appearance' })).toBeVisible();
});

test('the owner sets up single sign-on in Settings, and the sign-in page offers it', async ({ browser }) => {
	await own.goto('/settings/sign-in');
	const card = own.locator('section', { has: own.getByRole('heading', { name: 'Single sign-on' }) });
	// A stale local sign-in asks for the password first.
	const confirmBox = card.getByTestId('confirm-sign-in');
	if (await confirmBox.isVisible().catch(() => false)) {
		await confirmBox.getByLabel('HolyArrs password').fill(owner().password);
		await confirmBox.getByRole('button', { name: 'Confirm' }).click();
	}
	const form = card.getByTestId('sso-form');
	await expect(form).toBeVisible();
	await expect(card.getByTestId('sso-forward')).toContainText('set only in the environment');
	const issuer = `http://127.0.0.1:${jellyfinPort}/oidc`;
	await form.getByLabel('Issuer').fill(issuer);
	await form.getByRole('button', { name: 'Test' }).click();
	await expect(form.getByTestId('sso-tested')).toHaveText('OpenID Connect');
	await form.getByText('Show the single sign-on button on the sign-in page').click();
	await form.getByLabel('Client ID').fill('holyarrs');
	await form.getByLabel('Client secret').fill(`${owner().password}-client`);
	await expect(form.getByLabel('Callback address')).toHaveValue(/\/api\/v1\/auth\/oidc\/callback$/);
	// Saving needs the password with it.
	await form.getByLabel('Your HolyArrs password').fill('not it');
	await form.getByRole('button', { name: 'Save' }).click();
	await expect(form.getByRole('alert')).toContainText('password or code is wrong');
	await form.getByLabel('Your HolyArrs password').fill(owner().password);
	await form.getByRole('button', { name: 'Save' }).click();
	await expect(own.getByText('Single sign-on saved')).toBeVisible();
	await expect(form.getByLabel('Client secret')).toHaveValue('');
	// The secret typed before the wrong password was saved with the right one.
	await expect(form.getByText('A secret is saved; it is never shown.')).toBeVisible();

	const visitor = await fresh(browser);
	await visitor.goto('/login');
	await expect(visitor.getByRole('button', { name: /single sign-on/i })).toBeVisible();

	// Turn it off again so later tests see the sign-in page as before.
	await form.getByText('Show the single sign-on button on the sign-in page').click();
	await form.getByLabel('Your HolyArrs password').fill(owner().password);
	await form.getByRole('button', { name: 'Save' }).click();
	await expect(own.getByText('Single sign-on saved').first()).toBeVisible();
});

// The owner's fresh-password box, where a page asks for it first.
async function confirmIfAsked(where: Locator) {
	const box = where.getByTestId('confirm-sign-in');
	if (await box.isVisible().catch(() => false)) {
		await box.getByLabel('HolyArrs password').fill(owner().password);
		await box.getByRole('button', { name: 'Confirm' }).click();
	}
}

test('single sign-on links are undone: the owner unlinks a person, and the owner unlinks their own', async ({ browser }) => {
	await own.goto('/settings/sign-in');
	const card = own.locator('section', { has: own.getByRole('heading', { name: 'Single sign-on' }) });
	await confirmIfAsked(card);
	const form = card.getByTestId('sso-form');
	await form.getByLabel('Show the single sign-on button on the sign-in page').check();
	await form.getByLabel(/New people the provider signs in may join/).check();
	await form.getByTestId('sso-confirm-everyone').check();
	await form.getByLabel('Your HolyArrs password').fill(owner().password);
	await form.getByRole('button', { name: 'Save' }).click();
	await expect(own.getByText('Single sign-on saved').first()).toBeVisible();

	// Pat joins through the provider and has no HolyArrs password.
	const pat = await fresh(browser);
	await pat.goto('/login');
	await pat.getByRole('button', { name: /single sign-on/i }).click();
	await pat.getByLabel('Username').fill('pat');
	await pat.getByRole('button', { name: 'Sign in' }).click();
	await expect(pat).toHaveURL(/\/$/, { timeout: 15_000 });
	await expect(pat.getByRole('button', { name: 'Account and appearance' })).toBeVisible();

	// The owner unlinks Pat from Users; the browser asks twice, since it is Pat's only way in.
	await own.goto('/settings/users');
	const asked: string[] = [];
	const yes = (d: Dialog) => (asked.push(d.message()), d.accept());
	own.removeAllListeners('dialog'); // a once() above may still wait for a dialog that never came
	own.on('dialog', yes);
	await own.getByRole('button', { name: 'Unlink the single sign-on sign-in of pat' }).click();
	await expect(own.getByText('Sign-in unlinked for pat')).toBeVisible();
	own.off('dialog', yes);
	expect(asked).toHaveLength(2);
	expect(asked[1]).toContain('has no password and will be locked out');
	await expect(own.getByRole('button', { name: 'Unlink the single sign-on sign-in of pat' })).toHaveCount(0);
	// The device that single sign-on signed in is signed out.
	await pat.reload();
	await expect(pat).toHaveURL(/\/login/);

	// The owner links their own provider account from My account, then unlinks it there.
	await own.goto('/settings/account');
	await own.getByTestId('link-sign-in').click();
	await confirmIfAsked(own.locator('#linked-me'));
	await own.locator('#linked-me').getByRole('button', { name: 'Single sign-on' }).click();
	await own.getByLabel('Username').fill('boss');
	await own.getByRole('button', { name: 'Sign in' }).click();
	await expect(own.getByText('Single sign-on linked')).toBeVisible({ timeout: 15_000 });
	const mine = own.getByTestId('my-sso-links');
	await expect(mine).toContainText('as boss');
	await mine.getByRole('button', { name: /^Unlink Single sign-on at / }).click();
	await confirmIfAsked(mine);
	await mine.getByRole('button', { name: 'Unlink', exact: true }).click();
	await expect(own.getByText('Single sign-on unlinked', { exact: false })).toBeVisible();
	await expect(mine).toHaveCount(0);
	// The owner's password session is not one single sign-on made, so it stays.
	await own.reload();
	await expect(own).toHaveURL(/\/settings\/account/);

	await own.goto('/settings/sign-in');
	await confirmIfAsked(card);
	await form.getByLabel('Show the single sign-on button on the sign-in page').uncheck();
	await form.getByLabel(/New people the provider signs in may join/).uncheck();
	await form.getByLabel('Your HolyArrs password').fill(owner().password);
	await form.getByRole('button', { name: 'Save' }).click();
	await expect(own.getByText('Single sign-on saved').first()).toBeVisible();
});

test('sign in with Plex makes a member (fake Plex)', async ({ browser }) => {
	await own.goto('/settings/sign-in');
	const card = own.locator('section', { has: own.getByRole('heading', { name: 'Sign in with Plex' }) });
	await card.getByText('Show the Plex button on the sign-in page').click();
	await card.getByText('Type the server ID').click();
	await card.getByLabel('Server id people must share (optional)').fill('fake-server');
	await card.getByLabel('Who may join the first time').selectOption('everyone');
	await card.getByRole('button', { name: 'Save' }).click();
	await own.getByRole('button', { name: 'Let everyone join' }).click();
	await expect(own.getByText('Plex sign-in saved')).toBeVisible();

	const kid = await fresh(browser);
	await kid.goto('/login');
	const popup = kid.waitForEvent('popup');
	await kid.getByRole('button', { name: 'Plex', exact: true }).click();
	await (await popup).waitForLoadState().catch(() => {});
	await expect(kid).toHaveURL(/\/$/, { timeout: 15_000 });
	await expect(kid.getByRole('button', { name: 'Account and appearance' })).toBeVisible();
	// No HolyArrs password: the Plex link shows, and says a password comes first to change it.
	await kid.goto('/settings/account');
	await expect(kid.getByTestId('my-plex-link')).toContainText('Plex, as plexkid');
	await expect(kid.getByTestId('plex-needs-password')).toBeVisible();
	await expect(kid.getByRole('button', { name: 'Unlink Plex' })).toHaveCount(0);
	await expect(kid.getByTestId('link-sign-in')).toHaveCount(0);

	await own.goto('/settings/users');
	await expect(own.getByText('Jellyfin: Mum')).toBeVisible();
	await expect(own.getByText('Emby: Gran')).toBeVisible();
	await expect(own.getByText('Plex: plexkid')).toBeVisible();
});

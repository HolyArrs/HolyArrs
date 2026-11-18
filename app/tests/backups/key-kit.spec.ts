import { expect, test, type Page } from '@playwright/test';
import { copyFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { api } from '../helpers/api.ts';
import { dataDir } from '../helpers/data-folder.ts';
import { setUpFresh, signIn } from '../helpers/helpers.ts';

// The key kit reminder: nothing on a fresh install, a calm note once a
// secret is saved, a warning (with the Settings dot) once secrets and
// backups have gone a week without a kit, and gone at once after the
// download. Own fresh app: npx playwright test -c setups/key-kit.config.ts
test.describe.configure({ mode: 'serial' });

const tmp = dataDir('keykit');
const config = join(tmp, 'config');
const pw = `e2e-${Math.random().toString(36).slice(2)}-pw`;
const owner = () => ({ name: 'luis', password: pw });
const DAY = 24 * 60 * 60 * 1000;

let page: Page;

test.beforeAll(async ({ browser }) => {
	test.setTimeout(240_000);
	page = await browser.newPage();
	await setUpFresh(page, owner(), join(tmp, 'movies'));
});

test.afterAll(async () => {
	await page.context().close();
});

type Check = { key: string; state: string; message?: string };

async function keyCheck(): Promise<Check | undefined> {
	const call = await api(page);
	expect((await call('POST', '/health/checks/run?only=backups')).ok()).toBe(true);
	const r = (await (await call('GET', '/health/checks')).json()) as { checks: Check[] };
	return r.checks.find((c) => c.key === 'backups:key');
}

async function settingsItems(): Promise<string> {
	const call = await api(page);
	return JSON.stringify(await (await call('GET', '/settings/status')).json());
}

test('a fresh install does not ask for the key kit', async () => {
	const c = await keyCheck();
	expect(c?.state ?? 'ok').toBe('ok');
	expect(c?.message ?? '').toBe('');
	expect(await settingsItems()).not.toContain('key kit');
	await page.goto('/settings/backups');
	await expect(page.getByRole('button', { name: 'Download key kit' })).toBeVisible();
	await expect(page.getByTestId('key-kit-note')).toHaveCount(0);
});

test('the first saved secret brings a calm recommendation', async () => {
	const call = await api(page);
	const add = await call('POST', '/alerts', { name: 'Lab hook', kind: 'webhook', url: 'https://hooks.example.com/lab', moments: ['grab'] });
	expect(add.status(), await add.text()).toBe(201);
	const c = await keyCheck();
	expect(c?.state).toBe('ok');
	expect(c?.message).toBe('Recommended: download your key kit');
	expect(await settingsItems()).not.toContain('key kit');
	await page.goto('/settings/backups');
	const note = page.getByTestId('key-kit-note');
	await expect(note).toHaveAttribute('data-level', 'recommend');
	await expect(note).toHaveText('Recommended: download your key kit.');
	await expect(note).not.toHaveClass(/warn/);
	await page.goto('/health');
	await expect(page.getByTestId('key-kit-note')).toHaveText('Recommended: download your key kit');
});

test('a week of secrets and backups without a kit is a warning, cleared by the download', async () => {
	const call = await api(page);
	expect((await call('POST', '/backups')).ok()).toBe(true);
	// The saved secrets and the oldest backup go eight days back; the
	// newest backup stays fresh, as with daily scheduled backups.
	const old = new Date(Date.now() - 8 * DAY);
	const db = new DatabaseSync(join(config, 'holyarrs.db'));
	db.exec('PRAGMA busy_timeout = 5000');
	db.prepare("UPDATE secrets SET created_at = ? WHERE kind != 'check'").run(old.toISOString().replace(/\.(\d{3})Z$/, '.$1000000Z'));
	db.close();
	const dir = join(config, 'backups');
	const one = readdirSync(dir).find((n) => /^holyarrs-backup-manual-.*\.zip$/.test(n));
	expect(one).toBeTruthy();
	copyFileSync(join(dir, one!), join(dir, `holyarrs-backup-manual-${old.toISOString().replace(/[-:]/g, '')}.zip`));
	// Two runs: the menu dot follows a warning once the flap guard confirms it.
	await keyCheck();
	const c = await keyCheck();
	expect(c?.state).toBe('warning');
	expect(c?.message).toBe("Download your key kit: Backups don't include the master key");
	expect(await settingsItems()).toContain('key kit');
	await page.goto('/settings/backups');
	await expect(page.getByTestId('key-kit-note')).toHaveAttribute('data-level', 'warn');
	await expect(page.getByTestId('key-kit-note')).toHaveClass(/warn/);

	const got = page.waitForEvent('download');
	await page.getByRole('button', { name: 'Download key kit' }).click();
	expect((await got).suggestedFilename()).toMatch(/\.key$/);
	await expect(page.getByText('Key kit downloaded')).toBeVisible();
	await expect(page.getByTestId('key-kit-note')).toHaveCount(0);
	const after = await keyCheck();
	expect(after?.state).toBe('ok');
	expect(after?.message ?? '').toBe('');
	expect(await settingsItems()).not.toContain('key kit');
});

test('signing in again keeps it cleared', async ({ browser }) => {
	const p = await browser.newPage();
	await signIn(p, owner());
	await p.goto('/settings/backups');
	await expect(p.getByRole('button', { name: 'Download key kit' })).toBeVisible();
	await expect(p.getByTestId('key-kit-note')).toHaveCount(0);
	await p.close();
});

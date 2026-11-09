import { describe, expect, it } from 'vitest';
import { fromServer, groupState, needsAttention, pageState, stateWords, type ServerStatus } from './settings-status.ts';
import { groups, grouped, sectionFor, sections, visible } from '../core/settings.ts';
import { searchSettings, settingIndex } from '../../routes/settings/(home)/settings-search.ts';

const answer = (over: Partial<ServerStatus> = {}): ServerStatus => ({ items: [], unset: {}, known: [], metadata_up: true, ...over });

describe('fromServer', () => {
	it('turns the server answer into pages, groups and severities', () => {
		const a = fromServer(
			answer({
				items: [{ group: 'copies', page: '/settings/downloaders', text: 'qBit is not answering', severity: 'error' }],
				unset: { library: ['/settings/library-folders'] },
				known: ['/settings/sources'],
				metadata_up: false
			})
		);
		expect(a.items).toEqual([{ group: 'copies', href: '/settings/downloaders', text: 'qBit is not answering', severity: 'error' }]);
		expect(a.unset).toEqual({ library: ['/settings/library-folders'] });
		expect(a.known.has('/settings/sources')).toBe(true);
		expect(a.metadataUp).toBe(false);
	});

	it('copes with missing lists', () => {
		const a = fromServer({} as ServerStatus);
		expect(a.items).toEqual([]);
		expect(a.unset).toEqual({});
		expect(a.known.size).toBe(0);
	});

	it('an empty answer is not known anywhere', () => {
		expect(fromServer(null).known.size).toBe(0);
	});
});

describe('groupState and pageState', () => {
	it('says All set when every known page is fine', () => {
		const a = fromServer(answer({ known: ['/settings/sources', '/settings/downloaders'] }));
		expect(stateWords(groupState(a, ['/settings/sources', '/settings/downloaders']))).toBe('All set');
	});

	it('counts what needs attention per group and per page', () => {
		const a = fromServer(
			answer({
				items: [
					{ group: 'copies', page: '/settings/downloaders', text: 'qBit is not answering', severity: 'error' },
					{ group: 'copies', page: '/settings/downloaders', text: 'Downloader check failed', severity: 'error' }
				],
				known: ['/settings/sources']
			})
		);
		expect(groupState(a, ['/settings/sources', '/settings/downloaders'])).toEqual({ state: 'attention', count: 2 });
		expect(pageState(a, '/settings/sources').state).toBe('ok');
		expect(stateWords(pageState(a, '/settings/downloaders'))).toBe('2 need attention');
	});

	it('one item reads in the singular', () => {
		const a = fromServer(answer({ items: [{ group: 'maintenance', page: '/settings/backups', text: 'No backup yet', severity: 'warning' }] }));
		expect(stateWords(groupState(a, ['/settings/backups']))).toBe('1 needs attention');
	});

	it('says Not set up for pages the server lists as unset', () => {
		const a = fromServer(answer({ unset: { library: ['/settings/library-folders'], copies: ['/settings/sources', '/settings/downloaders'] } }));
		expect(stateWords(groupState(a, ['/settings/library-folders']))).toBe('Not set up');
		expect(pageState(a, '/settings/downloaders').state).toBe('unset');
	});

	it('says nothing about pages it knows nothing of', () => {
		const a = fromServer(answer());
		expect(stateWords(groupState(a, ['/settings/sign-in']))).toBe('');
		expect(stateWords({ state: 'attention', count: 2 })).toBe('2 need attention');
	});
});

describe('settings groups', () => {
	it('puts every page in a known group and keeps My account open to everyone', () => {
		const ids = new Set(groups.map((g) => g.id));
		for (const s of sections) expect(ids.has(s.group)).toBe(true);
		const none = visible(sections, () => false).map((s) => s.href);
		expect(none).toEqual(['/settings/appearance', '/settings/account']);
	});

	it('makes five home groups with a three-word line each, Danger zone last in Server, Schedules in Downloads', () => {
		const home = groups.filter((g) => g.id !== 'account');
		expect(home.map((g) => g.label)).toEqual(['Library', 'Downloads', 'People', 'Linked apps', 'Server']);
		for (const g of home) expect(g.line.split(' ')).toHaveLength(3);
		const m = grouped(sections).find((g) => g.group.id === 'system')!;
		expect(m.items.at(-1)?.href).toBe('/settings/danger');
		expect(grouped(sections).find((g) => g.group.id === 'downloads')!.items.map((i) => i.href)).toContain('/settings/schedules');
	});

	it('finds the page a nested path belongs to', () => {
		expect(sectionFor(sections, '/settings/sources/3')?.label).toBe('Sources');
	});
});

describe('searchSettings', () => {
	const all = () => true;
	it('finds one setting by its label, and labels that start with the words come first', () => {
		const r = searchSettings('keep deleted', all);
		expect(r[0]).toMatchObject({ href: '/settings/naming', anchor: 'holding-days' });
	});

	it('finds a setting by another word for it', () => {
		expect(searchSettings('dark mode', all)[0].anchor).toBe('theme');
		expect(searchSettings('indexer', all)[0].href).toBe('/settings/sources');
		expect(searchSettings('2fa', all).map((r) => r.anchor)).toContain('twofactor-me');
		expect(searchSettings('telegram', all)[0].href).toBe('/settings/alerts');
		for (const app of ['ntfy', 'gotify', 'pushover']) expect(searchSettings(app, all)[0].href).toBe('/settings/alerts');
	});

	it('every word must match, and pages a person may not open are left out', () => {
		expect(searchSettings('season pack zebra', all)).toEqual([]);
		const r = searchSettings('password', (h) => h === '/settings/account');
		expect(r.every((x) => x.href === '/settings/account')).toBe(true);
		expect(r.length).toBeGreaterThan(0);
	});

	it('matches the group name too', () => {
		const g = searchSettings('downloads sources', all, (h) => (sections.find((s) => s.href === h)?.group === 'downloads' ? 'Downloads' : ''));
		expect(g[0].href).toBe('/settings/sources');
	});

	it('every entry points at a page that exists', () => {
		const hrefs = new Set(sections.map((s) => s.href));
		for (const e of settingIndex) expect(hrefs.has(e.href), e.href).toBe(true);
	});
});

describe('every setting a search finds is on its page', () => {
	it('each anchor is an id in that page (or My account)', async () => {
		const { existsSync, readFileSync, readdirSync } = await import('node:fs');
		const { join, dirname } = await import('node:path');
		const root = join(process.cwd(), 'src');
		const missing: string[] = [];
		for (const e of settingIndex) {
			if (!e.anchor) continue;
			const file = e.href === '/settings/account' ? join(root, 'lib', 'accounts', 'Account.svelte') : join(root, 'routes', ...e.href.split('/').filter(Boolean), '+page.svelte');
			// A page's own parts (other .svelte files beside it) count too; My account is split across its parts in lib/accounts.
			const parts = readdirSync(dirname(file)).filter((f) => f.endsWith('.svelte') && f !== (e.href === '/settings/account' ? 'Account.svelte' : '+page.svelte'));
			// Parts the page takes from a module's index.ts count too.
			const page = readFileSync(file, 'utf8');
			const fromModules = [...page.matchAll(/import \{([^}]*)\} from '#lib\/([a-z]+)\/index\.ts'/g)].flatMap(([, names, m]) =>
				names.split(',').map((n) => join(root, 'lib', m, n.trim() + '.svelte'))
			);
			const text = [file, ...parts.map((f) => join(dirname(file), f)), ...fromModules.filter((f) => existsSync(f))].map((f) => readFileSync(f, 'utf8')).join('\n');
			const way = e.anchor.match(/^(plex|jellyfin|emby)-card$/);
			const found = text.includes(`id="${e.anchor}"`) || text.includes(`id: '${e.anchor}'`) || (way && text.includes('id="{w}-card"'));
			if (!found) missing.push(`${e.href}#${e.anchor}`);
		}
		expect(missing).toEqual([]);
	});
});

describe('needsAttention', () => {
	it('flags the side menu only when something needs a fix', () => {
		expect(needsAttention(null)).toBe(false);
		expect(needsAttention(fromServer(null))).toBe(false);
		expect(needsAttention(fromServer({ items: [], unset: { copies: ['/settings/sources'] }, known: [], metadata_up: true }))).toBe(false);
		expect(needsAttention(fromServer({ items: [{ group: 'copies', page: '/settings/sources', text: 'The Source A is close to its search limit (8 of 10)', severity: 'warning' }], unset: {}, known: [], metadata_up: true }))).toBe(true);
	});
});

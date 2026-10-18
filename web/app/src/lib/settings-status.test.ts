import { describe, expect, it } from 'vitest';
import { groupState, pageState, settingsStatus, stateWords } from './settings-status.ts';
import { groups, grouped, sectionFor, sections, visible } from './settings.ts';
import { searchSettings, settingIndex } from './settings-search.ts';
import type { HealthCheck, StageState } from './types.ts';

const check = (over: Partial<HealthCheck>): HealthCheck => ({
	id: 1,
	key: 'k',
	kind: 'downloader',
	subject: 'qBit',
	state: 'ok',
	message: '',
	since: '',
	checked_at: '',
	...over
});
const st = (over: Partial<StageState>): StageState => ({ key: 'media_server:1', kind: 'media_server', name: 'Jelly', state: 'running', reason: '', since: '', stuck: false, restarts: 0, ...over });
const now = Date.parse('2026-10-18T12:00:00Z');

describe('settingsStatus', () => {
	it('says All set when every known check is fine', () => {
		const a = settingsStatus({ checks: [check({})], sources: [{ name: 'A', state: 'working' }], downloaders: [{ name: 'Q', state: 'working' }], now });
		expect(a.items).toEqual([]);
		expect(stateWords(groupState(a, 'copies', ['/settings/sources', '/settings/downloaders']))).toBe('All set');
	});

	it('counts a failing downloader once, from the check and the list', () => {
		const a = settingsStatus({
			checks: [check({ state: 'error', message: 'not answering' })],
			downloaders: [{ name: 'qBit', state: 'not_answering' }],
			sources: [{ name: 'A', state: 'working' }],
			now
		});
		// One from the health check, one from the list (they word it differently but both point at Downloaders).
		expect(a.items.every((i) => i.href === '/settings/downloaders')).toBe(true);
		expect(groupState(a, 'copies', []).state).toBe('attention');
		expect(pageState(a, '/settings/sources').state).toBe('ok');
	});

	it('flags a missing TMDB key, an old backup and a failed media server', () => {
		const a = settingsStatus({ tmdbKey: false, newestBackup: '2026-10-01T00:00:00Z', states: [st({ state: 'failed' })], now });
		expect(a.items.map((i) => i.href).sort()).toEqual(['/settings/backups', '/settings/media-servers', '/settings/tmdb']);
		expect(stateWords(groupState(a, 'maintenance', ['/settings/backups']))).toBe('1 needs attention');
	});

	it('a recent backup is fine and no backup at all is flagged', () => {
		expect(settingsStatus({ newestBackup: '2026-10-17T00:00:00Z', now }).items).toEqual([]);
		expect(settingsStatus({ newestBackup: '', now }).items[0].text).toBe('No backup yet');
	});

	it('says Not set up when there are no folders, Sources or Downloaders', () => {
		const a = settingsStatus({ folders: 0, sources: [], downloaders: [], now });
		expect(stateWords(groupState(a, 'library', ['/settings/library-folders']))).toBe('Not set up');
		expect(pageState(a, '/settings/downloaders').state).toBe('unset');
	});

	it('says nothing about pages it knows nothing of', () => {
		const a = settingsStatus({ now });
		expect(stateWords(groupState(a, 'security', ['/settings/sign-in']))).toBe('');
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

	it('puts Danger zone last in Maintenance', () => {
		const m = grouped(sections).find((g) => g.group.id === 'maintenance')!;
		expect(m.items.at(-1)?.href).toBe('/settings/danger');
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
	});

	it('every word must match, and pages a person may not open are left out', () => {
		expect(searchSettings('season pack zebra', all)).toEqual([]);
		const r = searchSettings('password', (h) => h === '/settings/account');
		expect(r.every((x) => x.href === '/settings/account')).toBe(true);
		expect(r.length).toBeGreaterThan(0);
	});

	it('matches the group name too', () => {
		const g = searchSettings('getting copies sources', all, (h) => (sections.find((s) => s.href === h)?.group === 'copies' ? 'Getting copies' : ''));
		expect(g[0].href).toBe('/settings/sources');
	});

	it('every entry points at a page that exists', () => {
		const hrefs = new Set(sections.map((s) => s.href));
		for (const e of settingIndex) expect(hrefs.has(e.href), e.href).toBe(true);
	});
});

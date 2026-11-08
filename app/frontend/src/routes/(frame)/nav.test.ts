import { buildNav, currentHref, destinations, filterDestinations, homeHref, isCurrent, readSidebarPref, shellMode, writeSidebarPref, type NavInput } from './nav.ts';
import { describe, expect, it } from 'vitest';

const owner: NavInput = { allowed: () => true, off: () => false, kinds: ['movie', 'show', 'artist'], waitingAsks: 3, decisions: 1, unread: 0 };
const memberPerms = new Set(['wishlist']);
const member: NavInput = { ...owner, allowed: (n) => memberPerms.has(n) };

const labels = (input: NavInput) => buildNav(input).map((s) => [s.label, s.items.map((i) => i.label)]);

describe('buildNav', () => {
	it('flags Settings only for people who change settings', () => {
		const settings = (i: NavInput) => buildNav(i).flatMap((s) => s.items).find((x) => x.href === '/settings');
		expect(settings({ ...owner, settingsAttention: true })?.dot).toBe(true);
		expect(settings(owner)?.dot).toBeUndefined();
		const usersOnly = new Set(['staff', 'users', 'manage']);
		expect(settings({ ...owner, settingsAttention: true, allowed: (n) => usersOnly.has(n) })?.dot).toBeUndefined();
		expect(settings({ ...member, settingsAttention: true })).toBeUndefined();
	});
	it('gives owners every section, with Requests last under Your stuff', () => {
		expect(labels(owner)).toEqual([
			[null, ['Home', 'Explore', 'New & Popular']],
			['Library', ['Movies', 'TV shows', 'Music']],
			['Your stuff', ['Calendar', 'Requests']],
			['Manage', ['Downloads', 'History', 'Status', 'Settings']]
		]);
	});
	it('hides Music from a Kid, because music has no age ratings', () => {
		expect(labels({ ...member, kid: true })[1]).toEqual(['Library', ['Movies', 'TV shows']]);
	});
	it('hides Manage from members but keeps Home, and lets them watch their Downloads', () => {
		expect(labels(member)).toEqual([
			[null, ['Home', 'Explore', 'New & Popular']],
			['Library', ['Movies', 'TV shows', 'Music']],
			['Your stuff', ['Downloads', 'Calendar', 'Requests']]
		]);
	});
	it('shows each Manage item by its own permission', () => {
		const approverOnly = new Set(['approve', 'manage', 'wishlist']);
		const nav = buildNav({ ...owner, allowed: (n) => approverOnly.has(n) });
		expect(nav.find((s) => s.id === 'manage')).toBeUndefined();
		expect(nav.find((s) => s.id === 'yours')?.items.map((i) => i.label)).toEqual(['Downloads', 'Calendar', 'Requests']);
	});
	it('shows Settings to someone who only manages users', () => {
		const usersOnly = new Set(['staff']);
		const nav = buildNav({ ...owner, allowed: (n) => usersOnly.has(n) });
		expect(nav.find((s) => s.id === 'manage')?.items.map((i) => i.label)).toEqual(['Settings']);
	});
	it('drops places whose part is off', () => {
		const nav = buildNav({ ...owner, kinds: ['show'], off: (h) => h === '/requests' });
		expect(nav.find((s) => s.id === 'library')?.items.map((i) => i.label)).toEqual(['TV shows']);
		expect(nav.find((s) => s.id === 'yours')?.items.map((i) => i.label)).not.toContain('Requests');
	});
	it('carries the badge counts', () => {
		const nav = buildNav(owner);
		const manage = nav.find((s) => s.id === 'manage')!;
		expect(nav.find((s) => s.id === 'yours')!.items.find((i) => i.label === 'Requests')?.badge).toBe(3);
		expect(manage.items.find((i) => i.label === 'Downloads')?.badge).toBe(1);
	});
});

describe('current page', () => {
	const q = (s = '') => new URLSearchParams(s);
	it('matches kinds by query and paths by prefix', () => {
		expect(isCurrent({ href: '/library?kind=movie' }, '/library', q('kind=movie'))).toBe(true);
		expect(isCurrent({ href: '/library?kind=movie' }, '/library', q('kind=show'))).toBe(false);
		expect(isCurrent({ href: '/' }, '/library', q())).toBe(false);
		expect(isCurrent({ href: '/downloads', also: ['/downloads/decide'] }, '/downloads/decide', q())).toBe(true);
		expect(isCurrent({ href: '/settings' }, '/settings/naming', q())).toBe(true);
	});
	it('selects one row: the most specific', () => {
		const items = [{ href: '/downloads' }, { href: '/downloads/history' }];
		expect(currentHref(items, '/downloads/history', q())).toBe('/downloads/history');
		expect(currentHref(items, '/calendar', q())).toBeNull();
	});
});

describe('palette', () => {
	const list = destinations(buildNav(owner), [{ href: '/account', label: 'Account', icon: 'user', section: 'You' }]);
	it('lists each place once', () => {
		expect(list.filter((d) => d.href === '/requests')).toHaveLength(1);
		expect(list.at(-1)?.label).toBe('Account');
	});
	it('ranks starts-with before contains', () => {
		expect(filterDestinations(list, 'mo').map((d) => d.label).slice(0, 2)).toEqual(['Movies']);
		expect(filterDestinations(list, 'shows').map((d) => d.label)).toEqual(['TV shows']);
		expect(filterDestinations(list, 'zzz')).toEqual([]);
		expect(filterDestinations(list, ' ')).toHaveLength(list.length);
	});
});

describe('shell helpers', () => {
	it('reads and writes the sidebar choice safely', () => {
		const store = new Map<string, string>();
		const s = { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v) };
		expect(readSidebarPref(s)).toBe('expanded');
		writeSidebarPref(s, 'collapsed');
		expect(readSidebarPref(s)).toBe('collapsed');
		const refusing = {
			getItem: (): string | null => {
				throw Error('denied');
			},
			setItem: (): void => {
				throw Error('denied');
			}
		};
		expect(readSidebarPref(refusing)).toBe('expanded');
		expect(() => writeSidebarPref(refusing, 'collapsed')).not.toThrow();
		expect(readSidebarPref(undefined)).toBe('expanded');
	});
	it('maps widths to shell modes', () => {
		expect(shellMode(390)).toBe('phone');
		expect(shellMode(834)).toBe('tablet');
		expect(shellMode(1024)).toBe('desktop');
	});
	it('sends the logo Home for everyone', () => {
		expect(homeHref()).toBe('/');
	});
});

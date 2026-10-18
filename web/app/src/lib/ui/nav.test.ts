import { describe, expect, it } from 'vitest';
import { buildNav, currentHref, destinations, filterDestinations, homeHref, isCurrent, readSidebarPref, shellMode, writeSidebarPref, type NavInput } from './nav.ts';

const owner: NavInput = { allowed: () => true, off: () => false, kinds: ['movie', 'show', 'artist'], approvals: 3, decisions: 1, unread: 0 };
const memberPerms = new Set(['wishlist']);
const member: NavInput = { ...owner, allowed: (n) => memberPerms.has(n) };

const labels = (input: NavInput) => buildNav(input).map((s) => [s.label, s.items.map((i) => i.label)]);

describe('buildNav', () => {
	it('gives owners every section, with Approvals under Manage', () => {
		expect(labels(owner)).toEqual([
			[null, ['Home', 'Explore']],
			['Library', ['Movies', 'TV shows', 'Music', 'Movie series']],
			['Your stuff', ['Calendar']],
			['Manage', ['Downloads', 'Approvals', 'History', 'Library tools', 'Settings']]
		]);
	});
	it('hides Manage and Home from members', () => {
		expect(labels(member)).toEqual([
			[null, ['Explore']],
			['Library', ['Movies', 'TV shows', 'Music', 'Movie series']],
			['Your stuff', ['Wishlist']]
		]);
	});
	it('shows each Manage item by its own permission', () => {
		const approverOnly = new Set(['approve', 'manage', 'wishlist']);
		const nav = buildNav({ ...owner, allowed: (n) => approverOnly.has(n) });
		expect(nav.find((s) => s.id === 'manage')?.items.map((i) => i.label)).toEqual(['Approvals']);
	});
	it('drops places whose part is off', () => {
		const nav = buildNav({ ...owner, kinds: ['show'], off: (h) => h === '/wishlist' });
		expect(nav.find((s) => s.id === 'library')?.items.map((i) => i.label)).toEqual(['TV shows']);
		expect(nav.find((s) => s.id === 'manage')?.items.map((i) => i.label)).not.toContain('Approvals');
	});
	it('carries the badge counts', () => {
		const manage = buildNav(owner).find((s) => s.id === 'manage')!;
		expect(manage.items.find((i) => i.label === 'Approvals')?.badge).toBe(3);
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
		expect(list.filter((d) => d.href === '/wishlist')).toHaveLength(1);
		expect(list.at(-1)?.label).toBe('Account');
	});
	it('ranks starts-with before contains', () => {
		expect(filterDestinations(list, 'mo').map((d) => d.label).slice(0, 2)).toEqual(['Movies', 'Movie series']);
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
	it('sends the logo Home only when Home is allowed', () => {
		expect(homeHref(true)).toBe('/');
		expect(homeHref(false)).toBe('/library');
	});
});

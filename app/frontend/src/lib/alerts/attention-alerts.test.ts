import { describe, expect, it } from 'vitest';
import { alertFixes, pinned, type PinnedAlert } from './attention-alerts.ts';

const pages = [{ href: '/settings/backups', label: 'Backups' }, { href: '/settings/library-folders', label: 'Library folders' }];

describe('pinned', () => {
	it('drops the page prefix and splits title from the next step', () => {
		const [a] = pinned([{ text: "Backups: Download your key kit: Backups don't include the master key", href: '/settings/backups', group: 'system', severity: 'warning' }], pages);
		expect(a).toMatchObject({ title: 'Download your key kit', body: "Backups don't include the master key", tone: 'warn', label: 'Backups' });
	});

	it('keeps a one-part text as the title and maps errors to fail', () => {
		const [a] = pinned([{ text: 'Library folders: /media is not reachable.', href: '/settings/library-folders', group: 'library', severity: 'error' }], pages);
		expect(a).toMatchObject({ title: '/media is not reachable', body: undefined, tone: 'fail', label: 'Library folders' });
	});

	it('falls back to Settings for an unknown page', () => {
		const [a] = pinned([{ text: 'Something to check', href: '/settings/new', group: 'x', severity: 'info' }], pages);
		expect(a.label).toBe('Settings');
		expect(a.tone).toBe('info');
	});

	it('keeps a brand name that starts lower-case, like qBittorrent', () => {
		const [a] = pinned([{ text: 'qBittorrent is not reachable', href: '/settings/new', group: 'x', severity: 'warning' }], pages);
		expect(a.title).toBe('qBittorrent is not reachable');
	});
});

describe('alertFixes', () => {
	const update: PinnedAlert = { key: 'update', title: 'Update available', tone: 'info', href: '/health?tab=updates', label: 'Update' };
	it.each(['A newer HolyArrs is out', 'HolyArrs 1.2 is out'])('counts the update once: %s', (text) => {
		const items = [{ text, href: '/settings/server', group: 'system', severity: 'warning' as const }];
		expect(alertFixes(items, pages, [update])).toEqual([update]);
		expect(alertFixes(items, pages)).toHaveLength(1);
	});
	it('keeps other warnings and includes decision and clear rows', () => {
		const decision = { ...update, key: 'decision|1' };
		const clear = { ...update, key: 'clear' };
		const fixes = alertFixes([{ text: 'Folder unavailable', href: '/settings/library-folders', group: 'library', severity: 'error' }], pages, [update], [decision, clear]);
		expect(fixes.map((f) => f.key)).toEqual(['update', '/settings/library-folders|Folder unavailable', 'decision|1', 'clear']);
	});
});

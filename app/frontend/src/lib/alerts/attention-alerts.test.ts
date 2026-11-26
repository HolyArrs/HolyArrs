import { describe, expect, it } from 'vitest';
import { pinned, pinnedSettings } from './attention-alerts.ts';
import type { Attention } from './settings-status.ts';

const pages = [{ href: '/settings/backups', label: 'Backups' }, { href: '/settings/library-folders', label: 'Library folders' }];

const updateItem: Attention = { text: 'A newer HolyArrs is out', href: '/health', group: 'system', severity: 'warning' };
const otherItem: Attention = { text: "Backups: Download your key kit: Backups don't include the master key", href: '/settings/backups', group: 'system', severity: 'warning' };

describe('pinnedSettings', () => {
	it('drops the update warning when the Update row says it', () => {
		expect(pinnedSettings([updateItem, otherItem], true)).toEqual([otherItem]);
	});

	it('drops the versioned wording too ("HolyArrs 1.2 is out")', () => {
		const versioned = { ...updateItem, text: 'HolyArrs 1.2 is out' };
		expect(pinnedSettings([versioned, otherItem], true)).toEqual([otherItem]);
	});

	it('keeps the update warning when no Update row is shown', () => {
		expect(pinnedSettings([updateItem, otherItem], false)).toEqual([updateItem, otherItem]);
	});
});

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

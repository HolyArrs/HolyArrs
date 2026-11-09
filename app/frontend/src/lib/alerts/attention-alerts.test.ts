import { describe, expect, it } from 'vitest';
import { pinned } from './attention-alerts.ts';

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
});

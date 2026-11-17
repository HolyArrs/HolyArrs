import { describe, expect, it } from 'vitest';
import { dotReasons } from './dot-reasons.ts';
import type { Attention } from './settings-status.ts';

const kit: Attention = { text: "Backups: Download your key kit: Backups don't include the master key", href: '/settings/backups', group: 'maintenance', severity: 'warning' };
const none = { settings: [] as Attention[], decisions: 0, waiting: 0, unseen: 0, manager: true };

describe('dotReasons', () => {
	it('has no line when nothing lights the dot', () => {
		expect(dotReasons(none)).toEqual([]);
	});

	it('names each Settings item with a link to its page, worst first', () => {
		const bad: Attention = { text: 'Library folders: /media cannot be reached', href: '/settings/library-folders', group: 'library', severity: 'error' };
		const got = dotReasons({ ...none, settings: [kit, bad] });
		expect(got.map((r) => [r.title, r.href, r.tone])).toEqual([
			['/media cannot be reached', '/settings/library-folders', 'fail'],
			['Download your key kit', '/settings/backups', 'warn']
		]);
	});

	it('adds decisions, waiting requests and new alerts, each once', () => {
		const got = dotReasons({ ...none, decisions: 2, waiting: 3, unseen: 1 });
		expect(got.map((r) => r.key)).toEqual(['decisions', 'waiting', 'alerts']);
		expect(got[0].href).toBe('/downloads/decide');
		expect(got[1].href).toBe('/requests');
		expect(got[2].href).toBeUndefined();
	});

	it('shows only new alerts to someone who does not manage', () => {
		const got = dotReasons({ settings: [kit], decisions: 2, waiting: 3, unseen: 1, manager: false });
		expect(got.map((r) => r.key)).toEqual(['alerts']);
	});
});

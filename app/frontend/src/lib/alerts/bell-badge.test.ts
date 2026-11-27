import { describe, expect, it } from 'vitest';
import { alertFixes, alertItems, bellBadge, type PinnedAlert } from './attention-alerts.ts';
import type { Attention, StatusAnswer } from './settings-status.ts';

const pages = [{ href: '/settings/this-app', label: 'This app' }, { href: '/settings/backups', label: 'Backups' }];

const answer = (items: Attention[]): StatusAnswer => ({ items, unset: {}, known: new Set(), metadataUp: true });

const update: PinnedAlert = { key: 'update', title: 'Update available', tone: 'info', href: '/health?tab=updates', label: 'Update' };
const clear: PinnedAlert = { key: 'clear_waiting', title: '1 ready to clear', tone: 'warn', href: '/downloads', label: 'Downloads' };
const decision: PinnedAlert = { key: 'decision|1', title: 'Pick a release', tone: 'warn', href: '/downloads', label: 'Downloads' };
const updateNote: Attention = { text: 'A newer HolyArrs is out', href: '/settings/this-app', group: 'general', severity: 'info' };
const backupNote: Attention = { text: 'Backups: No backup in the last 7 days', href: '/settings/backups', group: 'maintenance', severity: 'warning' };

// The bell badge and the panel take their rows from the same two functions, so the count can only agree.
const rows = (a: StatusAnswer | null, maySee: boolean, before: PinnedAlert[] = [], after: PinnedAlert[] = []) => alertFixes(alertItems(a, maySee), pages, before, after);

describe('alertItems', () => {
	it('keeps a note, which the panel lists', () => {
		expect(alertItems(answer([updateNote]), true)).toHaveLength(1);
	});
	it('lists nothing without the Settings right', () => {
		expect(alertItems(answer([backupNote]), false)).toEqual([]);
		expect(alertItems(null, true)).toEqual([]);
	});
});

describe('the bell badge', () => {
	it('counts an available update once when the pinned row is there', () => {
		const fixes = rows(answer([updateNote]), true, [update]);
		expect(fixes).toEqual([update]);
		expect(bellBadge(fixes, 0)).toBe(1);
	});
	it('counts the Clear now row and the decisions the panel shows', () => {
		const fixes = rows(answer([]), true, [update], [decision, clear]);
		expect(bellBadge(fixes, 0)).toBe(3);
	});
	it('adds unread recent alerts to the pinned rows', () => {
		expect(bellBadge(rows(answer([backupNote]), true, [update]), 2)).toBe(4);
	});
	it('adds up to the rows the panel lists, for every mix of rows', () => {
		const mixes: [Attention[], PinnedAlert[], PinnedAlert[]][] = [
			[[updateNote], [update], []],
			[[updateNote], [], []],
			[[updateNote, backupNote], [update], [decision, clear]],
			[[backupNote], [], [clear]],
			[[], [update], [decision]]
		];
		for (const [items, before, after] of mixes) {
			const a = answer(items);
			expect(rows(a, true, before, after)).toEqual(alertFixes(alertItems(a, true), pages, before, after));
		}
	});
	it('shows only the pinned rows someone without the Settings right sees', () => {
		expect(bellBadge(rows(answer([backupNote]), false, [update]), 0)).toBe(1);
	});
});

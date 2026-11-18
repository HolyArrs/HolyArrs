import { rowExtra, rowStep } from './history-search.ts';
import { panelView } from './history-panel.ts';
import { rowNote, rowText } from './history-lines.ts';
import type { HistoryRow } from './history.ts';
import { describe, expect, it } from 'vitest';
import type { EventRecord } from '../core/types.ts';

// A person's note on an ask or a report is theirs: the row shows it where it always did, once, and never as a note about Sources.
describe('a person\'s note is not the Sources note', () => {
	const row = (Kind: string, Data: Record<string, unknown>): HistoryRow => {
		const e = { ID: 9000 + Kind.length, At: new Date(2026, 10, 5, 22).toISOString(), Kind, Summary: Kind, TitleID: 2, title_name: 'Dune', Data } as EventRecord;
		return { key: `e${e.ID}`, head: e, events: [e], steps: [] };
	};
	const who = () => 'Maya';
	const c = { now: new Date(2026, 10, 5, 22, 36), titles: new Map([[2, { kind: 'movie', artwork: [] }]]), people: new Map([[3, 'Maya']]), me: 1 } as never;
	const down = row('ask_turned_down', { turned_down_by: 'Maya', note: 'already have it' });
	const ok = row('ask_approved', { approved_by: 'Maya', auto: false, note: 'ok for the weekend' });
	const made = row('report_made', { kind: 'audio', note: 'audio out of sync' });
	const again = row('report_reopened', { kind: 'audio', note: 'still broken' });
	it('on the row', () => {
		expect(rowExtra(down, rowStep(down), who)).toBe('Maya');
		expect(rowExtra(ok, rowStep(ok), who)).toBe('Maya');
		expect(rowExtra(made, rowStep(made), who)).toBe('');
		expect(rowExtra(again, rowStep(again), who)).toBe('Opened again');
	});
	it('in the details', () => {
		expect(panelView(down, c).line).toBe('Already have it');
		expect(panelView(ok, c).line).toBe('ok for the weekend');
		expect(panelView(made, c).line).toBe('Audio \u00b7 \u201caudio out of sync\u201d');
		expect(panelView(again, c).line).toBe('Opened again \u00b7 Audio \u00b7 \u201cstill broken\u201d');
	});
	it('in the words', () => {
		expect(rowText(down, c).line).toBe('Maya \u00b7 Already have it');
		expect(rowText(ok, c).line).toBe('Maya \u00b7 Ok for the weekend');
		expect(rowText(made, c).line).toBe('Audio \u00b7 audio out of sync');
		expect(rowText(again, c).line).toBe('Audio \u00b7 still broken');
	});
	it('only a search or a pick carries the Sources note', () => {
		const note = "No Source this search can use has this folder's tags, so Sources without tags were asked too.";
		for (const k of ['search_done', 'search_failed', 'release_chosen', 'release_chosen_by_hand', 'release_held']) expect(rowNote(row(k, { sources_note: note })), k).toBe(note);
		for (const k of ['ask_approved', 'ask_turned_down', 'report_made', 'download_sent']) expect(rowNote(row(k, { sources_note: note, note })), k).toBe('');
		expect(rowNote(row('release_chosen', { note }))).toBe('');
	});
});

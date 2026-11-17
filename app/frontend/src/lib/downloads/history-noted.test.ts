import { stepKinds } from './history-search.ts';
import { dayItems, foldSame } from './history-tabs.ts';
import { rowTier } from './history-kinds.ts';
import { rowNote, rowText, runLine } from './history-lines.ts';
import { panelView } from './history-panel.ts';
import type { HistoryRow } from './history.ts';
import { describe, expect, it } from 'vitest';
import { lang } from '../core/i18n/i18n.svelte.ts';
import de from '../core/i18n/messages/de/server-lines.json';
import deDl from '../core/i18n/messages/de/downloads.json';
import en from '../core/i18n/messages/en/server-lines.json';
import enDl from '../core/i18n/messages/en/downloads.json';
import type { EventRecord } from '../core/types.ts';

// A search or feed check that says which Sources it left out is not routine: History's default view shows it, with its note.
describe('a search with a Sources note', () => {
	const kept = 'This folder only takes Sources tagged anime. None can be searched right now (ix-anime is paused by you), so nothing was asked.';
	const fallback = "No Source this search can use has this folder's tags, so Sources without tags were asked too.";
	let id = 0;
	const row = (Kind: string, Data: Record<string, unknown> = {}, Summary = Kind): HistoryRow => {
		const e = { ID: ++id, At: new Date(2026, 10, 14, 10).toISOString(), Kind, Summary, TitleID: 1, title_name: 'Frieren', Data } as EventRecord;
		return { key: `e${e.ID}`, head: e, events: [e], steps: [] };
	};
	const ctx = { now: new Date(2026, 10, 14, 11), titles: new Map([[1, { kind: 'show' }]]) };

	it('is asked for with no step picked, and only then', () => {
		expect(stepKinds([])).toEqual(expect.arrayContaining(['search_done:noted', 'feed_checked:noted']));
		expect(stepKinds([])).not.toContain('search_done');
		expect(stepKinds([])).not.toContain('feed_checked');
		expect(stepKinds(['sent']).some((k) => k.endsWith(':noted'))).toBe(false);
	});
	it('stands as its own row; a search without a note stays routine', () => {
		const noted = row('search_done', { sources_note: kept }, 'no Release passed the Pick rules (0 found, 0 refused)');
		const feed = row('feed_checked', { sources_note: fallback });
		const plain = row('search_done');
		expect(rowTier(noted)).toBe('changes');
		expect(rowTier(feed)).toBe('changes');
		expect(rowTier(plain)).toBe('routine');
		expect(dayItems([noted, feed]).map((i) => i.type)).toEqual(['row', 'row']);
	});
	it('says the note on the row', () => {
		expect(rowText(row('search_done', { sources_note: kept }, 'no Release passed the Pick rules (0 found, 0 refused)'), ctx).line).toContain(kept);
		expect(rowNote(row('feed_checked', { sources_note: fallback }))).toBe(fallback);
		expect(rowText(row('feed_checked', { sources_note: fallback }), ctx).line).toContain(fallback);
		expect(rowText(row('feed_checked'), ctx).line).not.toContain('Source');
	});
	it('shows the whole note in the details', () => {
		const c = { ...ctx, titles: new Map([[1, { kind: 'show', artwork: [] }]]) } as never;
		const p = panelView(row('search_done', { sources_note: kept }, 'no Release passed the Pick rules (0 found, 0 refused)'), c);
		expect(p.line).toContain(kept);
		expect(p.long).toBe(true);
		expect(panelView(row('feed_checked', { sources_note: fallback }), c).long).toBe(true);
	});
	const ep = (n: number, title = 1, note = fallback): HistoryRow => {
		const r = row('search_done', { sources_note: note }, `${title === 1 ? 'Frieren' : 'Arcane'}: no Release passed the Pick rules (${n % 3} found, 0 refused)`);
		r.head.TitleID = title;
		r.head.episode = `S01E${n}`;
		return r;
	};
	const fold = (rows: HistoryRow[]) => foldSame(dayItems(rows), (r) => rowText(r, ctx).line);
	it('folds one hand search per episode into one row with the note once', () => {
		const rows = Array.from({ length: 41 }, (_, i) => ep(41 - i));
		const items = fold(rows);
		expect(items).toHaveLength(1);
		expect(items[0].type === 'same' && items[0].rows.length).toBe(41);
		const line = runLine(rows);
		expect(line).toBe(`41 episodes · ${fallback}`);
		expect(line.split(fallback)).toHaveLength(2);
	});
	it('keeps two shows, or two notes, apart', () => {
		const mixed = Array.from({ length: 6 }, (_, i) => ep(i + 1, i % 2 ? 1 : 2));
		expect(fold(mixed)).toHaveLength(6);
		const two = [...[1, 2, 3].map((n) => ep(n, 1)), ...[4, 5].map((n) => ep(n, 2))];
		expect(fold(two).map((i) => (i.type === 'same' ? i.rows.length : 1))).toEqual([3, 2]);
		const notes = [...[1, 2].map((n) => ep(n, 1, kept)), ...[3, 4].map((n) => ep(n, 1))];
		expect(fold(notes).map((i) => (i.type === 'same' ? i.rows.length : 1))).toEqual([2, 2]);
	});
	it('keeps a hand search and an automatic search apart', () => {
		const by = (n: number, kind: string, actor = 0) => {
			const r = ep(n);
			r.head.Actor = { kind, id: actor } as never;
			return r;
		};
		const rows = [by(1, 'user', 1), by(2, 'user', 1), by(3, 'system'), by(4, 'system'), by(5, 'user', 2)];
		expect(fold(rows).map((i) => (i.type === 'same' ? i.rows.length : 1))).toEqual([2, 2, 1]);
	});
	it('shows the note in full in the details, never cut', () => {
		const c = { ...ctx, titles: new Map([[1, { kind: 'show', artwork: [] }]]) } as never;
		expect(panelView(ep(3), c).full).toBe(true);
		expect(panelView(row('release_held', { sources_note: fallback }), c).full).toBe(true);
		expect(panelView(row('release_chosen', { sources_note: fallback }), c).full).toBe(true);
		expect(panelView(row('search_done'), c).full).toBe(false);
	});
	it('says it in German', () => {
		const was = { value: lang.value, words: lang.words };
		lang.value = 'de';
		lang.words = { ...(en as Record<string, string>), ...(enDl as Record<string, string>), ...(de as Record<string, string>), ...(deDl as Record<string, string>) };
		try {
			const line = rowText(row('search_done', { sources_note: kept }, 'no Release passed the Pick rules (0 found, 0 refused)'), ctx).line;
			expect(line).toContain('(ix-anime wurde von dir pausiert), deshalb wurde niemand gefragt.');
			expect(line).not.toContain('nothing was asked');
			const feed = rowText(row('feed_checked', { sources_note: fallback }), ctx).line;
			expect(feed).not.toContain('Sources without tags');
			expect(feed).not.toContain('Checked the feeds');
			const run = runLine(Array.from({ length: 41 }, (_, i) => ep(i + 1)));
			expect(run.startsWith('41 Folgen · ')).toBe(true);
			expect(run).toContain('deshalb wurden auch Quellen ohne Tags gefragt.');
			const now = rowText(row('feed_checked', { sources_note: "No Source this search can use has this folder's tags, so Sources without the folder's tags were asked." }), ctx).line;
			expect(now).toContain('deshalb wurden Quellen ohne die Tags des Ordners gefragt.');
		} finally {
			lang.value = was.value;
			lang.words = was.words;
		}
	});
});

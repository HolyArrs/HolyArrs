import { describe, expect, it } from 'vitest';
import type { TitleView } from '../core/types.ts';
import { applyLayout, chosenColumns, columnsFor, columnSort, filterCounts, loadColumns, loadSaved, moveColumn, noFilter, passes, saveColumns, saveSaved, sortLabel, sortLabels, sortsFor, sortTitles, startDir, tableMinWidth, type FilterContext } from './table.ts';

function memory() {
	const m = new Map<string, string>();
	return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v), m };
}

function movie(id: number, name: string, extra: Partial<TitleView> = {}): TitleView {
	return { id, kind: 'movie', name, ids: null, keep_up_to_date: true, why_added: '', release_gate: '', gate_delay_days: 0, stage: '', gate_open: true, gate_reason: '', artwork: [], versions: [], slots: [], ...extra } as TitleView;
}
function show(id: number, name: string, s: Partial<NonNullable<TitleView['show']>>, extra: Partial<TitleView> = {}): TitleView {
	return movie(id, name, { kind: 'show', show: { details_source: '', episode_order: '', new_seasons: '', status: '', seasons: [], episodes: 10, missing: 0, not_out_yet: 0, in_library: 0, better_wanted: 0, ...s }, ...extra });
}

describe('column layout', () => {
	it('is saved per member and library, and read back in the saved order', () => {
		const store = memory();
		const cols = moveColumn(columnsFor('show'), 3, 0).map((c) => (c.id === 'network' ? { ...c, on: false } : c));
		saveColumns(7, 'show', cols, store);
		expect([...store.m.keys()]).toEqual(['holyarrs.library-columns.7.show']);
		const back = loadColumns(7, 'show', store);
		expect(back.map((c) => c.id)).toEqual(cols.map((c) => c.id));
		expect(back.find((c) => c.id === 'network')?.on).toBe(false);
		expect(loadColumns(8, 'show', store)).toEqual(columnsFor('show'));
		expect(loadColumns(7, 'movie', store)).toEqual(columnsFor('movie'));
	});
	it('drops unknown columns and keeps ones added since', () => {
		const got = applyLayout('artist', [{ id: 'size', on: false }, { id: 'nope' as never, on: true }]);
		expect(got[0]).toMatchObject({ id: 'status', on: true });
		expect(got.find((c) => c.id === 'size')).toMatchObject({ on: false });
		expect(got.map((c) => c.id).sort()).toEqual(columnsFor('artist').map((c) => c.id).sort());
	});
	it('survives broken storage', () => {
		expect(loadColumns(1, 'movie', { getItem: () => '{nope' })).toEqual(columnsFor('movie'));
		expect(() => saveColumns(1, 'movie', columnsFor('movie'), { setItem: () => { throw new Error('full'); } })).not.toThrow();
	});
	it('moves a column within bounds', () => {
		const ids = (c: ReturnType<typeof columnsFor>) => c.map((x) => x.id);
		const base = columnsFor('movie');
		expect(ids(moveColumn(base, 0, 99)).at(-1)).toBe('status');
		expect(moveColumn(base, -1, 2)).toBe(base);
	});
});

describe('sorting', () => {
	const ts = [
		movie(1, 'The Zebra', { year: 2001, size_bytes: 10, added_at: '2026-01-02T00:00:00Z' }),
		movie(2, 'Alpha', { year: 2020, size_bytes: 30, added_at: '2026-03-01T00:00:00Z' }),
		movie(3, 'a Beta', { size_bytes: 20 })
	];
	it('sorts names without a leading article', () => {
		expect(sortTitles(ts, 'name').map((t) => t.id)).toEqual([2, 3, 1]);
		expect(sortTitles(ts, 'name', 'desc').map((t) => t.id)).toEqual([1, 3, 2]);
	});
	it('keeps blanks last in both directions', () => {
		expect(sortTitles(ts, 'year', 'desc').map((t) => t.id)).toEqual([2, 1, 3]);
		expect(sortTitles(ts, 'year', 'asc').map((t) => t.id)).toEqual([1, 2, 3]);
		expect(sortTitles(ts, 'added', 'asc').map((t) => t.id)).toEqual([1, 2, 3]);
	});
	it('sorts by size and by episode progress', () => {
		expect(sortTitles(ts, 'size', 'desc').map((t) => t.id)).toEqual([2, 3, 1]);
		const shows = [show(4, 'Half', { in_library: 5 }), show(5, 'Full', { in_library: 10 }), show(6, 'None', { in_library: 0 })];
		expect(sortTitles(shows, 'progress', 'desc').map((t) => t.id)).toEqual([5, 4, 6]);
	});
	it('puts shows with no next episode last', () => {
		const shows = [show(7, 'Ended', {}), show(8, 'Soon', { next_air_date: '2026-11-01' }), show(9, 'Sooner', { next_air_date: '2026-10-25' })];
		expect(sortTitles(shows, 'next').map((t) => t.id)).toEqual([9, 8, 7]);
	});
	it('sorts by score, highest first, with unscored and low-vote titles last both ways', () => {
		const scored = [
			movie(1, 'Mid', { score: 7.4, votes: 900 }),
			movie(2, 'None', {}),
			movie(3, 'Top', { score: 8.1, votes: 5000 }),
			movie(4, 'Few votes', { score: 9.5, votes: 3 }),
			movie(5, 'Low', { score: 6, votes: 40 }),
			movie(6, 'Zero', { score: 0, votes: 0 })
		];
		expect(startDir('score')).toBe('desc');
		expect(sortTitles(scored, 'score', 'desc').map((t) => t.id)).toEqual([3, 1, 5, 4, 2, 6]);
		expect(sortTitles(scored, 'score', 'asc').map((t) => t.id)).toEqual([5, 1, 3, 4, 2, 6]);
	});
	it('breaks score ties by name', () => {
		const tie = [movie(1, 'Zulu', { score: 7, votes: 100 }), movie(2, 'Alpha', { score: 7, votes: 100 })];
		expect(sortTitles(tie, 'score', 'desc').map((t) => t.id)).toEqual([2, 1]);
	});
	it('offers score for movies, shows and All, never for music', () => {
		for (const k of ['movie', 'show', 'all'] as const) expect(sortsFor(k)).toContain('score');
		expect(sortsFor('artist')).not.toContain('score');
		expect(columnSort.score).toBe('score');
		expect(columnsFor('movie').find((c) => c.id === 'score')?.on).toBe(false);
		expect(columnsFor('show').find((c) => c.id === 'score')?.on).toBe(false);
		expect(columnsFor('artist').some((c) => c.id === 'score')).toBe(false);
	});
});

describe('filter counts', () => {
	const ts = [
		show(1, 'Missing kept', { missing: 2 }, { keep_up_to_date: true }),
		show(2, 'Missing not kept', { missing: 1 }, { keep_up_to_date: false }),
		show(3, 'Have it', { in_library: 10 }, { keep_up_to_date: true }),
		movie(4, 'Series film', { slots: [{ id: 1, version: 'Main', version_id: 1, state: 'in_library', state_label: '' }] as TitleView['slots'] })
	];
	const ctx: FilterContext = { askers: (t) => (t.id === 1 ? [5] : t.id === 3 ? [5, 6] : []), me: 6, seriesOf: (t) => (t.id === 4 ? 40 : undefined) };
	it('counts every choice', () => {
		const c = filterCounts(ts, noFilter, ctx);
		expect(c.status).toEqual({ all: 4, missing: 2, not_out_yet: 0, better: 0, have: 2 });
		expect(c.kept).toEqual({ any: 4, on: 3, off: 1 });
		expect(Object.fromEntries(c.askers)).toEqual({ 5: 2, 6: 1 });
		expect(Object.fromEntries(c.series)).toEqual({ 40: 1 });
	});
	it('counts each group under the other choices already made', () => {
		const f = { ...noFilter, kept: 'on' as const };
		const c = filterCounts(ts, f, ctx);
		expect(c.status.missing).toBe(1);
		expect(c.kept).toEqual({ any: 4, on: 3, off: 1 });
		expect(ts.filter((t) => passes(t, { ...f, asker: 'me' }, ctx)).map((t) => t.id)).toEqual([3]);
		expect(ts.filter((t) => passes(t, { ...noFilter, series: 40 }, ctx)).map((t) => t.id)).toEqual([4]);
	});
	it('filters movies by codec and titles by missing subtitles', () => {
		const copy = (codec: string) => ({ file_id: 1, version_id: 1, codec, container: 'matroska', audio_languages: ['en'], protected: false });
		const films = [movie(10, 'HEVC film', { copy: copy('hevc') }), movie(11, 'H.264 film', { copy: copy('h264') }), movie(12, 'No file', {})];
		const sub: FilterContext = { ...ctx, subsMissing: (t) => t.id === 11 };
		expect(films.filter((t) => passes(t, { ...noFilter, codec: 'hevc' }, sub)).map((t) => t.id)).toEqual([10]);
		expect(films.filter((t) => passes(t, { ...noFilter, subs: true }, sub)).map((t) => t.id)).toEqual([11]);
		const c = filterCounts(films, { ...noFilter, subs: true }, sub);
		expect(Object.fromEntries(c.codecs)).toEqual({ h264: 1 });
		expect(c.subs).toBe(1);
	});
});

describe('saved filters', () => {
	it('round-trip per member and library, filling choices added later', () => {
		const store = memory();
		saveSaved(3, 'show', [{ name: 'Missing, kept', filter: { ...noFilter, status: 'missing', kept: 'on' } }], store);
		expect(loadSaved(3, 'show', store)).toEqual([{ name: 'Missing, kept', filter: { status: 'missing', kept: 'on', asker: 'anyone', series: 0, codec: '', subs: false } }]);
		expect(loadSaved(3, 'movie', store)).toEqual([]);
		store.setItem('holyarrs.library-filters.3.movie', JSON.stringify([{ name: 'Old', filter: { status: 'have' } }, { bad: 1 }]));
		expect(loadSaved(3, 'movie', store)).toEqual([{ name: 'Old', filter: { ...noFilter, status: 'have' } }]);
	});
});

describe('tableMinWidth', () => {
	it('adds column floors, gaps and padding', () => {
		expect(tableMinWidth(['420px', '28px', 'minmax(180px, 1fr)', '200px'])).toBe(420 + 28 + 180 + 200 + 48 + 20);
	});
	it('ignores widths with no px floor', () => {
		expect(tableMinWidth(['1fr'], 16, 0)).toBe(0);
	});
});

describe('chosenColumns', () => {
	it('shows the score right after the status while sorting by score, without saving it', () => {
		const cols = columnsFor('movie');
		const before = JSON.stringify(cols);
		const ids = chosenColumns(cols, 'score').map((c) => c.id);
		expect(ids.slice(0, 2)).toEqual(['status', 'score']);
		expect(ids.filter((id) => id === 'score')).toHaveLength(1);
		expect(JSON.stringify(cols)).toBe(before);
		const store = memory();
		saveColumns(1, 'movie', cols, store);
		expect(loadColumns(1, 'movie', store).find((c) => c.id === 'score')?.on).toBe(false);
	});
	it('moves a score column that is on to the front, and leads with it when status is off', () => {
		const on = columnsFor('movie').map((c) => (c.id === 'score' ? { ...c, on: true } : c));
		expect(chosenColumns(on, 'score').map((c) => c.id).slice(0, 2)).toEqual(['status', 'score']);
		const noStatus = on.filter((c) => c.id !== 'status');
		expect(chosenColumns(noStatus, 'score')[0].id).toBe('score');
	});
	it('is only the columns that are on for any other sort, and for music', () => {
		const cols = columnsFor('movie');
		expect(chosenColumns(cols, 'name')).toEqual(cols.filter((c) => c.on));
		expect(chosenColumns(columnsFor('artist'), 'score')).toEqual(columnsFor('artist').filter((c) => c.on));
	});
});

describe('labels', () => {
	it('every sort and column has English words', () => {
		for (const [k, label] of Object.entries(sortLabels)) expect(label, k).not.toBe(`mediafiles.sort.${k}`);
		for (const kind of ['movie', 'show', 'artist'] as const) for (const c of columnsFor(kind)) expect(c.label, c.id).not.toBe(`mediafiles.col.${c.id}`);
		expect(sortLabels.score).toBe('Score');
	});
});

describe('sortLabel', () => {
	it('names the Music next sort a release', () => {
		expect(sortLabel('next', 'artist')).toBe('Next release');
		expect(sortLabel('next', 'show')).toBe('Next episode');
		expect(sortLabel('release', 'movie')).toBe('Out');
	});
});

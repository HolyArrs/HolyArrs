import { describe, expect, it } from 'vitest';
import { libraryState } from './library-state.ts';
import type { TitleView, SlotView, ShowInfo } from '../core/types.ts';

const now = Date.parse('2026-10-21T00:00:00Z');
const slot = (state: string, extra: Partial<SlotView> = {}): SlotView => ({ id: 1, version: 'HD', version_id: 1, state, state_label: state, ...extra });
const dl = (state: string, progress = 0.4) => ({ id: 1, state, label: state, text: '', progress, speed: 0, active: ['queued', 'downloading', 'stalled', 'paused', 'sent'].includes(state) });
const movie = (slots: SlotView[], extra: Partial<TitleView> = {}): TitleView =>
	({ id: 1, kind: 'movie', name: 'M', ids: null, keep_up_to_date: true, why_added: '', release_gate: '', gate_delay_days: 0, stage: '', gate_open: true, gate_reason: '', artwork: [], versions: [], slots, ...extra }) as TitleView;
const show = (s: Partial<ShowInfo>, extra: Partial<TitleView> = {}): TitleView =>
	movie([], { kind: 'show', show: { details_source: '', episode_order: '', new_seasons: '', status: 'running', seasons: [], episodes: 10, missing: 0, not_out_yet: 0, in_library: 10, better_wanted: 0, ...s }, ...extra });
const label = (t: TitleView) => libraryState(t, now)?.label;

describe('libraryState: movies', () => {
	it('tells queued, paused and stalled apart from downloading', () => {
		expect(label(movie([slot('downloading', { download: dl('downloading', 0.63) })]))).toBe('Downloading · 63%');
		expect(label(movie([slot('downloading', { download: dl('queued') })]))).toBe('Queued');
		expect(label(movie([slot('downloading', { download: dl('paused', 0.48) })]))).toBe('Paused · 48%');
		expect(label(movie([slot('stalled', { download: dl('stalled', 0.31) })]))).toBe('Stalled · 31%');
	});
	it('names a needed decision, a missing file and a better copy', () => {
		expect(label(movie([slot('needs_a_decision')]))).toBe('To review');
		expect(label(movie([slot('in_library', { file: { RelativePath: 'a', Size: 1, AddedAt: '', State: 'missing' } })]))).toBe('Missing file');
		expect(label(movie([slot('better_wanted', { file: { RelativePath: 'a', Size: 1, AddedAt: '', Quality: { resolution: 720 } } })]))).toBe('Better copy · 720p');
	});
	it('says not kept up to date, in Library with quality, asks and not out yet', () => {
		expect(label(movie([slot('wanted')], { keep_up_to_date: false }))).toBe('Not kept');
		expect(libraryState(movie([slot('wanted')], { keep_up_to_date: false }), now)?.icon).toBe('x');
		expect(label(movie([slot('in_library', { file: { RelativePath: 'a', Size: 1, AddedAt: '', Quality: { resolution: 2160 } } })]))).toBe('In library · 4K');
		const ask = (state: string) => [{ state, title_id: 1 }] as unknown as TitleView['asks'];
		expect(label(movie([slot('wanted')], { asks: ask('approved') }))).toBe('Searching');
		expect(label(movie([slot('wanted')], { asks: ask('asked') }))).toBe('Waiting');
		expect(label(movie([slot('wanted')], { asks: ask('turned_down') }))).toBe('Declined');
		expect(label(movie([slot('not_out_yet')], { cinema_date: '2027-05-01T00:00:00Z' }))).toMatch(/^Upcoming · /);
		expect(label(movie([slot('wanted')]))).toBe('Missing');
	});
	it('ranks problems first', () => {
		const failed = libraryState(movie([slot('failed')]), now)!;
		const have = libraryState(movie([slot('in_library')]), now)!;
		expect(failed.rank).toBeLessThan(have.rank);
	});
});

describe('libraryState: shows', () => {
	it('shows episodes downloading, not kept up to date and specials only', () => {
		expect(label(show({ downloading: 2, missing: 3 }))).toBe('Downloading · 2');
		expect(label(show({ missing: 0, in_library: 0, not_out_yet: 37 }, { keep_up_to_date: false }))).toBe('Not kept');
		const seasons = [0, 1].map((n) => ({ number: n, name: '', keep_up_to_date: n === 0, episodes: 1, aired: 1, in_library: 0, missing: 0, not_out_yet: 0, better_wanted: 0 }));
		expect(label(show({ missing: 195, in_library: 4, seasons }))).toBe('Specials only · 4');
	});
	it('shows missing counts, up to date and ended', () => {
		expect(label(show({ missing: 28, in_library: 18 }))).toBe('Missing · 28');
		expect(label(show({}))).toBe('Complete');
		expect(label(show({ status: 'ended' }))).toBe('Complete · ended');
	});
});

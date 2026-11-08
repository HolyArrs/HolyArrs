import { describe, expect, it } from 'vitest';
import {
	accountMissing,
	fromWords,
	jobsDone,
	kindCovers,
	kindLabel,
	missing,
	missingName,
	missingQuery,
	missingWords,
	offsetWords,
	ours,
	passes,
	present,
	scoreLines,
	siteBody,
	siteForm,
	siteState,
	sourceLabel,
	syncBody,
	wantLabel,
	wishLine,
	wishProblem,
	type SiteKind,
	type SubtitleRow
} from './api.ts';

const langs = [
	{ code: 'en', name: 'English' },
	{ code: 'es', name: 'Spanish' }
];
const when = (s: string) => `at ${s}`;

function row(p: Partial<SubtitleRow>): SubtitleRow {
	return { ID: 1, file_id: 1, language: 'en', forced: false, hearing_impaired: false, place: 'beside', score_parts: [], origin: 'fetched', state: 'present', tries: 0, sync: {}, ...p };
}

describe('wishes', () => {
	it('says a wish in words', () => {
		expect(wantLabel({ language: 'es', hearing_impaired: 'hi', forced: true }, langs)).toBe('Spanish, hearing-impaired only, forced only');
		expect(wantLabel({ language: 'en', hearing_impaired: 'normal', forced: false }, langs)).toBe('English');
		expect(wantLabel({ language: 'xx', hearing_impaired: 'either', forced: false }, langs)).toBe('xx, normal or hearing-impaired');
		expect(wishLine([], langs)).toBe('None');
		expect(wishLine([{ language: 'en', hearing_impaired: 'normal', forced: false }, { language: 'es', hearing_impaired: 'normal', forced: false }], langs)).toBe('English · Spanish');
	});
	it('says where a wish comes from', () => {
		expect(fromWords('default', true)).toBe('the default languages');
		expect(fromWords('title', true)).toBe('set for this movie');
		expect(fromWords('title', false)).toBe('set for this show');
		expect(fromWords('episode', false)).toBe('set for this episode');
	});
	it('refuses a language twice unless once forced', () => {
		const en = { language: 'en', hearing_impaired: 'normal' as const, forced: false };
		expect(wishProblem([en])).toBe('');
		expect(wishProblem([en, { ...en, forced: true }])).toBe('');
		expect(wishProblem([en, en])).toMatch(/twice/);
		expect(wishProblem([{ ...en, language: '' }])).toMatch(/Pick a language/);
		expect(wishProblem(Array.from({ length: 11 }, (_, i) => ({ ...en, language: 'l' + i })))).toMatch(/10/);
	});
});

describe('present and missing subtitles', () => {
	it('names kind and source', () => {
		expect(kindLabel({ forced: true, hearing_impaired: true })).toBe('Forced');
		expect(kindLabel({ forced: false, hearing_impaired: true })).toBe('Hearing-impaired');
		expect(sourceLabel({ place: 'built_in', origin: 'found', codec: 'S_TEXT/UTF8' })).toBe('Inside the video (S_TEXT/UTF8)');
		expect(sourceLabel({ place: 'beside', origin: 'found' })).toBe('Found beside the video');
		expect(sourceLabel({ place: 'beside', origin: 'fetched', site: 'Lab' })).toBe('From Lab');
		expect(sourceLabel({ place: 'beside', origin: 'by_hand', site: 'Lab' })).toBe('Chosen by hand from Lab');
	});
	it('only offers fixes for files HolyArrs saved', () => {
		expect(ours(row({}))).toBe(true);
		expect(ours(row({ origin: 'found' }))).toBe(false);
		expect(ours(row({ place: 'built_in' }))).toBe(false);
		expect(ours(row({ state: 'replaced' }))).toBe(false);
	});
	it('splits present from missing', () => {
		const rows = [row({ ID: 1, language: 'es' }), row({ ID: 2, place: 'built_in', language: 'fr' }), row({ ID: 3, place: 'wish', state: 'wanted' }), row({ ID: 4, state: 'replaced' })];
		expect(present(rows).map((r) => r.ID)).toEqual([2, 1]);
		expect(missing(rows).map((r) => r.ID)).toEqual([3]);
	});
	it('says why a subtitle is still missing', () => {
		expect(missingWords({ state: 'wanted', tries: 0 }, when)).toBe('Not looked for yet');
		expect(missingWords({ state: 'wanted', tries: 2 }, when)).toBe('Not found yet (2 tries)');
		expect(missingWords({ state: 'none_good_enough', tries: 1, next_try_at: 'noon' }, when)).toBe('None over the Pass mark yet (1 try); next try at noon');
	});
	it('names missing rows and builds the list query', () => {
		expect(missingName({ title: 'Show', kind: 'episode', season: 1, episode: 2 })).toBe('Show S01E02');
		expect(missingName({ title: 'Movie', kind: 'movie' })).toBe('Movie');
		expect(missingQuery({ language: 'es', kind: 'movie', title: 3 }, 100, 0)).toBe('language=es&kind=movie&title=3&limit=100');
		expect(missingQuery({}, 50, 100)).toBe('limit=50&offset=100');
	});
});

describe('sites', () => {
	const acct: SiteKind = { kind: 'fake_account', name: 'Lab with account', movies: true, shows: true, needs_account: true, account: ['api_key'] };
	const open: SiteKind = { kind: 'fake', name: 'Lab', movies: true, shows: false, needs_account: false };
	it('says the state in plain words', () => {
		const base = { state: 'enabled' as const, pause_reason: null, paused_until: null, has_account: false };
		expect(siteState(base, acct, when).label).toBe('Needs your account');
		expect(siteState(base, open, when).label).toBe('On');
		expect(siteState({ ...base, state: 'disabled' }, open, when).text).toMatch(/Paused by you/);
		expect(siteState({ ...base, state: 'paused', pause_reason: 'rate_limited', paused_until: 'noon' }, open, when).text).toBe(
			'Resting until at noon: it asked HolyArrs to slow down. HolyArrs tries it again after that.'
		);
		expect(kindCovers(open)).toBe('movies only');
	});
	it('needs the account details before a save, and keeps stored ones', () => {
		const f = siteForm(acct);
		expect(accountMissing(acct, f, false)).toEqual(['api_key']);
		expect(accountMissing(acct, f, true)).toEqual([]);
		expect(accountMissing(acct, { ...f, api_key: 'k' }, false)).toEqual([]);
		expect(accountMissing(open, f, false)).toEqual([]);
	});
	it('starts a default-off kind turned off, and needs every OpenSubtitles.com detail', () => {
		const pn: SiteKind = { kind: 'podnapisi', name: 'Podnapisi', movies: true, shows: true, needs_account: false, default_off: true };
		expect(siteForm(pn).enabled).toBe(false);
		expect(siteForm(open).enabled).toBe(true);
		const os: SiteKind = { kind: 'opensubtitles', name: 'OpenSubtitles.com', movies: true, shows: true, needs_account: true, account: ['api_key', 'username', 'password'] };
		expect(accountMissing(os, { ...siteForm(os), api_key: 'k' }, false)).toEqual(['username', 'password']);
	});
	it('sends account details only when typed', () => {
		const f = siteForm(open);
		expect(siteBody(f)).not.toHaveProperty('api_key');
		expect(siteBody({ ...f, api_key: 'k', priority: '3' as unknown as number })).toMatchObject({ api_key: 'k', priority: 3 });
	});
});

describe('scores, sync and progress', () => {
	it('lists what matched first', () => {
		const lines = scoreLines([
			{ name: 'group', points: 0, matched: false },
			{ name: 'name', points: 34, matched: true }
		]);
		expect(lines).toEqual([
			{ text: '+34 name matches', matched: true },
			{ text: 'release group does not match', matched: false }
		]);
		expect(passes(70, 70)).toBe(true);
		expect(passes(69, 70)).toBe(false);
	});
	it('builds a Sync fix', () => {
		expect(syncBody(1500, { on: false, from: 25, to: 23.976 })).toEqual({ offset_ms: 1500 });
		expect(syncBody(0, { on: true, from: 25, to: 23.976 })).toEqual({ from_fps: 25, to_fps: 23.976 });
		expect(syncBody(0, { on: true, from: 25, to: 25 })).toEqual({});
		expect(offsetWords(-500)).toBe('0.5 seconds earlier');
		expect(offsetWords(1000)).toBe('1 second later');
		expect(offsetWords(0)).toBe('No shift');
	});
	it('counts ended Jobs', () => {
		expect(jobsDone([1, 2, 3], { 1: 'done', 2: 'running', 3: 'failed' })).toBe(2);
	});
});

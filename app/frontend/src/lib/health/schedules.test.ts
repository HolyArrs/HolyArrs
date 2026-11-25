import { describe, expect, it } from 'vitest';
import { busyNow, dot, every, groupSchedules, lastSeen, mayRun, scheduleGroup, skipLinks, skipWords, unit, until } from './schedules.ts';

describe('schedules', () => {
	// The Last run column shows the newest run, run by hand or check,
	// and why the newest one did nothing.
	it.each([
		['never ran or checked', {}, { checked: false }],
		['clock start only', { last_run: '2026-11-19T10:00:00Z' }, { at: '2026-11-19T10:00:00Z', checked: false, why: undefined, took: undefined }],
		['run by hand after the clock', { last_run: '2026-11-19T10:00:00Z', last_job_ended: '2026-11-19T11:00:00Z', last_job_s: 2.5 }, { at: '2026-11-19T11:00:00Z', checked: false, took: 2.5 }],
		['run by hand that did nothing', { last_job_ended: '2026-11-19T11:00:00Z', last_job_note: 'setting_off', last_job_s: 0 }, { at: '2026-11-19T11:00:00Z', why: 'setting_off', took: 0 }],
		['older note under a newer clock start', { last_run: '2026-11-19T12:00:00Z', last_job_ended: '2026-11-19T11:00:00Z', last_job_note: 'nobody', last_job_s: 1 }, { at: '2026-11-19T12:00:00Z', why: undefined, took: undefined }],
		['check only', { last_checked: '2026-11-19T12:00:00Z', last_skip: 'nobody' }, { at: '2026-11-19T12:00:00Z', checked: true, why: 'nobody' }],
		['check with no reason', { last_checked: '2026-11-19T12:00:00Z' }, { at: '2026-11-19T12:00:00Z', checked: true, why: 'nothing' }],
		['check newer than a run', { last_run: '2026-11-19T10:00:00Z', last_job_ended: '2026-11-19T10:01:00Z', last_checked: '2026-11-19T12:00:00Z', last_skip: 'setting_off' }, { at: '2026-11-19T12:00:00Z', checked: true, why: 'setting_off' }],
		['run newer than a check', { last_run: '2026-11-19T13:00:00Z', last_checked: '2026-11-19T12:00:00Z', last_skip: 'setting_off' }, { at: '2026-11-19T13:00:00Z', checked: false, why: undefined }]
	])('last seen: %s', (_name, row, want) => {
		expect(lastSeen(row)).toMatchObject(want);
	});
	it('says why a job did nothing', () => {
		expect(skipWords('sync_plex_watchlists', 'setting_off')).toBe('Off: turn on Request from Plex watchlists in Request rules');
		expect(skipWords('sync_plex_watchlists', 'nobody')).toBe('Nothing to do: no one has turned their Plex watchlist on');
		expect(skipWords('feed_check', 'setting_off')).toBe('Off: turn it on in its settings');
		expect(skipWords('feed_check', 'nobody')).toBe('Nothing to do');
		expect(skipWords('sync_plex_watchlists', 'starting')).toBe('Skipped: HolyArrs was still starting');
		expect(skipWords('feed_check', 'nothing')).toBe('Nothing to do');
		expect(skipWords('feed_check', 'some_new_code')).toBe('Nothing to do');
		expect(skipWords('feed_check', undefined)).toBe('Nothing to do');
		expect(skipLinks('setting_off')).toBe(true);
		expect(skipLinks('nobody')).toBe(true);
		expect(skipLinks('nothing')).toBe(false);
	});
	it('says intervals in words', () => {
		expect(every(60)).toBe('every minute');
		expect(every(900)).toBe('every 15 minutes');
		expect(every(3600)).toBe('every hour');
		expect(every(43200)).toBe('every 12 hours');
		expect(every(86400)).toBe('every day');
		expect(every(90)).toBe('every 90 seconds');
		expect(every(0)).toBe('all the time');
	});
	it('picks the unit an interval is typed in', () => {
		expect(unit(3600)).toEqual({ size: 3600, word: 'hours' });
		expect(unit(60)).toEqual({ size: 60, word: 'minutes' });
		expect(unit(300)).toEqual({ size: 60, word: 'minutes' });
	});
	it('says when the next run is', () => {
		const now = Date.parse('2026-10-14T12:00:00Z');
		expect(until(undefined, now)).toBe('not planned');
		expect(until('2026-10-14T12:00:10Z', now)).toBe('now');
		expect(until('2026-10-14T12:15:00Z', now)).toBe('in 15 min');
		expect(until('2026-10-14T15:00:00Z', now)).toBe('in 3 h');
		expect(until('2026-10-16T12:00:00Z', now)).toBe('in 2 d');
	});
	it('colours states and offers Run now only when the part runs', () => {
		expect(mayRun({ state: 'waiting' })).toBe(true);
		expect(mayRun({ state: 'off' })).toBe(true);
		expect(mayRun({ state: 'part_off' })).toBe(false);
		expect(mayRun({ state: 'safe_mode' })).toBe(false);
	});
	it('sorts rows into four groups in page order, new kinds into Upkeep', () => {
		expect(scheduleGroup('feed_check')).toBe('copies');
		expect(scheduleGroup('refresh_titles')).toBe('metadata');
		expect(scheduleGroup('something_new')).toBe('upkeep');
		const g = groupSchedules([{ kind: 'health_check' }, { kind: 'feed_check' }, { kind: 'rescan_library' }, { kind: 'grab_held' }]);
		expect(g.map((x) => x.id)).toEqual(['library', 'copies', 'upkeep']);
		expect(g[1].rows.map((r) => r.kind)).toEqual(['feed_check', 'grab_held']);
	});
	it('finds what is running now and colours the dot', () => {
		expect(busyNow([{ state: 'running' }, { state: 'waiting' }, { state: 'queued' }])).toHaveLength(2);
		expect(dot({ state: 'running', last_job: 'failed' })).toBe('busy');
		expect(dot({ state: 'waiting', last_job: 'failed' })).toBe('fail');
		expect(dot({ state: 'waiting' })).toBe('ok');
		expect(dot({ state: 'off' })).toBe('off');
	});
});

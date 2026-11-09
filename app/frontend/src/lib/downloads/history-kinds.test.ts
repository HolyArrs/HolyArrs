import { byWhom, dayLabel, eventsQuery, tierOf, titleHref, verbOf } from './history-kinds.ts';
import { describe, expect, it } from 'vitest';
import type { EventRecord } from '../core/types.ts';

let id = 0;
function ev(Kind: string, At: string, extra: Partial<EventRecord> = {}): EventRecord {
	return { ID: ++id, At, Kind, Summary: extra.Summary ?? Kind, TitleID: null, ...extra };
}

describe('tiers', () => {
	it('files results, changes and upkeep apart', () => {
		expect(tierOf('import_done')).toBe('important');
		expect(tierOf('download_failed')).toBe('important');
		expect(tierOf('title_removed')).toBe('important');
		expect(tierOf('health_broken')).toBe('important');
		expect(tierOf('file_replaced')).toBe('changes');
		expect(tierOf('renamed')).toBe('changes');
		expect(tierOf('subtitle_saved')).toBe('changes');
		expect(tierOf('title_refreshed')).toBe('routine');
		expect(tierOf('search_done')).toBe('routine');
		expect(tierOf('albums_changed')).toBe('routine');
		expect(tierOf('something_new')).toBe('changes');
	});
	it('builds the events query with kinds replaced', () => {
		const f = { groups: ['scan'], limit: 50, offset: 0, title: 4 };
		expect(eventsQuery(f)).toContain('kind=');
		const q = new URLSearchParams(eventsQuery(f, { kinds: ['import_done', 'file_replaced'], limit: 12 }));
		expect(q.get('kind')).toBe('import_done,file_replaced');
		expect(q.get('limit')).toBe('12');
		expect(q.get('title')).toBe('4');
	});
});

describe('words', () => {
	it('names what happened', () => {
		expect(verbOf('import_done').word).toBe('Imported');
		expect(verbOf('file_replaced').word).toBe('Upgraded');
		expect(verbOf('download_failed')).toMatchObject({ word: 'Failed', tone: 'fail' });
		expect(verbOf('title_removed').word).toBe('Deleted');
		expect(verbOf('subtitle_saved').word).toBe('Subtitles');
	});
	it('says who did it', () => {
		expect(byWhom({ Actor: { kind: 'user', id: 3 }, Data: { asked_by: 'Maya' } })).toBe('Maya');
		expect(byWhom({ Actor: { kind: 'ai', id: 3 }, Data: { client: 'Claude' }, actor_name: 'Maya' })).toBe('Claude for Maya');
		expect(byWhom({ Actor: { kind: 'ai', id: 3 }, Data: {} })).toBe('An AI app');
		expect(byWhom({ Actor: { kind: 'ai', id: 3 }, Data: { client: 'an AI app' }, actor_name: 'Maya' })).toBe('An AI app for Maya');
		expect(byWhom({ Actor: { kind: 'user', id: 3 }, Data: {}, actor_name: 'Maya' })).toBe('Maya');
		expect(byWhom({ Actor: { kind: 'user', id: 1 }, Data: { approved_by: 'owner', asked_by: ['Maya'] } })).toBe('owner');
		expect(byWhom({ Actor: { kind: 'job', id: 9 } })).toBe('Background job');
		expect(byWhom({ Actor: { kind: 'system' } })).toBe('HolyArrs');
	});
	it('names days', () => {
		const now = new Date(2026, 9, 27, 12);
		expect(dayLabel('2026-10-27', now)).toBe('Today');
		expect(dayLabel('2026-10-26', now)).toBe('Yesterday');
		expect(dayLabel('2026-10-25', now)).toBe('Sun, Oct 25');
		expect(dayLabel('2025-10-25', now)).toContain('2025');
	});
	it('only links titles still in the Library', () => {
		const t = new Map([[5, {}]]);
		expect(titleHref(5, t)).toBe('/library/5');
		expect(titleHref(6, t)).toBeUndefined();
		expect(titleHref(null, t)).toBeUndefined();
	});
});

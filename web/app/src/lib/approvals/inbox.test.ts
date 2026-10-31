import { describe, expect, it } from 'vitest';
import { askedByOne, askers, movieRelease, readInfo, runtimeShort } from './inbox.ts';

const now = new Date('2026-10-27T12:00:00Z');

describe('requests inbox', () => {
	it('says a runtime and whether a movie is out', () => {
		expect(runtimeShort(130)).toBe('2h 10m');
		expect(runtimeShort(48)).toBe('48m');
		expect(runtimeShort(120)).toBe('2h');
		expect(movieRelease({ dates: { theaters: '2019-10-11', digital: '2020-01-14' } }, now)).toEqual({ words: 'out now', notOut: '' });
		expect(movieRelease({ dates: { theaters: '2026-10-01', digital: '2026-12-20' } }, now)).toEqual({ words: 'in cinemas only', notOut: '' });
		expect(movieRelease({ dates: { theaters: '2027-03-03' } }, now)).toEqual({ words: 'in cinemas Mar 3, 2027', notOut: 'Not out yet · in cinemas Mar 3, 2027' });
		expect(movieRelease({ dates: {}, status: 'Released' }, now).words).toBe('out now');
		expect(movieRelease(null, now).words).toBe('');
	});

	it('reads the metadata parts it needs', () => {
		const i = readInfo({ seasons: [{ number: 1, episode_count: 10, year: 2020 }], dates: { first_air: '2020-08-14', x: 3 }, status: 'Returning Series', original_language: 'en' });
		expect(i).toEqual({ seasons: [{ number: 1, episode_count: 10, year: 2020 }], dates: { first_air: '2020-08-14' }, status: 'Returning Series', originalLanguage: 'en' });
		expect(readInfo({})).toEqual({ seasons: [], dates: {}, status: undefined, originalLanguage: undefined });
	});

	it('lists the people who asked and filters by one', () => {
		const maya = { id: 2, name: 'Maya' }, leo = { id: 3, name: 'Leo' };
		const asks = [{ asked_by: maya, people: [{ ...maya, asked_at: '' }] }, { asked_by: leo, people: [{ ...leo, asked_at: '' }, { ...maya, asked_at: '' }] }, { asked_by: maya, people: [] }];
		expect(askers(asks)).toEqual([{ id: 3, name: 'Leo', n: 1 }, { id: 2, name: 'Maya', n: 2 }]);
		expect(asks.filter((a) => askedByOne(a, 2)).length).toBe(3);
	});

});

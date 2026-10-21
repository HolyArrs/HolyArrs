import { describe, expect, it } from 'vitest';
import { awardsTip, cleanWikiFacts, loadWikiFacts } from './wiki-facts.ts';

describe('wiki facts', () => {
	it('keeps badges, awards and a licensed Wikipedia bio', () => {
		const f = cleanWikiFacts({
			badges: ['Oscar winner', '3 Emmys', 7],
			awards: [{ name: 'Academy Award for Best Actor', family: 'oscar', won: true, year: 1995, for_work: 'Forrest Gump' }, { name: '' }],
			born: '1956-07-09',
			bio: { title: 'Tom Hanks', extract: 'An actor.', url: 'https://en.wikipedia.org/wiki/Tom_Hanks', license: 'CC BY-SA 4.0', license_url: 'https://creativecommons.org/licenses/by-sa/4.0/' },
		});
		expect(f.badges).toEqual(['Oscar winner', '3 Emmys']);
		expect(f.awards).toHaveLength(1);
		expect(f.born).toBe('1956-07-09');
		expect(f.bio?.url).toBe('https://en.wikipedia.org/wiki/Tom_Hanks');
	});

	it('drops a bio without a Wikipedia link or license', () => {
		expect(cleanWikiFacts({ bio: { extract: 'x', url: 'https://evil.test/', license: 'CC BY-SA 4.0' } }).bio).toBeNull();
		expect(cleanWikiFacts({ bio: { extract: 'x', url: 'https://en.wikipedia.org/wiki/X' } }).bio).toBeNull();
		expect(cleanWikiFacts(null)).toEqual({ badges: [], awards: [], bio: null });
	});

	it('lists major awards first for the hover', () => {
		const tip = awardsTip([
			{ name: 'Academy Award for Best Picture', family: 'oscar', won: true, year: 1995, for_work: '' },
			{ name: 'Saturn Award', family: '', won: true, year: 1995, for_work: '' },
			{ name: 'Golden Globe Award for Best Actor', family: 'golden_globe', won: false, year: 0, for_work: '' },
		]);
		expect(tip).toBe('Won: Academy Award for Best Picture (1995)\nNominated: Golden Globe Award for Best Actor');
	});

	it('asks once per id and answers empty on failure', async () => {
		let asked = 0;
		const get = <T>(path: string) => {
			asked++;
			return path === '/wiki/movie/13' ? Promise.resolve({ badges: ['6 Oscars'] } as T) : Promise.reject(new Error('down'));
		};
		expect((await loadWikiFacts(get, 'movie', '13')).badges).toEqual(['6 Oscars']);
		await loadWikiFacts(get, 'movie', '13');
		expect(asked).toBe(1);
		expect((await loadWikiFacts(get, 'person', '2')).bio).toBeNull();
		expect((await loadWikiFacts(get, 'person', '0x')).badges).toEqual([]);
	});
});

import { describe, expect, it } from 'vitest';
import { awardsTip, cleanWikiFacts, licenseURL, loadWikiFacts, wikipediaURL } from './wiki-facts.ts';

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

	it('never keeps a script or off-site link', () => {
		const f = cleanWikiFacts({ bio: { extract: 'x', url: 'https://de.wikipedia.org/wiki/X', license: 'CC BY-SA 4.0', license_url: 'javascript:alert(1)' } });
		expect(f.bio?.license_url).toBe('https://creativecommons.org/licenses/by-sa/4.0/');
		expect(f.bio?.url).toBe('https://de.wikipedia.org/wiki/X');
		for (const bad of ['javascript:alert(1)', 'http://en.wikipedia.org/wiki/X', 'https://en.wikipedia.org.evil.test/', 'https://evil.test/?en.wikipedia.org']) expect(wikipediaURL(bad)).toBe('');
		expect(licenseURL('https://creativecommons.org/licenses/by-sa/3.0/')).toBe('https://creativecommons.org/licenses/by-sa/3.0/');
		expect(licenseURL('https://creativecommons.org.evil.test/')).toBe('https://creativecommons.org/licenses/by-sa/4.0/');
	});

	it('keeps a person photo from the artwork cache or TVmaze only', () => {
		expect(cleanWikiFacts({ image: '/api/v1/artwork/img/person/287/500.webp?v=3f' }).image).toBe('/api/v1/artwork/img/person/287/500.webp?v=3f');
		expect(cleanWikiFacts({ image: 'https://static.tvmaze.com/uploads/images/medium_portrait/1/1.jpg' }).image).toContain('static.tvmaze.com');
		expect(cleanWikiFacts({ image: 'https://holyarrs-api.coderluii.dev/v1/img/person/287/500.webp' }).image).toBeUndefined();
		expect(cleanWikiFacts({ image: '/api/v1/artwork/img/person/../x.webp' }).image).toBeUndefined();
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

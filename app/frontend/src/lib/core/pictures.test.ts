import { afterEach, describe, expect, it } from 'vitest';
import { forgetPictures, pictureFor, pictureKey, pictureOf, registerPictures, versioned, withVersion } from './pictures.ts';
import { createClient } from './api.ts';
import { heroPictures } from '../discover/hero-prefetch.ts';

const al = '21159e3f-172e-43f6-aa7d-8e06a81fea49';
const ar = 'a74b1b7f-71a5-4011-9441-d0b5e4122711';

describe('picture records', () => {
	afterEach(() => forgetPictures());
	it('keeps sane records and finds them by address', () => {
		registerPictures({
			[`album:${al}`]: { v: 'c1', w: 1200, h: 1200, color: '#a0b0c0' },
			[`logo:${ar}`]: { v: 'l1', w: 800, h: 310, color: 'red' },
			[`artist:${ar}`]: { v: 'bad version', w: 1, h: 1 },
			'album:../x': { v: 'x', w: 1, h: 1 }
		});
		expect(pictureKey(`/holyarrs/api/v1/music-trending/art/album/${al}?size=500`)).toBe(`album:${al}`);
		expect(pictureOf(`/api/v1/music-trending/art/album/${al}?size=500`)?.color).toBe('#a0b0c0');
		expect(pictureFor('logo', ar)?.color).toBe('');
		expect(pictureFor('artist', ar)).toBeUndefined();
		expect(pictureOf('/api/v1/artwork/tmdb/w342/a.jpg')).toBeUndefined();
	});
	it('asks the versioned address once a record is known', () => {
		const p = `/api/v1/music-trending/art/album/${al}`;
		expect(withVersion(p)).toBe(p);
		registerPictures({ [`album:${al}`]: { v: 'c1', w: 500, h: 500, color: '' } });
		expect(withVersion(p)).toBe(`${p}?pv=c1`);
		expect(withVersion(`${p}?pv=c0`)).toBe(`${p}?pv=c0`);
		const photo = `/api/v1/music-trending/art/artist/${ar}`;
		expect(versioned(photo)).toBe(`${photo}?v=4`);
		registerPictures({ [`artist:${ar}`]: { v: 'p1', w: 1200, h: 1200, color: '' } });
		expect(versioned(photo)).toBe(`${photo}?pv=p1`);
	});
	it('takes the records from any answer that carries them', async () => {
		const api = createClient(async () => new Response(JSON.stringify({ pictures: { [`album:${al}`]: { v: 'c9', w: 500, h: 500, color: '#010203' } }, rows: [] }), { status: 200 }));
		await api.get('/explore/music');
		expect(pictureFor('album', al)?.v).toBe('c9');
	});
});

describe('hero ahead', () => {
	afterEach(() => forgetPictures());
	it('preloads the exact hero addresses of stored pictures only', () => {
		expect(heroPictures(`/music/album/${al}?artist=${ar}`, 2)).toEqual([]);
		registerPictures({
			[`album:${al}`]: { v: 'c1', w: 1200, h: 1200, color: '' },
			[`wide:${ar}`]: { v: 'w1', w: 1920, h: 1080, color: '' }
		});
		expect(heroPictures(`/music/album/${al}?artist=${ar}`, 2)).toEqual([`/api/v1/music-trending/art/album/${al}?pv=c1&size=1200`]);
		expect(heroPictures(`/music/album/${al}`, 1)).toEqual([`/api/v1/music-trending/art/album/${al}?pv=c1&size=500`]);
		expect(heroPictures(`/music/artist/${ar}`)).toEqual([`/api/v1/music-trending/art/wide/${ar}?pv=w1`]);
		expect(heroPictures('/title/movie/603')).toEqual([]);
	});
});

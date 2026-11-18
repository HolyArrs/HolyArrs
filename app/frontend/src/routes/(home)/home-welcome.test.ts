import { describe, expect, it } from 'vitest';
import { createHomeOverview } from './home-overview.svelte.ts';
import { setupBlocker, welcomeRows } from './home.ts';
import type { ServerRow } from '#lib/discover/index.ts';

// A fresh install: GET /home answers only the welcome row.
function fresh(over: Partial<{ owner: boolean; manager: boolean; canAsk: boolean }> = {}) {
	const o = { owner: true, manager: true, canAsk: true, ...over };
	return createHomeOverview({ manager: o.manager, kid: false, home: null, now: new Date(), rows: ['welcome'], owner: o.owner, canAsk: o.canAsk, libCards: [{ big: 0 }] as never, showDisk: false, fetchingAll: [], waiting: [] } as never);
}

const item = (id: number, kind = 'movie') => ({ kind, tmdb_id: id, name: `T${id}`, poster: `/p${id}.jpg`, backdrop: `/b${id}.jpg` });
const row = (id: string, n: number, kind = 'movie'): ServerRow => ({ id, type: id, title: id, items: Array.from({ length: n }, (_, i) => item(i + 1, kind)) as never });

describe('Home before setup', () => {
	it('shows the Home grid on a fresh install, with a Library place that points to setup and no Needs attention', () => {
		const v = fresh();
		expect(v.showOverview).toBe(true);
		const stats = v.places.map((p) => p.stat);
		expect(stats).toContain('setup');
		expect(stats).not.toContain('att');
		// An empty Library count would say the same as the setup place: only one of them shows.
		expect(stats).not.toContain('lib');
		expect(stats).toEqual(expect.arrayContaining(['explore', 'popular', 'search']));
		// Every place is filled.
		expect(v.places.every((p) => p.stat || p.pic)).toBe(true);
	});

	it('gives a member in an empty household the grid without the setup place', () => {
		const v = fresh({ owner: false, manager: false });
		expect(v.showOverview).toBe(true);
		expect(v.places.map((p) => p.stat)).not.toContain('setup');
	});

	it('keeps two picture rows from Explore that have enough movies and shows', () => {
		const out = welcomeRows([row('few', 2), row('music', 8, 'artist'), row('trending', 8), row('week', 6, 'show'), row('third', 8)]);
		expect(out.map((r) => r.id)).toEqual(['trending', 'week']);
		expect(out[0].cards).toHaveLength(8);
	});

	it('sends Add to the first setup step still missing, never to Search', () => {
		const step = (href: string, done: boolean) => ({ href, done });
		expect(setupBlocker([step('/settings/library-folders', true), step('/settings/sources', false), step('/settings/downloaders', false), step('/search', false)])).toBe('/settings/sources');
		expect(setupBlocker([step('/settings/library-folders', true), step('/settings/downloaders', true), step('/search', false)])).toBe('');
	});
});

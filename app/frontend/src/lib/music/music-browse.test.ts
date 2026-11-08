import { fetchMusicPage } from './music-trending.ts';
import { describe, expect, it } from 'vitest';

describe('music browse pages', () => {
	it('reads genre pages from the genre route and other kinds from the browse route', async () => {
		const seen: string[] = [];
		const client = { get: async <T,>(path: string) => { seen.push(path); return { rows: [] } as T; } };
		await fetchMusicPage('genre', 'hip-hop', 'US', client);
		await fetchMusicPage('mood', 'chill', 'US', client);
		await fetchMusicPage('season', 'halloween', 'global', client);
		expect(seen[0]).toBe('/music/genre/hip-hop?region=US');
		expect(seen[1]).toBe('/music/browse/mood/chill?region=US');
		expect(seen[2]).toBe('/music/browse/season/halloween?region=global');
	});
});

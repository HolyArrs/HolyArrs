import MusicBrowseSections from './MusicBrowseSections.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';

const sections = [
	{ id: 'genres', title: 'Genres', tiles: [{ key: 'genre:pop', name: 'Pop', href: '/music/genre/pop', hue: 3, covers: ['/api/v1/music-trending/art/album/a', '/api/v1/music-trending/art/album/b'] }] },
	{ id: 'moods', title: 'Moods', tiles: [{ key: 'mood:sleep', name: 'Sleep', href: '/music/mood/sleep', hue: 10, covers: [] }] }
];

describe('MusicBrowseSections', () => {
	it('titles each section and links each tile with covers to its page', () => {
		const { body } = render(MusicBrowseSections, { props: { sections } });
		expect(body).toContain('>Genres</h2>');
		expect(body).toContain('>Moods</h2>');
		expect(body).toMatch(/href="[^"]*\/music\/genre\/pop"/);
	});
	it('shows the first cover, and leaves out a tile that has none (never a placeholder)', () => {
		const { body } = render(MusicBrowseSections, { props: { sections } });
		expect(body).toContain('art/album/a');
		expect(body).not.toContain('art/album/b');
		expect(body).not.toContain('/music/mood/sleep');
		expect(body).not.toContain('art-placeholder');
		expect(body).not.toContain('>S</span>');
	});
});

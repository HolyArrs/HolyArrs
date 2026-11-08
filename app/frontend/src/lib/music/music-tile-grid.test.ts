import MusicTileGrid from './MusicTileGrid.svelte';
import MusicBrowseGrid from './MusicBrowseGrid.svelte';
import { TILE_PALETTE } from './tile-palette.ts';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';

const art = (id: string) => `/api/v1/music-trending/art/album/${id}`;
const tile = (key: string, covers: string[] = [art(key)]) => ({ key, name: key.split(':')[1], href: `/music/${key.replace(':', '/')}`, covers });
const keys = ['genre:pop', 'genre:hip-hop', 'mood:chill', 'genre:rock', 'season:halloween', 'genre:latin', 'decade:1990s', 'mood:party', 'genre:rnb', 'activity:driving', 'genre:country', 'mood:love', 'decade:2000s', 'genre:dance', 'genre:indie', 'genre:metal', 'genre:kpop', 'genre:afro'];
const hexOf = (body: string) => [...body.matchAll(/--tile:\s*(#[0-9a-f]{6})/g)].map((m) => m[1]);
const lis = (body: string) => body.match(/<li[^>]*>/g) ?? [];

describe('MusicTileGrid', () => {
	it('shows the tile as a colour, the name, and one album cover', () => {
		const { body } = render(MusicTileGrid, { props: { tiles: [tile('genre:pop')] } });
		expect(body).toContain('data-testid="music-browse-tile"');
		expect(body).toContain('>pop</strong>');
		expect(body).toContain('art/album/genre:pop');
		expect(body).toContain('size=250');
		expect(body).toContain('loading="eager"');
		expect(hexOf(body)).toEqual([TILE_PALETTE.find((c) => c.name === 'pink')!.hex]);
	});
	it('never shows a placeholder or initials', () => {
		const { body } = render(MusicTileGrid, { props: { tiles: [tile('genre:pop'), tile('genre:folk', [])] } });
		expect(body).not.toContain('art-placeholder');
	});
	it('leaves out a tile that has no cover, and one whose covers are all empty', () => {
		const { body } = render(MusicTileGrid, { props: { tiles: [tile('genre:pop'), tile('genre:folk', []), tile('genre:metal', ['', ''])] } });
		expect(body).toContain('/music/genre/pop');
		expect(body).not.toContain('/music/genre/folk');
		expect(body).not.toContain('/music/genre/metal');
	});
	it('shows the first cover of a tile and keeps the others for a failure', () => {
		const { body } = render(MusicTileGrid, { props: { tiles: [tile('genre:pop', [art('one'), art('two')])] } });
		expect(body).toContain('album/one');
		expect(body).not.toContain('album/two');
	});
	it('gives no two tiles on a grid the same colour', () => {
		const { body } = render(MusicTileGrid, { props: { tiles: keys.slice(0, 16).map((k) => tile(k)) } });
		const hex = hexOf(body);
		expect(hex).toHaveLength(16);
		expect(new Set(hex).size).toBe(16);
	});
	it('holds back the tiles past its rows, hidden, so a tile that drops out can be replaced', () => {
		const { body } = render(MusicTileGrid, { props: { tiles: keys.map((k) => tile(k)), rows: 2 } });
		const items = lis(body);
		expect(items).toHaveLength(keys.length);
		const shown = items.filter((l) => !l.includes('hidden'));
		expect(shown.length).toBeGreaterThanOrEqual(8);
		expect(shown.length).toBeLessThan(keys.length);
		expect(hexOf(body)).toHaveLength(shown.length);
	});
	it('shows every tile when it has no rows limit', () => {
		const { body } = render(MusicTileGrid, { props: { tiles: keys.map((k) => tile(k)) } });
		expect(lis(body).filter((l) => !l.includes('hidden'))).toHaveLength(keys.length);
	});
	it('draws ghost tiles while loading', () => {
		const { body } = render(MusicTileGrid, { props: { tiles: [], ghosts: 6 } });
		expect(body).toContain('data-testid="music-tile-ghosts"');
		expect(body.match(/class="ghost sk[ "]/g)).toHaveLength(6);
	});
});

describe('MusicBrowseGrid', () => {
	it('titles the grid and links Browse all', () => {
		const { body } = render(MusicBrowseGrid, { props: { tiles: [{ key: 'genre:pop', name: 'Pop', href: '/music/genre/pop', hue: 3, cover: art('a') }], allHref: '/search?kind=music', title: 'Explore more' } });
		expect(body).toContain('>Explore more</h2>');
		expect(body).toContain('Browse all');
		expect(body).toContain('href="/search?kind=music"');
		expect(body).toContain('art/album/a');
	});
	it('takes the server cover or a list to try, and leaves out a tile without', () => {
		const { body } = render(MusicBrowseGrid, { props: { tiles: [{ key: 'genre:pop', name: 'Pop', href: '/p', cover: art('a') }, { key: 'genre:rock', name: 'Rock', href: '/r', covers: [art('b')] }, { key: 'genre:metal', name: 'Metal', href: '/m', cover: '' }], allHref: '/x' } });
		expect(body).toContain('href="/p"');
		expect(body).toContain('href="/r"');
		expect(body).not.toContain('href="/m"');
	});
});

// The cover is about half the tile, turned 25 degrees, at the bottom right and cut off; the grid is auto-fill from 190 px with two columns on a phone; motion is switched off on request.
describe('MusicTileGrid style', () => {
	const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'MusicTileGrid.svelte'), 'utf8');
	const rule = (sel: string) => src.split('\n').find((l) => l.trimStart().startsWith(`${sel} {`)) ?? '';
	it('lays tiles out in an auto-fill grid of 16:9 tiles', () => {
		expect(rule('.mtg')).toMatch(/repeat\(auto-fill, minmax\(var\(--mtg-min\), 1fr\)\)/);
		expect(rule('.mtg')).toMatch(/--mtg-min: 190px/);
		expect(src).toMatch(/\.tile \{[^}]*aspect-ratio: 16 \/ 9/);
	});
	it('uses two columns on a phone', () => {
		expect(src).toMatch(/@media \(max-width: 559px\) \{ \.mtg \{ grid-template-columns: repeat\(2, minmax\(0, 1fr\)\)/);
	});
	it('puts the cover at the bottom right, turned 25 degrees, about half the tile wide', () => {
		const r = src.match(/\.cover \{([^}]+)\}/)?.[1] ?? '';
		expect(r).toMatch(/width: 50%/);
		expect(r).toMatch(/rotate\(25deg\)/);
		expect(r).toMatch(/right: -\d+%/);
		expect(r).toMatch(/bottom: -\d+%/);
	});
	it('names the tile top left in bold white', () => {
		expect(src).toMatch(/\.tile \{[^}]*color: #fff/);
		const r = src.match(/strong \{([^}]+)\}/)?.[1] ?? '';
		expect(r).toMatch(/left: \d+%/);
		expect(r).toMatch(/top: \d+%/);
		expect(r).toMatch(/font: 700 /);
	});
	it('honours reduced motion', () => {
		expect(src).toMatch(/prefers-reduced-motion: reduce/);
	});
});

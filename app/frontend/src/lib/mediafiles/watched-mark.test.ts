import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import WatchedMark from './WatchedMark.svelte';
import LibraryPoster from './LibraryPoster.svelte';

const poster = (watched: unknown) => ({ title: 'Glasshouse', year: 2016, href: '/t/1', status: null, bar: { tone: 'ok' as const, fill: 1 }, kept: false, kind: 'show' as const, watched: watched as never });

describe('WatchedMark', () => {
	it('draws a check badge for a watched title', () => {
		const { body } = render(WatchedMark, { props: { mark: { state: 'watched', seen: 8, of: 8 } } });
		expect(body).toContain('class="wm done');
		expect(body).toContain('aria-hidden="true"');
		expect(body).not.toContain('<circle');
	});
	it('draws a ring filled by the share watched for a partly watched title', () => {
		const { body } = render(WatchedMark, { props: { mark: { state: 'partly', seen: 3, of: 12 } } });
		expect(body).toContain('class="wm part');
		expect(body).toMatch(/pathLength="100"[^>]*stroke-dasharray="25 100"/);
	});
	it('keeps a sliver of ring when how much aired is not known', () => {
		const { body } = render(WatchedMark, { props: { mark: { state: 'partly', seen: 2, of: 0 } } });
		expect(body).toMatch(/stroke-dasharray="10 100"/);
	});
	it('draws a thin bar for the phone rows, full or part filled', () => {
		expect(render(WatchedMark, { props: { mark: { state: 'watched', seen: 1, of: 1 }, look: 'bar' } }).body).toContain('width: 100%');
		const part = render(WatchedMark, { props: { mark: { state: 'partly', seen: 1, of: 4 }, look: 'bar' } }).body;
		expect(part).toContain('class="wbar');
		expect(part).toContain('width: 25%');
	});
	it('keeps the phone bar seen on a light poster: a dark track under it', () => {
		const src = readFileSync(new URL('./WatchedMark.svelte', import.meta.url), 'utf8');
		expect(src).toMatch(/\.wbar \{[^}]*background: rgb\(20 20 22 \/ 72%\)/);
	});
	it('draws nothing when nothing is watched', () => {
		expect(render(WatchedMark, { props: { mark: null } }).body).not.toContain('wm');
	});
});

describe('LibraryPoster watched mark', () => {
	it('says Watched in the poster link and shows the badge', () => {
		const { body } = render(LibraryPoster, { props: poster({ state: 'watched', seen: 8, of: 8 }) });
		expect(body).toMatch(/aria-label="Glasshouse, 2016, In the Library, Watched"/);
		expect(body).toContain('class="wm done');
	});
	it('says how much is watched for a partly watched show', () => {
		const { body } = render(LibraryPoster, { props: poster({ state: 'partly', seen: 3, of: 8 }) });
		expect(body).toContain('Watched 3 of 8"');
		expect(body).toContain('class="wm part');
	});
	it('leaves an unwatched poster as it was', () => {
		const { body } = render(LibraryPoster, { props: poster(null) });
		expect(body).not.toContain('class="wm');
		expect(body).not.toContain('Watched');
	});
});

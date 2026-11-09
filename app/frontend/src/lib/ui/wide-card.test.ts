import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import WideCard from './WideCard.svelte';

const label = (body: string) => body.match(/aria-label="([^"]*)"/)?.[1]?.replace(/&amp;/g, '&');

describe('WideCard title card name', () => {
	it('reads a strip that repeats the line under the card once', () => {
		const { body } = render(WideCard, { props: { cover: true, title: 'Severance', kind: 'show', typed: true, href: '/library/1', subtitle: 'S2 · 3 new episodes', fresh: { kind: 'new_episode', text: 'S2 · 3 new episodes' }, row: 'home_new_season' } });
		expect(label(body)).toBe('Severance, Show, S2 · 3 new episodes');
	});
	it('reads an artist strip that ends with the line once', () => {
		const { body } = render(WideCard, { props: { cover: true, title: 'Copper Wren', kind: 'artist', typed: true, href: '/library/2', subtitle: '2 new albums', fresh: { kind: 'recently_added', text: 'Copper Wren · 2 new albums' }, row: 'home_following' } });
		expect(label(body)).toBe('Copper Wren, 2 new albums');
	});
	it('keeps a strip that says something else', () => {
		const { body } = render(WideCard, { props: { cover: true, title: 'Driftwood Moon', kind: 'movie', typed: true, href: '/library/3', subtitle: '2024', fresh: { kind: 'coming', text: 'Coming Friday' } } });
		expect(label(body)).toBe('Driftwood Moon, Movie, 2024, Coming Friday');
	});
	it('keeps the strip when there is no line', () => {
		const { body } = render(WideCard, { props: { cover: true, title: 'Driftwood Moon', kind: 'movie', href: '/library/3', fresh: { kind: 'coming', text: 'Coming Friday' } } });
		expect(label(body)).toBe('Driftwood Moon, Coming Friday');
	});
});

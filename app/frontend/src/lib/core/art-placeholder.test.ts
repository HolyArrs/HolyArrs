import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
import { inkOn, placeholderHue, placeholderText } from './art-placeholder.ts';

describe('picture placeholder', () => {
	it('shows two initials, a short number, or nothing', () => {
		expect(placeholderText('Frank Ocean')).toBe('FO');
		expect(placeholderText('SZA')).toBe('S');
		expect(placeholderText('The Dark Side of the Moon')).toBe('TD');
		expect(placeholderText('1989')).toBe('1989');
		expect(placeholderText('Abbey Road (Remastered)')).toBe('AR');
		expect(placeholderText('...')).toBe('');
	});
	it('takes a stable hue from the name', () => {
		expect(placeholderHue('Frank Ocean')).toBe(placeholderHue('frank ocean'));
		expect(placeholderHue('Frank Ocean')).not.toBe(placeholderHue('SZA'));
	});
	it('reads the main colour for its ink', () => {
		expect(inkOn('#f4e9d0')).toBe('dark');
		expect(inkOn('#1a2b3c')).toBe('light');
		expect(inkOn('red')).toBe('');
		expect(inkOn(null)).toBe('');
	});
	it('draws on the picture main colour when known', () => {
		const { body } = render(ArtPlaceholder, { props: { name: 'Daniel Caesar', shape: 'square', color: '#203040' } });
		expect(body).toContain('--c: #203040');
		expect(body).toMatch(/class="aph square[^"]*tinted/);
		expect(body).toMatch(/ink-light/);
		const plain = render(ArtPlaceholder, { props: { name: 'Daniel Caesar', color: 'javascript:x' } }).body;
		expect(plain).not.toContain('tinted');
	});
	it('renders the initials in the slot shape', () => {
		const { body } = render(ArtPlaceholder, { props: { name: 'Daniel Caesar', shape: 'circle' } });
		expect(body).toContain('DC');
		expect(body).toMatch(/class="aph circle/);
	});
});

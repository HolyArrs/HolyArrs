import { describe, expect, it } from 'vitest';
import { valueLabel, unitWord } from './library-stats.ts';

const t = (k: string, v?: Record<string, string | number>) => (v ? `${k}:${v.n}` : k);

describe('library stats labels', () => {
	it('names known values and words the special ones', () => {
		expect(valueLabel('video_codec', 'hevc', t)).toBe('HEVC (H.265)');
		expect(valueLabel('hdr', 'sdr', t)).toBe('SDR');
		expect(valueLabel('container', 'matroska', t)).toBe('MKV');
		expect(valueLabel('resolution', '1080p', t)).toBe('1080p');
		expect(valueLabel('hdr', 'other', t)).toBe('library.stats.value.other');
		expect(valueLabel('resolution', 'unchecked', t)).toBe('library.stats.value.unchecked');
		expect(valueLabel('audio_codec', 'unknown', t)).toBe('library.stats.value.unknown');
	});
	it('picks one or other', () => {
		expect(unitWord('movie', 1, t)).toBe('library.stats.unit.movie.one:1');
		expect(unitWord('play', 3, t)).toBe('library.stats.unit.play.other:3');
	});
});

describe('library stats page layout', async () => {
	const { readFileSync } = await import('node:fs');
	const src = readFileSync(new URL('../../routes/library/stats/+page.svelte', import.meta.url), 'utf8');
	it('grows bars from the start side in right-to-left languages', () => {
		expect(src).toMatch(/:global\(\[dir='rtl'\]\) \.bar i \{ transform-origin: right; \}/);
	});
	it('indents the phone counts row on the start side, so Arabic lines up under the name', () => {
		expect(src).not.toContain('padding-left');
		expect(src).toContain('padding-inline-start: 48px');
	});
	it('keeps big totals on one line on phones instead of breaking inside the number', () => {
		expect(src).not.toMatch(/\.tile b \{[^}]*overflow-wrap: anywhere/);
		expect(src).toMatch(/\.tile b \{[^}]*white-space: nowrap/);
	});
});

describe('library stats fact rows', async () => {
	const { readFileSync } = await import('node:fs');
	const src = readFileSync(new URL('../../routes/library/stats/+page.svelte', import.meta.url), 'utf8');
	it('shares one column grid per card so bars start and end at the same place', () => {
		expect(src).toMatch(/\.fact li \{[^}]*grid-template-columns: subgrid/);
		expect(src).toMatch(/\.fact ul \{[^}]*minmax\(3rem, 1fr\)/);
	});
	it('stacks the bar under the name whenever the card is narrow, not only on phones', () => {
		expect(src).toMatch(/\.fact \{[^}]*container-type: inline-size/);
		expect(src).toMatch(/@container \(max-width: 440px\)/);
	});
});

describe('library stats numbers', async () => {
	const { readFileSync } = await import('node:fs');
	const src = readFileSync(new URL('../../routes/library/stats/+page.svelte', import.meta.url), 'utf8');
	it('lines numbers up on the end edge of each card', () => {
		expect(src).toMatch(/\.num \{[^}]*justify-self: end/);
	});
	it('keeps "9086 TB" in reading order in Arabic', () => {
		expect(src).toMatch(/\.num, \.tile b \{ direction: ltr; unicode-bidi: isolate; \}/);
	});
});

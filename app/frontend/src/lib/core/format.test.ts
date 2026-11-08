import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ago, agoLong, bytes, date, duration, folderHealth, greeting, percent, tone, words } from './format.ts';

describe('format', () => {
	it('calendar days keep their day in every time zone', () => {
		expect(date('2023-11-21T00:00:00Z')).toContain('21');
		expect(date('2023-11-21')).toContain('21');
		expect(date('')).toBe('');
	});

	it('sizes', () => {
		expect(bytes(0)).toBe('0 B');
		expect(bytes(512)).toBe('512 B');
		expect(bytes(1536)).toBe('1.5 KB');
		expect(bytes(5 * 1024 ** 3)).toBe('5.0 GB');
		expect(bytes(300 * 1024 ** 2)).toBe('300 MB');
	});
	it('durations', () => {
		expect(duration(null)).toBe('');
		expect(duration(42)).toBe('42s');
		expect(duration(600)).toBe('10m');
		expect(duration(3 * 3600 + 120)).toBe('3h 2m');
		expect(duration(3 * 86400)).toBe('3d 0h');
	});
	it('percent from a 0..1 progress', () => {
		expect(percent(0.4567)).toBe(45.7);
		expect(percent(2)).toBe(100);
		expect(percent(undefined)).toBe(0);
	});
	it('ago', () => {
		const now = Date.parse('2026-10-13T12:00:00Z');
		expect(ago('2026-10-13T11:59:50Z', now)).toBe('just now');
		expect(ago('2026-10-13T11:30:00Z', now)).toBe('30 min ago');
		expect(ago('2026-10-13T09:00:00Z', now)).toBe('3 h ago');
		expect(ago('bad', now)).toBe('');
	});
	it('states', () => {
		expect(tone('in_library')).toBe('ok');
		expect(tone('stalled')).toBe('fail');
		expect(tone('downloading')).toBe('wait');
		expect(tone('whatever')).toBe('idle');
		expect(words('needs_a_decision')).toBe('Needs a decision');
		expect(folderHealth('no_hardlink')).toContain('copied');
		expect(greeting(9)).toBe('Good morning');
		expect(greeting(20)).toBe('Good evening');
	});
});

describe('tokens.css', () => {
	const here = dirname(fileURLToPath(import.meta.url));
	const ours = readFileSync(join(here, 'tokens.css'), 'utf8');
	const block = (selector: RegExp) => ours.match(selector)?.[1] ?? '';
	const declarations = (css: string) => Object.fromEntries([...css.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]));
	const shared = declarations(block(/:root\s*\{([^}]+)\}/));
	const light = declarations(block(/:root, \[data-theme='light'\]\s*\{([^}]+)\}/));
	const dark = declarations(block(/:root\[data-theme='dark'\], \[data-theme='dark'\]\s*\{([^}]+)\}/));
	const aliases = declarations(block(/:root, \[data-theme\]\s*\{([^}]+)\}/));
	const base = { ...shared, ...light, ...aliases };

	function luminance(hex: string) {
		if (!/^#[\da-f]{6}$/i.test(hex)) throw Error(`Expected an opaque color: ${hex}`);
		const rgb = [1, 3, 5].map((start) => parseInt(hex.slice(start, start + 2), 16) / 255).map((value) => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4);
		return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722;
	}

	it('retains shared component tokens and touch sizing', () => {
		for (const token of ['--ha-red', '--ha-red-hover', '--ha-red-press', '--ha-red-deep', '--ha-red-soft', '--font-ui', '--font-display', '--font-mono', '--focus-ring', '--sidebar', '--topbar', '--ok', '--wait', '--fail', '--info']) expect(base[token], token).toBeTruthy();
		expect(parseFloat(base['--tap'])).toBeGreaterThanOrEqual(44);
	});
	it('keeps text readable on the shared surfaces in both themes (spec §2)', () => {
		expect(Object.keys(light).length).toBeGreaterThan(10);
		expect(Object.keys(dark).length).toBeGreaterThan(10);
		const pairs: [string, string[], number][] = [
			['--text', ['--bg', '--sidebar-solid', '--surface-1', '--surface-2', '--surface-3'], 4.5],
			['--text-muted', ['--bg', '--sidebar-solid', '--surface-1'], 4.5],
			['--text-faint', ['--bg', '--surface-1'], 3],
			['--accent', ['--bg', '--surface-1'], 3],
			['--accent-text', ['--bg', '--surface-1', '--surface-2'], 4.5],
			['--text-on-red', ['--accent-fill'], 4.5]
		];
		for (const colors of [light, dark]) {
			for (const [fg, bgs, min] of [...pairs, ...['--ok', '--wait', '--fail', '--info'].map((t) => [t, ['--bg', '--surface-1'], 3] as [string, string[], number]), ...['--ok-text', '--wait-text', '--fail-text', '--info-text'].map((t) => [t, ['--bg', '--surface-1', '--surface-2'], 4.5] as [string, string[], number])]) {
				for (const bg of bgs) {
					const values = [luminance(colors[fg]), luminance(colors[bg])].sort((a, b) => b - a);
					expect((values[0] + 0.05) / (values[1] + 0.05), `${fg} on ${bg}`).toBeGreaterThanOrEqual(min);
				}
			}
		}
	});
	it('removes travel and entry staggering for reduced motion', () => {
		const reduced = ours.slice(ours.indexOf('@media (prefers-reduced-motion: reduce)'));
		expect(reduced).toMatch(/--lift:\s*0px/);
		expect(reduced).toMatch(/--stagger:\s*0ms/);
		expect(reduced).toMatch(/--dur-page:\s*1ms/);
		expect(reduced).toMatch(/--press:\s*1/);
	});
});

describe('agoLong', () => {
	const now = Date.parse('2026-10-31T12:00:00Z');
	it('says the unit in full words', () => {
		expect(agoLong('2026-10-24T12:00:00Z', now)).toBe('7 days ago');
		expect(agoLong('2026-10-30T12:00:00Z', now)).toBe('1 day ago');
		expect(agoLong('2026-10-31T11:00:00Z', now)).toBe('1 hour ago');
		expect(agoLong('2026-10-31T11:55:00Z', now)).toBe('5 minutes ago');
		expect(agoLong('2026-10-31T11:59:50Z', now)).toBe('just now');
		expect(agoLong(null, now)).toBe('');
	});
});

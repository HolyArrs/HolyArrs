import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ago, bytes, date, duration, folderHealth, greeting, percent, tone, words } from './format.ts';

describe('format', () => {
	it('calendar days keep their day in every time zone', () => {
		// TMDB release days arrive as midnight UTC; west of UTC they must not show the day before.
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

// The app owns its approved tokens. Check their shared component contract and
// accessibility without depending on an unrelated working copy's design file.
describe('tokens.css', () => {
	const here = dirname(fileURLToPath(import.meta.url));
	const ours = readFileSync(join(here, 'tokens.css'), 'utf8');
	const dark = ours.match(/:root\s*\{([^}]+)\}/)?.[1] ?? '';
	const light = ours.match(/:root\[data-theme=['"]light['"]\]\s*\{([^}]+)\}/)?.[1] ?? '';
	const declarations = (css: string) => Object.fromEntries([...css.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]));
	const base = declarations(dark);

	function luminance(hex: string) {
		if (!/^#[\da-f]{6}$/i.test(hex)) throw Error(`Expected an opaque color: ${hex}`);
		const rgb = [1, 3, 5].map((start) => parseInt(hex.slice(start, start + 2), 16) / 255).map((value) => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4);
		return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722;
	}

	it('retains shared component tokens and touch sizing', () => {
		for (const token of ['--ha-red', '--ha-red-hover', '--ha-red-press', '--ha-red-deep', '--ha-red-soft', '--font-ui', '--font-display', '--font-mono', '--focus-ring', '--sidebar', '--topbar', '--ok', '--wait', '--fail', '--info']) expect(base[token], token).toBeTruthy();
		expect(parseFloat(base['--tap'])).toBeGreaterThanOrEqual(44);
	});
	it('keeps normal text readable on every shared surface in both themes', () => {
		expect(light).not.toBe('');
		for (const colors of [base, { ...base, ...declarations(light) }]) {
			for (const foreground of ['--text', '--text-muted', '--text-faint', '--ok', '--wait', '--fail', '--info']) {
				for (const background of ['--bg', '--sidebar-bg', '--surface-1', '--surface-2', '--surface-3']) {
					const values = [luminance(colors[foreground]), luminance(colors[background])].sort((a, b) => b - a);
					expect((values[0] + 0.05) / (values[1] + 0.05), `${foreground} on ${background}`).toBeGreaterThanOrEqual(4.5);
				}
			}
		}
	});
	it('removes travel and entry staggering for reduced motion', () => {
		const reduced = ours.slice(ours.indexOf('@media (prefers-reduced-motion: reduce)'));
		expect(reduced).toMatch(/--lift:\s*0px/);
		expect(reduced).toMatch(/--stagger:\s*0ms/);
		expect(reduced).toMatch(/--dur-page:\s*0ms/);
	});
});

import { describe, expect, it } from 'vitest';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
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

// The app's tokens.css must be the design system's file, unchanged. The
// design folder lives next to the repo in the HolyArrs workspace; a plain
// clone of the repo has only the app's copy, so the check compares when the
// design folder is there and otherwise checks the copy is the v0.1 file.
describe('tokens.css', () => {
	const here = dirname(fileURLToPath(import.meta.url));
	const ours = readFileSync(join(here, 'tokens.css'), 'utf8');

	function findDesign(): string | null {
		let dir = here;
		for (let i = 0; i < 8; i++) {
			const p = join(dir, 'design', 'system', 'tokens.css');
			if (existsSync(p)) return p;
			const up = resolve(dir, '..');
			if (up === dir) break;
			dir = up;
		}
		return null;
	}

	it('equals design/system/tokens.css', () => {
		const design = findDesign();
		if (design) expect(ours.replace(/\r\n/g, '\n')).toBe(readFileSync(design, 'utf8').replace(/\r\n/g, '\n'));
		else expect(ours).toContain('HolyArrs design tokens, v0.1');
		expect(ours).toContain('--ha-red: #F23A2E');
		expect(ours).toContain(':root[data-theme="light"]');
		expect(ours).toContain('prefers-reduced-motion');
	});
});

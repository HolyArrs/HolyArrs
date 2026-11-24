import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// The page-arrival animation runs on #main. A transform there makes #main the
// box for position:fixed children, so the first-run welcome art (fixed, full
// screen) would shrink to the card height and show a dark band below it.
const html = readFileSync(new URL('../../app.html', import.meta.url), 'utf8');

function keyframes(name: string): string {
	const m = html.match(new RegExp(`@keyframes ${name}\{((?:[^{}]|\{[^}]*\})*)\}`));
	if (!m) throw new Error(`no @keyframes ${name}`);
	return m[1];
}

describe('arrive animation over full-screen art', () => {
	it('uses a fade without transform when #main holds the welcome art', () => {
		const rule = html.match(/html\.hl-arrive #main:has\(\.over-art\)\{animation-name:(\w+)\}/);
		expect(rule, 'override for #main:has(.over-art)').not.toBeNull();
		const frames = keyframes(rule![1]);
		expect(frames).toContain('opacity');
		expect(frames).not.toMatch(/transform|translate|scale|rotate|filter|perspective/);
	});
});

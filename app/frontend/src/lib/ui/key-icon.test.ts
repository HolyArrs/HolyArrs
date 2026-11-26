import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// The key's shaft must start at the ring's edge, not run through it.
const src = readFileSync(new URL('./Icon.svelte', import.meta.url), 'utf8');
const key = src.match(/key: '([^']+)'/)?.[1] ?? '';

describe('key icon', () => {
	it('draws the shaft from the ring edge and fits the box', () => {
		// Ring: starts at its leftmost point, radius r.
		const ring = key.match(/^M([\d.]+) ([\d.]+)a([\d.]+) /);
		const shaft = key.match(/M([\d.]+) ([\d.]+)h([\d.]+)/);
		expect(ring && shaft).toBeTruthy();
		const [left, cy, r] = ring!.slice(1).map(Number);
		const [sx, sy, len] = shaft!.slice(1).map(Number);
		expect(left).toBeGreaterThanOrEqual(2);
		expect(sx).toBeCloseTo(left + 2 * r, 1);
		expect(sy).toBe(cy);
		expect(sx + len).toBeLessThanOrEqual(22);
	});
});

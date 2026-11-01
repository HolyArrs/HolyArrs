import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

const src = readFileSync(new URL('./DlSections.svelte', import.meta.url), 'utf8');

describe('read-only rows', () => {
	it('disables queue and steer menu entries when the row cannot be acted on', () => {
		for (const id of ['dl-queue-', 'dl-steer-']) {
			const line = src.split('\n').find((l) => l.includes(`data-testid="${id}`)) ?? '';
			expect(line).toContain('!r.can_act');
		}
	});
});

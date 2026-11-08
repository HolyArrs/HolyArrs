import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

const src = readFileSync(new URL('./DlPane.svelte', import.meta.url), 'utf8');

describe('matched rows', () => {
	it('link to the title in the Library, but not unmatched outside rows', () => {
		const line = src.split('\n').find((l) => l.includes('data-testid="dl-open-title"')) ?? '';
		expect(line).toContain('/library/${r.title_id}');
		expect(line).toContain('r.linked || !r.outside');
	});
});

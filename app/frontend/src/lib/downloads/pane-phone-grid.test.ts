import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

const css = readFileSync(new URL('./DlPane.svelte', import.meta.url), 'utf8').split('<style>')[1] ?? '';

// The phone sheet hides the poster, so its body must be one column; a later wider rule must not bring the 56px poster column back.
describe('download pane on a phone', () => {
	it('ends with a one-column body at 640px and below', () => {
		const blocks = [...css.matchAll(/@media \(max-width: (\d+)px\) \{([\s\S]*?)\n\t\}/g)];
		const last = blocks.map((m) => ({ w: Number(m[1]), body: m[2] })).filter((b) => b.w >= 640 && /\.pb \{ grid-template-columns/.test(b.body)).pop();
		expect(last?.w).toBe(640);
		expect(last?.body).toMatch(/\.pb \{ grid-template-columns: minmax\(0, 1fr\); \}/);
	});
});

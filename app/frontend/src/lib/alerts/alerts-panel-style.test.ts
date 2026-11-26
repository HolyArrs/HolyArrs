import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';

// The alerts panel's row text, read from the compiled styles.
const css = compile(readFileSync(new URL('./AlertsPanel.svelte', import.meta.url), 'utf8'), { filename: 'AlertsPanel.svelte', css: 'external' }).css?.code.replace(/:where\(\.svelte-[\w-]+\)|\.svelte-[\w-]+/g, '') ?? '';
/** The declarations of the rule for one selector. */
const rule = (sel: string) => css.match(new RegExp(`(?:^|[}\\s])${sel.replace('.', '\\.')}\\s*\\{([^}]*)\\}`))?.[1] ?? '';

describe('Alerts panel row text', () => {
	// The panel is the one place an alert can be read after its toast is gone,
	// so neither line may be cut to one line with "…".
	for (const sel of ['.t', '.b']) {
		it(`${sel} wraps and is never cut off`, () => {
			const r = rule(sel);
			expect(r).toMatch(/overflow-wrap:\s*anywhere/);
			expect(r).not.toMatch(/white-space:\s*nowrap/);
			expect(r).not.toMatch(/text-overflow:\s*ellipsis/);
		});
	}
});

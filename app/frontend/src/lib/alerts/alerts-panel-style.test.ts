import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';

// The alerts panel's styles and markup, read from the component itself.
const src = readFileSync(new URL('./AlertsPanel.svelte', import.meta.url), 'utf8');
const css = compile(src, { filename: 'AlertsPanel.svelte', css: 'external' }).css?.code.replace(/:where\(\.svelte-[\w-]+\)|\.svelte-[\w-]+/g, '') ?? '';
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

describe('Alerts panel height and scrolling', () => {
	const panel = rule('.panel');

	// Many pinned rows must never push the panel past the window: the cap is
	// what makes the rows scroll instead of being cut off at the bottom, so it
	// is written twice: a browser that drops dvh drops only the second line.
	it('caps its height, with a vh line a browser without dvh keeps', () => {
		expect(panel).toMatch(/max-height:\s*min\(560px,\s*calc\(100vh - 32px\)\)/);
		expect(panel).toMatch(/max-height:\s*min\(560px,\s*calc\(100dvh - 32px\)\)/);
	});

	it('is a column, so one child can take the height the head leaves', () => {
		expect(panel).toMatch(/display:\s*flex/);
		expect(panel).toMatch(/flex-direction:\s*column/);
	});

	// One scroll area for both lists, and it may shrink: without min-height: 0
	// a flex child will not go below its content, so it grew past the panel.
	it('scrolls one area that is allowed to shrink', () => {
		const scroll = rule('.scroll');
		expect(scroll).toMatch(/overflow-y:\s*auto/);
		expect(scroll).toMatch(/min-height:\s*0/);
		expect(scroll).toMatch(/flex:\s*1 1 auto/);
	});

	// The regression itself: a list that scrolls on its own sits below the
	// panel's height, so its last rows are the ones cut off and unreachable.
	it('leaves the scrolling to .scroll, not to a list', () => {
		for (const sel of ['.rows', '.pins']) expect(rule(sel)).not.toMatch(/overflow(-y)?:\s*auto|overflow(-y)?:\s*scroll/);
	});

	it('renders both lists inside that one scroll area', () => {
		expect(src).toMatch(/<div class="scroll">\{@render rows\(\)\}<\/div>/);
	});
});

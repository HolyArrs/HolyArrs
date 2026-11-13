import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// The Settings page tabs (Library folders ... Import from other apps) must never sit past the screen edge, on a phone or on a wide screen with long words (de, hu, fi, ar): the row wraps onto more lines.
const src = readFileSync(new URL('../+layout.svelte', import.meta.url), 'utf8');
const style = src.slice(src.indexOf('<style>') + 7, src.indexOf('</style>'));

type Rule = { media: string; body: string };

// Every `.caps { ... }` rule in source order, with the @media query it sits in ('' for none).
function capsRules(css: string): Rule[] {
	const out: Rule[] = [];
	const media: string[] = [];
	let i = 0;
	while (i < css.length) {
		if (css.startsWith('/*', i)) {
			i = css.indexOf('*/', i) + 2;
			continue;
		}
		const open = css.indexOf('{', i);
		const close = css.indexOf('}', i);
		if (close !== -1 && (open === -1 || close < open)) {
			media.pop();
			i = close + 1;
			continue;
		}
		if (open === -1) break;
		const prelude = css.slice(i, open).replace(/\/\*[\s\S]*?\*\//g, '').trim();
		if (prelude.startsWith('@')) {
			media.push(prelude);
			i = open + 1;
			continue;
		}
		const end = css.indexOf('}', open);
		const selectors = prelude.split(',').map((s) => s.trim());
		if (selectors.includes('.caps')) out.push({ media: media.at(-1) ?? '', body: css.slice(open + 1, end) });
		media.push('rule');
		i = open + 1;
	}
	return out;
}

// The value a width ends up with: later rules win, and a rule counts when its @media query matches that width.
function finalValue(rules: Rule[], prop: string, width: number): string | undefined {
	let value: string | undefined;
	for (const r of rules) {
		const max = /max-width:\s*(\d+)px/.exec(r.media);
		const min = /min-width:\s*(\d+)px/.exec(r.media);
		if (r.media && !max && !min) continue;
		if (max && width > Number(max[1])) continue;
		if (min && width < Number(min[1])) continue;
		const m = new RegExp(`(?:^|;)\\s*${prop}:\\s*([^;]+)`).exec(r.body);
		if (m) value = m[1].trim();
	}
	return value;
}

describe('Settings tab row', () => {
	const rules = capsRules(style);
	it('finds the tab row rules', () => {
		expect(rules.length).toBeGreaterThan(0);
	});
	for (const width of [360, 390, 1024, 1440]) {
		it(`wraps at ${width}px instead of hiding tabs past the edge`, () => {
			expect(finalValue(rules, 'flex-wrap', width)).toBe('wrap');
			expect(finalValue(rules, 'overflow-x', width) ?? 'visible').toBe('visible');
		});
	}
});

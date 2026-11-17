import { describe, expect, it } from 'vitest';
import { focusAfterAsk } from './delete-focus.ts';

// A tiny stand-in for the page: elements found by selector, each records focus.
function page() {
	const focused: string[] = [];
	const el = (name: string) => ({ name, attrs: {} as Record<string, string>, setAttribute(k: string, v: string) { this.attrs[k] = v; }, hasAttribute(k: string) { return k in this.attrs; }, focus() { focused.push(name); } });
	const els: Record<string, ReturnType<typeof el>> = {
		'[data-testid="user-delete"][data-user="7"]': el('delete-7'),
		'#people-card h2': el('people-heading')
	};
	return { focused, els, root: { querySelector: (s: string) => els[s] ?? null } };
}

describe('focus after the delete ask closes', () => {
	it('Keep goes back to that person\'s Delete button', () => {
		const p = page();
		focusAfterAsk(p.root as never, 7, false);
		expect(p.focused).toEqual(['delete-7']);
	});
	it('after a delete it goes to the People heading, made focusable', () => {
		const p = page();
		focusAfterAsk(p.root as never, 7, true);
		expect(p.focused).toEqual(['people-heading']);
		expect(p.els['#people-card h2'].attrs.tabindex).toBe('-1');
	});
});

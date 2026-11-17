import { describe, expect, it } from 'vitest';
import { editorLines, lineTicked, onlyTicked, tickLine } from './person-qualities.ts';

const lines = [{ id: 'standard' }, { id: '4k' }, { id: 'own-1' }];

describe('the qualities one person may pick', () => {
	it('ticks every line when the person has no choice of their own', () => {
		expect(lines.map((l) => lineTicked(null, l.id))).toEqual([true, true, true]);
		expect(lineTicked(['4k'], 'standard')).toBe(false);
		expect(lineTicked(['4k'], '4k')).toBe(true);
	});

	it('keeps a list once one line is unticked, and goes back to every line when all are ticked', () => {
		const one = tickLine(lines, null, '4k', false);
		expect(one).toEqual(['standard', 'own-1']);
		expect(tickLine(lines, one, '4k', true)).toBeNull();
	});

	it('drops ids that are no longer lines and keeps the owner order', () => {
		expect(tickLine(lines, ['gone', 'own-1'], 'standard', true)).toEqual(['standard', 'own-1']);
	});

	it('never leaves a kind with nothing ticked', () => {
		expect(tickLine(lines, ['4k'], '4k', false)).toEqual(['4k']);
		expect(onlyTicked(['4k'], '4k')).toBe(true);
		expect(onlyTicked(null, '4k')).toBe(false);
		expect(onlyTicked(['4k', 'standard'], '4k')).toBe(false);
	});
});

describe('the lines the person editor shows', () => {
	const kind = [
		{ id: 'standard', hidden: false },
		{ id: '4k', hidden: true }
	];
	it('keeps a hidden line the person is limited to, and the kind with it', () => {
		expect(editorLines(kind, ['4k']).map((l) => l.id)).toEqual(['standard', '4k']);
	});
	it('shows a kind with one shown line when the person has a choice there, so it can be cleared', () => {
		expect(editorLines(kind, ['gone']).map((l) => l.id)).toEqual(['standard']);
		expect(tickLine(editorLines(kind, ['gone']), ['gone'], 'standard', true)).toBeNull();
	});
	it('hides a kind with one shown line and no choice, and hidden lines nobody is limited to', () => {
		expect(editorLines(kind, null)).toEqual([]);
		expect(editorLines([...kind, { id: 'own-1', hidden: false }], null).map((l) => l.id)).toEqual(['standard', 'own-1']);
	});
});

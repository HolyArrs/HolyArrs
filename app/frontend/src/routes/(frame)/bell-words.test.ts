import { cut, messageWords } from './bell-words.ts';
import { describe, expect, it } from 'vitest';

describe('bell words', () => {
	it('names the kind of request message and keeps its first sentence', () => {
		expect(messageWords({ kind: 'approved', text: 'Dune was approved. HolyArrs looks for it now.' })).toEqual({ title: 'Request approved', body: 'Dune was approved', tone: 'ok' });
		expect(messageWords({ kind: 'turned_down', text: 'Not this one' }).tone).toBe('fail');
		expect(messageWords({ kind: 'ready', text: 'Dune is ready' }).title).toBe('Ready to watch');
	});

	it('keeps second lines short', () => {
		expect(cut('x'.repeat(100))).toHaveLength(62);
		expect(cut('Short reason.')).toBe('Short reason');
	});
});

import { clearPin, cut, messageWords } from './bell-words.ts';
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

describe('clear pin', () => {
	const check = (o: Record<string, unknown>) => ({ id: 1, key: 'clear_waiting', kind: 'clear_waiting', subject: 'Downloads', state: 'warning', message: '3 imported downloads are still waiting to be cleared', since: '', checked_at: '', ...o }) as never;
	it('pins a count with Clear now while clears wait', () => {
		const run = async () => {};
		const p = clearPin([check({})], run);
		expect(p).toMatchObject({ key: 'clear_waiting', title: '3 imported downloads not cleared', body: 'HolyArrs tries again on its own', tone: 'warn', href: '/downloads', run: { label: 'Clear now', go: run } });
		expect(clearPin([check({ message: '1 imported download is still waiting to be cleared' })], run)?.title).toBe('Imported download not cleared');
	});
	it('pins nothing once every clear worked', () => {
		expect(clearPin([check({ state: 'ok', message: '' })], async () => {})).toBeNull();
		expect(clearPin([], async () => {})).toBeNull();
	});
});

import { describe, expect, it } from 'vitest';
import { makeSaver } from './view.ts';

function saver() {
	const sent: [number, number][] = [];
	let clock = 1_000_000;
	const s = makeSaver((sec, dur) => sent.push([sec, dur]), () => clock);
	return { s, sent, wait: (ms: number) => (clock += ms) };
}

describe('makeSaver', () => {
	it('saves on pause, on leave and every ten seconds, never twice in a row for the same second', () => {
		const { s, sent, wait } = saver();
		s.tick(1, 100);
		wait(9_999);
		s.tick(10, 100);
		expect(sent).toEqual([]);
		wait(1);
		s.tick(10.4, 100);
		expect(sent).toEqual([[10, 100]]);
		wait(2_000);
		s.tick(12, 100);
		expect(sent).toHaveLength(1);
		s.pause(12.7, 100);
		expect(sent).toEqual([[10, 100], [12, 100]]);
		s.leave(12.9, 100);
		expect(sent).toHaveLength(2);
		wait(20_000);
		s.tick(12.2, 100);
		expect(sent).toHaveLength(2);
		s.leave(40, 100);
		expect(sent).toEqual([[10, 100], [12, 100], [40, 100]]);
	});

	it('counts the ten seconds from the last save, whatever made it', () => {
		const { s, sent, wait } = saver();
		wait(5_000);
		s.pause(5, 100);
		wait(6_000);
		s.tick(11, 100);
		expect(sent).toEqual([[5, 100]]);
		wait(4_000);
		s.tick(15, 100);
		expect(sent).toEqual([[5, 100], [15, 100]]);
	});

	it('sends whole seconds and never a position before anything is known', () => {
		const { s, sent, wait } = saver();
		s.leave(NaN, NaN);
		s.pause(3, NaN);
		s.pause(3, 0);
		expect(sent).toEqual([]);
		wait(10_000);
		s.tick(59.99, 120.6);
		expect(sent).toEqual([[59, 121]]);
		s.leave(120.6, 120.6);
		expect(sent).toEqual([[59, 121], [120, 121]]);
	});
});

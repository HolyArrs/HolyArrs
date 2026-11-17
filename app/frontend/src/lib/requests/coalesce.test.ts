import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { coalesce } from './coalesce.ts';

describe('coalesce', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	it('runs once after a burst of 250 calls, not 250 times', () => {
		const fn = vi.fn();
		const soon = coalesce(fn, 400);
		for (let i = 0; i < 250; i++) soon();
		expect(fn).not.toHaveBeenCalled();
		vi.advanceTimersByTime(400);
		expect(fn).toHaveBeenCalledTimes(1);
	});

	it('cancel drops a call that is waiting, so a left page reads nothing', () => {
		const fn = vi.fn();
		const soon = coalesce(fn, 400);
		soon();
		soon.cancel();
		vi.advanceTimersByTime(1000);
		expect(fn).not.toHaveBeenCalled();
	});
});

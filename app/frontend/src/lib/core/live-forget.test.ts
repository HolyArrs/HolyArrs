import { describe, expect, it, vi } from 'vitest';
import { api } from './api.ts';
import { createLive } from './live.svelte.ts';

describe('live stream and shared reads', () => {
	it('forgets shared reads before listeners reload', () => {
		const forget = vi.spyOn(api, 'forget');
		const l = createLive(() => ({ onopen: null, onerror: null, onmessage: null, close() {} }));
		let seen = 0;
		l.on('title', () => {
			expect(forget).toHaveBeenCalled();
			seen++;
		});
		l.dispatch(JSON.stringify({ topic: 'title', data: {} }));
		expect(seen).toBe(1);
	});
});

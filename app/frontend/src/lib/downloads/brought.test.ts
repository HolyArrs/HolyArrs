import { describe, expect, it, vi, beforeEach } from 'vitest';

const calls: { m: string; path: string; body?: any }[] = [];
let testOk = true;
vi.mock('#lib/core/api.ts', async (orig) => ({
	...(await orig<any>()),
	api: {
		get: vi.fn(async (path: string) => {
			calls.push({ m: 'get', path });
			return { downloaders: [{ id: 7, name: 'SABnzbd', kind: 'sabnzbd', url: 'http://sab:8080', username: '', secret: '', category: 'tv', priority: 1, path_map: [], tags: [] }] };
		}),
		post: vi.fn(async (path: string) => {
			calls.push({ m: 'post', path });
			if (path.endsWith('/test')) return { ok: testOk, text: testOk ? '4.4' : "Can't reach" , health: { state: 'paused' } };
			return {};
		}),
		put: vi.fn(async (path: string, body: any) => {
			calls.push({ m: 'put', path, body });
			return {};
		}),
		del: vi.fn(),
		onSignedOut: vi.fn()
	}
}));

import { checkBrought, hiddenWhere, saveHiddenKey } from './brought.ts';

describe('brought Downloaders', () => {
	beforeEach(() => {
		calls.length = 0;
		testOk = true;
	});

	it('resumes one that answers and leaves one that does not', async () => {
		expect((await checkBrought(7)).ok).toBe(true);
		expect(calls.map((c) => c.path)).toEqual(['/downloaders/7/test', '/downloaders/7/resume']);
		calls.length = 0;
		testOk = false;
		expect((await checkBrought(7)).ok).toBe(false);
		expect(calls.map((c) => c.path)).toEqual(['/downloaders/7/test']);
	});

	it('saves the pasted key on that row, keeping the rest, then tests and resumes', async () => {
		const r = await saveHiddenKey(7, ' abc ');
		expect(r.ok).toBe(true);
		const put = calls.find((c) => c.m === 'put')!;
		expect(put.path).toBe('/downloaders/7');
		expect(put.body.secret).toBe('abc');
		expect(put.body.category).toBe('tv');
		expect(calls.at(-1)!.path).toBe('/downloaders/7/resume');
	});

	it('names where SABnzbd shows its key', () => {
		expect(hiddenWhere('sabnzbd')).toBe('SABnzbd > Config > General');
	});
});

import { beforeEach, describe, expect, it, vi } from 'vitest';

const get = vi.fn();
vi.mock('../core/api.ts', () => ({ api: { get: (url: string) => get(url) } }));
const { firstPageURL, prefetchPerson, takePrefetched } = await import('./person-prefetch.ts');

describe('person prefetch', () => {
	beforeEach(() => get.mockReset().mockImplementation(async (url: string) => ({ url })));

	it('fetches a person once and hands the answer to the page once', async () => {
		prefetchPerson('17', 'oldest', 1000);
		prefetchPerson('17', 'oldest', 1500);
		expect(get).toHaveBeenCalledTimes(1);
		const url = firstPageURL('17', 'oldest');
		expect(url).toBe('/people/17?role=all&sort=oldest&page_size=100&page=1');
		await expect(takePrefetched(url, 2000)).resolves.toEqual({ url });
		expect(takePrefetched(url, 2000)).toBeNull();
	});

	it('drops an answer older than a minute', () => {
		prefetchPerson('9', 'newest', 0);
		expect(takePrefetched(firstPageURL('9', 'newest'), 61_000)).toBeNull();
	});

	it('forgets a failed fetch so the page asks again', async () => {
		get.mockImplementationOnce(() => Promise.reject(new Error('down')));
		prefetchPerson('5', 'oldest', 0);
		await Promise.resolve();
		await Promise.resolve();
		expect(takePrefetched(firstPageURL('5', 'oldest'), 10)).toBeNull();
	});
});

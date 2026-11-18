import { describe, expect, it, vi } from 'vitest';

// On a full page load the page's load() runs before the layout has told the app its base, so the redirects take the base from SvelteKit itself.
vi.mock('$app/paths', () => ({ resolve: (p: string) => '/holyarrs' + p }));

function target(fn: () => unknown): string {
	try {
		fn();
	} catch (e) {
		return (e as { location: string }).location;
	}
	return '';
}

describe('old addresses keep the URL base', () => {
	it('account keeps the base and its query', async () => {
		const { load } = await import('../../routes/account/+page.ts');
		expect(target(() => load({ url: new URL('http://x/holyarrs/account?sso=finish') }))).toBe('/holyarrs/settings/account?sso=finish');
	});
	it('people keeps the base and the search words', async () => {
		const { load } = await import('../../routes/people/+page.ts');
		expect(target(() => (load as (a: unknown) => unknown)({ url: new URL('http://x/holyarrs/people?q=ann lee') }))).toBe('/holyarrs/search?kind=people&q=ann%20lee');
	});
	it('collections keeps the base', async () => {
		const { load } = await import('../../routes/library/collections/+page.ts');
		expect(target(() => load())).toBe('/holyarrs/library?kind=movie&group=series');
	});
});

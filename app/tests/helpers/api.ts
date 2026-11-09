import type { APIResponse, Page } from '@playwright/test';

export type Call = (method: string, path: string, data?: unknown) => Promise<APIResponse>;

/** api signs requests with the page's CSRF token, as the app does. */
export async function api(page: Page): Promise<Call> {
	const me = (await (await page.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	return (method, path, data) => page.request.fetch(`/api/v1${path}`, { method, data, headers: { 'X-CSRF-Token': me.csrf } });
}

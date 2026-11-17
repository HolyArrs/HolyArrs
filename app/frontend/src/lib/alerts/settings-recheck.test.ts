import { beforeEach, describe, expect, it, vi } from 'vitest';

const calls: string[] = [];
let runFails = false;
vi.mock('../core/api.ts', () => ({
	api: {
		post: async (p: string) => { calls.push(`post ${p}`); if (runFails) throw new Error('no'); return {}; },
		get: async (p: string) => { calls.push(`get ${p}`); return { items: [], known: [] }; }
	}
}));
vi.mock('../core/session.svelte.ts', () => ({ session: { can: () => true } }));

import { recheckSettingsStatus } from './settings-status.svelte.ts';

describe('recheckSettingsStatus', () => {
	beforeEach(() => { calls.length = 0; runFails = false; });

	it('runs just that group of checks, then reads the status once', async () => {
		await recheckSettingsStatus('backups');
		expect(calls).toEqual(['post /health/checks/run?only=backups', 'get /settings/status']);
	});

	it('still reads the status when the run is refused', async () => {
		runFails = true;
		await recheckSettingsStatus('downloader');
		expect(calls).toEqual(['post /health/checks/run?only=downloader', 'get /settings/status']);
	});

	it('only reads when there is no check to run', async () => {
		await recheckSettingsStatus();
		expect(calls).toEqual(['get /settings/status']);
	});
});

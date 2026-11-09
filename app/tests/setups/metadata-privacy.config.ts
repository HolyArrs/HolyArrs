import { defineConfig, devices } from '@playwright/test';
// Runs on its own (like csp-check): a fresh server, first-run setup, then the Metadata page.
export default defineConfig({
	testDir: '..',
	testMatch: '**/metadata/metadata-privacy.spec.ts',
	workers: 1,
	reporter: [['list']],
	use: { baseURL: 'http://127.0.0.1:18741', ...devices['Desktop Chrome'] },
	webServer: { command: 'node server.mjs', url: 'http://127.0.0.1:18741/readyz', reuseExistingServer: false, timeout: 180_000, stdout: 'ignore', stderr: 'pipe' }
});

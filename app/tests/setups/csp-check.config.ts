import { defineConfig, devices } from '@playwright/test';
export default defineConfig({
	testDir: '..',
	testMatch: '**/whole-app/csp-check.spec.ts',
	workers: 1,
	reporter: [['list']],
	use: { baseURL: 'http://127.0.0.1:18741', ...devices['Desktop Chrome'] },
	webServer: { command: 'node server.mjs', url: 'http://127.0.0.1:18741/readyz', reuseExistingServer: false, timeout: 180_000, stdout: 'ignore', stderr: 'pipe' }
});

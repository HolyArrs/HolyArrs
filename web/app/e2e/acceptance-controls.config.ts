import { defineConfig, devices } from '@playwright/test';

const port = process.env.HOLYARRS_E2E_PORT ?? '18734';

export default defineConfig({
	testDir: '.',
	testMatch: 'acceptance-controls.spec.ts',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: [['list']],
	use: {
		baseURL: `http://127.0.0.1:${port}`,
		trace: 'retain-on-failure'
	},
	webServer: {
		command: 'node server.mjs',
		url: `http://127.0.0.1:${port}/readyz`,
		reuseExistingServer: false,
		timeout: 180_000,
		stdout: 'ignore',
		stderr: 'ignore'
	},
	projects: [
		{ name: 'bootstrap', testMatch: 'app.spec.ts', use: { ...devices['Desktop Chrome'] } },
		{ name: 'lab-qbittorrent', testMatch: 'acceptance-controls.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['bootstrap'] },
		{ name: 'controls', testMatch: 'controls.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['lab-qbittorrent'] },
		{ name: 'parts', testMatch: 'parts.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['controls'] }
	]
});

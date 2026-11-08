import { defineConfig, devices } from '@playwright/test';

const port = process.env.HOLYARRS_E2E_PORT ?? '18734';
// server.mjs listens on HOLYARRS_E2E_PORT (its own default is 18741): hand it
// this config's port, or the webServer wait below never sees it start.
process.env.HOLYARRS_E2E_PORT = port;

export default defineConfig({
	testDir: '..',
	testMatch: '**/whole-app/acceptance-controls.spec.ts',
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
		{ name: 'bootstrap', testMatch: '**/whole-app/app.spec.ts', use: { ...devices['Desktop Chrome'] } },
		{ name: 'lab-qbittorrent', testMatch: '**/whole-app/acceptance-controls.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['bootstrap'] },
		{ name: 'controls', testMatch: '**/whole-app/controls.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['lab-qbittorrent'] },
		{ name: 'parts', testMatch: '**/whole-app/parts.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['controls'] }
	]
});

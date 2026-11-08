import { defineConfig, devices } from '@playwright/test';

const port = process.env.HOLYARRS_E2E_PORT ?? '18741';

// Music off: a fresh app (bootstrap makes the owner), then music-off.spec.ts.
export default defineConfig({
	testDir: '..',
	fullyParallel: false,
	workers: 2,
	retries: 0,
	reporter: [['list']],
	use: { baseURL: `http://127.0.0.1:${port}`, trace: 'retain-on-failure' },
	webServer: {
		command: 'node server.mjs',
		url: `http://127.0.0.1:${port}/readyz`,
		reuseExistingServer: false,
		timeout: 180_000,
		stdout: 'ignore',
		stderr: 'pipe'
	},
	projects: [
		{ name: 'bootstrap', testMatch: '**/whole-app/app.spec.ts', use: { ...devices['Desktop Chrome'] } },
		{ name: 'music-off', testMatch: '**/music/music-off.spec.ts', use: { ...devices['Desktop Chrome'] }, dependencies: ['bootstrap'] }
	]
});

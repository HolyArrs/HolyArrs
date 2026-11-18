import { defineConfig, devices } from '@playwright/test';

const port = process.env.HOLYARRS_E2E_PORT ?? '18921';

// Home before setup: a fresh app of its own (the phone audit's server), so
// Home is seen with an empty Library; 1280 and 390, light and dark.
export default defineConfig({
	testDir: '..',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: [['list']],
	outputDir: '../../frontend/test-results/home-fresh',
	use: { baseURL: `http://127.0.0.1:${port}`, trace: 'retain-on-failure' },
	webServer: {
		command: 'node phone-audit-server.mjs',
		env: { HOLYARRS_E2E_PORT: port },
		url: `http://127.0.0.1:${port}/readyz`,
		reuseExistingServer: false,
		timeout: 180_000,
		stdout: 'ignore',
		stderr: 'pipe'
	},
	projects: [{ name: 'home-fresh', testMatch: '**/whole-app/home-fresh.spec.ts', use: { ...devices['Desktop Chrome'] } }]
});

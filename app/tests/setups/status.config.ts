import { defineConfig, devices } from '@playwright/test';

const port = process.env.HOLYARRS_E2E_PORT ?? '18797';

// Status: a fresh app of its own (the phone audit's server), so it
// runs without the main suite; every tab at 1280 and 390.
export default defineConfig({
	testDir: '..',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: [['list']],
	use: { baseURL: `http://127.0.0.1:${port}`, trace: 'retain-on-failure' },
	webServer: {
		command: 'node phone-audit-server.mjs',
		url: `http://127.0.0.1:${port}/readyz`,
		reuseExistingServer: false,
		timeout: 180_000,
		stdout: 'ignore',
		stderr: 'pipe'
	},
	projects: [{ name: 'status', testMatch: '**/health/status.spec.ts', use: { ...devices['Desktop Chrome'] } }]
});

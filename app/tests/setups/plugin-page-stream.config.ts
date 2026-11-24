import { defineConfig, devices } from '@playwright/test';

const port = process.env.HOLYARRS_E2E_PORT ?? '18796';

// A plugin page that plays video: a fresh app of its own with a stub player plugin
// (see plugin-page-stream-server.mjs). Run: npx playwright test -c ../tests/setups/plugin-page-stream.config.ts
export default defineConfig({
	testDir: '..',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: [['list']],
	outputDir: '../../frontend/test-results/plugin-page-stream',
	use: { baseURL: `http://127.0.0.1:${port}`, trace: 'retain-on-failure' },
	webServer: {
		command: 'node plugin-page-stream-server.mjs',
		env: { HOLYARRS_E2E_PORT: port },
		url: `http://127.0.0.1:${port}/readyz`,
		reuseExistingServer: false,
		timeout: 240_000,
		stdout: 'ignore',
		stderr: 'pipe'
	},
	projects: [{ name: 'plugin-page-stream', testMatch: '**/plugins/plugin-page-stream.spec.ts', use: { ...devices['Desktop Chrome'] } }]
});

import { defineConfig, devices } from '@playwright/test';

const port = process.env.HOLYARRS_E2E_PORT ?? '18797';

// Phone audit (#79): a fresh app of its own in e2e/.tmp-phone on a port outside
// the block the main suite's fakes take, so it can run
// beside the main suite; every page at 360 and 390 wide.
export default defineConfig({
	testDir: '.',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: [['list']],
	use: { baseURL: `http://127.0.0.1:${port}`, trace: 'retain-on-failure' },
	webServer: {
		command: 'node phone-audit-server.mjs',
		url: `http://127.0.0.1:${port}/healthz`,
		reuseExistingServer: false,
		timeout: 180_000,
		stdout: 'ignore',
		stderr: 'pipe'
	},
	projects: [{ name: 'phone-audit', testMatch: 'phone-audit.spec.ts', use: { ...devices['Pixel 7'] } }]
});

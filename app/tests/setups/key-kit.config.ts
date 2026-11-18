import { defineConfig, devices } from '@playwright/test';

const port = process.env.HOLYARRS_E2E_PORT ?? '18901';

// The key kit reminder: a fresh app of its own, since it needs an install
// with nothing saved yet and moves the saved secrets back in time.
export default defineConfig({
	testDir: '..',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: [['list']],
	use: { baseURL: `http://127.0.0.1:${port}`, trace: 'retain-on-failure', acceptDownloads: true },
	webServer: {
		command: 'node key-kit-server.mjs',
		url: `http://127.0.0.1:${port}/readyz`,
		reuseExistingServer: false,
		timeout: 180_000,
		stdout: 'ignore',
		stderr: 'pipe'
	},
	projects: [{ name: 'keykit', testMatch: '**/backups/key-kit.spec.ts', use: { ...devices['Desktop Chrome'] } }]
});

import { defineConfig, devices } from '@playwright/test';

// Without the prepared run's address the spec skips itself with the reason.
const baseURL = process.env.HOLYARRS_OPTIONAL_UI_URL ?? 'http://127.0.0.1:9';

export default defineConfig({
	testDir: '..',
	testMatch: '**/downloads/optional-downloaders.spec.ts',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: [['list']],
	outputDir: process.env.HOLYARRS_OPTIONAL_UI_RESULTS,
	use: {
		...devices['Desktop Chrome'],
		baseURL,
		headless: true,
		viewport: { width: 1280, height: 900 },
		trace: 'off',
		screenshot: 'off',
		video: 'off'
	},
	projects: [{ name: 'optional-downloaders', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 900 } } }]
});

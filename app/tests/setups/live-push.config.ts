import { defineConfig, devices } from '@playwright/test';

// Push updates on the Downloads page against a throwaway HolyArrs and a fake
// qBittorrent (never a real client): `npx playwright test -c e2e/live-push.config.ts`.
const web = process.env.HOLYARRS_LIVE_WEB_PORT ?? '18734';
const backend = process.env.HOLYARRS_LIVE_BACKEND_PORT ?? '18732';

export default defineConfig({
	testDir: '..',
	testMatch: '**/whole-app/live-push.spec.ts',
	workers: 1,
	retries: 0,
	reporter: [['list']],
	use: { baseURL: `http://127.0.0.1:${web}`, trace: 'retain-on-failure', ...devices['Desktop Chrome'] },
	webServer: [
		{ command: 'node ../tests/setups/live-push-server.mjs', url: `http://127.0.0.1:${backend}/readyz`, reuseExistingServer: false, timeout: 180_000, stdout: 'ignore', stderr: 'pipe', cwd: '../../frontend' }
	]
});

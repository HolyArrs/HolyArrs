import { defineConfig, devices } from '@playwright/test';

const appPort = '18739';

export default defineConfig({
	testDir: '../tests',
	testMatch: '**/mediafiles/acceptance-library.spec.ts',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: [['list']],
	outputDir: 'test-results/acceptance-library',
	use: {
		...devices['Desktop Chrome'],
		baseURL: `http://127.0.0.1:${appPort}`,
		headless: true,
		trace: 'retain-on-failure'
	},
	webServer: {
		command: 'node ../tests/setups/acceptance-library-server.mjs',
		url: `http://127.0.0.1:${appPort}/healthz`,
		reuseExistingServer: false,
		timeout: 240_000,
		stdout: 'pipe',
		stderr: 'pipe',
		env: {
			HOLYARRS_E2E_PORT: appPort,
			HOLYARRS_E2E_MB_PORT: '18740',
			HOLYARRS_E2E_TMDB_PORT: '18741',
			HOLYARRS_E2E_SUBS_PORT: '18742',
			HOLYARRS_E2E_TVMAZE_PORT: '18743',
			HOLYARRS_E2E_PLEX_PORT: '0',
			HOLYARRS_E2E_JELLYFIN_PORT: '0',
			HOLYARRS_E2E_TRAKT_PORT: '0',
			HOLYARRS_E2E_DISCORD_PORT: '0',
			HOLYARRS_E2E_INDEXER_PORT: '0',
			HOLYARRS_E2E_QBIT_PORT: '0',
			HOLYARRS_E2E_SAB_PORT: '0',
			HOLYARRS_E2E_OLD_RADARR_PORT: '0',
			HOLYARRS_E2E_OLD_SEERR_PORT: '0'
		}
	}
});

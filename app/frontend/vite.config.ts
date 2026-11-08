import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { fewerChunks } from './build-chunks.ts';
import { buildVersion } from './build-version.ts';
import { bundledPackages } from './bundled-packages.ts';
import { defineConfig } from 'vitest/config';

// The app is built into dist, which the Go program embeds and serves at /.
// Every page renders in the browser; index.html is the fallback for app links.
const backend = process.env.HOLYARRS_DEV_BACKEND ?? 'http://127.0.0.1:18730';

export default defineConfig({
	plugins: [
		sveltekit({
			adapter: adapter({ pages: 'dist', assets: 'dist', fallback: 'index.html', strict: true }),
			paths: { relative: false },
			// Each page loads only its own code (plus the shared shell) instead of one bundle for every page.
			output: { bundleStrategy: 'split' },
			// A new name per build, checked every minute: an open tab reloads on its next move after an update (build-version.ts).
			version: { name: buildVersion(), pollInterval: 60_000 }
		}),
		// A few bigger shared script files instead of 120+ small ones (build-chunks.ts).
		fewerChunks(),
		// The npm packages the built app ships, for the third-party notices (bundled-packages.ts).
		bundledPackages()
	],
	server: {
		proxy: {
			'/api': backend,
			'/live': { target: backend, changeOrigin: false },
			'/healthz': backend,
			// AI app sign-in and the short AI address (the built app serves these itself).
			'/oauth': backend,
			'/.well-known': backend,
			'^/mcp$': backend
		}
	},
	test: {
		include: ['src/**/*.test.ts'],
		environment: 'node'
	}
});

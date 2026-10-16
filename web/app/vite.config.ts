import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

// The app is built into ../dist, which the Go program embeds and serves at /.
// Every page renders in the browser; index.html is the fallback for app links.
const backend = process.env.HOLYARRS_DEV_BACKEND ?? 'http://127.0.0.1:18730';

export default defineConfig({
	plugins: [
		sveltekit({
			adapter: adapter({ pages: '../dist', assets: '../dist', fallback: 'index.html', strict: true }),
			paths: { relative: false },
			output: { bundleStrategy: 'single' },
			version: { name: 'holyarrs' }
		})
	],
	server: {
		proxy: {
			'/api': backend,
			'/live': { target: backend, changeOrigin: false },
			'/healthz': backend
		}
	},
	test: {
		include: ['src/**/*.test.ts'],
		environment: 'node'
	}
});

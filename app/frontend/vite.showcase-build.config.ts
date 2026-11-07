import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { fewerChunks } from './build-chunks.ts';
import { buildVersion } from './build-version.ts';
import { defineConfig } from 'vite';

// The built showcase app for 18729 (vite.showcase-preview.config.ts serves it).
// Same app settings as vite.config.ts, but built into its own folder under
// .svelte-kit/showcase, so a rebuild never touches web/dist (the copy the Go
// program embeds) or the build files of a production build running beside it.
// `vite build --watch -c vite.showcase-build.config.ts` rebuilds on each edit.
const showcaseSite = '.svelte-kit/showcase/site';

export default defineConfig({
	plugins: [
		sveltekit({
			adapter: adapter({ pages: showcaseSite, assets: showcaseSite, fallback: 'index.html', strict: true }),
			outDir: '.svelte-kit/showcase',
			paths: { relative: false },
			output: { bundleStrategy: 'split' },
			// A new name per build, checked every minute: an open tab reloads on its next move after an update (build-version.ts).
			version: { name: buildVersion(), pollInterval: 60_000 }
		}),
		// The same script files as the real build (build-chunks.ts).
		fewerChunks(),
		// The page template is not a module, so the watching build would miss its edits.
		{ name: 'holyarrs-watch-template', buildStart() { this.addWatchFile('src/app.html'); } }
	],
	build: {
		sourcemap: false,
		// Faster rebuilds: no size report for each file.
		reportCompressedSize: false
	}
});

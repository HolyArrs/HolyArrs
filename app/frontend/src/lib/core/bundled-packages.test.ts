import { describe, expect, it } from 'vitest';
import { packageRoot } from '../../../bundled-packages.ts';

describe('packageRoot', () => {
	it('finds plain and scoped packages, the innermost node_modules first', () => {
		expect(packageRoot('/a/node_modules/svelte/src/internal/client/index.js')).toBe('/a/node_modules/svelte');
		expect(packageRoot(String.raw`C:\a\node_modules\@sveltejs\kit\src\entry.js`)).toBe('C:/a/node_modules/@sveltejs/kit');
		expect(packageRoot('/a/node_modules/x/node_modules/devalue/index.js?v=1')).toBe('/a/node_modules/x/node_modules/devalue');
		expect(packageRoot('\0/a/node_modules/vite/preload-helper.js')).toBe('/a/node_modules/vite');
	});
	it('leaves the app own files and virtual modules out', () => {
		expect(packageRoot('/a/src/routes/+page.svelte')).toBeNull();
		expect(packageRoot('\0virtual:entry')).toBeNull();
		expect(packageRoot('/a/node_modules/@scope')).toBeNull();
	});
});

describe('static files', () => {
	const parts = { ours: ['brand/', 'favicon.ico'], covered_by: [{ path: 'vendor/hls-' }], listed: [{ path: 'clients/deluge.svg' }] };

	it('lists every file under a folder, and nothing for a missing one', async () => {
		const { mkdtempSync, mkdirSync, writeFileSync } = await import('node:fs');
		const { join } = await import('node:path');
		const { tmpdir } = await import('node:os');
		const { staticFiles } = await import('../../../bundled-packages.ts');
		const dir = mkdtempSync(join(tmpdir(), 'static-'));
		mkdirSync(join(dir, 'fonts'));
		writeFileSync(join(dir, 'fonts', 'Paid.WOFF2'), '');
		writeFileSync(join(dir, 'favicon.ico'), '');
		expect(staticFiles(dir)).toEqual(['favicon.ico', 'fonts/Paid.WOFF2']);
		expect(staticFiles(join(dir, 'missing'))).toEqual([]);
	});

	it('names a file that is neither ours nor listed, and every font', async () => {
		const { unlisted } = await import('../../../bundled-packages.ts');
		expect(unlisted(['brand/mark.svg', 'favicon.ico', 'vendor/hls-1.7.3.min.js', 'clients/deluge.svg'], parts)).toEqual([]);
		expect(unlisted(['clients/plex.svg', 'brand/display.woff2', 'fonts/x.ttf', 'favicon.ico.bak'], parts)).toEqual(['clients/plex.svg', 'brand/display.woff2', 'fonts/x.ttf', 'favicon.ico.bak']);
	});
});

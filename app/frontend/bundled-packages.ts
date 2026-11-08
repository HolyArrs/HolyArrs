import { readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { Plugin } from 'vite';

// Which npm packages end up in the app the Go program ships (web/dist), so
// the third-party notices can list each one with its licence
// (scripts/notices.py). The build writes them, sorted, to
// bundled-packages.json beside this file; the file is committed with
// web/dist, and CI fails when a build changes it without a commit.
// Every module and file the build read counts, in the browser build and in
// the server half that renders the shipped page, even one left out of the
// bundle in the end: a notice too many costs nothing, one too few does.
//
// Files are checked too, because a font or a logo is easy to drop in and
// some are paid for or carry their own terms: the build fails on a file in
// static/ that static-parts.json does not name (static/ is copied as it
// is), and on a bundled font that comes from no npm package. Fonts are
// never inlined, so each one stays a file this check sees.

export interface BundledPackage {
	name: string;
	version: string;
	license: string;
	/** The package's folder, relative to web/app. */
	path: string;
}

/** packageRoot is the folder of the npm package a module file belongs to, or null. */
export function packageRoot(id: string): string | null {
	const path = id.replace(/^\0/, '').replace(/[?#].*$/, '').replaceAll('\\', '/');
	const at = path.lastIndexOf('/node_modules/');
	if (at < 0) return null;
	const rest = path.slice(at + '/node_modules/'.length).split('/');
	const parts = rest[0]?.startsWith('@') ? rest.slice(0, 2) : rest.slice(0, 1);
	if (parts.length !== (rest[0]?.startsWith('@') ? 2 : 1) || parts.some((p) => !p)) return null;
	return path.slice(0, at + '/node_modules/'.length) + parts.join('/');
}

const FONT = /\.(woff2?|ttf|otf|eot)$/i;

export interface StaticParts {
	ours: string[];
	covered_by: { path: string }[];
	listed: { path: string }[];
}

/** staticFiles lists the files under a folder, as paths relative to it. */
export function staticFiles(dir: string): string[] {
	let entries;
	try {
		entries = readdirSync(dir, { recursive: true, withFileTypes: true });
	} catch {
		return [];
	}
	return entries.filter((e) => e.isFile()).map((e) => relative(dir, join(e.parentPath, e.name)).replaceAll('\\', '/')).sort();
}

/** unlisted are the static files that are neither ours nor have a notice; a font always is. */
export function unlisted(files: string[], parts: StaticParts): string[] {
	return files.filter(
		(f) =>
			FONT.test(f) ||
			!(
				parts.ours.some((o) => f === o || (o.endsWith('/') && f.startsWith(o))) ||
				parts.listed.some((s) => f === s.path) ||
				parts.covered_by.some((c) => f.startsWith(c.path))
			)
	);
}

const appDir = fileURLToPath(new URL('.', import.meta.url));
const rel = (root: string) => relative(appDir, root).replaceAll('\\', '/');

export function bundledPackages(out = new URL('./bundled-packages.json', import.meta.url), staticDir = join(appDir, 'static')): Plugin {
	// One list for both halves of the build; each writes it, the last one with everything.
	const found = new Map<string, BundledPackage>();
	// Packages a CSS file pulls in by @import: the CSS pipeline inlines them, so they are no module of their own.
	const imported = new Set<string>();
	return {
		name: 'holyarrs-bundled-packages',
		apply: 'build',
		enforce: 'pre',
		config() {
			// A small font would become a data: address inside the CSS, out of sight of the check below.
			return { build: { assetsInlineLimit: (file: string) => (FONT.test(file) ? false : undefined) } };
		},
		buildStart() {
			const parts = JSON.parse(readFileSync(join(appDir, 'static-parts.json'), 'utf8')) as StaticParts;
			const bad = unlisted(staticFiles(staticDir), parts);
			if (bad.length) this.error(`static/ is copied into the app as it is, and these have no notice: ${bad.join(', ')}. Name each in static-parts.json with its licence (a font comes from an npm package instead).`);
		},
		transform(code, id) {
			if (!/\.(css|scss|sass|less|pcss|postcss)(\?|$)/.test(id)) return;
			for (const m of code.matchAll(/@import\s+(?:url\()?\s*['"]([^'"]+)['"]/g)) {
				const spec = m[1];
				if (/^(\.|\/|[a-z]+:)/i.test(spec)) continue;
				const name = spec.startsWith('@') ? spec.split('/').slice(0, 2).join('/') : spec.split('/')[0];
				imported.add(join(appDir, 'node_modules', name, 'package.json'));
			}
		},
		generateBundle(_options, bundle) {
			const ids = [...this.getModuleIds(), ...imported];
			for (const file of Object.values(bundle)) {
				if (file.type !== 'asset') continue;
				// The bundler names an asset's sources relative to the app folder.
				const from = (file.originalFileNames ?? []).map((f) => resolve(appDir, f));
				if (FONT.test(file.fileName) && (from.length === 0 || from.some((f) => !packageRoot(f)))) this.error(`font ${file.fileName} comes from no npm package (${from.join(', ') || 'unknown source'}), so no notice covers it`);
				ids.push(...from);
			}
			// The bundler adds small helpers of its own (module preloading, its runtime) as virtual modules.
			for (const helper of ['vite', 'rolldown']) ids.push(join(appDir, 'node_modules', helper, 'package.json'));
			const seen = new Set([...found.values()].map((p) => p.path));
			for (const id of ids) {
				const root = packageRoot(id);
				if (!root || seen.has(rel(root))) continue;
				seen.add(rel(root));
				let pkg: { name?: string; version?: string; license?: unknown };
				try {
					pkg = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8'));
				} catch {
					continue;
				}
				if (!pkg.name || !pkg.version) continue;
				found.set(`${pkg.name}@${pkg.version}`, { name: pkg.name, version: pkg.version, license: typeof pkg.license === 'string' ? pkg.license : 'see the package', path: rel(root) });
			}
			const list = [...found.values()].sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : a.version < b.version ? -1 : 1));
			writeFileSync(out, JSON.stringify(list, null, '\t') + '\n');
		}
	};
}

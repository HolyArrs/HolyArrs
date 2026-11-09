// The import graph of src/, read from the files, for the structure tests
// (lib/core/structure.test.ts and lib/ui/ui-usage.test.ts). It is not part of the app.
import { readFileSync, readdirSync, existsSync, statSync } from 'node:fs';
import { dirname, join, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

export const SRC = dirname(fileURLToPath(import.meta.url));

/** LAYER lists the modules from the bottom up; a module imports only modules with a lower number. */
export const LAYER: Record<string, number> = {
	metadata: 1,
	accounts: 2,
	mediafiles: 3,
	sources: 4,
	downloads: 5,
	subtitles: 6,
	mediaservers: 6,
	alerts: 6,
	movies: 7,
	shows: 7,
	music: 7,
	requests: 8,
	discover: 9,
	arrapi: 9,
	migrate: 9,
	ai: 9,
	backups: 9,
	health: 9,
	updates: 9,
	setup: 9
};

export function listFiles(dir = SRC, out: string[] = []): string[] {
	for (const e of readdirSync(dir, { withFileTypes: true })) {
		if (e.name === 'node_modules' || e.name === '.svelte-kit') continue;
		const p = join(dir, e.name);
		if (e.isDirectory()) listFiles(p, out);
		else out.push(relative(SRC, p).split(sep).join('/'));
	}
	return out;
}

const IMPORT = /(?:^|[\s;}])(?:import|export)\s+(?:type\s+)?(?:[^'"`;]*?\s+from\s+)?['"]([^'"]+)['"]|import\s*\(\s*['"]([^'"]+)['"]\s*\)|@import\s+['"]([^'"]+)['"]/g;

/** importsOf lists the src files a file imports (aliases and relative paths). */
export function importsOf(file: string, all: Set<string>): string[] {
	const text = readFileSync(join(SRC, file), 'utf8');
	const out = new Set<string>();
	for (const m of text.matchAll(IMPORT)) {
		const spec = m[1] ?? m[2] ?? m[3];
		if (!spec) continue;
		let r: string;
		if (spec.startsWith('#lib/') || spec.startsWith('$lib/')) r = 'lib/' + spec.slice(5);
		else if (spec.startsWith('.')) r = join(dirname(file), spec).split(sep).join('/');
		else continue;
		r = r.replace(/\?.*$/, '');
		const hit = [r, r + '.ts', r + '/index.ts'].find((c) => all.has(c) && existsSync(join(SRC, c)) && statSync(join(SRC, c)).isFile());
		if (hit) out.add(hit);
	}
	return [...out];
}

/** zoneOf says where a file lives: core, ui, a module name, routes or flat (not moved yet). */
export function zoneOf(file: string): string {
	if (file.startsWith('routes/')) return 'routes';
	if (!file.startsWith('lib/')) return 'other';
	const seg = file.split('/')[1];
	if (seg === 'core' || seg === 'ui') return seg;
	if (seg in LAYER) return seg;
	return file.split('/').length > 2 ? 'folder:' + seg : 'flat';
}

export const isTest = (f: string) => /\.test\.ts$/.test(f);
export const isCode = (f: string) => /\.(ts|svelte)$/.test(f) && !isTest(f);

/** routeModule maps a route file to the module whose page it is, for counting who uses a shared part. */
const ROUTES: [RegExp, string][] = [
	[/^routes\/(\(frame\)|\+layout)|^routes\/settings\/(\(home\)|\+)/, 'frame'],
	[/^routes\/(\(home\)|\+page|home-style)/, 'home'],
	[/^routes\/(ui|dev)\b/, 'devui'],
	[/^routes\/(account|login|invite)\b|^routes\/settings\/(account|sign-in|users|appearance)\b/, 'accounts'],
	[/^routes\/(welcome)\b|^routes\/settings\/parts\b/, 'setup'],
	[/^routes\/ai\b|^routes\/settings\/ai\b/, 'ai'],
	[/^routes\/library\/\[id\]\/albums\b/, 'music'],
	[/^routes\/(calendar|explore|popular|people|search|title)\b|^routes\/library\/\[id\]/, 'discover'],
	[/^routes\/downloads\b|^routes\/settings\/(downloaders|pick-rules)\b/, 'downloads'],
	[/^routes\/library\b|^routes\/settings\/(naming|library-folders|title-defaults)\b/, 'mediafiles'],
	[/^routes\/music\b|^routes\/settings\/music\b/, 'music'],
	[/^routes\/(reports|requests|wishlist)\b|^routes\/settings\/(wishlist|requests|outside-lists)\b/, 'requests'],
	[/^routes\/settings\/alerts/, 'alerts'],
	[/^routes\/settings\/api/, 'arrapi'],
	[/^routes\/settings\/(backups|danger)/, 'backups'],
	[/^routes\/(status|health)\b|^routes\/settings\/(health|logs|this-app|schedules)/, 'health'],
	[/^routes\/settings\/move/, 'migrate'],
	[/^routes\/settings\/media-servers/, 'mediaservers'],
	[/^routes\/settings\/(search|sources)/, 'sources'],
	[/^routes\/settings\/subtitles/, 'subtitles'],
	[/^routes\/settings\/tmdb/, 'metadata']
];

export function routeModule(file: string): string | null {
	for (const [re, m] of ROUTES) if (re.test(file)) return m;
	return file.startsWith('routes/') ? 'other-route' : null;
}

export interface Graph {
	files: string[];
	edges: Map<string, string[]>;
	importers: Map<string, string[]>;
}

export function buildGraph(): Graph {
	const files = listFiles().filter((f) => /\.(ts|svelte|css)$/.test(f));
	const set = new Set(listFiles());
	const edges = new Map<string, string[]>();
	const importers = new Map<string, string[]>();
	for (const f of files) {
		const to = importsOf(f, set);
		edges.set(f, to);
		for (const t of to) importers.set(t, [...(importers.get(t) ?? []), f]);
	}
	return { files, edges, importers };
}

/** usersOf lists the modules that reach a file through non-test importers, following lib files up to a route. */
export function usersOf(file: string, g: Graph, seen = new Set<string>()): Set<string> {
	const out = new Set<string>();
	if (seen.has(file)) return out;
	seen.add(file);
	for (const i of g.importers.get(file) ?? []) {
		if (isTest(i)) continue;
		const r = routeModule(i);
		if (r) {
			out.add(r);
			continue;
		}
		const z = zoneOf(i);
		if (z in LAYER) out.add(z);
		else for (const u of usersOf(i, g, seen)) out.add(u);
	}
	return out;
}

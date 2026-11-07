import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { LAYER, SRC, buildGraph, isCode, isTest, listFiles, zoneOf } from '../../structure-graph.ts';

// How src/ is laid out: where files live, what their names look like and which
// way imports may point. structure-pending.json lists what may still break a
// rule; every entry is removed by a later step, and an entry that no longer
// applies fails the test so the list only shrinks.
// Refresh it after a move with: UPDATE_PENDING=1 npx vitest run src/lib/core/structure.test.ts

const MAX_CORE_FILES = 40;
/** Route folders that may hold tests next to their code. */
const ROUTE_TEST_FOLDERS = ['routes/(home)/', 'routes/(frame)/', 'routes/settings/(home)/'];

const PENDING_FILE = join(SRC, 'lib/core/structure-pending.json');
const pending: Record<'names' | 'flat' | 'routeTests' | 'edges' | 'pageEdges' | 'noIndex', string[]> = existsSync(PENDING_FILE)
	? JSON.parse(readFileSync(PENDING_FILE, 'utf8'))
	: { names: [], flat: [], routeTests: [], edges: [], pageEdges: [], noIndex: [] };

const graph = buildGraph();
const all = listFiles();
const lib = graph.files.filter((f) => f.startsWith('lib/'));

const names = all.filter((f) => {
	const name = f.split('/').pop()!;
	if (name.startsWith('+') || f.startsWith('static/')) return false;
	if (name.endsWith('.svelte')) return !/^[A-Z][A-Za-z0-9]*\.svelte$/.test(name);
	if (name.endsWith('.ts')) return !/^[a-z0-9]+(-[a-z0-9]+)*(\.svelte|\.test|\.lab\.test|\.d)?\.ts$/.test(name);
	return false;
});
const flat = lib.filter((f) => ['flat', 'folder'].includes(zoneOf(f).split(':')[0]) && !f.endsWith('.css'));
const routeTests = all.filter((f) => f.startsWith('routes/') && isTest(f) && !ROUTE_TEST_FOLDERS.some((d) => f.startsWith(d)));

const edges: string[] = [];
for (const f of lib.filter(isCode)) {
	const from = zoneOf(f);
	for (const t of graph.edges.get(f) ?? []) {
		const to = zoneOf(t);
		if (from === to || !['core', 'ui', ...Object.keys(LAYER)].includes(from)) continue;
		const where = `${f} -> ${t}`;
		if (from === 'core') {
			// Icon names are the one type core reads from a part.
			if (!(to === 'core' || t === 'lib/ui/Icon.svelte')) edges.push(where);
		} else if (from === 'ui') {
			if (to !== 'core') edges.push(where);
		} else if (to === 'core' || to === 'ui') {
			continue;
		} else if (!(to in LAYER && LAYER[to] < LAYER[from] && t === `lib/${to}/index.ts`)) {
			edges.push(where);
		}
	}
}
const pageEdges: string[] = [];
for (const f of all.filter((x) => x.startsWith('routes/') && isCode(x))) {
	for (const t of graph.edges.get(f) ?? []) {
		const z = zoneOf(t);
		if (z in LAYER && t !== `lib/${z}/index.ts`) pageEdges.push(`${f} -> ${t}`);
	}
}

const noIndex = Object.keys(LAYER).filter((m) => lib.some((f) => f.startsWith('lib/' + m + '/')) && !lib.includes('lib/' + m + '/index.ts'));

const found = { names, flat, routeTests, edges, pageEdges, noIndex };

if (process.env.UPDATE_PENDING) {
	writeFileSync(PENDING_FILE, JSON.stringify(Object.fromEntries(Object.entries(found).map(([k, v]) => [k, [...v].sort()])), null, '\t') + '\n');
}

function same(kind: keyof typeof found) {
	const allowed = new Set(pending[kind]);
	const now = new Set(found[kind]);
	return {
		newBreaks: [...now].filter((x) => !allowed.has(x)),
		stale: [...allowed].filter((x) => !now.has(x))
	};
}

describe('file names', () => {
	it('keeps Svelte parts PascalCase and scripts kebab-case', () => {
		const r = same('names');
		expect(r.newBreaks).toEqual([]);
		expect(r.stale).toEqual([]);
	});
});

describe('where files live', () => {
	it('keeps tests out of page folders except the ones that own parts', () => {
		const r = same('routeTests');
		expect(r.newBreaks).toEqual([]);
		expect(r.stale).toEqual([]);
	});

	it('has no home folder in lib/core', () => {
		expect(all.filter((f) => f.startsWith('lib/core/home'))).toEqual([]);
	});

	it(`keeps lib/core to ${MAX_CORE_FILES} code files, trailer playback the only subfolder`, () => {
		const core = all.filter((f) => f.startsWith('lib/core/') && /\.(ts|svelte)$/.test(f) && !isTest(f));
		expect(core.length).toBeLessThanOrEqual(MAX_CORE_FILES);
		const folders = new Set(core.map((f) => f.split('/')).filter((p) => p.length > 3).map((p) => p[2]));
		expect([...folders]).toEqual(folders.size ? ['trailer'] : []);
	});

	it('leaves in lib/ only what its page moves later', () => {
		const r = same('flat');
		expect(r.newBreaks).toEqual([]);
		expect(r.stale).toEqual([]);
	});

	it('has an index.ts in every module folder', () => {
		const r = same('noIndex');
		expect(r.newBreaks).toEqual([]);
		expect(r.stale).toEqual([]);
	});
});

describe('which way imports point', () => {
	it('keeps core and ui free of modules and modules to lower modules through index.ts', () => {
		const r = same('edges');
		expect(r.newBreaks).toEqual([]);
		expect(r.stale).toEqual([]);
	});

	it('lets pages reach a module only through its index.ts', () => {
		const r = same('pageEdges');
		expect(r.newBreaks).toEqual([]);
		expect(r.stale).toEqual([]);
	});
});

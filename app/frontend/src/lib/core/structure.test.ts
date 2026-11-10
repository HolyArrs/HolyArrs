import { describe, expect, it } from 'vitest';
import { LAYER, buildGraph, isCode, isTest, listFiles, zoneOf } from '../../structure-graph.ts';

const MAX_CORE_FILES = 40;
const ROUTE_TEST_FOLDERS = ['routes/(home)/', 'routes/(frame)/', 'routes/settings/(home)/'];


const graph = buildGraph();
const all = listFiles();
const lib = graph.files.filter((f) => f.startsWith('lib/'));

const names = all.filter((f) => {
	const name = f.split('/').pop()!;
	if (name.startsWith('+') || /^hooks\.(client|server)\.ts$/.test(name) || f.startsWith('static/')) return false;
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

function same(kind: keyof typeof found) {
	return { newBreaks: found[kind], stale: [] as string[] };
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

	it(`keeps lib/core to ${MAX_CORE_FILES} code files, trailer playback and the app's words the only subfolders`, () => {
		const core = all.filter((f) => f.startsWith('lib/core/') && /\.(ts|svelte)$/.test(f) && !isTest(f));
		expect(core.length).toBeLessThanOrEqual(MAX_CORE_FILES);
		const folders = new Set(core.map((f) => f.split('/')).filter((p) => p.length > 3).map((p) => p[2]));
		expect([...folders].sort()).toEqual(['i18n', 'trailer'].filter((f) => folders.has(f)));
		expect([...folders].every((f) => f === 'i18n' || f === 'trailer')).toBe(true);
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

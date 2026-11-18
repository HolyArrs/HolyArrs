import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { builtInSites } from './api.ts';

const root = new URL('../../../../../', import.meta.url);
const read = (p: string) => readFileSync(new URL(p, root), 'utf8');
const en = (file: string): Record<string, string> => JSON.parse(read(`app/frontend/src/lib/core/i18n/messages/en/${file}.json`));

describe('built-in sites are told about', () => {
	it('lists the same sites as the server', () => {
		const go = read('app/backend/internal/sources/sites.go');
		const ids = [...go.matchAll(/\{ID: "(\w+)", Name: "(\w+)"/g)].map((m) => `${m[1]}:${m[2]}`);
		expect(builtInSites.map((s) => `${s.id}:${s.name}`)).toEqual(ids);
	});

	it('the Sources page says they need no Jackett or Prowlarr, with the real labels', () => {
		const m = en('settings-sources');
		expect(m['settings.sources.builtIn']).toBeDefined();
		expect(m['settings.sources.builtIn']).toMatch(/\{sites\}/);
		expect(m['settings.sources.builtIn']).toMatch(/\{site\}/);
		expect(m['settings.sources.builtIn']).toMatch(/\{kind\}/);
		expect(m['settings.sources.noneText']).toMatch(/built-in site/i);
		const page = read('app/frontend/src/routes/settings/sources/+page.svelte');
		expect(page).toContain("t('settings.sources.builtIn'");
	});

	it('the first-run step mentions them', () => {
		expect(en('welcome')['welcome.linkProwlarrOrJackett']).toMatch(/built-in site/i);
	});

	it('the Sources guide names every built-in site and does not say Jackett or Prowlarr is the only way', () => {
		const guide = read('docs/downloads/sources.md');
		for (const s of builtInSites) expect(guide).toContain(s.name);
		expect(guide).toContain('built-in-sites');
		expect(guide).not.toMatch(/Most torrent sites have no feed of their own\. For those, run Jackett or Prowlarr/);
		for (const f of ['docs/start/connect.md', 'docs/downloads/index.md']) expect(read(f)).toMatch(/built-in/i);
	});
});

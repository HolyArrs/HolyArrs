// Sources and Downloads in first-run setup as app rows, the same look as Bring my apps: a tick for what is in,
// a "From Radarr" tag for what an import brought, and the connect screen behind each row.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { kindWords } from '#lib/downloads/index.ts';
import type { IconName } from '#lib/ui/Icon.svelte';

export type ToolRow = { key: string; name: string; logo?: string; icon?: IconName; done: boolean; line?: string; from: string[]; open: () => void; manage?: () => void; another?: () => void };

const host = (u: string) => u.replace(/^https?:\/\//, '').replace(/\/$/, '');

export function createWelcomeTools(ctx: any) {
	// The connect screen open behind a row (Sources: Prowlarr or Jackett; Downloads: the downloader form), or the rows.
	let srcOpen = $state<'prowlarr' | 'jackett' | null>(null);
	let dlOpen = $state(false);

	const sourceRows = $derived.by((): ToolRow[] => {
		const bridges: string[] = ctx.finish.bridges;
		const rows: ToolRow[] = (['prowlarr', 'jackett'] as const).map((k) => ({
			key: k,
			name: k === 'prowlarr' ? 'Prowlarr' : 'Jackett',
			logo: ctx.bring.appLogo(k),
			icon: 'search',
			done: bridges.includes(k),
			from: [],
			open: () => (srcOpen = k)
		}));
		// Indexers an import brought from Radarr or Sonarr. When they are Prowlarr's own feeds, Prowlarr is in use: its card is
		// ticked with the count; else they are one ticked card of their own. Either way the card says which apps brought them.
		const n: number = ctx.sourcesStep.sources;
		const apps = (['radarr', 'sonarr', 'lidarr'] as const).filter((k) => (ctx.bring.reads[k] ?? []).length).map((k) => ctx.appName(k));
		if (n > 0 && ctx.sourcesStep.viaProwlarr > 0) Object.assign(rows[0], { done: true, line: t('welcome.choose.nIndexers', { n }), from: apps, manage: rows[0].open, another: rows[0].open });
		else if (n > 0 && apps.length) rows.push({ key: 'indexers', name: t('welcome.choose.indexers', { n }), icon: 'list', done: true, from: apps, open: () => undefined });
		return rows;
	});

	function openDl(kind?: string, adding = true) {
		ctx.error = '';
		ctx.downloaderStep.dlAdding = adding;
		if (kind) ctx.downloaderStep.pickDl(kind);
		dlOpen = true;
	}
	const dlRows = $derived.by((): ToolRow[] => {
		const list: { id: number; kind: string; name: string; url: string }[] = ctx.downloaderStep.dlList;
		const kinds = [...new Set(['qbittorrent', 'sabnzbd', ...list.map((d) => d.kind)])];
		return kinds.map((k) => {
			const mine = list.filter((d) => d.kind === k);
			const from = [...new Set(mine.map((d) => ctx.downloaderStep.found[d.id]).filter(Boolean).map((a: string) => ctx.appName(a)))];
			return {
				key: k,
				name: (kindWords as Record<string, string>)[k] ?? k,
				logo: k === 'qbittorrent' || k === 'sabnzbd' ? ctx.bring.appLogo(k) : undefined,
				icon: 'downloader' as IconName,
				done: mine.length > 0,
				line: mine.map((d) => host(d.url)).join(' · '),
				from,
				open: () => (mine.length ? openDl(undefined, false) : openDl(k)),
				manage: () => openDl(undefined, false),
				another: () => openDl(k)
			};
		});
	});

	return {
		get srcOpen() { return srcOpen; },
		set srcOpen(v) { srcOpen = v; },
		get dlOpen() { return dlOpen; },
		set dlOpen(v) { dlOpen = v; },
		get sourceRows() { return sourceRows; },
		get dlRows() { return dlRows; }
	};
}

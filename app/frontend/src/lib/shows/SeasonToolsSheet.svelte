<script lang="ts">
	// Side sheet for one season: files on disk, rename preview and history.
	import { seasonName } from './labels.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { RenamePreview } from '#lib/mediafiles/index.ts';
	import type { SheetEpisode } from './EpisodeSheet.svelte';
	import { api } from '#lib/core/api.ts';
	import { ago, bytes } from '#lib/core/format.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let {
		open = $bindable(false),
		mode = $bindable('files'),
		titleId,
		season,
		episodes,
		onchange,
		ondelete
	}: { open?: boolean; mode?: 'files' | 'rename' | 'history'; titleId: number; season: number; episodes: SheetEpisode[]; onchange?: () => void; ondelete?: () => void } = $props();

	let events = $state<{ ID: number; At?: string; Kind?: string; Summary?: string }[]>([]);
	let loadingEvents = $state(false);

	// Rows come from each episode's first slot; episodes without a file are skipped.
	const rows = $derived(
		episodes
			.map((e) => ({ e, f: e.slots[0]?.file as Record<string, any> | null | undefined }))
			.filter((x) => x.f)
			.map(({ e, f }) => {
				const q = (f!.Quality ?? {}) as Record<string, any>;
				const m = (f!.MediaInfo ?? {}) as Record<string, any>;
				const langs = [...(Array.isArray(m.audio_languages) ? m.audio_languages : []), ...(Array.isArray(f!.Languages) ? f!.Languages : [])].map((x) => String(x).toUpperCase());
				return {
					id: e.id,
					ep: e.episode,
					name: String(f!.RelativePath ?? '').split(/[\\/]/).pop() ?? '',
					quality: [q.resolution ? `${q.resolution}p` : '', sources[q.source] ?? ''].filter(Boolean).join(' '),
					size: f!.Size ? bytes(f!.Size) : '',
					// Four tags is all the row has room for.
					langs: [...new Set(langs)].slice(0, 4),
					group: (f!.Group as string | null) ?? ''
				};
			})
	);
	const total = $derived(episodes.reduce((n, e) => n + Number(e.slots[0]?.file?.Size ?? 0), 0));
	const label = $derived(seasonName(season));

	// History loads only when its tab is open.
	$effect(() => {
		if (!open || mode !== 'history') return;
		loadingEvents = true;
		void api
			.get<{ events?: typeof events }>(`/events?title=${titleId}&season=${season}&limit=50`)
			.then((r) => (events = r.events ?? []), () => (events = []))
			.finally(() => (loadingEvents = false));
	});

	// Short source names for the quality tag.
	const sources: Record<string, string> = { web_dl: 'WEB', webrip: 'WEB', bluray: 'BluRay', remux: 'Remux', hdtv: 'TV', dvd: 'DVD' };
</script>

<Sheet bind:open title={label} placement="right" size="md">
	<div class="st" data-testid="season-tools">
		<div class="st-seg" role="tablist">
			<button type="button" role="tab" class:on={mode === 'files'} aria-selected={mode === 'files'} onclick={() => (mode = 'files')}>{t('shows.tools.files')}</button>
			<button type="button" role="tab" class:on={mode === 'rename'} aria-selected={mode === 'rename'} onclick={() => (mode = 'rename')}>{t('shows.tools.rename')}</button>
			<button type="button" role="tab" class:on={mode === 'history'} aria-selected={mode === 'history'} onclick={() => (mode = 'history')}>{t('shows.tools.history')}</button>
		</div>

		{#if mode === 'files'}
			<p class="st-sum">{t('shows.tools.onDisk', { have: rows.length, all: episodes.length })}{total ? ` · ${bytes(total)}` : ''}</p>
			<ul class="st-files">
				{#each rows as r (r.id)}
					<li>
						<span class="st-n">{r.ep}</span>
						<span class="st-main"><b title={r.name}>{r.name}</b><small>{[r.quality, r.size, r.group].filter(Boolean).join(' · ')}</small></span>
						{#if r.langs.length}<span class="st-langs">{#each r.langs as l (l)}<span>{l}</span>{/each}</span>{/if}
					</li>
				{:else}<li class="muted">{t('shows.tools.noFiles')}</li>{/each}
			</ul>
		{:else if mode === 'rename'}
			{#key season}<RenamePreview {titleId} {season} onDone={() => onchange?.()} />{/key}
		{:else}
			<ol class="st-tl">
				{#each events as e (e.ID)}<li><span>{(e.Summary ?? e.Kind ?? '').replace(/^./, (c) => c.toUpperCase())}</span><span class="muted">{e.At ? ago(e.At) : ''}</span></li>
				{:else}<li class="muted">{loadingEvents ? t('shows.tools.loading') : t('shows.tools.nothingYet')}</li>{/each}
			</ol>
		{/if}
		{#if ondelete}<button type="button" class="st-del" onclick={ondelete}><Icon name="trash" size={16} />{t('shows.tools.delete', { season: seasonName(season) })}</button>{/if}
	</div>
</Sheet>

<style>
	.st { display: grid; gap: 14px; }
	.st-seg { display: inline-flex; justify-self: start; padding: 3px; border-radius: 999px; background: color-mix(in srgb, var(--text) 7%, transparent); }
	.st-seg button { height: 30px; padding: 0 16px; border: 0; border-radius: 999px; background: none; color: var(--text); font: 600 13px/1 var(--font-ui); cursor: pointer; }
	.st-seg button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Bigger touch target on touch screens. */
	@media (pointer: coarse) { .st-seg button { height: 40px; } }
	.st-seg button.on { background: var(--surface-1, var(--bg)); color: var(--text); box-shadow: 0 1px 3px rgb(0 0 0 / 18%); }
	.st-del { justify-self: start; display: inline-flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 16px; border: 0; border-radius: 12px; background: var(--surface-2); color: var(--fail-text); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	.st-del:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.st-sum { margin: 0; color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.st-files, .st-tl { display: grid; gap: 6px; margin: 0; padding: 0; list-style: none; }
	.st-files li { display: grid; grid-template-columns: 28px minmax(0, 1fr) auto; align-items: center; gap: 12px; padding: 10px 12px; border-radius: 12px; background: var(--surface-2); }
	.st-n { display: grid; place-items: center; width: 26px; height: 26px; border-radius: 7px; background: color-mix(in srgb, var(--text) 8%, transparent); font: 700 12px/1 var(--font-ui); }
	.st-main { display: grid; gap: 3px; min-width: 0; }
	.st-main b { font: 600 13px/1.3 var(--font-mono, ui-monospace, monospace); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.st-main small { color: var(--text-muted); font: 500 12.5px/1.2 var(--font-ui); }
	.st-langs { display: flex; gap: 4px; }
	.st-langs span { display: inline-flex; align-items: center; height: 20px; padding: 0 7px; border-radius: 99px; border: 1px solid color-mix(in srgb, var(--text) 28%, transparent); color: var(--text-muted); font: 700 10.5px/1 var(--font-ui); }
	.st-tl li { display: flex; justify-content: space-between; gap: 12px; padding: 6px 0; font: 500 14px/1.35 var(--font-ui); }
</style>

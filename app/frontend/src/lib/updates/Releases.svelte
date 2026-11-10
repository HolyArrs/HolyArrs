<script lang="ts">
	import '#lib/ui/plate.css';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { page } from '$app/state';
	import { ago } from '../core/format.ts';
	import { noteLines, plainVersion, whatsNew, type ReleaseNote } from './update-notice.ts';

	type Update = { on: boolean; version: string; latest?: string; newer: boolean; link?: string; notes?: string; checked_at?: string; problem?: string; no_release?: boolean };
	type Rel = { version: string; date?: string; notes?: string; link?: string };

	let { upd }: { upd: Update | null } = $props();

	const STEP = 3;
	let shown = $state(STEP);
	let open = $state<Record<string, boolean>>({});

	// Made-up history for dev builds, shown with ?releases=demo.
	const demo: Rel[] = [
		{ version: '1.4.0', date: '2026-10-30', notes: 'Status page with Health, Parts and Jobs.\nRequests page shows live steps for each ask.\nHistory keeps sign-ins and settings changes for a year.' },
		{ version: '1.3.2', date: '2026-10-21', notes: 'Fixed a stuck download that never moved to import.\nFaster first load of Explore.' },
		{ version: '1.3.1', date: '2026-10-14', notes: 'Fixed search for titles with an apostrophe.' },
		{ version: '1.3.0', date: '2026-10-02', notes: 'Music library and artist pages.\nNew free-space check for every library folder.' },
		{ version: '1.2.4', date: '2026-09-20', notes: 'Smaller backups.\nFixed time zones in the calendar.' },
		{ version: '1.2.0', date: '2026-09-04', notes: 'Media server links for Jellyfin and Plex.' },
		{ version: '1.1.0', date: '2026-08-15', notes: 'Subtitles from outside sites.' },
		{ version: '1.0.0', date: '2026-07-30', notes: 'First release.' }
	];
	const isDemo = $derived(import.meta.env.DEV && page.url.searchParams.get('releases') === 'demo');

	const list = $derived<ReleaseNote[]>(isDemo ? demo.map((r) => ({ ...r, lines: noteLines(r.notes) })) : whatsNew(upd));
	const current = $derived(isDemo ? '1.3.2' : upd?.version ? plainVersion(upd.version) : undefined);
	// Call the first row newest only when a check ran and worked.
	const known = $derived(isDemo || !!(upd && upd.on && !upd.problem && !upd.no_release && upd.checked_at));
	const isOpen = (r: ReleaseNote, i: number) => open[r.version] ?? i === 0;
</script>

{#snippet head(r: ReleaseNote, i: number, notes: boolean)}
	<span class="chev" aria-hidden="true">{#if notes}<svg viewBox="0 0 20 20"><path d="M7 5l5 5-5 5" /></svg>{/if}</span>
	<span class="rv">{plainVersion(r.version)}</span>
	{#if i === 0 && known}<span class="chip ok">{t('updates.releases.newest')}</span>{/if}
	{#if r.version === current}<span class="chip red">{t('updates.releases.yours')}</span>{/if}
	{#if r.date}<span class="rd">{ago(r.date)}</span>{/if}
{/snippet}

<section class="st-plate" aria-labelledby="s-rel" data-testid="whats-new">
	<h2 id="s-rel">{t('updates.releases.title')}{#if list.length > 1}<span class="cnt">{list.length}</span>{/if}</h2>

	{#if !list.length}<p class="none">{t('updates.releases.loading')}</p>{/if}
	<ol class="rels">
		{#each list.slice(0, shown) as r, i (r.version)}
			{@const o = r.lines.length > 0 && isOpen(r, i)}
			<li class="rel" class:open={o}>
				{#if r.lines.length}
					<button type="button" class="rh" aria-expanded={o} onclick={() => (open[r.version] = !o)}>{@render head(r, i, true)}</button>
				{:else}
					<div class="rh">{@render head(r, i, false)}</div>
				{/if}
				{#if o}
					<div class="body">
						<ul class="notes">{#each r.lines as line}<li>{line}</li>{/each}</ul>
						{#if r.link}<a class="xbtn" href={r.link} target="_blank" rel="noopener noreferrer">{t('updates.releases.page')}</a>{/if}
					</div>
				{/if}
			</li>
		{/each}
	</ol>
	{#if list.length > shown}
		<div class="more"><Button variant="secondary" full onclick={() => (shown += STEP)}>{t('updates.releases.more', { n: Math.min(STEP, list.length - shown), older: list.length - shown })}</Button></div>
	{:else if shown > STEP}
		<div class="more"><Button variant="secondary" full onclick={() => (shown = STEP)}>{t('updates.releases.fewer')}</Button></div>
	{/if}
</section>

<style>
	.none { margin: 0; font-size: 14px; color: var(--text-muted); }
	.rels { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
	.rel { border: 1px solid var(--line); border-radius: 12px; background: var(--surface-1); overflow: hidden; }
	.rel.open { border-color: var(--line-strong); box-shadow: 0 8px 20px -14px rgb(0 0 0 / 0.35); }
	.rh {
		display: flex; align-items: center; flex-wrap: wrap; gap: 6px 10px; width: 100%; min-height: 48px; padding: 0 14px; border: 0; background: none;
		color: var(--text); font: inherit; text-align: left; box-sizing: border-box;
	}
	button.rh { cursor: pointer; }
	.rh:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }
	.chev { display: grid; place-items: center; width: 18px; color: var(--text-muted); }
	.chev svg { width: 16px; height: 16px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; transition: transform 160ms var(--ease-out); }
	.rel.open .chev svg { transform: rotate(90deg); }
	.rv { font: 700 15.5px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.rd { margin-left: auto; font-size: 12.5px; color: var(--text-muted); }
	.body { display: flex; flex-direction: column; gap: 12px; padding: 4px 14px 14px 42px; border-top: 1px solid var(--line); }
	.notes { margin: 10px 0 0; padding-left: 18px; font-size: 14px; line-height: 1.6; color: var(--text); }
	/* Looks like the secondary Button, which cannot open a new tab. */
	.xbtn { display: inline-flex; align-items: center; justify-content: center; align-self: flex-start; min-height: 36px; padding: 0 16px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-headline); font-size: 15px; letter-spacing: -0.01em; text-decoration: none; white-space: nowrap; }
	.xbtn:hover { background: var(--surface-3); }
	.xbtn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .xbtn { min-height: 44px; } }
	.more { margin-top: 12px; }
	@media (max-width: 767px) {
		.body { padding-left: 14px; }
	}
	@media (prefers-reduced-motion: reduce) { .chev svg { transition: none; } }
</style>

<script lang="ts">
	// Layout 1 · Cards (Seerr-like): a grid of backdrop cards. Problems lead
	// with a red edge; the progress runs along each card's bottom edge.
	import Poster from '../Poster.svelte';
	import Button from '../ui/Button.svelte';
	import Icon from '../Icon.svelte';
	import EmptyState from '../ui/EmptyState.svelte';
	import DlArt from './DlArt.svelte';
	import DlMore from './DlMore.svelte';
	import DlImports from './DlImports.svelte';
	import { to } from '../paths.ts';
	import { ago, bytes, duration, percent } from '../format.ts';
	import { rowName, manyWords, type DownloadRow } from '../downloaders.ts';
	import { finished, isProblem, moving, needsAttention, note, queue, tone } from './layouts.ts';
	import { backdrop, poster, who, type LayoutProps } from './shared.ts';

	let { rows, imports, titles, busy, ondetails, onfail, onremove }: LayoutProps = $props();
	const now = $derived(queue(rows));
	const done = $derived(finished(rows));
	const attention = $derived(now.filter(needsAttention).length);
	const going = $derived(now.filter((r) => r.phase === 'downloading' || r.phase === 'importing' || r.phase === 'waiting_to_import').length);
	const waiting = $derived(now.filter((r) => r.phase === 'waiting').length);
	const glyph = { info: 'download', ok: 'check', fail: 'alert', idle: 'pause', wait: 'clock' } as const;
</script>

{#snippet card(r: DownloadRow, small = false)}
	{@const pct = percent(r.progress)}
	{@const t = tone(r)}
	{@const n = note(r)}
	<article class="dcard {t}" class:problem={isProblem(r)} class:mini={small} data-testid="dl-card" data-phase={r.phase}>
		<DlArt wide={backdrop(r, titles)} tall={poster(r, titles)} />
		<span class="scrim" aria-hidden="true"></span>
		<div class="top">
			<span class="state-chip" data-testid="row-state"><Icon name={glyph[t]} size={small ? 14 : 16} stroke={2.2} />{r.label}</span>
			<DlMore {r} name={who(r)} {busy} {ondetails} {onfail} {onremove} on="art" details={false} />
		</div>
		<div class="main">
			<div class="names">
				{#if r.title_id}<a class="nm" href={to(`/library/${r.title_id}`)}>{rowName(r)}</a>{:else}<span class="nm">{rowName(r)}</span>{/if}
				{#if manyWords(r.kind) && r.covers}<span class="sub">{r.covers}</span>{/if}
				{#if r.version && r.version !== 'Main'}<span class="ver">{r.version}</span>{/if}
			</div>
			{#if !small}<span class="inset"><Poster src={poster(r, titles)} name={r.title || r.name} /></span>{/if}
		</div>
		{#if n && !small}<p class="why" class:bad={t === 'fail'}>{n}</p>{/if}
		<div class="foot">
			<p class="meta">
				{#if moving(r) && r.phase !== 'done'}<b data-testid="row-percent">{pct}%</b>{/if}
				{#if r.active && r.eta && r.phase === 'downloading'}<span data-testid="row-eta">{duration(r.eta)} left</span>{/if}
				{#if r.active && r.speed && r.phase === 'downloading'}<span>{bytes(r.speed)}/s</span>{/if}
				{#if !r.active}<span>{ago(r.finished_at ?? r.updated_at)}</span>{/if}
				{#if r.size}<span class="size">{bytes(r.size)}</span>{/if}
			</p>
			<span class="acts">
				{#if r.phase === 'needs_a_decision'}<a class="glass strong" href={to('/downloads/decide')}>Decide</a>{/if}
				<button type="button" class="glass" aria-label="Details for {who(r)}" onclick={() => ondetails(r)}>Details</button>
			</span>
		</div>
		{#if moving(r) && r.phase !== 'done'}
			<span class="edge" aria-hidden="true"><i style="width: {pct}%"></i></span>
		{/if}
	</article>
{/snippet}

<div class="cards-view">
	<section aria-labelledby="c-now">
		<header class="sec">
			<h2 id="c-now">Now</h2><span class="count">{now.length}</span>
			{#if now.length}
				<p class="sum">
					{#if attention}<span class="s fail"><i></i>{attention} need{attention === 1 ? 's' : ''} you</span>{/if}
					{#if going}<span class="s info"><i></i>{going} downloading</span>{/if}
					{#if waiting}<span class="s wait"><i></i>{waiting} waiting</span>{/if}
				</p>
			{/if}
		</header>
		{#if now.length === 0}
			<div class="empty" data-testid="dl-empty">
				<EmptyState icon="download" title="Nothing is downloading" text="When HolyArrs picks a Release, it shows up here as a card with its progress along the bottom." action={{ label: 'Add media', href: to('/search'), icon: 'search' }} />
			</div>
		{:else}
			<div class="grid" class:one={now.length === 1}>{#each now as r (r.id)}{@render card(r)}{/each}</div>
		{/if}
	</section>

	{#if done.length}
		<section aria-labelledby="c-done">
			<header class="sec"><h2 id="c-done">Finished</h2><span class="count">{done.length}</span></header>
			<div class="grid mini">{#each done as r (r.id)}{@render card(r, true)}{/each}</div>
		</section>
	{/if}

	<DlImports {imports} />
</div>

<style>
	.cards-view { display: flex; flex-direction: column; gap: 32px; min-width: 0; }
	.sec { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin-bottom: 12px; }
	.sec h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.count { min-width: 24px; height: 24px; padding: 0 8px; display: inline-grid; place-items: center; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.sum { display: flex; flex-wrap: wrap; gap: 4px 14px; margin-left: auto; color: var(--text-muted); font: var(--t-subhead); }
	.s { display: inline-flex; align-items: center; gap: 6px; }
	.s i { width: 7px; height: 7px; border-radius: 50%; background: var(--info); }
	.s.fail i { background: var(--fail); }
	.s.wait i { background: var(--wait); }

	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 360px), 1fr)); gap: 16px; }
	.grid.one { grid-template-columns: minmax(0, 520px); }
	.grid.mini { grid-template-columns: repeat(auto-fill, minmax(min(100%, 260px), 1fr)); gap: 12px; }

	.dcard { position: relative; isolation: isolate; display: flex; flex-direction: column; min-height: 212px; padding: 14px 16px 18px; border-radius: var(--r-card); overflow: hidden; color: var(--on-art); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	.dcard:hover { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
	.dcard.mini { min-height: 132px; padding: 12px 14px 14px; }
	.dcard.problem { box-shadow: inset 4px 0 0 var(--fail-on-art), 0 0 0 1px color-mix(in srgb, var(--fail) 55%, transparent), var(--shadow-card); }
	.dcard.problem::after { content: ''; position: absolute; inset: 0 auto 0 0; width: 4px; background: var(--fail-on-art); z-index: 3; }
	.scrim { position: absolute; inset: 0; z-index: 0; background: linear-gradient(90deg, var(--on-art-scrim) 0%, var(--on-art-scrim-mid) 58%, rgb(0 0 0 / 30%) 100%), linear-gradient(0deg, rgb(0 0 0 / 55%), transparent 55%); }
	.dcard > :not(.scrim, :global(.dl-art)) { position: relative; z-index: 1; }

	.top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
	.state-chip { display: inline-flex; align-items: center; gap: 7px; height: 32px; padding: 0 14px 0 11px; border-radius: var(--r-pill); background: rgb(0 0 0 / 45%); -webkit-backdrop-filter: blur(14px) saturate(160%); backdrop-filter: blur(14px) saturate(160%); box-shadow: inset 0 0 0 1px var(--on-art-ring); font: var(--t-headline); font-size: 14px; white-space: nowrap; max-width: 100%; overflow: hidden; text-overflow: ellipsis; }
	.mini .state-chip { height: 26px; font: var(--t-subhead); font-weight: 600; padding: 0 10px 0 8px; }
	.state-chip :global(svg) { color: var(--info-on-art); flex: none; }
	.ok .state-chip :global(svg) { color: var(--ok-on-art); }
	.fail .state-chip :global(svg) { color: var(--fail-on-art); }
	.wait .state-chip :global(svg) { color: var(--wait-on-art); }
	.idle .state-chip :global(svg) { color: var(--on-art-muted); }

	.main { display: flex; align-items: flex-end; justify-content: space-between; gap: 14px; margin-top: auto; padding-top: 14px; }
	.names { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.nm { color: var(--on-art); font: var(--t-title3); letter-spacing: var(--t-title3-track); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; text-shadow: 0 1px 12px rgb(0 0 0 / 40%); }
	.mini .nm { font: var(--t-headline); }
	a.nm:hover { text-decoration: underline; text-underline-offset: 3px; }
	a.nm:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 4px; }
	.sub { color: var(--on-art-soft); font: var(--t-callout); font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ver { align-self: flex-start; margin-top: 4px; padding: 1px 7px; border-radius: 5px; background: var(--on-art-fill); font: var(--t-caption); }
	.inset { flex: none; width: 74px; margin-bottom: -2px; border-radius: 7px; box-shadow: 0 8px 22px rgb(0 0 0 / 50%); }
	.why { margin-top: 10px; padding: 6px 10px; border-radius: 8px; background: rgb(0 0 0 / 38%); -webkit-backdrop-filter: blur(10px); backdrop-filter: blur(10px); color: var(--on-art-soft); font: var(--t-subhead); font-weight: 500; overflow-wrap: anywhere; }
	.why.bad { box-shadow: inset 2px 0 0 var(--fail-on-art); color: var(--on-art); }

	.foot { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-top: 12px; }
	.meta { display: flex; flex-wrap: wrap; align-items: baseline; gap: 2px 12px; min-width: 0; color: var(--on-art-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.meta b { color: var(--on-art); font: var(--t-headline); font-size: 17px; font-variant-numeric: tabular-nums; }
	.acts { display: inline-flex; gap: 6px; flex: none; }
	.glass { display: inline-flex; align-items: center; min-height: 32px; padding: 0 13px; border: 0; border-radius: var(--r-pill); background: var(--on-art-fill); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); color: var(--on-art); font: var(--t-subhead); font-weight: 600; cursor: pointer; white-space: nowrap; transition: background var(--dur-fast); }
	.glass:hover { background: var(--on-art-fill-hover); }
	.glass:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.glass.strong { background: var(--on-art-fill-strong); color: var(--on-art-ink); }

	.edge { position: absolute !important; z-index: 2 !important; left: 0; right: 0; bottom: 0; height: 4px; background: rgb(255 255 255 / 18%); }
	.edge i { display: block; height: 100%; background: var(--info-on-art); border-radius: 0 2px 2px 0; transition: width var(--dur-slow) var(--ease-out); }
	.fail .edge i { background: var(--fail-on-art); }
	.idle .edge i { background: var(--on-art-muted); }
	.wait .edge i { background: var(--wait-on-art); }

	.empty { border-radius: var(--r-panel); border: 1.5px dashed var(--line-strong); background: repeating-linear-gradient(135deg, transparent 0 14px, color-mix(in srgb, var(--surface-2) 70%, transparent) 14px 15px); }

	@media (max-width: 767px) {
		.grid { gap: 12px; }
		.dcard { min-height: 196px; padding: 12px 14px 16px; }
		.inset { width: 60px; }
		.nm { font: var(--t-headline); font-size: 17px; }
		.size { display: none; }
	}
	@media (prefers-reduced-motion: reduce) { .dcard, .edge i { transition: none; } .dcard:hover { transform: none; } }
</style>

<script lang="ts">
	// Layout 3 · Up Next (Apple TV-like): the download closest to done as a
	// hero, the rest of the queue as a row, and what needs a person in a short
	// section of its own with the actions right there.
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Button from '../ui/Button.svelte';
	import StatusPill from '../ui/StatusPill.svelte';
	import DlArt from './DlArt.svelte';
	import DlMore from './DlMore.svelte';
	import DlImports from './DlImports.svelte';
	import { to } from '../paths.ts';
	import { ago, bytes, duration, percent } from '../format.ts';
	import { rowName, manyWords, type DownloadRow } from '../downloaders.ts';
	import { finished, needsAttention, note, pillKind, queue, tone } from './layouts.ts';
	import { backdrop, logo, poster, who, type LayoutProps } from './shared.ts';

	let { rows, imports, titles, busy, ondetails, onfail, onremove }: LayoutProps = $props();
	const all = $derived(queue(rows));
	const attention = $derived(all.filter(needsAttention));
	const flowing = $derived(all.filter((r) => !needsAttention(r)));
	// The hero: what is downloading and closest to done; else the first in line.
	const hero = $derived.by(() => {
		const going = flowing.filter((r) => r.phase === 'downloading' && r.active);
		going.sort((a, b) => (a.eta ?? Infinity) - (b.eta ?? Infinity) || b.progress - a.progress);
		return going[0] ?? flowing[0] ?? null;
	});
	const next = $derived(flowing.filter((r) => r !== hero));
	const done = $derived(finished(rows));
	let logoBroken = $state(false);
	$effect(() => {
		void hero?.id;
		logoBroken = false;
	});
</script>

{#snippet tile(r: DownloadRow)}
	{@const pct = percent(r.progress)}
	{@const t = tone(r)}
	<li class="tile" data-testid="dl-tile" data-phase={r.phase}>
		<button type="button" class="tart" aria-label="Details for {who(r)}" onclick={() => ondetails(r)}>
			<DlArt wide={backdrop(r, titles)} tall={poster(r, titles)} />
			<span class="tscrim" aria-hidden="true"></span>
			<span class="tstate"><StatusPill kind={pillKind(r)} label={r.phase === 'downloading' ? `${pct}%` : r.label} on="art" /></span>
			{#if r.phase !== 'done'}<span class="tedge {t}" aria-hidden="true"><i style="width: {pct}%"></i></span>{/if}
		</button>
		<div class="tmeta">
			<div class="ttxt">
				<p class="tnm" title={rowName(r)}>{rowName(r)}</p>
				<p class="tsub">
					{#if manyWords(r.kind) && r.covers}{r.covers} · {/if}{#if r.phase === 'downloading' && r.eta}{duration(r.eta)} left{:else if r.phase === 'done'}{ago(r.finished_at ?? r.updated_at)}{:else}{r.label}{/if}
				</p>
			</div>
			<DlMore {r} name={who(r)} {busy} {ondetails} {onfail} {onremove} />
		</div>
	</li>
{/snippet}

<div class="un">
	{#if hero}
		{@const pct = percent(hero.progress)}
		{@const lg = logo(hero, titles)}
		<section class="hero {tone(hero)}" aria-label="Downloading now" data-testid="dl-hero" data-phase={hero.phase}>
			<DlArt wide={backdrop(hero, titles, 'large')} tall={poster(hero, titles)} eager />
			<span class="hscrim" aria-hidden="true"></span>
			<div class="hbody">
				<p class="eyebrow"><span class="live" aria-hidden="true"></span>{hero.phase === 'downloading' ? 'Downloading now' : hero.label}</p>
				{#if lg && !logoBroken}
					<img class="logo" src={lg} alt={rowName(hero)} onerror={() => (logoBroken = true)} />
				{:else}
					<h2 class="htitle">{rowName(hero)}</h2>
				{/if}
				{#if manyWords(hero.kind) && hero.covers}<p class="hsub">{hero.covers}</p>{/if}
				<div class="hprog">
					<p class="big" data-testid="row-percent">{pct}<small>%</small></p>
					<div class="hbar" role="progressbar" aria-label="Progress of {who(hero)}" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)}><i style="width: {pct}%"></i></div>
				</div>
				<dl class="hfacts">
					{#if hero.active && hero.eta}<div><dt>Time left</dt><dd data-testid="row-eta">{duration(hero.eta)}</dd></div>{/if}
					{#if hero.active && hero.speed && hero.phase === 'downloading'}<div><dt>Speed</dt><dd>{bytes(hero.speed)}/s</dd></div>{/if}
					{#if hero.size}<div><dt>Size</dt><dd>{bytes(hero.size)}</dd></div>{/if}
					{#if hero.version}<div class="wide-only"><dt>Version</dt><dd>{hero.version}</dd></div>{/if}
				</dl>
				{#if note(hero) && hero.phase !== 'downloading'}<p class="hnote">{note(hero)}</p>{/if}
				<div class="hacts">
					<button type="button" class="hbtn strong" aria-label="Details for {who(hero)}" onclick={() => ondetails(hero)}><Icon name="info" size={18} />Details</button>
					{#if hero.title_id}<a class="hbtn" href={to(`/library/${hero.title_id}`)}>Open title</a>{/if}
					<DlMore r={hero} name={who(hero)} {busy} {ondetails} {onfail} {onremove} on="art" details={false} />
				</div>
			</div>
			<span class="hposter" aria-hidden="true"><Poster src={poster(hero, titles)} name={hero.title || hero.name} /></span>
		</section>
	{:else}
		<section class="hero calm" aria-label="Downloading now" data-testid="dl-empty">
			<div class="hbody">
				<span class="calm-ic" aria-hidden="true"><Icon name="check" size={30} stroke={2} /></span>
				<h2 class="htitle">All quiet</h2>
				<p class="hsub">{attention.length ? 'Nothing is downloading right now. The items below need you first.' : 'Nothing is downloading. When HolyArrs picks a Release, it plays out here.'}</p>
				<div class="hacts"><a class="hbtn strong" href={to('/search')}><Icon name="search" size={18} />Add media</a></div>
			</div>
		</section>
	{/if}

	{#if next.length}
		<section aria-labelledby="u-next">
			<header class="sec"><h2 id="u-next">Up next in the queue</h2><span class="count">{next.length}</span></header>
			<ul class="shelf">{#each next as r (r.id)}{@render tile(r)}{/each}</ul>
		</section>
	{/if}

	{#if attention.length}
		<section aria-labelledby="u-att" class="att">
			<header class="sec"><h2 id="u-att">Needs attention</h2><span class="count bad">{attention.length}</span></header>
			<ul class="alist">
				{#each attention as r (r.id)}
					{@const n = note(r)}
					<li class="arow {tone(r)}" data-testid="dl-attention" data-phase={r.phase}>
						<span class="apost"><Poster src={poster(r, titles)} name={r.title || r.name} /></span>
						<div class="atxt">
							<div class="ahead">
								{#if r.title_id}<a class="anm" href={to(`/library/${r.title_id}`)}>{rowName(r)}</a>{:else}<span class="anm">{rowName(r)}</span>{/if}
								<StatusPill kind={pillKind(r)} label={r.label} testid="row-state" />
							</div>
							{#if manyWords(r.kind) && r.covers}<p class="asub">{r.covers}</p>{/if}
							{#if n}<p class="awhy">{n}{#if r.phase === 'paused'}{' '}Resume it in the Downloader, or remove it.{/if}</p>{/if}
						</div>
						<div class="aacts">
							{#if r.phase === 'needs_a_decision'}<Button size="sm" variant="primary" href={to('/downloads/decide')}>Decide</Button>{/if}
							{#if r.can_act && r.phase !== 'paused'}<Button size="sm" variant="secondary" onclick={() => onfail(r)} disabled={busy === r.id}>Mark failed</Button>{/if}
							{#if r.can_act}<Button size="sm" variant="secondary" onclick={() => onremove(r)} disabled={busy === r.id}>Remove</Button>{/if}
							<Button size="sm" variant="plain" aria-label="Details for {who(r)}" onclick={() => ondetails(r)}>Details</Button>
							<DlMore {r} name={who(r)} {busy} {ondetails} {onfail} {onremove} details={false} />
						</div>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	{#if done.length}
		<section aria-labelledby="u-done">
			<header class="sec"><h2 id="u-done">Recently finished</h2><span class="count">{done.length}</span></header>
			<ul class="shelf done-shelf">{#each done as r (r.id)}{@render tile(r)}{/each}</ul>
		</section>
	{/if}

	<DlImports {imports} compact />
</div>

<style>
	.un { display: flex; flex-direction: column; gap: 34px; min-width: 0; }
	.sec { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; }
	.sec h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.count { min-width: 24px; height: 24px; padding: 0 8px; display: inline-grid; place-items: center; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.count.bad { background: color-mix(in srgb, var(--fail) 14%, transparent); color: var(--fail-text); }

	/* Hero */
	.hero { position: relative; isolation: isolate; min-height: 380px; display: flex; align-items: flex-end; border-radius: var(--r-panel); overflow: hidden; color: var(--on-art); box-shadow: var(--shadow-lift); }
	.hscrim { position: absolute; inset: 0; z-index: 0; background: linear-gradient(90deg, rgb(0 0 0 / 88%) 0%, rgb(0 0 0 / 62%) 42%, rgb(0 0 0 / 10%) 78%), linear-gradient(0deg, rgb(0 0 0 / 70%), transparent 60%); }
	.hbody { position: relative; z-index: 1; display: flex; flex-direction: column; align-items: flex-start; gap: 10px; width: min(560px, 100%); padding: 32px 36px; }
	.eyebrow { display: inline-flex; align-items: center; gap: 8px; color: var(--on-art-soft); font: var(--t-caption); letter-spacing: 0.08em; text-transform: uppercase; }
	.live { width: 8px; height: 8px; border-radius: 50%; background: var(--info-on-art); box-shadow: 0 0 0 0 var(--info-on-art); animation: pulse 2s var(--ease-out) infinite; }
	.fail .live { background: var(--fail-on-art); }
	.idle .live, .wait .live { background: var(--wait-on-art); animation: none; }
	@keyframes pulse { 0% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--info-on-art) 60%, transparent); } 70%, 100% { box-shadow: 0 0 0 10px transparent; } }
	.logo { max-width: min(340px, 80%); max-height: 96px; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 2px 14px rgb(0 0 0 / 50%)); }
	.htitle { font: var(--t-large-title); letter-spacing: var(--t-large-title-track); overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; text-shadow: 0 2px 18px rgb(0 0 0 / 40%); }
	.hsub { color: var(--on-art-soft); font: var(--t-headline); font-weight: 500; }
	.hprog { display: flex; align-items: center; gap: 16px; width: 100%; margin-top: 6px; }
	.big { font: 700 52px / 1 var(--font-display); letter-spacing: -0.04em; font-variant-numeric: tabular-nums; }
	.big small { font-size: 0.45em; margin-left: 2px; color: var(--on-art-muted); letter-spacing: 0; }
	.hbar { flex: 1; height: 8px; border-radius: 4px; background: rgb(255 255 255 / 22%); overflow: hidden; }
	.hbar i { display: block; height: 100%; border-radius: 4px; background: var(--on-art); transition: width var(--dur-slow) var(--ease-out); }
	.hfacts { display: flex; flex-wrap: wrap; gap: 6px 28px; margin: 0; }
	.hfacts div { display: flex; flex-direction: column; }
	.hfacts dt { color: var(--on-art-muted); font: var(--t-caption); letter-spacing: 0.04em; text-transform: uppercase; }
	.hfacts dd { margin: 0; font: var(--t-headline); font-variant-numeric: tabular-nums; }
	.hnote { color: var(--on-art-soft); font: var(--t-callout); }
	.hacts { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin-top: 8px; }
	.hbtn { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 20px; border: 0; border-radius: var(--r-pill); background: var(--on-art-fill); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); color: var(--on-art); font: var(--t-headline); font-size: 15px; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.hbtn:hover { background: var(--on-art-fill-hover); }
	.hbtn.strong { background: var(--on-art-fill-strong); color: var(--on-art-ink); }
	.hbtn.strong:hover { background: var(--on-art); }
	.hbtn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.hacts :global(.dl-more button) { width: 44px; height: 44px; }
	.hposter { position: absolute; z-index: 1; right: 36px; bottom: 32px; width: 132px; border-radius: 8px; box-shadow: 0 18px 40px rgb(0 0 0 / 55%); }
	.hero.calm { min-height: 260px; align-items: center; justify-content: center; text-align: center; background: radial-gradient(120% 140% at 50% 0%, color-mix(in srgb, var(--ok) 26%, #1a1a1d), #0d0d0f 70%); }
	.calm .hbody { align-items: center; }
	.calm-ic { display: grid; place-items: center; width: 64px; height: 64px; border-radius: 50%; background: var(--on-art-fill); color: var(--ok-on-art); box-shadow: inset 0 0 0 1px var(--on-art-ring); }
	.calm .hsub { max-width: 420px; font: var(--t-body); }

	/* Needs attention */
	.alist { list-style: none; margin: 0; padding: 6px; display: flex; flex-direction: column; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.arow { display: grid; grid-template-columns: 44px minmax(0, 1fr) auto; align-items: center; gap: 14px; padding: 12px 12px 12px 14px; border-radius: var(--r-btn); box-shadow: inset 3px 0 0 var(--fail); }
	.arow.idle { box-shadow: inset 3px 0 0 var(--text-faint); }
	.arow + .arow { margin-top: 2px; }
	.apost { width: 44px; }
	.atxt { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
	.ahead { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 10px; }
	.anm { color: var(--text); font: var(--t-headline); overflow-wrap: anywhere; }
	a.anm:hover { text-decoration: underline; text-underline-offset: 3px; }
	.asub { color: var(--text-muted); font: var(--t-callout); }
	.awhy { color: var(--text-muted); font: var(--t-callout); overflow-wrap: anywhere; }
	.aacts { display: flex; align-items: center; flex-wrap: wrap; justify-content: flex-end; gap: 6px; }

	/* Shelf */
	.shelf { list-style: none; margin: 0 calc(-1 * var(--gutter)); padding: 4px var(--gutter) 10px; display: grid; grid-auto-flow: column; grid-auto-columns: 280px; gap: 16px; overflow-x: auto; scroll-snap-type: x mandatory; scroll-padding-inline: var(--gutter); scrollbar-width: thin; }
	.shelf.done-shelf { grid-auto-columns: 220px; }
	.tile { scroll-snap-align: start; min-width: 0; display: flex; flex-direction: column; gap: 8px; }
	.tart { position: relative; display: block; width: 100%; aspect-ratio: 16 / 9; padding: 0; border: 0; border-radius: var(--r-card); overflow: hidden; cursor: pointer; box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	.tart:hover { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift); }
	.tart:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tscrim { position: absolute; inset: 0; background: linear-gradient(0deg, rgb(0 0 0 / 55%), transparent 55%); }
	.tstate { position: absolute; left: 8px; bottom: 12px; }
	.tedge { position: absolute; left: 0; right: 0; bottom: 0; height: 4px; background: rgb(255 255 255 / 20%); }
	.tedge i { display: block; height: 100%; background: var(--info-on-art); }
	.tedge.wait i { background: var(--wait-on-art); }
	.tmeta { display: flex; align-items: flex-start; justify-content: space-between; gap: 6px; }
	.ttxt { min-width: 0; }
	.tnm { color: var(--text); font: var(--t-headline); font-size: 15px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.tsub { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-variant-numeric: tabular-nums; }

	@media (max-width: 1100px) {
		.hposter { display: none; }
	}
	@media (max-width: 767px) {
		.un { gap: 28px; }
		.hero { min-height: 0; aspect-ratio: 4 / 5; max-height: 560px; border-radius: var(--r-card); }
		.hero.calm { aspect-ratio: auto; min-height: 240px; }
		.hscrim { background: linear-gradient(0deg, rgb(0 0 0 / 92%) 0%, rgb(0 0 0 / 72%) 45%, rgb(0 0 0 / 0%) 78%); }
		.hbody { padding: 20px 18px; gap: 8px; }
		.htitle { font: var(--t-title1); letter-spacing: var(--t-title1-track); }
		.logo { max-height: 72px; max-width: 72%; }
		.big { font-size: 40px; }
		.hfacts { gap: 4px 20px; }
		.wide-only { display: none !important; }
		.hacts { width: 100%; }
		.hbtn.strong { flex: 1; justify-content: center; }
		.sec h2 { font: var(--t-title3); }
		.arow { grid-template-columns: 40px minmax(0, 1fr); gap: 12px; padding: 12px 10px 12px 12px; }
		.apost { width: 40px; }
		.aacts { grid-column: 1 / -1; justify-content: flex-start; }
		.aacts :global(.ui-btn) { min-height: 40px; }
		.shelf { grid-auto-columns: 74%; gap: 12px; }
		.shelf.done-shelf { grid-auto-columns: 58%; }
	}
	@media (prefers-reduced-motion: reduce) { .live { animation: none; } .tart, .hbar i, .hbtn { transition: none; } .tart:hover { transform: none; } }
</style>

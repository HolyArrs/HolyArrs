<script lang="ts">
	// Layout 2 · List (Netflix Downloads-like): grouped by show or movie, wide
	// thumbs with a progress ring, one clean line of status. The release name
	// and the rest of the facts live behind Details.
	import { SvelteSet } from 'svelte/reactivity';
	import Icon, { type IconName } from '../Icon.svelte';
	import Button from '../ui/Button.svelte';
	import DlArt from './DlArt.svelte';
	import DlMore from './DlMore.svelte';
	import DlImports from './DlImports.svelte';
	import ProgressRing from './ProgressRing.svelte';
	import { to } from '../paths.ts';
	import { ago, bytes, duration, percent } from '../format.ts';
	import { rowName, manyWords, type DownloadRow } from '../downloaders.ts';
	import { finished, groupByTitle, isProblem, note, queue, tone, type TitleGroup } from './layouts.ts';
	import { backdrop, poster, who, type LayoutProps } from './shared.ts';

	let { rows, imports, titles, busy, ondetails, onfail, onremove }: LayoutProps = $props();
	const groups = $derived(groupByTitle(queue(rows), rowName));
	const done = $derived(finished(rows));
	const count = $derived(groups.reduce((n, g) => n + g.rows.length, 0));
	const shut = new SvelteSet<string>();
	let allDone = $state(false);

	function ringIcon(r: DownloadRow): IconName | undefined {
		if (r.phase === 'done') return 'check';
		if (isProblem(r)) return 'alert';
		if (r.phase === 'paused') return 'pause';
		if (r.phase === 'waiting') return 'clock';
		return undefined;
	}
	/** One line: the state, then what matters for it. */
	function line(r: DownloadRow): { lead: string; rest: string } {
		if (r.phase === 'downloading') return { lead: r.label, rest: [r.eta ? `${duration(r.eta)} left` : '', r.speed ? `${bytes(r.speed)}/s` : ''].filter(Boolean).join(' · ') };
		if (r.phase === 'done') return { lead: r.label, rest: [ago(r.finished_at ?? r.updated_at), r.size ? bytes(r.size) : ''].filter(Boolean).join(' · ') };
		return { lead: r.label, rest: r.phase === 'waiting' ? (r.size ? bytes(r.size) : '') : (note(r) ?? '') };
	}
	function groupLine(g: TitleGroup): string {
		const w = manyWords(g.kind);
		const parts = [`${g.rows.length} ${g.rows.length === 1 ? (w?.one ?? 'download') : (w?.many ?? 'downloads')}`];
		const going = g.rows.filter((r) => r.phase === 'downloading').length;
		const bad = g.rows.filter(isProblem).length;
		if (bad) parts.push(`${bad} need${bad === 1 ? 's' : ''} you`);
		if (going) parts.push(`${going} downloading`);
		const size = g.rows.reduce((n, r) => n + (r.size || 0), 0);
		if (size) parts.push(bytes(size));
		return parts.join(' · ');
	}
	const groupPct = (g: TitleGroup) => g.rows.reduce((n, r) => n + percent(r.progress), 0) / g.rows.length;
	function toggle(key: string) {
		if (shut.has(key)) shut.delete(key);
		else shut.add(key);
	}
</script>

{#snippet thumb(r: DownloadRow, big: boolean)}
	{@const t = tone(r)}
	<button type="button" class="thumb" class:big aria-label="Details for {who(r)}" onclick={() => ondetails(r)}>
		<DlArt wide={backdrop(r, titles)} tall={poster(r, titles)} />
		<span class="dim" aria-hidden="true"></span>
		<span class="ring-at"><ProgressRing value={r.phase === 'done' ? 100 : percent(r.progress)} label="Progress of {who(r)}" size={big ? 46 : 40} tone={t} icon={ringIcon(r)} on="art" /></span>
	</button>
{/snippet}

{#snippet item(r: DownloadRow, child: boolean)}
	{@const l = line(r)}
	{@const t = tone(r)}
	<li class="nrow" class:child class:problem={isProblem(r)} data-testid="dl-row" data-phase={r.phase}>
		{@render thumb(r, !child)}
		<div class="txt">
			{#if child}
				<p class="nm">{r.covers || rowName(r)}</p>
			{:else}
				{#if r.title_id}<a class="nm" href={to(`/library/${r.title_id}`)}>{rowName(r)}</a>{:else}<p class="nm">{rowName(r)}</p>{/if}
				{#if manyWords(r.kind) && r.covers}<p class="sub">{r.covers}</p>{/if}
			{/if}
			<p class="line {t}" data-testid="row-state" title={l.rest ? `${l.lead} · ${l.rest}` : l.lead}>
				<b>{l.lead}</b>{#if r.phase === 'downloading'}<span class="pct" data-testid="row-percent">&nbsp;· {percent(r.progress)}%</span>{/if}{#if l.rest}<span>&nbsp;· {l.rest}</span>{/if}
			</p>
		</div>
		<div class="end">
			{#if r.phase === 'needs_a_decision'}<Button size="sm" variant="secondary" href={to('/downloads/decide')}>Decide</Button>{/if}
			<DlMore {r} name={who(r)} {busy} {ondetails} {onfail} {onremove} />
		</div>
	</li>
{/snippet}

<div class="nf">
	<section aria-labelledby="n-now">
		<header class="sec"><h2 id="n-now">Now</h2><span class="count">{count}</span></header>
		{#if groups.length === 0}
			<div class="empty" data-testid="dl-empty">
				<span class="big-ic"><Icon name="download" size={40} stroke={1.5} /></span>
				<h3>Nothing is downloading</h3>
				<p>Movies and episodes that HolyArrs picks show up here while they download, grouped by show.</p>
				<Button variant="primary" icon="search" href={to('/search')}>Find something to add</Button>
			</div>
		{:else}
			<ul class="list">
				{#each groups as g (g.key)}
					{#if g.rows.length === 1}
						{@render item(g.rows[0], false)}
					{:else}
						{@const open = !shut.has(g.key)}
						{@const lead = g.rows[0]}
						<li class="group" class:problem={g.rows.some(isProblem)} data-testid="dl-group">
							<button type="button" class="ghead" aria-expanded={open} aria-controls="g-{g.key}" onclick={() => toggle(g.key)}>
								<span class="thumb big static">
									<DlArt wide={backdrop(lead, titles)} tall={poster(lead, titles)} />
									<span class="dim" aria-hidden="true"></span>
									<span class="ring-at"><ProgressRing value={groupPct(g)} label="Progress of {g.name}" size={46} tone={g.rows.some(isProblem) ? 'fail' : 'info'} on="art" /></span>
									<span class="count-badge" aria-hidden="true">{g.rows.length}</span>
								</span>
								<span class="txt">
									<span class="nm">{g.name}</span>
									<span class="line">{groupLine(g)}</span>
								</span>
								<span class="chev" class:open aria-hidden="true"><Icon name="chevronDown" size={20} /></span>
							</button>
							{#if open}
								<ul class="kids" id="g-{g.key}">{#each g.rows as r (r.id)}{@render item(r, true)}{/each}</ul>
							{/if}
						</li>
					{/if}
				{/each}
			</ul>
		{/if}
	</section>

	{#if done.length}
		<section aria-labelledby="n-done">
			<header class="sec"><h2 id="n-done">Done</h2><span class="count">{done.length}</span></header>
			<ul class="list quiet">{#each allDone ? done : done.slice(0, 3) as r (r.id)}{@render item(r, false)}{/each}</ul>
			{#if done.length > 3}<button type="button" class="more" onclick={() => (allDone = !allDone)}>{allDone ? 'Show fewer' : `Show all ${done.length}`}</button>{/if}
		</section>
	{/if}

	<DlImports {imports} compact />
</div>

<style>
	.nf { display: flex; flex-direction: column; gap: 32px; max-width: 880px; min-width: 0; }
	.sec { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
	.sec h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.count { min-width: 24px; height: 24px; padding: 0 8px; display: inline-grid; place-items: center; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.list, .kids { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
	.list { gap: 4px; }
	.nrow, .ghead { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 16px; padding: 8px; border-radius: var(--r-card); }
	.nrow { transition: background var(--dur-fast); }
	.nrow:hover, .ghead:hover { background: var(--surface-2); }
	.ghead { width: 100%; border: 0; background: none; color: inherit; text-align: left; cursor: pointer; font: inherit; }
	.ghead:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.group.problem > .ghead .line, .nrow.problem .line b { color: var(--fail-text); }
	.kids { margin: 2px 0 6px 36px; padding-left: 14px; border-left: 2px solid var(--line); }
	.nrow.child { gap: 14px; padding: 6px 8px; }

	.thumb { position: relative; display: block; flex: none; width: 128px; aspect-ratio: 16 / 9; padding: 0; border: 0; border-radius: 8px; overflow: hidden; cursor: pointer; background: var(--surface-3); box-shadow: 0 0 0 1px var(--line); }
	.thumb.big { width: 168px; }
	.thumb.static { cursor: inherit; }
	button.thumb:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.dim { position: absolute; inset: 0; background: rgb(0 0 0 / 38%); transition: background var(--dur-fast); }
	button.thumb:hover .dim { background: rgb(0 0 0 / 20%); }
	.ring-at { position: absolute; inset: 0; display: grid; place-items: center; }
	.ring-at :global(.ring) { border-radius: 50%; background: rgb(0 0 0 / 45%); -webkit-backdrop-filter: blur(6px); backdrop-filter: blur(6px); }
	.count-badge { position: absolute; right: 6px; bottom: 6px; min-width: 22px; height: 22px; padding: 0 6px; display: grid; place-items: center; border-radius: 6px; background: var(--on-art-fill-strong); color: var(--on-art-ink); font: var(--t-caption); }

	.txt { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
	.nm { color: var(--text); font: var(--t-headline); font-size: 16px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	a.nm:hover { text-decoration: underline; text-underline-offset: 3px; }
	a.nm:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 4px; }
	.child .nm { font-size: 15px; font-weight: 500; }
	.sub { color: var(--text-muted); font: var(--t-callout); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.line { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.line b { font-weight: 600; color: var(--text); }
	.line.info b { color: var(--info-text); }
	.line.fail b { color: var(--fail-text); }
	.line.ok b { color: var(--ok-text); }
	.end { display: inline-flex; align-items: center; gap: 4px; }
	.chev { display: grid; place-items: center; width: 36px; height: 36px; color: var(--text-muted); transition: transform var(--dur) var(--ease-out); }
	.chev.open { transform: rotate(180deg); }
	.quiet .thumb { width: 112px; }
	.more { margin-top: 6px; min-height: 44px; padding: 0 8px; border: 0; background: none; color: var(--accent-text); font: var(--t-callout); font-weight: 600; cursor: pointer; border-radius: 8px; }
	.more:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	.empty { display: flex; flex-direction: column; align-items: center; text-align: center; gap: 10px; padding: 56px 24px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); }
	.big-ic { display: grid; place-items: center; width: 96px; height: 96px; margin-bottom: 8px; border-radius: 50%; background: var(--surface-2); color: var(--text-muted); box-shadow: inset 0 0 0 6px var(--surface-1), inset 0 0 0 8px var(--line); }
	.empty h3 { font: var(--t-title3); }
	.empty p { max-width: 380px; color: var(--text-muted); font: var(--t-body); margin-bottom: 8px; }

	@media (max-width: 767px) {
		.nrow, .ghead { gap: 12px; padding: 8px 4px; }
		.thumb, .thumb.big { width: 116px; }
		.nrow.child .thumb { width: 96px; }
		.quiet .thumb { width: 96px; }
		.kids { margin-left: 12px; padding-left: 10px; }
		.nm { white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow-wrap: anywhere; }
		.line { white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
		.end :global(.ui-btn) { display: none; }
	}
	@media (max-width: 380px) {
		.thumb, .thumb.big, .nrow.child .thumb, .quiet .thumb { width: 88px; }
	}
	@media (prefers-reduced-motion: reduce) { .chev, .nrow, .dim { transition: none; } }
</style>

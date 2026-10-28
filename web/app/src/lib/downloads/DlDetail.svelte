<script lang="ts" module>
	/** The five Download details looks Luis is comparing (?details=1…5). */
	export type DetailLook = 'panel' | 'inline' | 'card' | 'drawer' | 'track';
</script>

<script lang="ts">
	// What a Download details view holds, in one of five looks. Every look
	// answers the same three questions in the same words (details.ts): what it
	// is, where it is, what happens next; then the actions HolyArrs has today.
	// Empty facts are left out. The containers (side panel, table row, card,
	// drawer) are the page's and the table's; this is only the inside.
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Button from '../ui/Button.svelte';
	import SegmentedControl from '../ui/SegmentedControl.svelte';
	import StatusPill from '../ui/StatusPill.svelte';
	import DecisionPanel from './DecisionPanel.svelte';
	import SlimBar from './SlimBar.svelte';
	import { to } from '../paths.ts';
	import { dateTime } from '../format.ts';
	import { plainReason } from '../plain.ts';
	import { episodeStateWords, manyWords, phaseTone, rowName, type Decision, type DownloadRow } from '../downloaders.ts';
	import type { ImportRecord } from '../types.ts';
	import { activityOf, detailFacts, filesOf, iconOf, type Fact } from './details.ts';
	import { statusDetail } from './imports.ts';
	import { statusWord } from './status-tabs.ts';
	import { tone } from './layouts.ts';

	let {
		r,
		im,
		decisions = [],
		art,
		admin = false,
		busy = 0,
		look,
		onfail,
		onremove,
		ondone
	}: {
		r: DownloadRow;
		im?: ImportRecord;
		decisions?: Decision[];
		art?: string;
		admin?: boolean;
		busy?: number;
		look: DetailLook;
		onfail: (r: DownloadRow) => void;
		onremove: (r: DownloadRow) => void;
		ondone?: () => void;
	} = $props();

	const f = $derived(detailFacts(r, im));
	const t = $derived(tone(r));
	const word = $derived(statusWord(r));
	const more = $derived(statusDetail(r, im));
	// The Downloader's own line ("63% done") only adds something when the download is stuck or failed.
	const note = $derived(r.text && !decisions.length && ['stalled', 'failed', 'decide', 'paused'].includes(f.stage) ? plainReason(r.text) : null);
	// A failed import already reads in the status line; the fact list does not repeat it.
	const nextFacts = $derived(more ? f.next.filter((x) => !x.value.startsWith('Failed')) : f.next);
	const w = $derived(manyWords(r.kind));
	const sub = $derived([w && r.covers ? r.covers : '', r.version && r.version !== 'Main' ? r.version : ''].filter(Boolean).join(' · '));
	// The header already shows progress, speed and time left.
	const inHead = new Set(['progress', 'speed', 'eta']);
	const whereRest = $derived(f.where.filter((x) => !inHead.has(x.key)));
	const live = $derived([f.where.find((x) => x.key === 'speed')?.value, f.where.find((x) => x.key === 'eta')?.value ? `${f.where.find((x) => x.key === 'eta')?.value} left` : ''].filter(Boolean).join(' · '));
	const files = $derived(filesOf(im));
	const story = $derived(activityOf(r, im));
	const eps = $derived(w && r.episodes && r.episodes.length > 1 ? r.episodes : []);
	const hasActs = $derived(r.can_act || !!r.title_id);

	let tab = $state<'overview' | 'files' | 'activity'>('overview');
	const tabs = $derived([
		{ value: 'overview' as const, label: 'Overview' },
		{ value: 'files' as const, label: files.length || eps.length ? `Files (${files.length || eps.length})` : 'Files' },
		{ value: 'activity' as const, label: 'Activity' }
	]);
</script>

{#snippet status()}
	<span class="st {t}" title={more ? `${word} · ${more}` : word}><Icon name={iconOf(r)} size={14} stroke={2.2} /><span>{word}{#if more}<span class="sd"> · {more}</span>{/if}</span></span>
{/snippet}

{#snippet head(withArt: boolean)}
	<div class="head" class:noart={!withArt}>
		{#if withArt}<span class="art" class:square={r.kind === 'artist'} aria-hidden="true"><Poster src={art} name={r.title || r.name} /></span>{/if}
		<div class="names">
			<p class="title" data-testid="detail-title">{rowName(r)}</p>
			{#if sub}<p class="sub">{sub}</p>{/if}
			{@render status()}
		</div>
	</div>
	{#if f.moving}
		<div class="prog">
			<SlimBar value={r.progress * 100} tone={t} label="Progress of {rowName(r)}" />
			{#if live}<span class="live">{live}</span>{/if}
		</div>
	{/if}
{/snippet}

{#snippet facts(list: Fact[], cls = '')}
	{#if list.length}
		<dl class="dfacts {cls}">
			{#each list as x (x.key)}<div class="fact" data-testid="detail-fact" data-key={x.key}><dt>{x.label}</dt><dd>{x.value}</dd></div>{/each}
		</dl>
	{/if}
{/snippet}

{#snippet nextLine()}
	<div class="nx">
	<p class="next" class:bad={phaseTone(r.phase) === 'fail'} data-testid="detail-next"><Icon name="arrow" size={14} stroke={2.2} /><span>{f.line}</span></p>
	{#if note && note.text !== f.line}<p class="note" data-testid="row-text">{note.text}</p>{/if}
	</div>
{/snippet}

{#snippet release()}
	<p class="rel" title={r.name}>{r.name}</p>
	{#if admin && note?.detail}<p class="rel">{note.detail}</p>{/if}
{/snippet}

{#snippet actions()}
	{#if decisions.length}<DecisionPanel items={decisions} {ondone} compact />{/if}
	{#if hasActs}
		<div class="acts" data-testid="detail-actions">
			{#if r.can_act}
				<Button size="sm" variant="secondary" icon="alert" onclick={() => onfail(r)} disabled={busy === r.id}>Mark failed</Button>
				<Button size="sm" variant="destructive" icon="trash" onclick={() => onremove(r)} disabled={busy === r.id}>Remove</Button>
			{/if}
			{#if r.title_id}<Button size="sm" variant="plain" href={to(`/library/${r.title_id}`)} iconAfter="chevronRight">Open title</Button>{/if}
		</div>
	{/if}
{/snippet}

{#snippet episodes()}
	{#if eps.length}
		<ul class="eplist" data-testid="row-episodes">
			{#each eps as e (e.slot_id)}
				{@const st = episodeStateWords(e.state)}
				<li data-testid="row-episode"><span class="eplabel">{e.label}</span><span class="epname">{e.name ?? ''}</span><StatusPill kind={st.tone === 'ok' ? 'in_library' : st.tone === 'fail' ? 'failed' : st.tone === 'idle' ? 'turned_down' : 'missing'} label={st.text} /></li>
			{/each}
		</ul>
	{/if}
{/snippet}

<div class="dd look-{look}" data-testid="download-detail" data-look={look}>
	{#if look === 'panel'}
		<!-- 1 · Slim side panel: summary header, one fact list, actions. -->
		{@render head(true)}
		{@render nextLine()}
		{@render facts([...f.what, ...whereRest, ...nextFacts])}
		{@render release()}
		{@render actions()}
	{:else if look === 'inline'}
		<!-- 2 · The row opens in place: what happens next, the facts the row
		     does not already show, and the actions, in two or three lines. -->
		{@const extra = [...f.what.filter((x) => x.key !== 'size' && x.key !== 'covers'), ...f.where.filter((x) => x.key === 'ratio'), ...nextFacts]}
		<div class="iline">
			{@render nextLine()}
			{#if hasActs && !decisions.length}{@render actions()}{/if}
		</div>
		{#if extra.length}
			<dl class="dfacts inl">{#each extra as x (x.key)}<div class="fact" data-testid="detail-fact" data-key={x.key}><dt>{x.label}</dt><dd>{x.value}</dd></div>{/each}<div class="fact dlr"><dt>Downloader</dt><dd>{r.downloader}</dd></div></dl>
		{:else if r.downloader}
			<dl class="dfacts inl"><div class="fact dlr"><dt>Downloader</dt><dd>{r.downloader}</dd></div></dl>
		{/if}
		<p class="rel" title={r.name}>{r.name}</p>
		{#if decisions.length}{@render actions()}{/if}
	{:else if look === 'card'}
		<!-- 3 · A small card at the row: status line, a 2-column fact grid, actions. -->
		{@render head(false)}
		{@render nextLine()}
		{@render facts([...f.what, ...whereRest, ...nextFacts], 'grid2')}
		{@render actions()}
	{:else if look === 'drawer'}
		<!-- 4 · Bottom drawer with tabs. -->
		<div class="drawer-top">
			<div class="dt-head">{@render head(true)}</div>
			<SegmentedControl options={tabs} bind:value={tab} label="Download details" size="sm" testid="detail-tabs" />
		</div>
		{#if tab === 'overview'}
			{@render nextLine()}
			{@render facts([...f.what, ...whereRest, ...nextFacts], 'grid4')}
			{@render actions()}
		{:else if tab === 'files'}
			{@render release()}
			{@render episodes()}
			{#if files.length}
				<ul class="files" data-testid="detail-files">
					{#each files as x, i (i)}
						<li><span class="fname" title={x.name}>{x.name}</span>{#if x.to}<span class="fto"><Icon name="folder" size={13} />{x.to}</span>{/if}{#if x.outcome}<span class="fout">{x.outcome === 'placed' ? 'Placed' : x.outcome === 'failed' ? 'Failed' : x.outcome === 'skipped' ? 'Skipped' : x.outcome}</span>{/if}</li>
					{/each}
				</ul>
			{:else if !eps.length}
				<p class="quiet">The file list shows once HolyArrs plans the import.</p>
			{/if}
		{:else}
			<ol class="story" data-testid="detail-activity">
				{#each story as s (s.key)}<li><span class="dot" aria-hidden="true"></span><span>{s.label}</span><time datetime={s.at}>{dateTime(s.at)}</time></li>{/each}
			</ol>
			{#if note}<p class="note">{note.text}</p>{/if}
		{/if}
	{:else}
		<!-- 5 · Track: Download → Import → Library, each step with its own facts. -->
		{@render head(true)}
		<ol class="track" data-testid="detail-steps">
			{#each f.steps as s (s.key)}
				<li class="step {s.state}" data-state={s.state} aria-current={s.state === 'now' || s.state === 'fail' ? 'step' : undefined}>
					<span class="mark" aria-hidden="true">{#if s.state === 'done'}<Icon name="check" size={12} stroke={2.6} />{:else if s.state === 'fail'}<Icon name="x" size={12} stroke={2.6} />{/if}</span>
					<span class="sl">{s.label}</span>
					<span class="sr-only">{s.state === 'done' ? 'done' : s.state === 'now' ? 'now' : s.state === 'fail' ? 'stopped here' : 'later'}</span>
				</li>
			{/each}
		</ol>
		{@render nextLine()}
		{@render facts([...f.what, ...whereRest, ...nextFacts], 'grid2')}
		{@render actions()}
	{/if}
</div>

<style>
	.dd { display: flex; flex-direction: column; gap: 12px; min-width: 0; font: var(--t-callout); }
	.head { display: flex; gap: 12px; align-items: center; min-width: 0; }
	.art { flex: none; width: 44px; --poster-ratio: 2 / 3; }
	.art.square { --poster-ratio: 1; }
	.art :global(.poster) { border-radius: 6px; }
	.names { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; min-width: 0; }
	.title { font: var(--t-headline); overflow-wrap: anywhere; }
	.sub { color: var(--text-muted); font: var(--t-subhead); }
	.st { display: inline-flex; align-items: center; gap: 5px; color: var(--text); font: var(--t-footnote); font-weight: 600; }
	.st :global(svg) { flex: none; color: var(--info); }
	.st.ok :global(svg) { color: var(--ok); }
	.st.fail, .st.fail :global(svg) { color: var(--fail-text); }
	.st.wait :global(svg) { color: var(--wait); }
	.st.idle { color: var(--text-muted); }
	.st.idle :global(svg) { color: var(--text-faint); }
	.sd { color: var(--text-muted); font-weight: 400; }
	.prog { display: flex; flex-direction: column; gap: 4px; }
	.live { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.nx { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.next { display: flex; gap: 6px; align-items: flex-start; font: var(--t-subhead); color: var(--text); }
	.next :global(svg) { flex: none; margin-top: 3px; color: var(--text-faint); }
	.next.bad :global(svg) { color: var(--fail); }
	.note { color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.rel { color: var(--text-faint); font: var(--t-caption); font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.quiet { color: var(--text-muted); font: var(--t-footnote); }
	.dfacts { display: grid; grid-template-columns: minmax(0, 1fr); margin: 0; font: var(--t-subhead); }
	.fact { display: flex; justify-content: space-between; gap: 12px; padding: 5px 0; border-top: 1px solid var(--line); min-width: 0; }
	.fact:first-child { border-top: 0; }
	dt { flex: none; color: var(--text-muted); }
	dd { margin: 0; min-width: 0; font: inherit; text-align: right; font-variant-numeric: tabular-nums; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.dfacts.tight .fact { padding: 2px 0; border: 0; }
	.dfacts.grid2 { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px 14px; }
	.dfacts.grid4 { grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 8px 16px; }
	.grid2 .fact, .grid4 .fact { flex-direction: column; justify-content: flex-start; gap: 0; padding: 0; border: 0; }
	.grid2 dt, .grid4 dt { font: var(--t-caption); }
	.grid2 dd, .grid4 dd { text-align: left; }
	.acts { display: flex; flex-wrap: wrap; gap: 6px; }

	/* 2 · inline */
	.look-inline { gap: 6px; }
	.iline { display: flex; align-items: center; justify-content: space-between; gap: 8px 16px; flex-wrap: wrap; }
	.dfacts.inl { display: flex; flex-wrap: wrap; gap: 2px 18px; }
	.inl .fact { justify-content: flex-start; gap: 6px; padding: 0; border: 0; }
	.inl dd { text-align: left; }
	/* The table shows the Downloader column from 1360 px; under that it moves here. */
	.inl .dlr { display: none; }
	@media (max-width: 1359px) { .inl .dlr { display: flex; } }

	/* 4 · drawer */
	.drawer-top { display: flex; align-items: center; justify-content: space-between; gap: 16px; flex-wrap: wrap; }
	.dt-head { display: flex; flex-direction: column; gap: 6px; flex: 1 1 260px; min-width: 0; }
	.files, .eplist, .story { list-style: none; margin: 0; padding: 0; display: grid; gap: 6px; font: var(--t-subhead); }
	.files li { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; gap: 10px; align-items: center; }
	.fname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: var(--font-mono); font-size: 12px; }
	.fto { display: inline-flex; align-items: center; gap: 4px; color: var(--text-muted); }
	.fout { color: var(--text-muted); font: var(--t-caption); }
	.eplist li { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 10px; }
	.eplabel { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.epname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.story li { display: grid; grid-template-columns: 10px minmax(0, 1fr) auto; align-items: center; gap: 10px; }
	.story .dot { width: 7px; height: 7px; border-radius: 50%; background: var(--line-strong); }
	.story time { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }

	/* 5 · track */
	.track { list-style: none; margin: 2px 0 0; padding: 0; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); }
	.step { position: relative; display: flex; flex-direction: column; align-items: center; gap: 4px; font: var(--t-caption); color: var(--text-muted); }
	.step + .step::before { content: ''; position: absolute; top: 9px; right: calc(50% + 14px); left: calc(-50% + 14px); height: 2px; border-radius: 1px; background: var(--surface-3); }
	.step.done + .step::before { background: var(--ok); }
	.mark { display: grid; place-items: center; width: 20px; height: 20px; border-radius: 50%; background: var(--surface-3); color: var(--bg); }
	.step.done .mark { background: var(--ok); }
	.step.now .mark { background: var(--surface-1); box-shadow: inset 0 0 0 2px var(--info), inset 0 0 0 5px var(--surface-1), inset 0 0 0 10px var(--info); }
	.step.fail .mark { background: var(--fail); }
	.step.now, .step.fail { color: var(--text); font-weight: 600; }
	.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }

	@media (max-width: 767px) {
		.inl .dlr { display: flex; }
		.dfacts.grid4 { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.files li { grid-template-columns: minmax(0, 1fr) auto; }
		.fout { display: none; }
	}
</style>

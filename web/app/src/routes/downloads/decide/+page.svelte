<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago } from '#lib/format.ts';
	import { decisionName, episodeStateWords, groupDecisions, manyWords, whyWords, type Decision, type DecisionGroup } from '#lib/downloaders.ts';
	import { decisions, refreshDecisions } from '#lib/decisions.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { plainReason } from '#lib/plain.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import OpsDownloadsTabs from '#lib/ui/OpsDownloadsTabs.svelte';
	import Button from '#lib/ui/Button.svelte';
	import StatusPill, { type StatusKind } from '#lib/ui/StatusPill.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { SkTableRow } from '#lib/ui/sk/index.ts';
	import EditionDecision from '#lib/EditionDecision.svelte';
	import Poster from '#lib/Poster.svelte';
	import type { TitleView } from '#lib/types.ts';

	let loaded = $state(false);
	let error = $state('');
	let titles = $state<TitleView[]>([]);
	const groups = $derived(groupDecisions(decisions.items));
	const titleById = $derived(new Map(titles.map((title) => [title.id, title])));

	async function load() {
		try {
			const r = await api.get<{ items: Decision[]; count: number }>('/needs-decision');
			decisions.items = r.items;
			decisions.count = r.count;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	onMount(() => {
		void load();
		void api.get<{ titles: TitleView[] }>('/titles').then((r) => (titles = r.titles)).catch(() => {});
		const offs = ['slot', 'releases', 'resync'].map((t) => live.on(t, () => void refreshDecisions()));
		return () => offs.forEach((off) => off());
	});

	type Action = 'search' | 'turn_down' | 'stop';

	let busy = $state('');
	async function act(d: Decision, action: Action, pack = false, key = `slot-${d.slot_id}`) {
		const name = decisionName(d);
		const w = manyWords(d.kind);
		const whole = d.kind === 'artist' ? 'artist' : 'show';
		const episodes = pack ? `these ${w?.many}` : w ? name : d.title;
		const ask = {
			search: '',
			turn_down: pack
				? `Add ${d.release_name ?? `the ${w?.pack}`} to Turned down for every ${w?.one} it holds and search again for these ${w?.many}?`
				: w
					? `Add ${d.release_name ?? 'its last Release'} to Turned down for ${name} and search again? The Release stays usable for other ${w.many}.`
					: `Add ${d.release_name ?? 'its last Release'} to Turned down and search again?`,
			stop: w
				? `Stop looking for ${episodes}? Keep up to date is turned off for ${pack ? 'each of them' : `this ${w.one}`}; the rest of the ${whole} goes on, and files on disk are not touched.`
					: `Stop looking for ${name} (${d.version})? Keep up to date is turned off for this Version; files on disk are not touched.`
		}[action];
		if (ask && !confirm(ask)) return;
		busy = key;
		try {
			await api.post(`/needs-decision/${d.slot_id}/decide`, pack ? { action, pack: true } : { action });
			toasts.push(
				{ search: `Searching again for ${episodes}.`, turn_down: `Turned down. Searching again for ${episodes}.`, stop: `Stopped looking for ${episodes}.` }[action],
				'ok'
			);
			await refreshDecisions();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}

	const admin = $derived(session.can('settings'));
	function toneKind(t: string): StatusKind {
		return t === 'ok' ? 'in_library' : t === 'fail' ? 'failed' : t === 'idle' ? 'turned_down' : 'downloading';
	}

	function packLine(g: DecisionGroup): string {
		const all = g.lead.pack ?? [];
		const w = manyWords(g.lead.kind);
		return `${g.items.length} of the ${all.length} ${w ? w.many : 'episodes'} in this download need a decision`;
	}
</script>

<svelte:head><title>Needs a decision · HolyArrs</title></svelte:head>

<PageHeader title="Needs a decision" subtitle="Movies, episodes and tracks HolyArrs cannot finish on its own. Pick what happens next." />
<OpsDownloadsTabs />

{#snippet cause(d: Decision, bold: boolean)}
	{@const r = d.reason ? plainReason(d.reason) : null}
	<div class="cause">
		<p class="why" data-testid="decision-why">{#if bold}<b>{whyWords(d)}</b>{:else}{whyWords(d)}{/if}</p>
		{#if r}<p class="reason" data-testid="decision-reason">{r.text}</p>{/if}
		{#if admin && (r?.detail || d.release_name)}
			<details class="raw">
				<summary>Details</summary>
				{#if d.release_name}<p>Last Release: {d.release_name}</p>{/if}
				{#if r?.detail}<p>{r.detail}</p>{/if}
			</details>
		{/if}
	</div>
{/snippet}

{#snippet choices(d: Decision, key: string)}
	{#if d.kind === 'movie' && d.download_id}<EditionDecision downloadId={d.download_id} onchange={refreshDecisions} />{/if}
	<div class="acts">
		<Button variant="primary" onclick={() => act(d, 'search', false, key)} disabled={busy === key} busy={busy === key}>Search again</Button>
		<Button variant="secondary" href={to(`/library/${d.title_id}#h-rel`)}>Choose a release by hand</Button>
		{#if !manyWords(d.kind)}
			<Button variant="secondary" href={to(`/library/${d.title_id}#h-versions`)}>{d.versions > 1 ? 'Try another Version' : 'Add another Version'}</Button>
		{/if}
		{#if d.release_id}
			<Button variant="plain" onclick={() => act(d, 'turn_down', false, key)} disabled={busy === key}>Add to Turned down</Button>
		{/if}
		<Button variant="plain" onclick={() => act(d, 'stop', false, key)} disabled={busy === key}>Stop looking</Button>
	</div>
{/snippet}

{#if !loaded}
	<div class="loading" aria-busy="true" aria-label="Loading decisions">{#each Array.from({ length: 5 }, (_, i) => i) as i (i)}<SkTableRow cols={2} height={72} />{/each}</div>
{:else if error && decisions.items.length === 0}
	<LoadError hasData={false} message="Decisions could not be loaded." onretry={load} />
{:else if decisions.items.length === 0}
	<EmptyState icon="check" title="Nothing needs you" text="When HolyArrs runs out of Releases for a movie, an episode or a track, or an import cannot finish, it waits here for you." action={{ label: 'Downloads', href: to('/downloads'), icon: 'download' }} />
{:else}
	{#if error}<LoadError hasData message="Decisions could not be loaded." onretry={load} />{/if}
	<section aria-labelledby="decision-queue-title">
		<header class="sec"><h2 id="decision-queue-title">Waiting for you</h2><span class="count">{decisions.items.length}</span></header>
		<ul class="list">
			{#each groups as g (g.key)}
				{@const d = g.lead}
				{#if g.pack}
					{@const w = manyWords(d.kind)}
					<li class="dec" data-testid="decision-pack">
						<div class="head">
							<span class="art" class:square={d.kind === 'artist'} aria-hidden="true"><Poster src={d.artwork ?? titleById.get(d.title_id)?.artwork[0]} name={d.title} /></span>
							<div class="names">
								<a class="nm" href={to(`/library/${d.title_id}`)}>{d.album ? `${d.title} – ${d.album}` : d.title}</a>
								<span class="sub">{d.kind === 'artist' ? 'Album' : 'Season pack'} · {packLine(g)} · waiting {ago(d.since)}</span>
							</div>
						</div>
						<ul class="eplist" aria-label={d.kind === 'artist' ? 'Tracks in this album' : 'Episodes in this pack'}>
							{#each d.pack ?? [] as e (e.slot_id)}
								{@const st = episodeStateWords(e.state)}
								<li>
									<span class="eplabel">{e.label}</span>
									<span class="epname">{e.name ?? ''}</span>
									<StatusPill kind={toneKind(st.tone)} label={st.text} />
								</li>
							{/each}
						</ul>
						{#if admin && d.release_name}<details class="raw"><summary>Details</summary><p>{d.kind === 'artist' ? 'Album' : 'Pack'}: {d.release_name}</p></details>{/if}
						<div class="acts">
							<Button variant="primary" onclick={() => act(d, 'search', true, g.key)} disabled={busy === g.key} busy={busy === g.key}>Search again for these {w?.many}</Button>
							{#if d.release_id}
								<Button variant="plain" onclick={() => act(d, 'turn_down', true, g.key)} disabled={busy === g.key}>Add the {w?.pack === 'album' ? 'album' : 'pack'} to Turned down</Button>
							{/if}
							<Button variant="plain" onclick={() => act(d, 'stop', true, g.key)} disabled={busy === g.key}>Stop looking for these {w?.many}</Button>
						</div>
						<ul class="subs">
							{#each g.items as one (one.slot_id)}
								<li data-testid="decision">
									<p class="one">{decisionName(one)}</p>
									{@render cause(one, false)}
									{@render choices(one, `slot-${one.slot_id}`)}
								</li>
							{/each}
						</ul>
					</li>
				{:else}
					<li class="dec" data-testid="decision">
						<div class="head">
							<span class="art" class:square={d.kind === 'artist'} aria-hidden="true"><Poster src={d.artwork ?? titleById.get(d.title_id)?.artwork[0]} name={d.title} /></span>
							<div class="names">
								<a class="nm" href={to(`/library/${d.title_id}`)}>{decisionName(d)}</a>
								<span class="sub">{d.version} Version · waiting {ago(d.since)}</span>
								{#if d.has_copy}<span class="sub">A copy is already in the Library.</span>{/if}
							</div>
						</div>
						{@render cause(d, true)}
						{@render choices(d, g.key)}
					</li>
				{/if}
			{/each}
		</ul>
	</section>
{/if}

<style>
	.loading { display: grid; gap: 0; max-width: 980px; }
	.sec { display: flex; align-items: center; gap: 10px; margin-bottom: 8px; }
	.sec h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.count { min-width: 24px; height: 24px; padding: 0 8px; display: inline-grid; place-items: center; border-radius: 999px; background: var(--accent-soft); color: var(--accent-text); font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 14px; max-width: 980px; }
	.dec { display: flex; flex-direction: column; gap: 12px; padding: 18px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); min-width: 0; }
	.head { display: flex; gap: 14px; align-items: center; min-width: 0; }
	.art { flex: none; width: 44px; --poster-ratio: 2 / 3; }
	.art.square { --poster-ratio: 1; }
	.names { display: flex; flex-direction: column; min-width: 0; }
	.nm { color: var(--text); font: var(--t-headline); overflow-wrap: anywhere; }
	.nm:hover { text-decoration: underline; text-underline-offset: 3px; }
	.sub { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.cause { padding: 12px 14px; border-radius: var(--r-btn); background: var(--surface-2); box-shadow: inset 3px 0 0 var(--wait); display: flex; flex-direction: column; gap: 4px; overflow-wrap: anywhere; }
	.why { font: var(--t-callout); }
	.reason { color: var(--text-muted); font: var(--t-callout); }
	.raw { color: var(--text-muted); font: var(--t-footnote); }
	.raw summary { width: max-content; min-height: 24px; cursor: pointer; border-radius: 6px; }
	.raw summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.raw p { margin-top: 4px; font-family: var(--font-mono); overflow-wrap: anywhere; }
	.acts { display: flex; flex-wrap: wrap; gap: 8px; }
	.eplist { list-style: none; padding: 0; margin: 0; display: grid; gap: 6px; }
	.eplist li { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 10px; font: var(--t-callout); }
	.eplabel { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.epname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.subs { list-style: none; padding: 0; margin: 0; display: grid; gap: 14px; }
	.subs > li { display: flex; flex-direction: column; gap: 8px; padding-top: 14px; border-top: 1px solid var(--line); min-width: 0; }
	.one { font: var(--t-headline); font-size: 15px; overflow-wrap: anywhere; }
	@media (max-width: 640px) {
		.dec { padding: 14px; }
		.acts > :global(*) { flex: 1 1 100%; min-height: 44px; }
		.eplist li { grid-template-columns: auto minmax(0, 1fr); }
		.eplist li > :global(.ui-pill) { grid-column: 2; justify-self: start; }
		.epname { white-space: normal; overflow-wrap: anywhere; }
	}
</style>

<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago } from '#lib/format.ts';
	import { decisionName, episodeStateWords, groupDecisions, manyWords, whyWords, type Decision, type DecisionGroup } from '#lib/downloaders.ts';
	import { decisions, refreshDecisions } from '#lib/decisions.svelte.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import EditionDecision from '#lib/EditionDecision.svelte';

	let loaded = $state(false);
	let error = $state('');
	const groups = $derived(groupDecisions(decisions.items));

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

	function packLine(g: DecisionGroup): string {
		const all = g.lead.pack ?? [];
		const w = manyWords(g.lead.kind);
		return `${g.items.length} of the ${all.length} ${w ? w.many : 'episodes'} in this download need a decision`;
	}
</script>

<svelte:head><title>Needs a decision · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<h1>Needs a decision</h1>
		<p>Movies, episodes and tracks HolyArrs cannot finish on its own: it ran out of Releases, or an import hit a conflict. Pick what happens next.</p>
	</div>
</header>

{#snippet choices(d: Decision, key: string)}
	{#if d.kind === 'movie' && d.download_id}<EditionDecision downloadId={d.download_id} onchange={refreshDecisions} />{/if}
	<div class="acts">
		<button class="btn btn-red" type="button" onclick={() => act(d, 'search', false, key)} disabled={busy === key}>Search again</button>
		<a class="btn btn-ghost" href={to(`/library/${d.title_id}#h-rel`)}>Choose a release by hand</a>
		{#if !manyWords(d.kind)}
			<a class="btn btn-ghost" href={to(`/library/${d.title_id}#h-versions`)}>{d.versions > 1 ? 'Try another Version' : 'Add another Version'}</a>
		{/if}
		{#if d.release_id}
			<button class="btn btn-ghost" type="button" onclick={() => act(d, 'turn_down', false, key)} disabled={busy === key}>Add to Turned down</button>
		{/if}
		<button class="btn btn-quiet" type="button" onclick={() => act(d, 'stop', false, key)} disabled={busy === key}>Stop looking</button>
	</div>
{/snippet}

{#if !loaded}
	<div class="decision-loading"><div class="sk" style="height: 220px"></div><div class="sk" style="height: 220px"></div></div>
{:else if error}
	<div class="notice bad load-error" role="alert">
		<div><b>Decisions could not be loaded.</b><br /><span class="small">{error}</span></div>
		<button class="btn btn-ghost" type="button" onclick={load}>Try again</button>
	</div>
{:else if decisions.items.length === 0}
	<EmptyState icon="check" title="Nothing needs you">
		<p>When HolyArrs runs out of Releases for a movie, an episode or a track, or an import cannot finish, it waits here for you.</p>
		{#snippet actions()}
			<a class="btn btn-ghost" href={to(`/downloads`)}>Downloads</a>
		{/snippet}
	</EmptyState>
{:else}
	<ul class="cards">
		{#each groups as g (g.key)}
			{@const d = g.lead}
			{#if g.pack}
				{@const w = manyWords(d.kind)}
				<li class="card dec" data-testid="decision-pack">
					<div class="names">
						<a class="nm" href={to(`/library/${d.title_id}`)}>{d.album ? `${d.title} – ${d.album}` : d.title}</a>
						<span class="xs muted">{d.kind === 'artist' ? 'Album' : 'Season pack'} · {packLine(g)} · waiting {ago(d.since)}</span>
					</div>
					{#if d.release_name}<p class="mono xs muted path">{d.kind === 'artist' ? 'Album' : 'Pack'}: {d.release_name}</p>{/if}
					<ul class="eplist" aria-label="{d.kind === 'artist' ? 'Tracks in this album' : 'Episodes in this pack'}">
						{#each d.pack ?? [] as e (e.slot_id)}
							{@const st = episodeStateWords(e.state)}
							<li>
								<span class="mono xs">{e.label}</span>
								<span class="small ellipsis">{e.name ?? ''}</span>
								<span class="chip {st.tone}">{st.text}</span>
							</li>
						{/each}
					</ul>
					<div class="acts">
						<button class="btn btn-red" type="button" onclick={() => act(d, 'search', true, g.key)} disabled={busy === g.key}>Search again for these {w?.many}</button>
						{#if d.release_id}
							<button class="btn btn-ghost" type="button" onclick={() => act(d, 'turn_down', true, g.key)} disabled={busy === g.key}>Add the {w?.pack === 'album' ? 'album' : 'pack'} to Turned down</button>
						{/if}
						<button class="btn btn-quiet" type="button" onclick={() => act(d, 'stop', true, g.key)} disabled={busy === g.key}>Stop looking for these {w?.many}</button>
					</div>
					<ul class="sub">
						{#each g.items as one (one.slot_id)}
							<li data-testid="decision">
								<p class="small"><b>{decisionName(one)}</b></p>
								<p class="small" data-testid="decision-why">{whyWords(one)}</p>
								{#if one.reason}<p class="small muted" data-testid="decision-reason">{one.reason}</p>{/if}
								{@render choices(one, `slot-${one.slot_id}`)}
							</li>
						{/each}
					</ul>
				</li>
			{:else}
				<li class="card dec" data-testid="decision">
					<div class="head">
						<div class="names">
							<a class="nm" href={to(`/library/${d.title_id}`)}>{decisionName(d)}</a>
							<span class="xs muted">{d.version} Version · waiting {ago(d.since)}</span>
						</div>
					</div>
					<p class="small" data-testid="decision-why"><b>{whyWords(d)}</b></p>
					{#if d.reason}<p class="small muted" data-testid="decision-reason">{d.reason}</p>{/if}
					{#if d.release_name}<p class="mono xs muted path">Last Release: {d.release_name}</p>{/if}
					{#if d.has_copy}<p class="xs muted">A copy is already in the Library.</p>{/if}
					{@render choices(d, g.key)}
				</li>
			{/if}
		{/each}
	</ul>
{/if}

<style>
	.decision-loading,
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 380px), 1fr));
		gap: var(--s-4);
		list-style: none;
		padding: 0;
		margin: 0;
	}
	.dec {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--s-2);
		min-width: 0;
		align-content: start;
		border-left: 2px solid var(--info);
	}
	.names {
		display: grid;
		min-width: 0;
	}
	.nm {
		font-weight: var(--weight-semibold);
	}
	.path,
	.dec p {
		/* reasons can hold long file paths */
		overflow-wrap: anywhere;
	}
	.acts {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-2);
		margin-top: var(--s-2);
	}
	.load-error {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-4);
	}
	.eplist {
		list-style: none;
		padding: 0;
		margin: 0;
		display: grid;
		gap: var(--s-1);
	}
	.eplist li {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: var(--s-2);
	}
	.sub {
		list-style: none;
		padding: 0;
		margin: var(--s-3) 0 0;
		display: grid;
		gap: var(--s-4);
	}
	.sub > li {
		border-top: 1px solid var(--line);
		padding-top: var(--s-3);
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--s-1);
		min-width: 0;
	}
	@media (max-width: 620px) {
		.acts,
		.acts :global(.btn) {
			width: 100%;
		}
		.acts :global(.btn) {
			min-height: 44px;
		}
		.load-error {
			align-items: stretch;
			flex-direction: column;
		}
		.eplist li {
			grid-template-columns: auto minmax(0, 1fr);
		}
		.eplist .chip {
			grid-column: 2;
			justify-self: start;
		}
	}
</style>

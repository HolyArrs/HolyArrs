<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { ago, bytes, duration, greeting, percent } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import EmptyState from '#lib/EmptyState.svelte';
	import { searchLine } from '#lib/releases.ts';
	import { checkLabel, problems, stateWord } from '#lib/health.ts';
	import { groupLabel, kindTone } from '#lib/history.ts';
	import type { Download, EventRecord, Health, HealthChecks, LibraryFolder, SearchView } from '#lib/types.ts';

	interface MissingTitle { id: number; name: string; year?: number; state_label: string }

	let loaded = $state(false);
	let titleCount = $state(0);
	let missing = $state<MissingTitle[]>([]);
	let folders = $state<LibraryFolder[]>([]);
	let downloads = $state<Download[]>([]);
	let events = $state<EventRecord[]>([]);
	let health = $state<Health | null>(null);
	let checks = $state<HealthChecks | null>(null);
	const issues = $derived(problems(checks?.checks));
	let tmdbSet = $state(false);
	let searching = $state<SearchView[]>([]);
	let found = $state<SearchView[]>([]);

	const steps = $derived([
		{ done: tmdbSet, text: 'Add your TMDB key', href: '/settings/tmdb' },
		{ done: folders.length > 0, text: 'Add a Library folder', href: '/settings/library-folders' },
		{ done: (health?.sources?.length ?? 0) > 0, text: 'Add a Source', href: '/settings/sources' },
		{ done: (health?.downloaders?.length ?? 0) > 0, text: 'Add a Downloader', href: '/settings/downloaders' },
		{ done: titleCount > 0, text: 'Add your first movie', href: '/search' }
	]);
	const doneCount = $derived(steps.filter((s) => s.done).length);
	const active = $derived(downloads.filter((d) => d.State === 'downloading' || d.State === 'queued' || d.State === 'stalled'));

	async function loadAll() {
		const results = await Promise.allSettled([
			api.get<{ total: number; missing: MissingTitle[] }>('/library-summary'),
			api.get<{ library_folders: LibraryFolder[] }>('/library-folders'),
			api.get<{ downloads: Download[] }>('/downloads?limit=20'),
			api.get<{ events: EventRecord[] }>('/events?limit=12'),
			api.get<Health>('/health'),
			api.get<{ token: string }>('/settings/tmdb-token'),
			api.get<{ running: SearchView[]; recent: SearchView[] }>('/searches?limit=6'),
			api.get<HealthChecks>('/health/checks')
		]);
		const [t, f, d, e, h, k, s, c] = results;
		if (c.status === 'fulfilled') checks = c.value;
		if (s.status === 'fulfilled') {
			searching = s.value.running;
			found = s.value.recent;
		}
		if (t.status === 'fulfilled') {
			titleCount = t.value.total;
			missing = t.value.missing;
		}
		if (f.status === 'fulfilled') folders = f.value.library_folders;
		if (d.status === 'fulfilled') downloads = d.value.downloads;
		if (e.status === 'fulfilled') events = e.value.events;
		if (h.status === 'fulfilled') health = h.value;
		// Without the token route (the token comes from a file), TMDB counts as set.
		tmdbSet = k.status === 'fulfilled' ? !!k.value.token : true;
		loaded = true;
	}

	onMount(() => {
		void loadAll();
		let timer: ReturnType<typeof setTimeout> | undefined;
		const soon = () => {
			clearTimeout(timer);
			timer = setTimeout(loadAll, 250);
		};
		const offs = [
			live.on('title', soon),
			live.on('event', soon),
			live.on('import', soon),
			live.on('resync', soon),
			live.on('releases', soon),
			live.on('health', soon),
			live.on('job', (data) => {
				const j = data as { kind?: string };
				if (j?.kind === 'search_slot' || j?.kind === 'search_missing' || j?.kind === 'feed_check' || j?.kind === 'search_season') soon();
			}),
			live.on('download', (data) => {
				const d = data as Download;
				if (!d || typeof d.ID !== 'number') return soon();
				const i = downloads.findIndex((x) => x.ID === d.ID);
				if (i >= 0) downloads[i] = d;
				else downloads = [d, ...downloads];
			})
		];
		return () => {
			clearTimeout(timer);
			offs.forEach((off) => off());
		};
	});
</script>

<svelte:head><title>Home · HolyArrs</title></svelte:head>

<header class="page-head rise">
	<div>
		<h1>{greeting()}{session.s.name ? `, ${session.s.name}` : ''}</h1>
		<p>
			{#if !loaded}
				Looking at your Library…
			{:else if titleCount === 0}
				Your Library is empty. Here is how to get going.
			{:else}
				{titleCount} {titleCount === 1 ? 'title' : 'titles'} in your Library, {active.length} downloading now.
			{/if}
		</p>
	</div>
	<a class="btn btn-ghost home-search" href={to(`/search`)}><Icon name="search" size={17} /> Search</a>
</header>

<nav class="status-strip" aria-label="Library overview" aria-live="polite">
	<a href={to(`/library`)}><span class="status-value">{loaded ? titleCount : '—'}</span><span class="status-label">Library titles</span></a>
	<a href={to(`/downloads`)}><span class="status-value">{loaded ? active.length : '—'}</span><span class="status-label">Downloading</span></a>
	<a href={to('/library?show=missing')}><span class="status-value">{loaded ? missing.length : '—'}</span><span class="status-label">Missing shown</span></a>
	{#if session.can('settings')}
		<a href={to(`/settings/this-app`)}><span class="status-value">{loaded ? issues.length : '—'}</span><span class="status-label">Health issues</span></a>
	{/if}
</nav>

<div class="home">
	<div class="main-col">
		<section class="block" aria-labelledby="h-dl">
			<div class="block-head">
				<h2 id="h-dl" class="h2">Downloading now</h2>
				<a class="link small muted" href={to(`/downloads`)}>All downloads</a>
			</div>
			{#if !loaded}
				<div class="sk" style="height: 96px"></div>
			{:else if active.length === 0}
				<p class="quiet card">Nothing is downloading. When HolyArrs picks a Release, it shows up here with its progress.</p>
			{:else}
				<ul class="card rows">
					{#each active as d (d.ID)}
						<li>
							<div class="grow">
								<div class="ellipsis small">{d.FilesSeen?.[0]?.name ?? d.ExternalID}</div>
								<div class="bar" style="margin-top: 8px">
									<i style="transform: scaleX({percent(d.Progress) / 100})"></i>
								</div>
							</div>
							<span class="mono xs muted nums">
								{percent(d.Progress)}% · {bytes(d.Speed)}/s{d.ETA ? ` · ${duration(d.ETA)}` : ''}
							</span>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<section class="block" aria-labelledby="h-search" data-testid="home-searches">
			<div class="block-head">
				<h2 id="h-search" class="h2">Searches</h2>
				<a class="link small muted" href={to(`/library/held`)}>Held and Turned down</a>
			</div>
			{#if !loaded}
				<div class="sk" style="height: 64px"></div>
			{:else if searching.length === 0 && found.length === 0}
				<p class="quiet card">No searches yet. When HolyArrs looks for a movie, you see it here as it happens and what it found.</p>
			{:else}
				<ul class="card rows" aria-live="polite">
					{#each searching as x (x.job_id)}
						<li data-testid="search-running">
							<div class="grow">
								<div class="small">{searchLine(x)}</div>
								<div class="bar" style="margin-top: 8px"><i style="transform: scaleX({percent(x.progress) / 100})"></i></div>
							</div>
							<span class="chip wait">{x.state === 'queued' ? 'Waiting' : 'Searching'}</span>
						</li>
					{/each}
					{#each found as x (x.job_id)}
						<li data-testid="search-found">
							{#if x.title_id}
								<a class="grow small" href={to(`/library/${x.title_id}`)}>{searchLine(x)}</a>
							{:else}
								<span class="grow small">{searchLine(x)}</span>
							{/if}
							<time class="mono xs muted" datetime={x.ended_at}>{ago(x.ended_at)}</time>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<section class="block" aria-labelledby="h-mis">
			<div class="block-head">
				<h2 id="h-mis" class="h2">Missing</h2>
				<a class="link small muted" href={to(`/library`)}>Library</a>
			</div>
			{#if !loaded}
				<div class="sk" style="height: 64px"></div>
			{:else if missing.length === 0}
				<p class="quiet card">
					{titleCount ? 'Every title you keep up to date has a file. Nice.' : 'Movies you add but do not have yet show up here.'}
				</p>
			{:else}
				<ul class="card rows">
					{#each missing as t (t.id)}
						<li>
							<a class="grow ellipsis" href={to(`/library/${t.id}`)}>{t.name}{t.year ? ` (${t.year})` : ''}</a>
							<span class="chip wait">{t.state_label}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<section class="block" aria-labelledby="h-ev">
			<div class="block-head">
				<h2 id="h-ev" class="h2">Latest Events</h2>
				<a class="link small muted" href={to(`/downloads/history`)}>All history</a>
			</div>
			{#if !loaded}
				<div class="sk" style="height: 120px"></div>
			{:else if events.length === 0}
				<p class="quiet card">No Events yet. Every search, download and import writes one here.</p>
			{:else}
				<ul class="card rows events">
					{#each events as e (e.ID)}
						<li>
							<span class="chip {kindTone(e.Kind)}">{groupLabel(e.Kind)}</span>
							<span class="grow small">{e.Summary}</span>
							<time class="mono xs muted" datetime={e.At}>{ago(e.At)}</time>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	</div>

	<aside class="side-col" aria-label="Getting started">
		{#if loaded && doneCount < steps.length}
			<section class="card setup rise" style="--i: 1" aria-labelledby="h-setup">
				<div class="card-head">
					<h2 id="h-setup" class="h2">Get started</h2>
					<span class="mono xs muted">{doneCount} of {steps.length} done</span>
				</div>
				<div class="segs" aria-hidden="true">
					{#each steps as s, i (i)}<i class:on={s.done}></i>{/each}
				</div>
				<ul class="checks">
					{#each steps as s (s.text)}
						<li class:done={s.done}>
							{#if s.done}
								<Icon name="check" size={18} /><span>{s.text}</span>
							{:else}
								<a href={to(s.href)}><span class="dot" aria-hidden="true"></span><span>{s.text}</span><Icon name="arrow" size={16} /></a>
							{/if}
						</li>
					{/each}
				</ul>
			</section>
		{:else if loaded && titleCount === 0}
			<EmptyState icon="search" title="Add your first movie">
				<p>Everything is set up. Find a movie and add it to your Library.</p>
				{#snippet actions()}<a class="btn btn-red" href={to(`/search`)}>Search</a>{/snippet}
			</EmptyState>
		{/if}
		{#if loaded && checks}
			<section class="card" aria-labelledby="h-health" data-testid="home-health">
				<div class="card-head">
					<h2 id="h-health" class="h2">Health</h2>
					<span class="chip {issues.length === 0 ? 'ok' : checks.worst === 'error' ? 'fail' : 'wait'}">
						{issues.length === 0 ? 'All fine' : `${issues.length} to look at`}
					</span>
				</div>
				{#if issues.length === 0}
					<p class="small muted">{checks.checks.length ? 'Every health check passes.' : 'The health checks have not run yet.'}</p>
				{:else}
					<ul class="issues">
						{#each issues.slice(0, 5) as c (c.key)}
							<li data-testid="home-health-issue">
								<span class="chip {c.state === 'error' ? 'fail' : 'wait'}">{stateWord(c.state)}</span>
								<span class="small">{c.message || `${checkLabel(c.kind)} ${c.subject}`}</span>
							</li>
						{/each}
					</ul>
				{/if}
				{#if session.can('settings')}<a class="link small muted" href={to(`/settings/this-app`)}>All health checks</a>{/if}
			</section>
		{/if}
		{#if loaded && health}
			<section class="card" aria-labelledby="h-state">
				<h2 id="h-state" class="h2" style="margin-bottom: var(--s-3)">This HolyArrs</h2>
				<dl class="facts">
					<dt>Version</dt>
					<dd>{health.version}</dd>
					<dt>Running for</dt>
					<dd>{duration(health.uptime_s)}</dd>
					<dt>Sources</dt>
					<dd>{health.sources?.length ?? 0}</dd>
					<dt>Downloaders</dt>
					<dd>{health.downloaders?.length ?? 0}</dd>
				</dl>
			</section>
		{/if}
	</aside>
</div>

<style>
	.home-search {
		align-self: flex-start;
	}
	.status-strip {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
		margin-bottom: var(--s-6);
		border: 1px solid var(--line);
		border-radius: var(--r-card);
		background: var(--surface-1);
		overflow: hidden;
	}
	.status-strip a {
		display: grid;
		gap: 2px;
		min-width: 0;
		padding: var(--s-3) var(--s-4);
		transition: background var(--dur) var(--ease-out);
	}
	.status-strip a + a {
		border-left: 1px solid var(--line);
	}
	.status-strip a:hover {
		background: var(--surface-2);
	}
	.status-value {
		font: var(--weight-semibold) var(--text-lg) / 1.15 var(--font-display);
		font-variant-numeric: tabular-nums;
		letter-spacing: -0.02em;
	}
	.status-label {
		font-size: var(--text-xs);
		color: var(--text-muted);
	}
	.home {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 300px;
		gap: var(--s-5);
		align-items: start;
	}
	.side-col {
		display: flex;
		flex-direction: column;
		gap: var(--s-4);
		position: sticky;
		top: calc(var(--topbar) + var(--s-6));
	}
	.side-col :global(.card + .card) {
		margin-top: 0;
	}
	.block + .block {
		margin-top: var(--s-5);
	}
	.block-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		margin-bottom: var(--s-3);
	}
	.link {
		min-height: 32px;
		display: inline-flex;
		align-items: center;
		padding: 0 var(--s-2);
		border-radius: var(--r-sm);
	}
	.link:hover {
		color: var(--text);
	}
	.quiet {
		color: var(--text-muted);
		font-size: var(--text-sm);
	}
	.rows {
		padding: 0;
		overflow: hidden;
	}
	.rows a:hover {
		color: var(--info);
	}
	.rows > li {
		padding-inline: var(--s-4);
	}
	.rows > li:hover {
		background: var(--surface-2);
	}
	.nums {
		white-space: nowrap;
	}
	.segs {
		display: grid;
		grid-template-columns: repeat(5, 1fr);
		gap: 4px;
		margin-bottom: var(--s-3);
	}
	.segs i {
		height: 4px;
		border-radius: 4px;
		background: var(--line-strong);
	}
	.segs i.on {
		background: var(--info);
	}
	.checks li {
		min-height: var(--tap);
		display: flex;
		align-items: center;
		gap: var(--s-3);
		color: var(--text-muted);
		font-size: var(--text-sm);
	}
	.checks li.done {
		color: var(--text);
	}
	.checks li.done :global(.icon) {
		color: var(--ok-text);
	}
	.checks a {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		width: 100%;
		min-height: var(--tap);
		border-radius: var(--r-sm);
		color: var(--text);
		font-weight: var(--weight-medium);
	}
	.checks a :global(.icon) {
		margin-left: auto;
		color: var(--text-muted);
		transition: transform var(--dur) var(--ease-out);
	}
	.checks a:hover :global(.icon) {
		transform: translateX(3px);
		color: var(--info);
	}
	.issues {
		display: flex;
		flex-direction: column;
		gap: var(--s-2);
		margin-bottom: var(--s-2);
	}
	.issues li {
		display: flex;
		align-items: flex-start;
		gap: var(--s-2);
		overflow-wrap: anywhere;
	}
	.dot {
		width: 18px;
		height: 18px;
		border-radius: 50%;
		border: 1.75px solid var(--line-strong);
		flex: none;
	}
	.rows li {
		min-width: 0;
	}
	.rows .grow {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	@media (max-width: 600px) {
		.home-search {
			width: 100%;
			min-height: 44px;
		}
		.status-strip {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
		.status-strip a {
			min-height: 64px;
		}
		.status-strip a:nth-child(3) {
			border-left: 0;
			border-top: 1px solid var(--line);
		}
		.status-strip a:nth-child(4) {
			border-top: 1px solid var(--line);
		}
		.block-head {
			flex-wrap: wrap;
			gap: var(--s-2);
		}
		.rows li {
			flex-wrap: wrap;
		}
		.nums {
			white-space: normal;
		}
	}
	@media (max-width: 1100px) {
		.home {
			grid-template-columns: 1fr;
		}
		.side-col {
			position: static;
			order: -1;
		}
	}
</style>

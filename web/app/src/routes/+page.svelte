<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, ApiError } from '#lib/api.ts';
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
	let tmdbStatus = $state<'unknown' | 'ready' | 'missing'>('unknown');
	let tmdbNeedsProof = $state(false);
	let searching = $state<SearchView[]>([]);
	let found = $state<SearchView[]>([]);

	const steps = $derived([
		{
			done: tmdbStatus === 'ready',
			text: tmdbStatus === 'unknown' ? 'Check TMDB connection' : 'Connect TMDB',
			detail: tmdbStatus === 'unknown' ? 'Confirm the protected connection state before marking TMDB ready.' : 'Use TMDB artwork and metadata when you add media.',
			href: '/settings/tmdb'
		},
		{ done: folders.length > 0, text: 'Choose a Library folder', detail: 'Tell HolyArrs where completed media should live.', href: '/settings/library-folders' },
		{ done: (health?.sources?.length ?? 0) > 0, text: 'Add a Source', detail: 'Connect a place where HolyArrs can find Releases.', href: '/settings/sources' },
		{ done: (health?.downloaders?.length ?? 0) > 0, text: 'Add a Downloader', detail: 'Connect the app that will fetch chosen Releases.', href: '/settings/downloaders' },
		{ done: titleCount > 0, text: 'Add your first movie', detail: 'Search for a movie and begin your Library.', href: '/search' }
	]);
	const doneCount = $derived(steps.filter((s) => s.done).length);
	const nextStep = $derived(steps.find((s) => !s.done));
	const active = $derived(downloads.filter((d) => d.State === 'downloading' || d.State === 'queued' || d.State === 'stalled'));

	async function loadAll() {
		const mayReadTMDB = session.can('settings') && !tmdbNeedsProof && (session.s.via === 'api_key' || session.s.localRecent);
		const results = await Promise.allSettled([
			api.get<{ total: number; missing: MissingTitle[] }>('/library-summary'),
			api.get<{ library_folders: LibraryFolder[] }>('/library-folders'),
			api.get<{ downloads: Download[] }>('/downloads?limit=20'),
			api.get<{ events: EventRecord[] }>('/events?limit=12'),
			api.get<Health>('/health'),
			mayReadTMDB ? api.get<{ token: string }>('/settings/tmdb-token') : Promise.resolve(null),
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
		if (mayReadTMDB) {
			if (k.status === 'rejected' && k.reason instanceof ApiError && k.reason.status === 403) tmdbNeedsProof = true;
			tmdbStatus = k.status === 'fulfilled' && k.value ? (k.value.token ? 'ready' : 'missing') : 'unknown';
		} else {
			tmdbStatus = 'unknown';
		}
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

<header class="page-head home-hero rise">
	<div>
		<p class="eyebrow">Workspace overview</p>
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
	<a class="btn btn-dark home-search" href={to(`/search`)}><Icon name="search" size={17} /> Search media</a>
</header>

<nav class="metric-band" aria-label="Library overview" aria-live="polite">
	<a href={to(`/library`)}><span class="status-value">{loaded ? titleCount : '—'}</span><span class="status-label">Library titles</span></a>
	<a href={to(`/downloads`)}><span class="status-value">{loaded ? active.length : '—'}</span><span class="status-label">Downloading</span></a>
	<a href={to('/library?show=missing')}><span class="status-value">{loaded ? missing.length : '—'}</span><span class="status-label">Missing shown</span></a>
	{#if session.can('settings')}
		<a href={to(`/settings/this-app`)}><span class="status-value">{loaded ? issues.length : '—'}</span><span class="status-label">Health issues</span></a>
	{/if}
</nav>

{#if session.can('settings') && loaded && doneCount < steps.length && nextStep}
	<section class="setup-workspace rise" aria-labelledby="h-setup">
		<div class="setup-heading">
			<div>
				<p class="eyebrow">Set up your workspace</p>
				<h2 id="h-setup">Finish setting up HolyArrs</h2>
			</div>
			<span class="progress-count">{doneCount} of {steps.length} complete</span>
		</div>
		<div class="segs" aria-hidden="true">
			{#each steps as s, i (i)}<i class:on={s.done}></i>{/each}
		</div>
		<div class="setup-grid">
			<div class="setup-primary">
				<span class="step-number">Next step</span>
				<h3>{nextStep.text}</h3>
				<p>{nextStep.detail}</p>
				<a class="btn btn-dark" href={to(nextStep.href)}>Continue setup <Icon name="arrow" size={16} /></a>
				<ul class="setup-list">
					{#each steps as s, i (i)}
						{#if s.text !== nextStep.text}
							<li class:done={s.done}>
								{#if s.done}
									<span class="step-state"><Icon name="check" size={16} /></span>
									<span>{s.text}</span>
									<span class="step-meta">Complete</span>
								{:else}
									<a href={to(s.href)}>
										<span class="step-state">{i + 1}</span>
										<span>{s.text}</span>
										<Icon name="arrow" size={15} />
									</a>
								{/if}
							</li>
						{/if}
					{/each}
				</ul>
			</div>
			<aside class="readiness" aria-label="Workspace readiness">
				<div class="readiness-head">
					<span>Workspace readiness</span>
					<strong>{Math.round((doneCount / steps.length) * 100)}%</strong>
				</div>
				<div class="readiness-ring" style={`--progress: ${(doneCount / steps.length) * 360}deg`}>
					<span>{doneCount}/{steps.length}</span>
				</div>
				<dl class="readiness-facts">
					<div><dt>Library folders</dt><dd>{folders.length}</dd></div>
					<div><dt>Sources</dt><dd>{health?.sources?.length ?? 0}</dd></div>
					<div><dt>Downloaders</dt><dd>{health?.downloaders?.length ?? 0}</dd></div>
					<div><dt>Health issues</dt><dd>{issues.length}</dd></div>
				</dl>
			</aside>
		</div>
	</section>
{:else if loaded && titleCount === 0}
	<EmptyState icon="search" title="Add your first movie">
		<p>{session.can('settings') ? 'Everything is set up. Find a movie and add it to your Library.' : 'Your Library is empty. Find a movie to get started.'}</p>
		{#snippet actions()}<a class="btn btn-dark" href={to(`/search`)}>Search media</a>{/snippet}
	</EmptyState>
{/if}

<div class="section-heading">
	<div>
		<p class="eyebrow">Live operations</p>
		<h2>Activity</h2>
	</div>
	<a class="link small muted" href={to(`/downloads/history`)}>View all history</a>
</div>

<div class="home">
	<div class="main-col activity-grid">
		<section class="block" aria-labelledby="h-dl">
			<div class="block-head">
				<h2 id="h-dl" class="h2">Downloading now</h2>
				<a class="link small muted" href={to(`/downloads`)}>All downloads</a>
			</div>
			{#if !loaded}
				<div class="sk" style="height: 96px"></div>
			{:else if active.length === 0}
				<p class="quiet empty-row">Nothing is downloading. When HolyArrs picks a Release, it shows up here with its progress.</p>
			{:else}
				<ul class="rows">
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
				<p class="quiet empty-row">No searches yet. When HolyArrs looks for a movie, you see it here as it happens and what it found.</p>
			{:else}
				<ul class="rows" aria-live="polite">
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
				<p class="quiet empty-row">
					{titleCount ? 'Every title you keep up to date has a file. Nice.' : 'Movies you add but do not have yet show up here.'}
				</p>
			{:else}
				<ul class="rows">
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
				<p class="quiet empty-row">No Events yet. Every search, download and import writes one here.</p>
			{:else}
				<ul class="rows events">
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

	<aside class="side-col" aria-label="System status">
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
	.home-hero {
		margin-bottom: var(--s-5);
	}
	.eyebrow {
		margin: 0 0 var(--s-2);
		color: var(--text-muted);
		font-size: 11px;
		font-weight: var(--weight-semibold);
		letter-spacing: 0.09em;
		text-transform: uppercase;
	}
	.home-search {
		align-self: flex-start;
	}
	.metric-band {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
		margin-bottom: 32px;
		border: 1px solid var(--line);
		border-radius: var(--r-card);
		background: var(--surface-1);
		overflow: hidden;
	}
	.metric-band a {
		display: flex;
		flex-direction: column-reverse;
		gap: var(--s-2);
		min-width: 0;
		padding: 20px 24px;
		transition: background var(--dur) var(--ease-out);
	}
	.metric-band a + a {
		border-left: 1px solid var(--line);
	}
	.metric-band a:hover {
		background: var(--surface-2);
	}
	.status-value {
		font: var(--weight-semibold) 34px / 1 var(--font-display);
		font-variant-numeric: tabular-nums;
		letter-spacing: -0.04em;
	}
	.status-label {
		font-size: 13px;
		color: var(--text-muted);
	}
	.setup-workspace {
		margin-bottom: 36px;
	}
	.setup-heading,
	.section-heading {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--s-4);
		margin-bottom: var(--s-4);
	}
	.setup-heading h2,
	.section-heading h2 {
		margin: 0;
		font-size: clamp(20px, 2vw, 24px);
		letter-spacing: -0.025em;
	}
	.progress-count {
		color: var(--text-muted);
		font-size: 13px;
		white-space: nowrap;
	}
	.segs {
		display: grid;
		grid-template-columns: repeat(5, 1fr);
		gap: 4px;
		margin-bottom: var(--s-3);
	}
	.segs i {
		height: 3px;
		border-radius: 3px;
		background: var(--line-strong);
	}
	.segs i.on {
		background: var(--text);
	}
	.setup-grid {
		display: grid;
		grid-template-columns: minmax(0, 1.45fr) minmax(280px, 0.75fr);
		min-height: 330px;
		border: 1px solid var(--line);
		border-radius: var(--r-card);
		background: var(--surface-1);
		overflow: hidden;
	}
	.setup-primary {
		padding: 28px 32px 0;
	}
	.step-number {
		display: block;
		margin-bottom: var(--s-3);
		color: var(--text-muted);
		font-size: 13px;
		font-weight: var(--weight-medium);
	}
	.setup-primary h3 {
		margin: 0 0 var(--s-2);
		font-size: clamp(22px, 2.4vw, 30px);
		letter-spacing: -0.035em;
	}
	.setup-primary > p {
		max-width: 54ch;
		margin: 0 0 20px;
		color: var(--text-muted);
		font-size: 15px;
	}
	.setup-primary > .btn {
		margin-bottom: 28px;
	}
	.setup-list {
		border-top: 1px solid var(--line);
	}
	.setup-list li {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-height: 48px;
		border-bottom: 1px solid var(--line);
		font-size: 14px;
		font-weight: var(--weight-medium);
	}
	.setup-list li:last-child {
		border-bottom: 0;
	}
	.setup-list li.done {
		color: var(--text-muted);
	}
	.setup-list li > a {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		width: 100%;
		min-height: 48px;
	}
	.setup-list li > a :global(.icon) {
		margin-left: auto;
		color: var(--text-muted);
	}
	.step-state {
		display: grid;
		place-items: center;
		width: 24px;
		height: 24px;
		border: 1px solid var(--line-strong);
		border-radius: 50%;
		font-size: 11px;
		font-variant-numeric: tabular-nums;
		flex: none;
	}
	.step-meta {
		margin-left: auto;
		font-size: 12px;
		font-weight: var(--weight-regular);
	}
	.readiness {
		display: flex;
		flex-direction: column;
		justify-content: center;
		padding: 28px;
		border-left: 1px solid var(--line);
		background: var(--surface-2);
	}
	.readiness-head {
		display: flex;
		justify-content: space-between;
		gap: var(--s-3);
		margin-bottom: 24px;
		font-size: 13px;
	}
	.readiness-head span {
		color: var(--text-muted);
	}
	.readiness-ring {
		display: grid;
		place-items: center;
		width: 120px;
		height: 120px;
		margin: 0 auto 28px;
		border-radius: 50%;
		background: conic-gradient(var(--text) var(--progress), var(--line-strong) 0);
		position: relative;
	}
	.readiness-ring::after {
		content: '';
		position: absolute;
		inset: 8px;
		border-radius: inherit;
		background: var(--surface-2);
	}
	.readiness-ring span {
		position: relative;
		z-index: 1;
		font-size: 24px;
		font-weight: var(--weight-semibold);
		letter-spacing: -0.04em;
	}
	.readiness-facts {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		border: 1px solid var(--line);
		border-radius: var(--r-sm);
		background: var(--surface-1);
	}
	.readiness-facts div {
		padding: var(--s-3);
	}
	.readiness-facts div:nth-child(even) {
		border-left: 1px solid var(--line);
	}
	.readiness-facts div:nth-child(n + 3) {
		border-top: 1px solid var(--line);
	}
	.readiness-facts dt {
		color: var(--text-muted);
		font-size: 11px;
	}
	.readiness-facts dd {
		margin: 4px 0 0;
		font-size: 17px;
		font-weight: var(--weight-semibold);
	}
	.section-heading {
		margin-bottom: var(--s-3);
	}
	.home {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 300px;
		gap: var(--s-4);
		align-items: start;
	}
	.activity-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		border: 1px solid var(--line);
		border-radius: var(--r-card);
		background: var(--surface-1);
		overflow: hidden;
	}
	.block {
		min-width: 0;
		padding: 20px;
	}
	.block:nth-child(even) {
		border-left: 1px solid var(--line);
	}
	.block:nth-child(n + 3) {
		border-top: 1px solid var(--line);
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
	.block-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		margin-bottom: var(--s-2);
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
	.empty-row {
		min-height: 88px;
		margin: 0;
		padding: var(--s-4);
		border: 1px dashed var(--line-strong);
		border-radius: var(--r-sm);
	}
	.rows {
		padding: 0;
		border-top: 1px solid var(--line);
	}
	.rows a:hover {
		color: var(--info);
	}
	.rows > li {
		padding-inline: var(--s-2);
	}
	.rows > li:hover {
		background: var(--surface-2);
	}
	.nums {
		white-space: nowrap;
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
		.metric-band {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
		.metric-band a {
			min-height: 88px;
			padding: var(--s-4);
		}
		.metric-band a:nth-child(3) {
			border-left: 0;
			border-top: 1px solid var(--line);
		}
		.metric-band a:nth-child(4) {
			border-top: 1px solid var(--line);
		}
		.status-value {
			font-size: 30px;
		}
		.setup-heading,
		.section-heading {
			align-items: flex-start;
			flex-direction: column;
		}
		.setup-primary,
		.readiness {
			padding: 20px;
		}
		.setup-primary > .btn {
			width: 100%;
			min-height: 44px;
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
		.setup-grid {
			grid-template-columns: 1fr;
		}
		.readiness {
			border-top: 1px solid var(--line);
			border-left: 0;
		}
	}
	@media (max-width: 1600px) {
		.home {
			grid-template-columns: 1fr;
		}
		.side-col {
			position: static;
		}
	}
	@media (max-width: 1100px) {
		.activity-grid {
			grid-template-columns: 1fr;
		}
		.block:nth-child(even) {
			border-left: 0;
		}
		.block:nth-child(n + 2) {
			border-top: 1px solid var(--line);
		}
	}
</style>

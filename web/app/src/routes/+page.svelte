<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, ApiError } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { ago, greeting } from '#lib/format.ts';
	import { searchLine } from '#lib/releases.ts';
	import { problems } from '#lib/health.ts';
	import { groupLabel } from '#lib/history.ts';
	import { comingSoon, markTransitions, recentlyAdded, titleCard, type ExploreCard } from '#lib/explore.ts';
	import { attention, downloadLine, downloadSubtitle, greetName, isActive } from '#lib/home.ts';
	import { ymd } from '#lib/calendar.ts';
	import { personName } from '#lib/plain.ts';
	import type { DownloadRow, Decision } from '#lib/downloaders.ts';
	import type { Ask, CalendarEntry, EventRecord, Health, HealthChecks, LibraryFolder, SearchView, TitleView } from '#lib/types.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import ListRow from '#lib/ui/ListRow.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Icon from '#lib/Icon.svelte';

	interface MissingTitle { id: number; name: string; year?: number; state_label: string }

	let loaded = $state(false);
	let failed = $state(false);
	let retrying = $state(false);
	// The newest titles, when the server sends them with the bounded summary; Home never reads the whole Library.
	let titles = $state<TitleView[]>([]);
	let total = $state(0);
	let missing = $state<MissingTitle[]>([]);
	let folders = $state<LibraryFolder[]>([]);
	let downloads = $state<DownloadRow[]>([]);
	let events = $state<EventRecord[]>([]);
	let dates = $state<CalendarEntry[]>([]);
	let asks = $state<Ask[]>([]);
	let decisionItems = $state<Decision[]>([]);
	let health = $state<Health | null>(null);
	let checks = $state<HealthChecks | null>(null);
	let tmdbStatus = $state<'unknown' | 'ready' | 'missing'>('unknown');
	let tmdbNeedsProof = $state(false);
	let searching = $state<SearchView[]>([]);
	let found = $state<SearchView[]>([]);

	const admin = $derived(session.can('settings'));
	const manager = $derived(session.can('manage_library'));
	const approver = $derived(session.can('approve'));
	const name = $derived(greetName(session.s.name));
	const issues = $derived(problems(checks?.checks));

	const steps = $derived([
		{
			done: tmdbStatus === 'ready',
			text: tmdbStatus === 'unknown' ? 'Check the TMDB connection' : 'Connect TMDB',
			detail: tmdbStatus === 'unknown' ? 'Confirm the connection before HolyArrs marks TMDB ready.' : 'Use TMDB artwork and details when you add media.',
			href: '/settings/tmdb'
		},
		{ done: folders.length > 0, text: 'Choose a Library folder', detail: 'Tell HolyArrs where finished media should live.', href: '/settings/library-folders' },
		{ done: (health?.sources?.length ?? 0) > 0, text: 'Add a Source', detail: 'Connect a place where HolyArrs can find Releases.', href: '/settings/sources' },
		{ done: (health?.downloaders?.length ?? 0) > 0, text: 'Add a Downloader', detail: 'Connect the app that fetches chosen Releases.', href: '/settings/downloaders' },
		{ done: total > 0, text: 'Add your first title', detail: 'Search for a movie, show or artist to begin your Library.', href: '/search' }
	]);
	const doneCount = $derived(steps.filter((s) => s.done).length);
	const nextStep = $derived(steps.find((s) => !s.done));
	const showSetup = $derived(admin && loaded && !!nextStep);

	const active = $derived(downloads.filter(isActive));
	const failedDownloads = $derived(downloads.filter((d) => d.state === 'failed'));
	const byId = $derived(new Map(titles.map((t) => [t.id, t])));
	const recent = $derived(recentlyAdded(titles, 20).map((t) => titleCard(t)));
	const soon = $derived(comingSoon(dates, titles, new Date(), 20));
	const waiting = $derived(asks.filter((a) => a.state === 'asked'));
	const mine = $derived(asks.filter((a) => a.mine && a.state !== 'available').slice(0, 20));
	const fixes = $derived(manager || admin ? attention({ failed: failedDownloads, decisions: decisionItems, health: admin ? (checks?.checks ?? []) : [] }) : []);
	const missingCards = $derived(
		missing.slice(0, 20).map((m): ExploreCard => {
			const t = byId.get(m.id);
			return t ? { ...titleCard(t, m.state_label, `m${m.id}`), status: 'missing', statusLabel: m.state_label } : { key: `m${m.id}`, kind: 'movie', name: m.name, year: m.year, titleId: m.id, subtitle: m.state_label, status: 'missing', statusLabel: m.state_label, badges: [] };
		})
	);
	// One view-transition name per title on the page: the first poster wins.
	const rows = $derived(
		markTransitions([
			{ id: 'soon', title: '', cards: soon },
			{ id: 'recent', title: '', cards: recent },
			{ id: 'missing', title: '', cards: missingCards }
		])
	);
	let hasData = $state(false);

	function summary(): string {
		if (!loaded) return 'Looking at your Library…';
		if (total === 0) return 'Your Library is empty. Here is how to get going.';
		const n = total;
		const parts = [`${n} ${n === 1 ? 'title' : 'titles'} in your Library`];
		if (active.length) parts.push(`${active.length} downloading now`);
		return parts.join(', ') + '.';
	}

	function titleHref(id: number): string {
		return to(`/library/${id}`);
	}

	async function loadAll() {
		const mayReadTMDB = admin && !tmdbNeedsProof && (session.s.via === 'api_key' || session.s.localRecent);
		const from = ymd(new Date());
		const until = ymd(new Date(Date.now() + 30 * 86400000));
		const [su, fo, dl, ev, he, tk, se, ch, ca, as, de] = await Promise.allSettled([
			api.get<{ total: number; missing: MissingTitle[]; recent?: TitleView[] }>('/library-summary'),
			manager ? api.get<{ library_folders: LibraryFolder[] }>('/library-folders') : Promise.resolve(null),
			// Members may only read the Library and their own asks (perms.go); the rest needs manage_library.
			manager ? api.get<{ downloads: DownloadRow[] }>('/downloads/view?limit=50') : Promise.resolve(null),
			manager ? api.get<{ events: EventRecord[] }>('/events?limit=12') : Promise.resolve(null),
			manager ? api.get<Health>('/health') : Promise.resolve(null),
			mayReadTMDB ? api.get<{ token: string }>('/settings/tmdb-token') : Promise.resolve(null),
			manager ? api.get<{ running: SearchView[]; recent: SearchView[] }>('/searches?limit=6') : Promise.resolve(null),
			admin && manager ? api.get<HealthChecks>('/health/checks') : Promise.resolve(null),
			manager ? api.get<{ dates: CalendarEntry[] | null }>(`/calendar?from=${from}&to=${until}`) : Promise.resolve(null),
			session.can('ask') || approver ? api.get<{ asks: Ask[] }>('/asks') : Promise.resolve(null),
			manager ? api.get<{ items: Decision[]; count: number }>('/needs-decision') : Promise.resolve(null)
		]);
		// A failed refresh keeps what was shown before; only the Library summary decides the error banner.
		failed = su.status === 'rejected';
		if (su.status === 'fulfilled') {
			total = su.value.total ?? 0;
			missing = su.value.missing ?? [];
			titles = su.value.recent ?? [];
			hasData = true;
		}
		if (fo.status === 'fulfilled' && fo.value) folders = fo.value.library_folders;
		if (dl.status === 'fulfilled' && dl.value) downloads = dl.value.downloads ?? [];
		if (ev.status === 'fulfilled' && ev.value) events = ev.value.events ?? [];
		if (he.status === 'fulfilled' && he.value) health = he.value;
		if (se.status === 'fulfilled' && se.value) {
			searching = se.value.running ?? [];
			found = se.value.recent ?? [];
		}
		if (ch.status === 'fulfilled' && ch.value) checks = ch.value;
		if (ca.status === 'fulfilled' && ca.value) dates = ca.value.dates ?? [];
		if (as.status === 'fulfilled' && as.value) asks = as.value.asks ?? [];
		if (de.status === 'fulfilled' && de.value) decisionItems = de.value.items ?? [];
		if (mayReadTMDB) {
			if (tk.status === 'rejected' && tk.reason instanceof ApiError && tk.reason.status === 403) tmdbNeedsProof = true;
			tmdbStatus = tk.status === 'fulfilled' && tk.value ? (tk.value.token ? 'ready' : 'missing') : 'unknown';
		} else {
			tmdbStatus = 'unknown';
		}
		loaded = true;
	}

	async function retry() {
		retrying = true;
		try {
			await loadAll();
		} finally {
			retrying = false;
		}
	}

	onMount(() => {
		void loadAll();
		let timer: ReturnType<typeof setTimeout> | undefined;
		const soonLoad = () => {
			clearTimeout(timer);
			timer = setTimeout(loadAll, 250);
		};
		const offs = [
			live.on('title', soonLoad),
			live.on('event', soonLoad),
			live.on('import', soonLoad),
			live.on('resync', soonLoad),
			live.on('releases', soonLoad),
			live.on('health', soonLoad),
			live.on('download', soonLoad),
			live.on('job', (data) => {
				const j = data as { kind?: string };
				if (j?.kind === 'search_slot' || j?.kind === 'search_missing' || j?.kind === 'feed_check' || j?.kind === 'search_season') soonLoad();
			})
		];
		return () => {
			clearTimeout(timer);
			offs.forEach((off) => off());
		};
	});
</script>

<svelte:head><title>Home · HolyArrs</title></svelte:head>

<div class="home">
	<PageHeader title="Home" subtitle="{greeting()}{name ? `, ${name}` : ''}. {summary()}">
		{#snippet actions()}
			<Button variant="secondary" icon="search" href={to('/search')}>Search</Button>
			<Button variant="secondary" icon="compass" href={to('/explore')}>Explore</Button>
		{/snippet}
	</PageHeader>

	{#if failed && !hasData}
		<LoadError hasData={false} message="Could not load your Library." onretry={retry} busy={retrying} />
	{:else}
		{#if failed}<div class="banner"><LoadError hasData={true} message="Could not refresh Home." onretry={retry} busy={retrying} /></div>{/if}

		{#if !loaded}
			<section class="loading" aria-label="Loading" aria-busy="true">
				<Skeleton width="180px" />
				<div class="sk-row">{#each Array.from({ length: 6 }, (_, i) => i) as i (i)}<Skeleton shape="poster" />{/each}</div>
			</section>
		{/if}

		{#if showSetup && nextStep}
			<section class="setup" aria-labelledby="h-setup">
				<div class="setup-main">
					<p class="kicker">{doneCount} of {steps.length} done</p>
					<h2 id="h-setup">Finish setting up HolyArrs</h2>
					<div class="segs" aria-hidden="true">{#each steps as s, i (i)}<i class:on={s.done}></i>{/each}</div>
					<h3>{nextStep.text}</h3>
					<p>{nextStep.detail}</p>
					<Button variant="primary" size="lg" href={to(nextStep.href)} iconAfter="chevronRight">Continue setup</Button>
				</div>
				<ol class="steps">
					{#each steps as s, i (i)}
						<li class:done={s.done}>
							<a href={to(s.href)}>
								<span class="dot">{#if s.done}<Icon name="check" size={14} />{:else}{i + 1}{/if}</span>
								<span class="grow">{s.text}</span>
								<span class="sr-only">{s.done ? '(done)' : '(to do)'}</span>
								<Icon name="chevronRight" size={15} />
							</a>
						</li>
					{/each}
				</ol>
			</section>
		{:else if loaded && total === 0}
			<EmptyState
				icon="search"
				title="Add your first title"
				text={admin || manager ? 'Everything is set up. Find a movie, show or artist and add it to your Library.' : 'The Library is empty. Find a movie or show and ask for it.'}
				action={{ label: 'Search', href: to('/search'), icon: 'search' }}
			/>
		{/if}

		{#if fixes.length}
			<section class="attention" aria-labelledby="h-fix">
				<header>
					<h2 id="h-fix">Needs attention</h2>
					<span class="count">{fixes.length}</span>
				</header>
				<div class="list">
					{#each fixes.slice(0, 6) as f (f.key)}
						<ListRow title={f.text} href={to(f.href)} icon={f.tone === 'fail' ? 'alert' : 'clock'}>
							{#snippet trailing()}
								<StatusPill kind={f.tone === 'fail' ? 'failed' : 'missing'} label={f.word} />
								<span class="go">{f.action}<Icon name="chevronRight" size={14} /></span>
							{/snippet}
						</ListRow>
						{#if admin && f.detail}
							<details class="detail"><summary>Details</summary><p>{f.detail}</p></details>
						{/if}
					{/each}
				</div>
				{#if fixes.length > 6}<a class="more" href={to('/downloads')}>{fixes.length - 6} more in Downloads</a>{/if}
			</section>
		{/if}

		{#if active.length}
			<Shelf title="Downloading now" subtitle="{active.length} {active.length === 1 ? 'download' : 'downloads'}" href={to('/downloads')} item="300px">
				{#each active.slice(0, 20) as d (d.id)}
					<WideCard
						title={d.title || d.name}
						href={d.title_id ? titleHref(d.title_id) : to('/downloads')}
						image={d.artwork || byId.get(d.title_id ?? -1)?.artwork?.[0]}
						subtitle={downloadSubtitle(d)}
						meta={downloadLine(d)}
						progress={Math.round(d.progress * 100)}
						status={d.state === 'stalled' ? 'missing' : 'downloading'}
						statusLabel={d.state === 'stalled' ? 'Stalled' : d.state === 'queued' ? 'Waiting' : undefined}
					/>
				{/each}
			</Shelf>
		{/if}

		{#if rows[0].cards.length}
			<Shelf title="Coming soon" subtitle="The next 30 days" href={to('/calendar')}>
				{#each rows[0].cards as c (c.key)}
					<PosterCard title={c.name} href={c.titleId ? titleHref(c.titleId) : undefined} image={c.image} subtitle={c.subtitle} status={c.status} statusLabel={c.statusLabel} vtName={c.vtName} />
				{/each}
			</Shelf>
		{/if}

		{#if rows[1].cards.length}
			<Shelf title="Recently added" href={to('/library')}>
				{#each rows[1].cards as c (c.key)}
					<PosterCard title={c.name} href={titleHref(c.titleId!)} image={c.image} subtitle={c.subtitle} badges={c.badges} status={c.status === 'in_library' ? undefined : c.status} statusLabel={c.statusLabel} vtName={c.vtName} />
				{/each}
			</Shelf>
		{/if}

		{#if approver && waiting.length}
			<Shelf title="Waiting for approval" subtitle="{waiting.length} {waiting.length === 1 ? 'ask' : 'asks'} on the Wishlist" href={to('/wishlist')}>
				{#each waiting.slice(0, 20) as a (a.id)}
					<PosterCard title={a.name} href={to('/wishlist')} image={a.poster} subtitle="Asked by {personName(a.asked_by?.name, 'someone in your household')}" status="requested" statusLabel="Waiting" />
				{/each}
			</Shelf>
		{:else if !approver && mine.length}
			<Shelf title="Your Wishlist" href={to('/wishlist')}>
				{#each mine as a (a.id)}
					<PosterCard
						title={a.name}
						href={a.title_id ? titleHref(a.title_id) : to('/wishlist')}
						image={a.poster}
						subtitle={a.season_words || a.album_words || (a.year ? String(a.year) : undefined)}
						status={a.state === 'approved' ? 'approved' : a.state === 'turned_down' ? 'turned_down' : 'requested'}
						statusLabel={a.state === 'asked' ? 'Requested' : undefined}
					/>
				{/each}
			</Shelf>
		{/if}

		{#if !manager && loaded}
			<section class="prompt" aria-labelledby="h-explore">
				<div>
					<h2 id="h-explore">Looking for something new?</h2>
					<p>Explore popular movies and shows, then tap Ask to add one to your Wishlist.</p>
				</div>
				<Button variant="primary" icon="compass" href={to('/explore')}>Explore</Button>
			</section>
		{/if}

		{#if manager && rows[2].cards.length}
			<Shelf title="Missing" subtitle="Titles you keep up to date that have no file yet" href={to('/library?show=missing')}>
				{#each rows[2].cards as c (c.key)}
					<PosterCard title={c.name} href={titleHref(c.titleId!)} image={c.image} subtitle={c.subtitle} status={c.status} statusLabel={c.statusLabel} vtName={c.vtName} />
				{/each}
			</Shelf>
		{/if}

		{#if loaded && (searching.length || found.length || events.length || (admin && health))}
			<div class="lately">
				{#if searching.length || found.length}
					<section class="panel" aria-labelledby="h-search" data-testid="home-searches">
						<header><h2 id="h-search">Searches</h2><a href={to('/library/held')}>Held and Turned down</a></header>
						<div class="list" aria-live="polite">
							{#each searching as x (x.job_id)}
								<div data-testid="search-running"><ListRow title={searchLine(x)} icon="search">{#snippet trailing()}<StatusPill kind="downloading" label={x.state === 'queued' ? 'Waiting' : 'Searching'} />{/snippet}</ListRow></div>
							{/each}
							{#each found as x (x.job_id)}
								<div data-testid="search-found"><ListRow title={searchLine(x)} icon="search" meta={ago(x.ended_at)} href={x.title_id ? titleHref(x.title_id) : undefined} /></div>
							{/each}
						</div>
					</section>
				{/if}
				{#if events.length}
					<section class="panel" aria-labelledby="h-ev">
						<header><h2 id="h-ev">Latest Events</h2><a href={to('/downloads/history')}>History</a></header>
						<div class="list">
							{#each events as e (e.ID)}
								<ListRow title={e.Summary} subtitle={groupLabel(e.Kind)} meta={ago(e.At)} icon="clock" />
							{/each}
						</div>
					</section>
				{/if}
				{#if admin && health}
					<section class="panel" aria-labelledby="h-state" data-testid="home-health">
						<header><h2 id="h-state">This HolyArrs</h2><a href={to('/settings/this-app')}>All health checks</a></header>
						<dl class="facts">
							<div><dt>Health</dt><dd>{#if checks}<StatusPill kind={issues.length === 0 ? 'in_library' : checks.worst === 'error' ? 'failed' : 'missing'} label={issues.length === 0 ? 'All fine' : `${issues.length} to look at`} />{:else}—{/if}</dd></div>
							<div><dt>Version</dt><dd>{health.version}</dd></div>
							<div><dt>Sources</dt><dd>{health.sources?.length ?? 0}</dd></div>
							<div><dt>Downloaders</dt><dd>{health.downloaders?.length ?? 0}</dd></div>
						</dl>
					</section>
				{/if}
			</div>
		{/if}
	{/if}
</div>

<style>
	.home { display: flex; flex-direction: column; gap: 36px; padding-bottom: 48px; }
	.home > :global(.ui-head) { margin-bottom: -12px; }
	.banner { margin-bottom: -16px; }
	.loading { display: flex; flex-direction: column; gap: 14px; }
	.sk-row { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: var(--shelf-gap); max-height: 260px; overflow: hidden; }

	.setup { display: grid; grid-template-columns: minmax(0, 1.3fr) minmax(260px, 1fr); gap: 0; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); outline: 1px solid var(--line); overflow: hidden; }
	.setup-main { padding: clamp(20px, 3vw, 32px); display: flex; flex-direction: column; align-items: flex-start; gap: 8px; }
	.kicker { color: var(--text-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.setup h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.setup h3 { font: var(--t-headline); margin-top: 8px; }
	.setup-main > p:not(.kicker) { color: var(--text-muted); max-width: 52ch; margin-bottom: 8px; }
	.segs { display: grid; grid-template-columns: repeat(5, 1fr); gap: 4px; width: min(320px, 100%); margin: 4px 0; }
	.segs i { height: 4px; border-radius: 2px; background: var(--surface-3); }
	.segs i.on { background: var(--ok); }
	.steps { list-style: none; margin: 0; padding: 12px; background: var(--surface-2); display: flex; flex-direction: column; justify-content: center; }
	.steps a { display: flex; align-items: center; gap: 12px; min-height: 48px; padding: 0 10px; border-radius: var(--r-btn); color: var(--text); font: var(--t-callout); font-weight: 500; }
	.steps a:hover { background: var(--surface-3); }
	.steps a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.steps a > :global(.icon) { color: var(--text-muted); }
	.steps .done a { color: var(--text-muted); }
	.dot { width: 26px; height: 26px; flex: none; display: grid; place-items: center; border-radius: 50%; background: var(--surface-1); outline: 1px solid var(--line-strong); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.done .dot { background: var(--ok); color: #fff; outline: none; }
	.grow { flex: 1; min-width: 0; }

	.attention header, .panel header { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; margin-bottom: 8px; }
	.attention header { justify-content: flex-start; }
	.attention h2, .panel h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.panel h2 { font: var(--t-title3); }
	.count { min-width: 24px; height: 24px; padding: 0 8px; display: inline-grid; place-items: center; border-radius: 999px; background: var(--accent-soft); color: var(--accent-text); font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.detail { margin: -2px 12px 6px 52px; color: var(--text-muted); font: var(--t-footnote); }
	.detail summary { width: max-content; min-height: 24px; cursor: pointer; border-radius: 6px; }
	.detail summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.detail p { margin-top: 4px; font-family: var(--font-mono); overflow-wrap: anywhere; }
	.attention .list { border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); padding: 6px; }
	.go { display: inline-flex; align-items: center; gap: 2px; color: var(--accent-text); font: var(--t-subhead); font-weight: 600; }
	.more, .panel header a { display: inline-flex; align-items: center; min-height: 44px; color: var(--accent-text); font: var(--t-callout); font-weight: 500; }
	.more:hover, .panel header a:hover { text-decoration: underline; text-underline-offset: 3px; }

	.prompt { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 16px; padding: clamp(18px, 3vw, 28px); border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.prompt h2 { font: var(--t-title3); }
	.prompt p { color: var(--text-muted); max-width: 56ch; margin-top: 4px; }

	.lately { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 340px), 1fr)); gap: 20px; align-items: start; }
	.panel { min-width: 0; padding: 16px 16px 10px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.panel .list { margin: 0 -8px; }
	.facts { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; padding: 4px 0 8px; }
	.facts dt { color: var(--text-muted); font: var(--t-footnote); margin-bottom: 4px; }
	.facts dd { margin: 0; font: var(--t-headline); font-variant-numeric: tabular-nums; }

	@media (max-width: 900px) { .setup { grid-template-columns: 1fr; } }
	@media (max-width: 767px) { .home { gap: 28px; } }
</style>

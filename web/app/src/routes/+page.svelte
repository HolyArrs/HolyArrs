<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/api.ts';
	import { afterHandoff } from '#lib/loader.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { arrivedHead, askProgressLine, attention, downloadLine, downloadMoved, downloadingNow, fallbackRows, groupAttention, homeFailures, homeStrip, libraryCards, moreLink, n, uncheckedLine, type Attention, type HomeAnswer, type HomeCard, type HomeRow } from '#lib/home.ts';
	import { decisions } from '#lib/decisions.svelte.ts';
	import { METADATA_DOWN, metadataOutage } from '#lib/metadata-health.svelte.ts';
	import { bytes } from '#lib/format.ts';
	import { homeFacts, homeLine } from '#lib/home-line.ts';
	import { personName } from '#lib/plain.ts';
	import type { Fresh } from '#lib/fresh.ts';
	import type { DownloadRow } from '#lib/downloaders.ts';
	import type { Ask, Health, HealthChecks, LibraryFolder } from '#lib/types.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import MetadataNotice from '#lib/MetadataNotice.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Icon from '#lib/Icon.svelte';

	// Home is "my stuff": the server sends the row order per person (GET /home);
	// every row hides when it has nothing.
	let loaded = $state(false);
	// The bigger asks (asks, downloads) have answered: the greeting waits for them so it does not change under your eyes.
	let settled = $state(false);
	let now = $state(new Date());
	let failed = $state(false);
	let retrying = $state(false);
	let home = $state<HomeAnswer | null>(null);
	let folders = $state<LibraryFolder[]>([]);
	let downloads = $state<DownloadRow[]>([]);
	let asks = $state<Ask[]>([]);
	let health = $state<Health | null>(null);
	let checks = $state<HealthChecks | null>(null);
	// The downloads or health read failed on the last load: Home could not check, so it does not say All good.
	let uncheckedDl = $state(false);
	let uncheckedHealth = $state(false);

	const admin = $derived(session.can('settings'));
	const manager = $derived(session.can('manage_library'));
	const approver = $derived(session.can('approve'));
	const owner = $derived(home ? home.owner : manager);

	const steps = $derived([
		{ done: folders.length > 0, text: 'Choose a Library folder', detail: 'Tell HolyArrs where finished media should live.', href: '/settings/library-folders' },
		{ done: (health?.sources?.length ?? 0) > 0, text: 'Add a Source', detail: 'Connect a place where HolyArrs can find Releases.', href: '/settings/sources' },
		{ done: (health?.downloaders?.length ?? 0) > 0, text: 'Add a Downloader', detail: 'Connect the app that fetches chosen Releases.', href: '/settings/downloaders' },
		{ done: false, text: 'Add your first title', detail: 'Search for a new movie, show or artist to fetch, or import the folders you already have on disk.', href: '/search' }
	]);
	const doneCount = $derived(steps.filter((s) => s.done).length);
	const nextStep = $derived(steps.find((s) => !s.done) ?? steps[3]);

	const rows = $derived<HomeRow[]>(home?.rows ?? (loaded ? fallbackRows(owner) : []));
	// Needs attention is what needs you: failed downloads or imports (the same rule as Downloads), decisions,
	// health problems and a metadata outage while it lasts. Decisions come from the list the layout keeps fresh.
	const fixes = $derived(
		owner
			? attention({
					failed: homeFailures(downloads),
					decisions: manager ? decisions.items : [],
					health: checks?.checks ?? [],
					metadata: metadataOutage.s.down ? METADATA_DOWN : undefined,
					admin
				})
			: []
	);
	const unchecked = $derived(owner && (uncheckedDl || uncheckedHealth));
	// Items that share a cause are one row ("2 downloads failed"); six rows show, the rest link to the page that holds most of them.
	const fixRows = $derived(groupAttention(fixes));
	const fixShown = $derived(fixRows.slice(0, 6));
	const fixHidden = $derived(moreLink(fixRows.slice(6), admin));
	let fixOpen = $state<Record<string, boolean>>({});
	/** The plain note and, for admins, the raw text: what the Details link shows. */
	function fixMore(i: Attention): string[] {
		return [i.note, admin ? i.detail : undefined].filter((t): t is string => !!t && t !== i.title);
	}
	// Your Library: one card per kind that is on, and for owners what it all takes on disk.
	const libCards = $derived(libraryCards(home?.library, manager ? '/downloads' : undefined));
	const diskTotal = $derived(libCards.reduce((t, c) => t + c.size, 0));
	const showDisk = $derived(owner && libCards.length > 0);
	const showOverview = $derived(!rows.includes('welcome') && (libCards.length > 0 || owner));
	const waiting = $derived(asks.filter((a) => a.state === 'asked'));
	const mine = $derived(asks.filter((a) => a.mine && a.state !== 'available'));
	const mineShown = $derived(mine.slice(0, 20));
	const progress = $derived(home?.ask_progress ?? {});
	// Only a card on Your asks needs the live poll; downloads of asks with no card there do not.
	const busy = $derived(mineShown.some((a) => !!a.title_id && !!progress[a.title_id]));
	// Downloading (managers): what HolyArrs fetches now, from the downloads this page already read.
	const fetchingAll = $derived(manager ? downloadingNow(downloads, Infinity) : []);
	const fetchingShown = $derived(fetchingAll.slice(0, 20));
	// The download rows carry no poster; the titles Home already shows lend theirs.
	const posters = $derived.by(() => {
		const m = new Map<number, string>();
		for (const c of [...(home?.arrived ?? []), ...(home?.new_seasons ?? []), ...(home?.following ?? []), ...(home?.soon ?? [])]) if (c.poster) m.set(c.title_id, c.poster);
		for (const a of asks) if (a.title_id && a.poster) m.set(a.title_id, a.poster);
		return m;
	});
	// The line under Home: your name, the time and day, and what is going on, from what this page already loaded.
	const line = $derived(
		homeLine({
			name: session.s.name,
			now,
			kid: home?.kid ?? session.kid,
			facts: homeFacts({ home, asks, downloads, approver, manager, kid: home?.kid ?? session.kid, now })
		})
	);

	/** The ask's one round status mark: live progress first. */
	function askMark(a: Ask): { state: 'available' | 'requested' | 'declined' | 'downloading'; label: string } | null {
		const p = a.title_id ? progress[a.title_id] : undefined;
		if (p) return { state: 'downloading', label: askProgressLine(p) };
		if (a.state === 'available') return { state: 'available', label: 'In your library' };
		if (a.state === 'turned_down') return { state: 'declined', label: 'Turned down' };
		return { state: 'requested', label: a.state === 'approved' ? 'Approved, waiting for a copy' : 'Requested' };
	}

	/** A server-filled row's strip: what is new on the card, in the row's own kind. */
	function strip(c: HomeCard, type: string): Fresh | null {
		const own = homeStrip(c);
		if (own || !c.line || type === 'home_arrived') return own;
		return { kind: type === 'home_soon' ? 'coming' : 'new_season', text: c.line };
	}

	async function loadHome() {
		try {
			home = await api.get<HomeAnswer>('/home');
			failed = false;
		} catch {
			failed = true;
		}
	}

	async function loadAll() {
		// The first screen (the Home answer) loads now; the bigger asks wait until
		// the sign-in hand-off has left, so its frames stay smooth.
		const first = loadHome().then(() => {
			if (home) loaded = true;
		});
		await afterHandoff();
		const [, fo, dl, he, ch, as] = await Promise.allSettled([
			first,
			manager ? api.get<{ library_folders: LibraryFolder[] }>('/library-folders') : Promise.resolve(null),
			// Members may only read the Library and their own asks (perms.go); the rest needs manage_library.
			manager ? api.get<{ downloads: DownloadRow[] }>('/downloads/view?limit=100') : Promise.resolve(null),
			manager ? api.get<Health>('/health') : Promise.resolve(null),
			// The checks need manage_library only; people without Settings get Status links instead of Fix.
			manager ? api.get<HealthChecks>('/health/checks') : Promise.resolve(null),
			session.can('ask') || approver ? api.get<{ asks: Ask[] }>('/asks') : Promise.resolve(null)
		]);
		if (fo.status === 'fulfilled' && fo.value) folders = fo.value.library_folders;
		if (dl.status === 'fulfilled' && dl.value) downloads = dl.value.downloads ?? [];
		if (he.status === 'fulfilled' && he.value) health = he.value;
		if (ch.status === 'fulfilled' && ch.value) checks = ch.value;
		if (as.status === 'fulfilled' && as.value) asks = as.value.asks ?? [];
		uncheckedDl = dl.status === 'rejected';
		uncheckedHealth = ch.status === 'rejected';
		now = new Date();
		loaded = true;
		settled = true;
	}

	async function retry() {
		retrying = true;
		try {
			await loadAll();
		} finally {
			retrying = false;
		}
	}

	// Live progress on Your asks: poll every 15 s, only while a card is downloading and the tab is in view.
	$effect(() => {
		if (!busy) return;
		const t = setInterval(() => {
			if (!document.hidden) void loadHome();
		}, 15000);
		return () => clearInterval(t);
	});

	// Members get no live title or import news. When their download ends, Home reads everything once;
	// while an ask of theirs is approved and waiting for a copy, it reads again once a minute (in view only).
	let wasBusy = false;
	$effect(() => {
		const b = busy;
		if (wasBusy && !b && !manager) void loadAll();
		wasBusy = b;
	});
	const waitingCopy = $derived(!manager && !busy && mine.some((a) => a.state === 'approved'));
	$effect(() => {
		if (!waitingCopy) return;
		const t = setInterval(() => {
			if (!document.hidden) void loadAll();
		}, 60000);
		return () => clearInterval(t);
	});

	onMount(() => {
		void loadAll();
		// The greeting's time of day and "today" move on without a fetch.
		const clock = setInterval(() => (now = new Date()), 300000);
		let timer: ReturnType<typeof setTimeout> | undefined;
		const soonLoad = () => {
			clearTimeout(timer);
			timer = setTimeout(loadAll, 250);
		};
		// A download's progress ticks every few seconds; Home re-reads on a new state, or at most once a minute for progress.
		const seen = new Map<number, string>();
		let lastTick = Date.now();
		const onDownload = (x: unknown) => {
			if (!downloadMoved(seen, x) && Date.now() - lastTick < 60000) return;
			lastTick = Date.now();
			soonLoad();
		};
		const offs = [live.on('title', soonLoad), live.on('import', soonLoad), live.on('resync', soonLoad), live.on('health', soonLoad), live.on('ask', soonLoad), live.on('ask_removed', soonLoad), live.on('download', onDownload)];
		return () => {
			clearTimeout(timer);
			clearInterval(clock);
			offs.forEach((off) => off());
		};
	});
</script>

<svelte:head><title>Home · HolyArrs</title></svelte:head>

<div class="home">
	<PageHeader title="Home" subtitle={settled ? line : '\u00a0'}>
		{#snippet actions()}
			<Button variant="secondary" icon="search" href={to('/search')}>Search</Button>
			<Button variant="secondary" icon="compass" href={to('/explore')}>Explore</Button>
		{/snippet}
	</PageHeader>

	{#if failed && !home && loaded}
		<LoadError hasData={false} message="Could not load Home." onretry={retry} busy={retrying} />
	{:else}
		{#if failed}<div class="banner"><LoadError hasData={true} message="Could not refresh Home." onretry={retry} busy={retrying} /></div>{/if}
		<MetadataNotice />

		{#snippet wideRow(cards: HomeCard[], type: string, opts: { title: string; subtitle?: string; href?: string })}
			{#if cards.length}
				<div class="wide-row" data-testid={type}>
					<Shelf title={opts.title} subtitle={opts.subtitle} href={opts.href} item="var(--title-item)">
						{#each cards as c (c.key)}
							<!-- Every card is 16:9; an artist shows its wide picture with the new album's cover inset. -->
							<WideCard cover title={c.name} kind={c.kind} typed row={type} href={to(c.href)} image={c.backdrop || c.poster || undefined} logo={c.kind === 'artist' ? null : c.logo} inset={c.kind === 'artist' ? c.albums?.[0]?.cover : undefined} subtitle={c.line} fresh={strip(c, type)} />
						{/each}
					</Shelf>
				</div>
			{/if}
		{/snippet}

		<PageLoader loading={!loaded} label="Loading Home" onretry={retry}>
		{#if showOverview}
			<div class="overview" class:solo={!owner} data-testid="home-overview">
				{#if libCards.length}
					<section class="library" aria-labelledby="h-lib" data-testid="home-library">
						<header class="col-head">
							<h2 id="h-lib">Your Library</h2>
							<a class="col-link" href={to('/library')}>See all<span class="sr-only"> Your Library</span><Icon name="chevronRight" size={14} /></a>
						</header>
						<ul class="lib-grid">
							{#each libCards as c, i (c.kind)}
								<li class="lib-card k-{c.kind}" style="--i: {i}">
									<div class="lib-top">
										<span class="lib-icon" aria-hidden="true"><Icon name={c.icon} size={20} /></span>
										<h3><a class="stretch" href={to(c.href)}>{c.label}</a></h3>
										<span class="lib-go" aria-hidden="true"><Icon name="chevronRight" size={16} /></span>
									</div>
									<p class="lib-big"><span class="num">{n(c.big)}</span> <span class="unit">{c.unit}</span></p>
									{#if c.detail}<p class="lib-detail">{c.detail}</p>{/if}
									<div class="bar" role="img" aria-label={c.segments.map((s) => `${n(s.count)} ${s.label}`).join(', ')}>
										{#each c.segments as s (s.key)}{#if s.count}<i class="seg {s.key}" style="flex-grow: {s.share}"></i>{/if}{/each}
										<i class="rest" style="flex-grow: {Math.max(0, 1 - c.segments.reduce((t, s) => t + s.share, 0))}"></i>
									</div>
									<ul class="legend">
										{#each c.segments as s (s.key)}
											{#if s.count || s.key === 'on_disk'}
												<li>
													{#if s.href}
														<a class="leg {s.key}" href={to(s.href)}><i aria-hidden="true"></i><strong>{n(s.count)}</strong> {s.label}<span class="sr-only"> ({c.label})</span></a>
													{:else}
														<span class="leg {s.key}"><i aria-hidden="true"></i><strong>{n(s.count)}</strong> {s.label}</span>
													{/if}
												</li>
											{/if}
										{/each}
									</ul>
								</li>
							{/each}
							{#if showDisk}
								<li class="lib-card k-disk" class:wide={libCards.length % 2 === 0} style="--i: {libCards.length}">
									<div class="lib-top">
										<span class="lib-icon" aria-hidden="true"><Icon name="archive" size={20} /></span>
										<h3>On disk</h3>
									</div>
									<p class="lib-big"><span class="num">{bytes(diskTotal)}</span></p>
									<p class="lib-detail">Across your Library folders</p>
									<div class="bar" role="img" aria-label={libCards.map((c) => `${c.label} ${bytes(c.size)}`).join(', ')}>
										{#each libCards as c (c.kind)}{#if c.size}<i class="seg k-{c.kind}" style="flex-grow: {c.size / Math.max(diskTotal, 1)}"></i>{/if}{/each}
										{#if !diskTotal}<i class="rest" style="flex-grow: 1"></i>{/if}
									</div>
									<ul class="legend">
										{#each libCards as c (c.kind)}
											<li><span class="leg k-{c.kind}"><i aria-hidden="true"></i><strong>{bytes(c.size)}</strong> {c.label}</span></li>
										{/each}
									</ul>
								</li>
							{/if}
						</ul>
					</section>
				{/if}
				{#if owner}
					<section class="attention" aria-labelledby="h-fix" data-testid="home-attention">
						<header class="col-head">
							<h2 id="h-fix">Needs attention</h2>
						</header>
						<div class="att-card" class:calm={settled && !fixes.length && !unchecked}>
							{#if !settled}
								<div class="att-wait" aria-hidden="true"><i></i><i></i><i></i></div>
								<span class="sr-only">Checking</span>
							{:else if !fixes.length && unchecked}
								<div class="all-good" role="status">
									<span class="att-icon" aria-hidden="true"><Icon name="alert" size={20} /></span>
									<p class="good-title">Could not check</p>
									<p class="good-line">{uncheckedLine(uncheckedDl, uncheckedHealth)}</p>
									<Button size="sm" variant="secondary" busy={retrying} onclick={retry}>Try again</Button>
								</div>
							{:else if !fixes.length}
								<div class="all-good">
									<span class="good-mark" aria-hidden="true"><Icon name="check" size={22} /></span>
									<p class="good-title">All good</p>
									<p class="good-line">Downloads, imports and checks are running fine.</p>
								</div>
							{:else}
								<div class="att-sum">
									<span class="att-icon" aria-hidden="true"><Icon name="alert" size={20} /></span>
									<p class="lib-big"><span class="num">{n(fixes.length)}</span> <span class="unit">{fixes.length === 1 ? 'thing needs a look' : 'things need a look'}</span></p>
								</div>
								<ul class="list">
									{#each fixShown as g (g.key)}
										{@const many = g.items.length > 1}
										{@const more = many ? [] : fixMore(g.items[0])}
										<li class="fix">
											<span class="tone {g.tone}" aria-hidden="true"></span>
											<div class="body">
												{#if many}
													<button class="lead" type="button" aria-expanded={!!fixOpen[g.key]} aria-controls="fix-{g.key}" onclick={() => (fixOpen[g.key] = !fixOpen[g.key])}>
														<span class="ltxt"><span class="sr-only">{g.word}: </span><strong class="tt">{g.title}</strong><span class="why">{g.reason}</span></span>
														<span class="caret" class:up={fixOpen[g.key]}><Icon name="chevronDown" size={14} /></span>
													</button>
												{:else}
													<span class="lead"><span class="ltxt"><span class="sr-only">{g.word}: </span><strong class="tt">{g.title}</strong><span class="why">{g.reason}</span></span></span>
												{/if}
												<span class="acts">
													{#if more.length}<button class="link" type="button" aria-expanded={!!fixOpen[g.key]} aria-controls="fix-{g.key}" onclick={() => (fixOpen[g.key] = !fixOpen[g.key])}>Details<span class="sr-only"> for {g.title}</span></button>{/if}
													<a class="go link" href={to(g.href)} aria-label="{g.action} ({g.title})">{g.action}<Icon name="chevronRight" size={13} /></a>
												</span>
												{#if many && fixOpen[g.key]}
													<ul class="members" id="fix-{g.key}">
														{#each g.items as it (it.key)}
															{@const sub = fixMore(it)}
															<li>
																<span class="mt">{it.title}</span>
																{#if sub.length}<button class="link" type="button" aria-expanded={!!fixOpen[it.key]} aria-controls="fix-{it.key}" onclick={() => (fixOpen[it.key] = !fixOpen[it.key])}>Details<span class="sr-only"> for {it.title}</span></button>{/if}
																{#if sub.length && fixOpen[it.key]}<p class="raw" id="fix-{it.key}">{sub.join(' ')}</p>{/if}
															</li>
														{/each}
													</ul>
												{:else if more.length && fixOpen[g.key]}
													<p class="raw" id="fix-{g.key}">{more.join(' ')}</p>
												{/if}
											</div>
										</li>
									{/each}
								</ul>
								{#if fixHidden}<a class="more" href={to(fixHidden.href)}>{fixHidden.text}<Icon name="chevronRight" size={13} /></a>{/if}
							{/if}
						</div>
					</section>
				{/if}
			</div>
		{/if}
		{#if fetchingShown.length}
			<Shelf title="Downloading" subtitle="{fetchingAll.length} in progress" href={to('/downloads')} item="var(--poster-item)">
				{#each fetchingShown as d (d.id)}
					<PosterCard
						title={d.title || d.name}
						href={to(`/downloads?open=${d.id}`)}
						image={d.artwork ?? (d.title_id ? posters.get(d.title_id) : undefined)}
						subtitle={d.covers ? `${d.covers} · ${downloadLine(d)}` : downloadLine(d)}
						progress={d.state === 'downloading' ? Math.round(Math.max(0, Math.min(1, d.progress)) * 100) : undefined}
					/>
				{/each}
			</Shelf>
		{/if}
		{#each rows as row (row)}
			{#if row === 'welcome'}
				{#if owner}
					<section class="setup" aria-labelledby="h-setup" data-testid="home-welcome">
						<div class="setup-main">
							<p class="kicker">{doneCount} of {steps.length} done</p>
							<h2 id="h-setup">Welcome to HolyArrs</h2>
							<div class="segs" aria-hidden="true">{#each steps as s, i (i)}<i class:on={s.done}></i>{/each}</div>
							<h3>{nextStep.text}</h3>
							<p>{nextStep.detail}</p>
							<div class="welcome-acts">
								<Button variant="primary" size="lg" href={to(nextStep.href)} iconAfter="chevronRight">Continue setup</Button>
								<Button variant="secondary" size="lg" href={to('/library/import')}>Import what you have</Button>
							</div>
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
				{:else}
					<section class="prompt" aria-labelledby="h-welcome" data-testid="home-welcome">
						<div>
							<h2 id="h-welcome">Welcome to HolyArrs</h2>
							<p>Nothing here yet. Find a movie, show or artist you love and ask for it. It shows up here when it arrives.</p>
						</div>
						<Button variant="primary" size="lg" icon="compass" href={to('/explore')}>Start asking</Button>
					</section>
				{/if}
			{:else if row === 'arrived'}
				{@render wideRow(home?.arrived ?? [], 'home_arrived', { ...arrivedHead(home?.arrived ?? [], now.getTime()), href: to('/library') })}
			{:else if row === 'approval' && waiting.length}
				<Shelf title="Waiting for approval" subtitle="{waiting.length} {waiting.length === 1 ? 'ask' : 'asks'} on the Requests page" href={to('/requests')} item="var(--poster-item)">
					{#each waiting.slice(0, 20) as a (a.id)}
						<PosterCard title={a.name} href={to(`/requests?ask=${a.id}`)} image={a.poster} subtitle="Asked by {personName(a.asked_by?.name, 'someone in your household')}" owned={askMark(a)} />
					{/each}
				</Shelf>
			{:else if row === 'asks' && mine.length}
				<Shelf title="Your asks" subtitle="{mine.length} {mine.length === 1 ? 'request' : 'requests'} still open" href={to('/requests')} item="var(--poster-item)">
					{#each mineShown as a (a.id)}
						{@const p = a.title_id ? progress[a.title_id] : undefined}
						<PosterCard
							title={a.name}
							href={a.title_id ? to(`/library/${a.title_id}`) : to(`/requests?ask=${a.id}`)}
							image={a.poster}
							subtitle={p ? askProgressLine(p).replace(/^Downloading /, '') : a.season_words || a.album_words || (a.year ? String(a.year) : undefined)}
							progress={p && p.state === 'downloading' ? Math.round(p.progress * 100) : undefined}
							owned={askMark(a)}
						/>
					{/each}
				</Shelf>
			{:else if row === 'new_season'}
				{@render wideRow(home?.new_seasons ?? [], 'home_new_season', { title: 'New season of a show you have' })}
			{:else if row === 'following'}
				{@render wideRow(home?.following ?? [], 'home_following', { title: 'Following', subtitle: 'Shows and artists you follow' })}
			{:else if row === 'soon'}
				{@render wideRow(home?.soon ?? [], 'home_soon', { title: 'Coming soon', subtitle: 'The next 30 days', href: owner ? to('/calendar') : undefined })}
			{/if}
		{/each}
		</PageLoader>
	{/if}
</div>

<style>
	/* The same sizes as Explore: 279px wide cards at a 1536px window (Netflix), 5px gaps on wide rows, one 32px rhythm between blocks. */
	.home { container: home / inline-size; --title-item: max(200px, calc((100vw - 96px) / 5.16)); --poster-item: 168px; --home-gap: 32px; display: flex; flex-direction: column; gap: var(--home-gap); padding-bottom: 48px; }
	.home > :global(.ui-head) { margin-bottom: -8px; }
	/* Phones: Search and Explore sit on their own row from the first frame, so the header does not jump when the greeting arrives. */
	@media (max-width: 767px) { .home > :global(.ui-head .actions) { flex-basis: 100%; } }
	/* High Contrast (forced colors) drops the box-shadow focus rings: draw a system outline instead. */
	@media (forced-colors: active) { .home :global(:focus-visible) { outline: 2px solid Highlight !important; outline-offset: 2px; } }
	.banner { margin-bottom: -16px; }
	.wide-row { display: contents; }
	@media (min-width: 768px) { .wide-row { --shelf-gap: 5px; } }

	.setup { display: grid; grid-template-columns: minmax(0, 1.3fr) minmax(260px, 1fr); gap: 0; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); outline: 1px solid var(--line); overflow: hidden; }
	.setup-main { padding: clamp(20px, 3vw, 32px); display: flex; flex-direction: column; align-items: flex-start; gap: 8px; }
	.kicker { color: var(--text-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.setup h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.setup h3 { font: var(--t-headline); margin-top: 8px; }
	.setup-main > p:not(.kicker) { color: var(--text-muted); max-width: 52ch; margin-bottom: 8px; }
	.segs { display: grid; grid-auto-flow: column; grid-auto-columns: 1fr; gap: 4px; width: min(320px, 100%); margin: 4px 0; }
	.segs i { height: 4px; border-radius: 2px; background: var(--surface-3); }
	.segs i.on { background: var(--ok); }
	.steps { list-style: none; margin: 0; padding: 12px; background: var(--surface-2); display: flex; flex-direction: column; justify-content: center; }
	.steps a { display: flex; align-items: center; gap: 12px; min-height: 48px; padding: 0 10px; border-radius: var(--r-btn); color: var(--text); font: var(--t-callout); font-weight: 500; }
	.steps a:hover { background: var(--surface-3); }
	.steps a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.steps a > :global(.icon) { color: var(--text-muted); }
	.steps .done a { color: var(--text-muted); }
	.dot { width: 26px; height: 26px; flex: none; display: grid; place-items: center; border-radius: 50%; background: var(--surface-1); outline: 1px solid var(--line-strong); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.done .dot { background: var(--ok); color: var(--on-art); outline: none; }
	.grow { flex: 1; min-width: 0; }

	/* The top of Home, Apple TV style: Your Library on the left, Needs attention on the right. Soft lifted
	   surfaces, a big number per card, quiet labels; no hard borders (dark mode keeps the token hairline). */
	.overview { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 28px; align-items: stretch; }
	.overview > section { display: flex; flex-direction: column; min-width: 0; }
	.overview.solo { grid-template-columns: minmax(0, 1fr); }
	.col-head { display: flex; align-items: center; gap: 10px; min-height: 32px; margin-bottom: 14px; }
	.col-head h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.col-link { margin-left: auto; display: inline-flex; align-items: center; gap: 2px; min-height: 32px; padding: 0 4px 0 8px; border-radius: var(--r-sm); color: var(--accent-text); font: var(--t-subhead); font-weight: 600; }
	.col-link:hover { background: var(--accent-soft); }
	.col-link:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	/* Each kind has its own hue: Movies the app's red, TV shows blue, Music indigo, On disk neutral. */
	.k-movie { --kc: var(--accent); }
	.k-show { --kc: var(--info); }
	.k-artist { --kc: var(--mark-dl); }
	.k-disk { --kc: var(--text-muted); }
	.lib-grid { flex: 1; list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); grid-auto-rows: 1fr; gap: 14px; }
	.solo .lib-grid { grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr)); }
	.lib-card {
		--glow: color-mix(in oklab, var(--kc) 16%, transparent);
		position: relative; isolation: isolate; display: flex; flex-direction: column; min-width: 0; padding: 18px 20px 18px;
		border-radius: 22px; overflow: hidden;
		background: radial-gradient(120% 90% at 0% 0%, var(--glow), transparent 62%), var(--surface-1);
		box-shadow: var(--shadow-card), inset 0 1px 0 rgb(255 255 255 / 5%);
		transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out);
		animation: lib-in var(--dur-slow) var(--ease-out) both; animation-delay: calc(var(--i, 0) * var(--stagger) * 2);
	}
	.lib-card.wide { grid-column: 1 / -1; }
	.lib-card:has(.stretch):hover { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift), inset 0 1px 0 rgb(255 255 255 / 6%); }
	.lib-card:has(.stretch:focus-visible) { box-shadow: var(--focus-ring); }
	@keyframes lib-in { from { opacity: 0; transform: translateY(var(--rise)); } }
	.lib-top { display: flex; align-items: center; gap: 10px; margin-bottom: 14px; }
	.lib-icon { width: 36px; height: 36px; flex: none; display: grid; place-items: center; border-radius: 11px; background: color-mix(in oklab, var(--kc) 16%, var(--surface-2)); color: var(--kc); }
	.lib-top h3 { flex: 1; min-width: 0; font: var(--t-headline); color: var(--text); }
	.stretch { color: inherit; outline: none; }
	.stretch::after { content: ''; position: absolute; inset: 0; z-index: 0; border-radius: inherit; }
	.lib-go { color: var(--text-faint); display: inline-grid; transition: transform var(--dur) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.lib-card:hover .lib-go { color: var(--text-muted); transform: translateX(2px); }
	.lib-big { display: flex; align-items: baseline; flex-wrap: wrap; gap: 0 8px; font-variant-numeric: tabular-nums; }
	.lib-big .num { font: 700 44px / 1 var(--font-display); letter-spacing: -0.035em; color: var(--text); }
	.lib-big .unit { font: var(--t-callout); color: var(--text-muted); }
	.lib-detail { margin-top: 6px; font: var(--t-subhead); color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.bar { display: flex; gap: 2px; height: 6px; margin-top: 16px; border-radius: 999px; overflow: hidden; background: var(--surface-3); }
	.bar i { display: block; min-width: 3px; flex-basis: 0; }
	.bar .rest { min-width: 0; background: transparent; }
	.seg.on_disk, .leg.on_disk i { background: var(--ok); }
	.seg.downloading, .leg.downloading i { background: var(--info); }
	.seg.missing, .leg.missing i { background: var(--wait); }
	.seg.k-movie, .seg.k-show, .seg.k-artist, .leg.k-movie i, .leg.k-show i, .leg.k-artist i { background: var(--kc); }
	.legend { list-style: none; margin: 10px 0 0; padding: 0; display: flex; flex-wrap: wrap; gap: 2px 14px; }
	.leg { position: relative; z-index: 1; display: inline-flex; align-items: center; gap: 6px; min-height: 28px; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.leg strong { color: var(--text); font-weight: 600; }
	.leg i { width: 7px; height: 7px; border-radius: 50%; flex: none; }
	a.leg { border-radius: var(--r-sm); margin: 0 -4px; padding: 0 4px; }
	a.leg:hover { color: var(--text); background: var(--surface-2); }
	a.leg:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	/* Needs attention: one lifted card; each cause is one row, title over reason, the action at the right edge. */
	.att-card { flex: 1; display: flex; flex-direction: column; border-radius: 22px; background: var(--surface-1); box-shadow: var(--shadow-card), inset 0 1px 0 rgb(255 255 255 / 5%); padding: 6px; animation: lib-in var(--dur-slow) var(--ease-out) both; }
	.att-card.calm { flex: 1; background: radial-gradient(120% 90% at 100% 0%, color-mix(in oklab, var(--ok) 14%, transparent), transparent 60%), var(--surface-1); }
	.att-wait { display: flex; flex-direction: column; gap: 10px; padding: 18px 16px; }
	.att-wait i { display: block; height: 14px; border-radius: 7px; background: var(--sk-base); }
	.att-wait i:nth-child(2) { width: 72%; }
	.att-wait i:nth-child(3) { width: 48%; }
	.all-good { flex: 1; display: flex; flex-direction: column; align-items: flex-start; justify-content: center; gap: 4px; padding: 22px 20px 24px; }
	.att-sum { display: flex; flex-direction: column; gap: 14px; padding: 12px 14px 14px; }
	.att-icon { width: 36px; height: 36px; display: grid; place-items: center; border-radius: 11px; background: color-mix(in oklab, var(--fail) 16%, var(--surface-2)); color: var(--fail); }
	.att-card .list { box-shadow: inset 0 1px 0 var(--line); margin: 0 6px; }
	.att-card .fix { padding-left: 6px; padding-right: 4px; }
	.good-mark { width: 44px; height: 44px; display: grid; place-items: center; margin-bottom: 10px; border-radius: 50%; background: color-mix(in oklab, var(--ok) 18%, var(--surface-2)); color: var(--ok); }
	.good-title { font: var(--t-title3); letter-spacing: var(--t-title3-track); color: var(--text); }
	.good-line { font: var(--t-callout); color: var(--text-muted); max-width: 34ch; }
	.list { list-style: none; margin: 0; padding: 0; }
	.fix { display: flex; align-items: flex-start; gap: 12px; padding: 10px 12px; border-radius: 14px; }
	.fix + .fix { box-shadow: inset 0 1px 0 var(--line); border-radius: 0; }
	.tone { flex: none; width: 8px; height: 8px; margin-top: 8px; border-radius: 50%; background: var(--text-faint); }
	.tone.fail { background: var(--fail); box-shadow: 0 0 0 4px color-mix(in oklab, var(--fail) 18%, transparent); }
	.tone.wait { background: var(--wait); box-shadow: 0 0 0 4px color-mix(in oklab, var(--wait) 18%, transparent); }
	/* The title and its reason stack; the action sits right of them and wraps under on a narrow column. */
	.body { flex: 1; min-width: 0; display: flex; flex-wrap: wrap; align-items: center; column-gap: 8px; }
	.lead { flex: 1 1 210px; min-width: 0; display: flex; align-items: center; gap: 4px; min-height: 40px; padding: 0; border: 0; background: none; color: var(--text); font: var(--t-callout); text-align: left; }
	.ltxt { min-width: 0; display: flex; flex-direction: column; gap: 1px; }
	.tt { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; }
	.why { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	button.lead { cursor: pointer; border-radius: 8px; }
	button.lead:hover .why { color: var(--text); }
	.caret { flex: none; display: inline-grid; place-items: center; width: 24px; height: 24px; color: var(--text-muted); transition: rotate var(--dur-fast) var(--ease-out); }
	.caret.up { rotate: 180deg; }
	.acts { flex: none; margin-left: auto; display: flex; align-items: center; gap: 2px; }
	.link { display: inline-flex; align-items: center; gap: 2px; min-height: 32px; padding: 0 8px; border: 0; border-radius: 8px; background: none; color: var(--text-muted); font: var(--t-subhead); cursor: pointer; }
	.link:hover { color: var(--text); background: var(--surface-2); }
	.go { color: var(--accent-text); font-weight: 600; }
	.go:hover { color: var(--accent-text); background: var(--accent-soft); }
	.lead:focus-visible, .link:focus-visible, .more:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.raw { flex-basis: 100%; margin: 0 0 8px; color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.members { flex-basis: 100%; list-style: none; margin: 2px 0 6px; padding: 0; }
	.members li { display: flex; flex-wrap: wrap; align-items: center; gap: 0 8px; min-height: 32px; font: var(--t-callout); }
	.members .mt { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.members .link { min-height: 28px; }
	.more { display: flex; align-items: center; gap: 2px; min-height: 44px; margin: 0 6px; padding: 0 6px; box-shadow: inset 0 1px 0 var(--line); color: var(--accent-text); font: var(--t-subhead); font-weight: 600; }
	.more:hover { text-decoration: underline; text-underline-offset: 3px; }

	/* Narrow: the columns stack, Library first. */
	@container home (max-width: 900px) {
		.overview { grid-template-columns: minmax(0, 1fr); gap: 26px; }
	}
	/* Phones: Library cards as a two-column mini grid, a size that fits two numbers side by side. */
	@media (max-width: 599px) {
		.lib-grid, .solo .lib-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
		.lib-card { padding: 14px 14px 12px; border-radius: 18px; }
		.lib-top { margin-bottom: 10px; gap: 8px; }
		.lib-icon { width: 30px; height: 30px; border-radius: 9px; }
		.lib-top h3 { font: var(--t-subhead); font-weight: 600; }
		.lib-go { display: none; }
		.lib-big { flex-direction: column; align-items: flex-start; }
		.lib-big .num { font-size: 32px; }
		.lib-big .unit { font: var(--t-footnote); }
		.lib-detail { font: var(--t-footnote); margin-top: 4px; }
		.bar { margin-top: 12px; height: 5px; }
		.legend { flex-direction: column; gap: 0; margin-top: 6px; }
		/* Plain rows stay compact; the legend links are full phone taps above the card's own link. */
		.leg { min-height: 24px; }
		a.leg { min-height: 36px; }
		.att-card { border-radius: 18px; }
		.att-sum { flex-direction: row; align-items: center; gap: 12px; padding: 10px 10px 12px; }
		.att-icon { width: 30px; height: 30px; border-radius: 9px; }
		.att-sum .lib-big { flex-direction: row; align-items: baseline; }
		.acts { margin-left: -8px; }
		.link, .members .link { min-height: 36px; }
		.members li { min-height: 36px; }
	}

	.prompt { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 16px; padding: clamp(16px, 3vw, 24px); border-radius: var(--r-panel); background: var(--surface-2); }
	.prompt h2 { font: var(--t-title3); }
	.prompt p { color: var(--text-muted); max-width: 56ch; margin-top: 4px; }


	.welcome-acts { display: flex; flex-wrap: wrap; gap: 8px; }

	@media (max-width: 900px) { .setup { grid-template-columns: 1fr; } }
	@media (max-width: 767px) { .home { --home-gap: 26px; --poster-item: 54vw; --title-item: calc((100cqi - var(--shelf-gap)) / 1.6 / .78); } }
</style>

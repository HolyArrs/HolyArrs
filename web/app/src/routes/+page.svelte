<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/api.ts';
	import { afterHandoff } from '#lib/loader.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { askProgressLine, attention, fallbackRows, groupAttention, homeStrip, type Attention, type HomeAnswer, type HomeCard, type HomeRow } from '#lib/home.ts';
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
	// Needs attention is only for failures: failed downloads or imports and health problems.
	const fixes = $derived(owner ? attention({ failed: downloads.filter((d) => d.state === 'failed'), decisions: [], health: admin ? (checks?.checks ?? []) : [] }) : []);
	// Items that share a cause are one row ("2 downloads failed"); six rows show, the rest wait in Downloads.
	const fixRows = $derived(groupAttention(fixes));
	const fixShown = $derived(fixRows.slice(0, 6));
	const fixHidden = $derived(fixRows.slice(6).reduce((n, g) => n + g.items.length, 0));
	let fixOpen = $state<Record<string, boolean>>({});
	/** The plain note and, for admins, the raw text: what the Details link shows. */
	function fixMore(i: Attention): string[] {
		return [i.note, admin ? i.detail : undefined].filter((t): t is string => !!t && t !== i.title);
	}
	const waiting = $derived(asks.filter((a) => a.state === 'asked'));
	const mine = $derived(asks.filter((a) => a.mine && a.state !== 'available').slice(0, 20));
	const progress = $derived(home?.ask_progress ?? {});
	const busy = $derived(Object.keys(progress).length > 0);
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
			manager ? api.get<{ downloads: DownloadRow[] }>('/downloads/view?limit=50') : Promise.resolve(null),
			manager ? api.get<Health>('/health') : Promise.resolve(null),
			admin && manager ? api.get<HealthChecks>('/health/checks') : Promise.resolve(null),
			session.can('ask') || approver ? api.get<{ asks: Ask[] }>('/asks') : Promise.resolve(null)
		]);
		if (fo.status === 'fulfilled' && fo.value) folders = fo.value.library_folders;
		if (dl.status === 'fulfilled' && dl.value) downloads = dl.value.downloads ?? [];
		if (he.status === 'fulfilled' && he.value) health = he.value;
		if (ch.status === 'fulfilled' && ch.value) checks = ch.value;
		if (as.status === 'fulfilled' && as.value) asks = as.value.asks ?? [];
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

	// Live progress on Your asks: poll every 15 s, only while something is downloading.
	$effect(() => {
		if (!busy) return;
		const t = setInterval(loadHome, 15000);
		return () => clearInterval(t);
	});

	onMount(() => {
		void loadAll();
		let timer: ReturnType<typeof setTimeout> | undefined;
		const soonLoad = () => {
			clearTimeout(timer);
			timer = setTimeout(loadAll, 250);
		};
		const offs = [live.on('title', soonLoad), live.on('import', soonLoad), live.on('resync', soonLoad), live.on('health', soonLoad), live.on('download', soonLoad)];
		return () => {
			clearTimeout(timer);
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
			{:else if row === 'attention' && fixes.length}
				<section class="attention" aria-labelledby="h-fix" data-testid="home-attention">
					<header>
						<h2 id="h-fix">Needs attention</h2>
						<span class="count">{fixes.length}</span>
					</header>
					<ul class="list">
						{#each fixShown as g (g.key)}
							{@const many = g.items.length > 1}
							{@const more = many ? [] : fixMore(g.items[0])}
							<li class="fix">
								<span class="dot {g.tone}" aria-hidden="true"></span>
								<div class="body">
									{#if many}
										<button class="lead" type="button" aria-expanded={!!fixOpen[g.key]} aria-controls="fix-{g.key}" onclick={() => (fixOpen[g.key] = !fixOpen[g.key])}>
											<span class="ltxt many"><span class="sr-only">{g.word}: </span><strong class="tt">{g.title}</strong><span class="why">· {g.reason}</span></span>
											<span class="caret" class:up={fixOpen[g.key]}><Icon name="chevronDown" size={14} /></span>
										</button>
									{:else}
										<span class="lead"><span class="ltxt"><span class="sr-only">{g.word}: </span><strong class="tt">{g.title}</strong><span class="why">· {g.reason}</span></span></span>
									{/if}
									<span class="acts">
										{#if more.length}<button class="link" type="button" aria-expanded={!!fixOpen[g.key]} aria-controls="fix-{g.key}" onclick={() => (fixOpen[g.key] = !fixOpen[g.key])}>Details</button>{/if}
										<a class="go link" href={to(g.href)} aria-label="{g.action} ({g.title})">{g.action}<Icon name="chevronRight" size={13} /></a>
									</span>
									{#if many && fixOpen[g.key]}
										<ul class="members" id="fix-{g.key}">
											{#each g.items as it (it.key)}
												{@const sub = fixMore(it)}
												<li>
													<span class="mt">{it.title}</span>
													{#if sub.length}<button class="link" type="button" aria-expanded={!!fixOpen[it.key]} aria-controls="fix-{it.key}" onclick={() => (fixOpen[it.key] = !fixOpen[it.key])}>Details</button>{/if}
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
					{#if fixHidden}<a class="more" href={to('/downloads')}>{fixHidden} more in Downloads</a>{/if}
				</section>
			{:else if row === 'arrived'}
				{@render wideRow(home?.arrived ?? [], 'home_arrived', { title: 'Just arrived', subtitle: 'New in the last two weeks', href: to('/library') })}
			{:else if row === 'approval' && waiting.length}
				<Shelf title="Waiting for approval" subtitle="{waiting.length} {waiting.length === 1 ? 'ask' : 'asks'} on the Wishlist" href={to('/wishlist')} item="var(--poster-item)">
					{#each waiting.slice(0, 20) as a (a.id)}
						<PosterCard title={a.name} href={to('/wishlist')} image={a.poster} subtitle="Asked by {personName(a.asked_by?.name, 'someone in your household')}" owned={askMark(a)} />
					{/each}
				</Shelf>
			{:else if row === 'asks' && mine.length}
				<Shelf title="Your asks" subtitle="{mine.length} on your Wishlist" href={to('/wishlist')} item="var(--poster-item)">
					{#each mine as a (a.id)}
						{@const p = a.title_id ? progress[a.title_id] : undefined}
						<PosterCard
							title={a.name}
							href={a.title_id ? to(`/library/${a.title_id}`) : to('/wishlist')}
							image={a.poster}
							subtitle={p ? askProgressLine(p) : a.season_words || a.album_words || (a.year ? String(a.year) : undefined)}
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
			{:else if row === 'discover'}
				<a class="discover" href={to('/explore')}>Find more to ask for <span aria-hidden="true">→</span></a>
			{/if}
		{/each}
		</PageLoader>
	{/if}
</div>

<style>
	/* The same sizes as Explore: 279px wide cards at a 1536px window (Netflix), 5px gaps on wide rows, one 32px rhythm between blocks. */
	.home { container: home / inline-size; --title-item: max(200px, calc((100vw - 96px) / 5.16)); --poster-item: 168px; --home-gap: 32px; display: flex; flex-direction: column; gap: var(--home-gap); padding-bottom: 48px; }
	.home > :global(.ui-head) { margin-bottom: -8px; }
	.banner { margin-bottom: -16px; }
	.wide-row { display: contents; }
	@media (min-width: 768px) { .wide-row { --shelf-gap: 5px; } }

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
	.done .dot { background: var(--ok); color: var(--on-art); outline: none; }
	.grow { flex: 1; min-width: 0; }

	.attention header { display: flex; align-items: baseline; justify-content: flex-start; gap: 12px; margin-bottom: 12px; }
	.attention h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.count { min-width: 24px; height: 24px; padding: 0 8px; display: inline-grid; place-items: center; border-radius: 999px; background: var(--accent-soft); color: var(--accent-text); font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	/* Management widgets: calm grouped lists, a hairline and no lift, like Settings. One 40px row per cause: a dot, "Title · why", Details and the action at the right edge. */
	.attention .list { list-style: none; margin: 0; padding: 2px 6px; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: inset 0 0 0 1px var(--line); }
	.fix { display: flex; align-items: flex-start; gap: 10px; padding: 0 8px; border-radius: var(--r-btn); }
	.fix + .fix { box-shadow: inset 0 1px 0 var(--line); border-radius: 0; }
	.dot { flex: none; width: 8px; height: 8px; margin-top: 16px; border-radius: 50%; background: var(--text-faint); }
	.dot.fail { background: var(--fail); }
	.dot.wait { background: var(--wait); }
	/* The title and the action share a line; when they do not fit (a phone), the action wraps under the title instead of squeezing it. */
	.body { flex: 1; min-width: 0; display: flex; flex-wrap: wrap; align-items: center; column-gap: 12px; }
	.lead { flex: 1 1 auto; min-width: 0; max-width: 100%; display: flex; align-items: center; gap: 2px; height: 40px; padding: 0; border: 0; background: none; color: var(--text); font: var(--t-callout); text-align: left; }
	/* A lone item cuts its long title short and keeps the reason; a merged row keeps its lead and cuts the list of titles. */
	.ltxt { min-width: 0; display: flex; gap: 0.4em; white-space: nowrap; }
	.tt { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
	.why { flex: none; }
	.many .tt { flex: none; }
	.many .why { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
	button.lead { cursor: pointer; border-radius: 6px; }
	button.lead:hover .why { color: var(--text); }
	.lead strong { font-weight: 600; }
	.why { color: var(--text-muted); }
	.caret { flex: none; display: inline-grid; place-items: center; width: 24px; height: 24px; color: var(--text-muted); transition: rotate var(--dur-fast) var(--ease-out); }
	.caret.up { rotate: 180deg; }
	.acts { flex: none; margin-left: auto; display: flex; align-items: center; gap: 4px; }
	.link { display: inline-flex; align-items: center; gap: 2px; min-height: 32px; padding: 0 6px; border: 0; border-radius: 6px; background: none; color: var(--text-muted); font: var(--t-subhead); cursor: pointer; }
	.link:hover { color: var(--text); background: var(--surface-2); }
	.go { color: var(--accent-text); font-weight: 600; }
	.go:hover { color: var(--accent-text); }
	.lead:focus-visible, .link:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.raw { flex-basis: 100%; margin: 0 0 8px; color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.members { flex-basis: 100%; list-style: none; margin: 0 0 6px; padding: 0 0 0 2px; }
	.members li { display: flex; flex-wrap: wrap; align-items: center; gap: 0 8px; min-height: 32px; font: var(--t-callout); }
	.members .mt { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.members .link { min-height: 28px; }
	@media (max-width: 599px) {
		.acts { margin-left: -6px; }
		.link { min-height: 36px; padding: 0 8px; }
		.members li { min-height: 36px; }
	}
	.more { display: inline-flex; align-items: center; min-height: 44px; color: var(--accent-text); font: var(--t-callout); font-weight: 500; }
	.more:hover { text-decoration: underline; text-underline-offset: 3px; }

	.prompt { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 16px; padding: clamp(16px, 3vw, 24px); border-radius: var(--r-panel); background: var(--surface-2); }
	.prompt h2 { font: var(--t-title3); }
	.prompt p { color: var(--text-muted); max-width: 56ch; margin-top: 4px; }


	.welcome-acts { display: flex; flex-wrap: wrap; gap: 8px; }
	.discover:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.discover { align-self: center; display: inline-flex; align-items: center; gap: 6px; min-height: 44px; padding: 0 12px; border-radius: var(--r-btn); color: var(--accent-text); font: var(--t-headline); }
	.discover:hover { text-decoration: underline; text-underline-offset: 3px; }

	@media (max-width: 900px) { .setup { grid-template-columns: 1fr; } }
	@media (max-width: 767px) { .home { --home-gap: 26px; --poster-item: 54vw; --title-item: calc((100cqi - var(--shelf-gap)) / 1.6 / .78); } }
</style>

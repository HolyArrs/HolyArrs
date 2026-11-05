<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/api.ts';
	import { afterHandoff } from '#lib/loader.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { stageOf } from '#lib/downloads/status-tabs.ts';
	import type { IconName } from '#lib/Icon.svelte';
	import { arrivedHead, askProgressLine, attention, downloadLine, downloadMoved, downloadingNow, fallbackRows, freeSpace, groupAttention, homeFailures, homeStrip, libraryCards, moreLink, n, partCount, uncheckedLine, type HomeAnswer, type HomeCard, type HomeRow } from '#lib/home.ts';
	import { decisions } from '#lib/decisions.svelte.ts';
	import { METADATA_DOWN, metadataOutage } from '#lib/metadata-health.svelte.ts';
	import { bytes, duration, percent } from '#lib/format.ts';
	import { homeFacts, homeGreeting, homeNews, localDay } from '#lib/home-line.ts';
	import { personName } from '#lib/plain.ts';
	import type { Fresh } from '#lib/fresh.ts';
	import type { DownloadRow } from '#lib/downloaders.ts';
	import type { Ask, Health, HealthChecks, LibraryFolder } from '#lib/types.ts';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
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
	// The Needs attention tile opens where the problems are: the one cause's own page, or the page that holds most of them.
	const fixHref = $derived(fixRows.length === 1 ? fixRows[0].href : (moreLink(fixRows, admin)?.href ?? (admin ? '/settings/health' : '/status')));
	// Your Library: one card per kind that is on, and for owners what it all takes on disk.
	const libCards = $derived(libraryCards(home?.library, manager ? '/downloads' : undefined));
	const diskTotal = $derived(libCards.reduce((t, c) => t + c.size, 0));
	const showDisk = $derived(owner && libCards.length > 0);
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
	// The top of Home: a greeting that shows on the first frame, and a short note beside the switch once the data has answered.
	const kid = $derived(home?.kid ?? session.kid);
	const greeting = $derived(homeGreeting({ name: session.s.name, now }));
	const news = $derived(settled ? homeNews({ now, kid, facts: homeFacts({ home, asks, downloads, approver, manager, kid, now }) }) : '');

	// Bento: the Home grid (pictures and counts) and, for people who manage the Library, a System grid of the same size.
	let board = $state<'home' | 'system'>('home');
	const showSystem = $derived(manager && !kid);
	const shown = $derived(showSystem ? board : 'home');
	// Pictures for the big, tall and wide tiles: new seasons, arrivals, coming soon and follows, taken in turn, each title once.
	const pics = $derived.by(() => {
		const lists: [HomeCard[], string][] = [
			[home?.new_seasons ?? [], 'New season'],
			[home?.arrived ?? [], 'Just arrived'],
			[home?.soon ?? [], 'Coming soon'],
			[home?.following ?? [], 'Following']
		];
		const seen = new Set<number>();
		const out: { c: HomeCard; label: string }[] = [];
		for (let i = 0; out.length < 3 && lists.some(([l]) => i < l.length); i++) {
			for (const [l, label] of lists) {
				const c = l[i];
				if (c && (c.backdrop || c.poster) && !seen.has(c.title_id) && out.length < 3) {
					seen.add(c.title_id);
					out.push({ c, label });
				}
			}
		}
		return out;
	});
	const showOverview = $derived(!rows.includes('welcome') && (libCards.length > 0 || owner || pics.length > 0));
	// Home always fills its seven places. Each count tile below is used when it has something to show, in this order;
	// Explore and Search always can, so a place whose own tile is empty takes the next one and the grid never has a hole.
	const newCount = $derived((home?.arrived?.length ?? 0) + (home?.new_seasons?.length ?? 0));
	const statPool = $derived(
		[
			owner ? 'att' : '',
			(approver && !kid && waiting.length) || mine.length ? 'ask' : '',
			libCards.length ? 'lib' : '',
			showDisk ? 'disk' : '',
			fetchingAll.length ? 'dl' : '',
			owner && home?.soon?.length && !pics.some((p) => p.label === 'Coming soon') ? 'soon' : '',
			newCount ? 'new' : '',
			'explore',
			'search'
		].filter(Boolean)
	);
	const slots = $derived.by(() => {
		const pool = [...statPool];
		const s = [pool.shift(), pool.shift(), pool.shift(), pool.shift()];
		return { s, tall: pics[1] ? undefined : pool.shift(), wide: pics[2] ? undefined : pool.shift(), big: pics[0] ? undefined : pool.shift() };
	});
	const libTotal = $derived(libCards.reduce((t, c) => t + c.big, 0));
	const arrivedCapped = $derived((home?.arrived?.length ?? 0) >= 20);
	const arrivedToday = $derived((home?.arrived ?? []).filter((c) => c.at && localDay(new Date(c.at)) === localDay(now)).length);
	const waitingPics = $derived(waiting.filter((a) => a.poster).slice(0, 3));
	// System numbers, from what this page already read.
	const speed = $derived(fetchingAll.reduce((t, d) => t + (d.speed || 0), 0));
	const queued = $derived(fetchingAll.filter((d) => stageOf(d) === 'queued').length);
	const running = $derived(fetchingAll.length - queued);
	const srcs = $derived(partCount(health?.sources));
	const dls = $derived(partCount(health?.downloaders));
	const free = $derived(freeSpace(folders));
	const sysOk = $derived(settled && !fixes.length && !unchecked);
	const sysNote = $derived(
		!settled ? '' : (unchecked && !fixes.length ? 'Could not check everything' : fixes.length ? `${n(fixes.length)} ${fixes.length === 1 ? 'thing needs' : 'things need'} a look` : 'All good') + (running ? ` · ${n(running)} downloading${speed ? ` at ${bytes(speed)}/s` : ''}` : '')
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
	<h1 class="sr-only">Home</h1>
	<header class="top">
		<p class="greet">{greeting}</p>
		<div class="top-row">
			{#if showSystem && showOverview && loaded}
				<SegmentedControl options={[{ value: 'home', label: 'Home', icon: 'home' }, { value: 'system', label: 'System', icon: 'gauge' }]} bind:value={board} label="Show on Home" testid="home-switch" announce={(l) => `Showing ${l}`} />
			{/if}
			<p class="note" class:pending={!settled}>
				{#if shown === 'system'}{#if sysNote}<span class="dot" class:ok={sysOk} class:warn={settled && !sysOk} aria-hidden="true"></span>{sysNote}{/if}{:else}{news}{/if}
			</p>
			<div class="acts">
				<span class="wide-only"><Button variant="secondary" icon="search" href={to('/search')}>Search</Button><Button variant="secondary" icon="compass" href={to('/explore')}>Explore</Button></span>
				<span class="phone-only"><IconButton icon="search" label="Search" variant="tinted" size="lg" href={to('/search')} tooltip={false} /><IconButton icon="compass" label="Explore" variant="tinted" size="lg" href={to('/explore')} tooltip={false} /></span>
			</div>
		</div>
	</header>

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

		<!-- One picture tile: the big and wide ones use the wide picture (with the title logo), the tall one the poster. -->
		{#snippet picTile(p: { c: HomeCard; label: string }, cls: string, tall: boolean)}
			<a class="tile pic {cls}" href={to(p.c.href)}>
				<img src={tall ? p.c.poster || p.c.backdrop : p.c.backdrop || p.c.poster} alt="" loading="lazy" decoding="async" />
				<span class="lab">{p.label}</span>
				{#if !tall && p.c.backdrop && p.c.logo && p.c.kind !== 'artist'}
					<img class="logo" src={p.c.logo} alt={p.c.name} loading="lazy" decoding="async" />
				{:else}
					<span class="nm">{p.c.name}</span>
				{/if}
				{#if p.c.line}<span class="sm">{p.c.line}</span>{/if}
			</a>
		{/snippet}

		<PageLoader loading={!loaded} label="Loading Home" onretry={retry}>
		{#if showOverview}
			<div class="boards" data-testid="home-overview">
				<!-- A tile's head: its coloured icon chip, its name, and an arrow when it opens a page. -->
				{#snippet head(icon: IconName, label: string, go = true, extra = '')}
					<span class="head"><span class="kchip" aria-hidden="true"><Icon name={icon} size={18} /></span><span class="lab">{label}</span>{#if extra}<em>{extra}</em>{/if}{#if go}<span class="go" aria-hidden="true"><Icon name="chevronRight" size={16} /></span>{/if}</span>
				{/snippet}
				<!-- One count tile. Home always fills every slot: a slot whose own tile has nothing to show takes the next one from the list. -->
				{#snippet stat(key: string, cls: string)}
					{#if key === 'att' && settled && unchecked && !fixes.length}
						<button type="button" class="tile stat {cls}" style="--kc: var(--wait)" onclick={retry} aria-busy={retrying || undefined} data-testid="home-attention">
							{@render head('refresh', 'Needs attention', false)}
							<span class="word">Could not check</span>
							<span class="sm">{uncheckedLine(uncheckedDl, uncheckedHealth)} Tap to try again.</span>
						</button>
					{:else if key === 'att'}
						<a class="tile stat {cls}" style="--kc: var({sysOk ? '--ok' : '--fail'})" href={to(fixHref)} data-testid="home-attention">
							{@render head(sysOk ? 'check' : 'alert', 'Needs attention')}
							{#if !settled}
								<span class="wait" aria-hidden="true"><i></i><i></i></span><span class="sr-only">Checking</span>
							{:else if fixes.length}
								<span class="num fail">{n(fixes.length)}</span>
								<span class="sm">{fixRows[0].title}</span>
							{:else if unchecked}
								<span class="word">Could not check</span>
								<span class="sm">{uncheckedLine(uncheckedDl, uncheckedHealth)}</span>
							{:else}
								<span class="word ok">All good</span>
								<span class="sm">Downloads, imports and checks</span>
							{/if}
						</a>
					{:else if key === 'ask' && approver && !kid && waiting.length}
						<a class="tile stat {cls}" style="--kc: var(--wait)" href={to('/requests')}>
							{@render head('requests', 'To approve')}
							<span class="ask-stack" aria-hidden="true">{#each waitingPics as a (a.id)}<img src={a.poster} alt="" loading="lazy" onerror={(e) => ((e.currentTarget as HTMLImageElement).hidden = true)} />{/each}{#if waiting.length > waitingPics.length}<b>+{n(waiting.length - waitingPics.length)}</b>{/if}</span>
							<span class="sm">{n(waiting.length)} {waiting.length === 1 ? 'ask' : 'asks'} to look at</span>
						</a>
					{:else if key === 'ask'}
						<a class="tile stat {cls}" style="--kc: var(--wait)" href={to('/requests')}>
							{@render head('requests', 'Your asks')}
							<span class="num">{n(mine.length)}</span>
							<span class="sm">still open</span>
						</a>
					{:else if key === 'lib'}
						<a class="tile stat {cls}" style="--kc: var(--accent)" href={to('/library')} data-testid="home-library">
							{@render head('library', 'Library')}
							<span class="num">{n(libTotal)}</span>
							<span class="sm">{libCards.map((c) => `${n(c.big)} ${c.unit}`).join(' · ')}</span>
						</a>
					{:else if key === 'disk'}
						<a class="tile stat {cls}" style="--kc: var(--ok)" href={to(admin ? '/settings/library-folders' : '/library')}>
							{@render head('archive', 'On disk')}
							<span class="num">{bytes(diskTotal)}</span>
							<span class="sm">{free ? `${bytes(free.free)} free${free.many ? ' on the fullest disk' : ''}` : 'Across your Library folders'}</span>
							{#if free}<span class="meter" aria-hidden="true"><i style="width: {Math.max(2, (diskTotal / Math.max(1, diskTotal + free.free)) * 100)}%"></i></span>{/if}
						</a>
					{:else if key === 'dl'}
						<a class="tile stat {cls}" style="--kc: var(--info)" href={to('/downloads')}>
							{@render head('download', 'Downloading')}
							<span class="num">{n(fetchingAll.length)}</span>
							<span class="sm">{speed ? `${bytes(speed)}/s right now` : 'in progress'}</span>
						</a>
					{:else if key === 'soon'}
						<a class="tile stat {cls}" style="--kc: var(--info)" href={to('/calendar')}>
							{@render head('calendar', 'Coming soon')}
							<span class="num">{n(home?.soon?.length ?? 0)}</span>
							<span class="sm">in the next 30 days</span>
						</a>
					{:else if key === 'new'}
						<a class="tile stat {cls}" style="--kc: var(--mark-dl)" href={to('/library')}>
							{@render head('sparkle', 'New for you')}
							<span class="num">{n(newCount)}</span>
							<span class="sm">arrivals and new seasons</span>
						</a>
					{:else if key === 'explore'}
						<a class="tile stat {cls}" style="--kc: var(--mark-dl)" href={to('/explore')}>
							{@render head('compass', 'Explore')}
							<span class="word">Find something new</span>
							<span class="sm">Movies, shows and music to ask for</span>
						</a>
					{:else}
						<a class="tile stat {cls}" style="--kc: var(--text-muted)" href={to('/search')}>
							{@render head('search', 'Search')}
							<span class="word">Look anything up</span>
							<span class="sm">Any movie, show or artist</span>
						</a>
					{/if}
				{/snippet}

				<div class="bento" class:gone={shown !== 'home'} inert={shown !== 'home'} data-testid="home-board">
					{#if pics[0]}{@render picTile(pics[0], 'big', false)}{:else if slots.big}{@render stat(slots.big, 'big')}{/if}
					{#if slots.s[0]}{@render stat(slots.s[0], '')}{/if}
					{#if slots.s[1]}{@render stat(slots.s[1], '')}{/if}
					{#if pics[1]}{@render picTile(pics[1], 'tall', true)}{:else if slots.tall}{@render stat(slots.tall, 'tall')}{/if}
					{#if slots.s[2]}{@render stat(slots.s[2], '')}{/if}
					{#if pics[2]}{@render picTile(pics[2], 'wide', false)}{:else if slots.wide}{@render stat(slots.wide, 'wide')}{/if}
					{#if slots.s[3]}{@render stat(slots.s[3], 'fill')}{/if}
				</div>
				{#if showSystem}
					<div class="bento" class:gone={shown !== 'system'} inert={shown !== 'system'} data-testid="home-system">
						<a class="tile stat big dl" style="--kc: var(--info)" href={to('/downloads')}>
							{@render head('download', 'Downloading')}
							{#if speed}
								<span class="num">{bytes(speed).split(' ')[0]}<small>{bytes(speed).split(' ')[1]}/s</small></span>
							{:else}
								<span class="num">{n(fetchingAll.length)}<small>{fetchingAll.length === 1 ? 'download' : 'downloads'}</small></span>
							{/if}
							<span class="sm">{n(running)} running · {n(queued)} waiting</span>
							{#if fetchingAll.length}
								<ul class="dl-list">
									{#each fetchingAll.slice(0, 4) as d (d.id)}
										<li><b>{d.title || d.name}</b><span>{Math.round(percent(d.progress))}%{d.speed ? ` · ${bytes(d.speed)}/s` : ''}</span><i class="pb"><i style="width: {percent(d.progress)}%"></i></i></li>
									{/each}
								</ul>
							{:else}
								<span class="quiet"><span class="kchip" aria-hidden="true"><Icon name="download" size={26} /></span>Nothing is downloading right now.</span>
							{/if}
						</a>
						<a class="tile stat wide" style="--kc: var(--ok)" href={to(admin ? '/settings/library-folders' : '/library')}>
							{@render head('archive', 'Disk', true, free ? `${bytes(free.free)} free` : '')}
							<span class="num">{bytes(diskTotal)}<small>used by your Library</small></span>
							<span class="split" aria-hidden="true">
								{#each libCards as c (c.kind)}{#if c.size}<i class="k-{c.kind}" style="flex-grow: {c.size}"></i>{/if}{/each}
								<i class="rest" style="flex-grow: {free?.free ?? 0}"></i>
							</span>
							<span class="legend">{#each libCards as c (c.kind)}<span><i class="k-{c.kind}"></i>{c.label} {bytes(c.size)}</span>{/each}</span>
						</a>
						<a class="tile stat" style="--kc: var({srcs.on && srcs.working < srcs.on ? '--fail' : '--ok'})" href={to(admin ? '/settings/sources' : '/status')}>
							{@render head('source', 'Sources')}
							<span class="num">{n(srcs.working)}<small>of {n(srcs.on)} working</small></span>
							<span class="sm">{srcs.off ? `${n(srcs.off)} turned off` : srcs.on ? 'All turned on' : 'None added yet'}</span>
							<span class="pips" aria-hidden="true">{#each Array(Math.max(1, srcs.on)) as _, i (i)}<i class:off={i >= srcs.working} class:none={!srcs.on}></i>{/each}</span>
						</a>
						<a class="tile stat" style="--kc: var({dls.on && dls.working < dls.on ? '--fail' : '--ok'})" href={to(admin ? '/settings/downloaders' : '/status')}>
							{@render head('downloader', 'Downloaders')}
							<span class="num">{n(dls.working)}<small>of {n(dls.on)} working</small></span>
							<span class="sm">{(health?.downloaders ?? []).map((d) => d.name).join(' · ') || 'None added yet'}</span>
							<span class="pips" aria-hidden="true">{#each Array(Math.max(1, dls.on)) as _, i (i)}<i class:off={i >= dls.working} class:none={!dls.on}></i>{/each}</span>
						</a>
						<a class="tile stat" style="--kc: var({sysOk ? '--ok' : '--fail'})" href={to(fixHref)}>
							{@render head(sysOk ? 'check' : 'alert', 'Checks')}
							{#if !settled}
								<span class="wait" aria-hidden="true"><i></i><i></i></span><span class="sr-only">Checking</span>
							{:else if fixes.length}
								<span class="num fail">{n(fixes.length)}</span><span class="sm">{fixes.length === 1 ? 'thing needs' : 'things need'} a look</span>
							{:else if unchecked}
								<span class="word">Could not check</span>
							{:else}
								<span class="word ok">All good</span><span class="sm">Nothing needs you</span>
							{/if}
						</a>
						<a class="tile stat" style="--kc: var({metadataOutage.s.down ? '--fail' : '--mark-dl'})" href={to('/status')}>
							{@render head('cloudOff', 'Metadata server')}
							<span class="word" class:fail={metadataOutage.s.down}>{metadataOutage.s.down ? 'Down' : 'Online'}</span>
							<span class="sm">{metadataOutage.s.down ? 'New details wait until it is back' : 'Pictures and details come in'}</span>
						</a>
						<a class="tile stat" style="--kc: var(--accent)" href={to('/library')}>
							{@render head('sparkle', 'Arrived today')}
							<span class="num">{n(arrivedToday)}{arrivedCapped && arrivedToday >= 20 ? '+' : ''}</span>
							<span class="sm">{arrivedToday === 1 ? 'title' : 'titles'} in the Library</span>
						</a>
						<a class="tile stat" style="--kc: var(--info)" href={to('/status')}>
							{@render head('clock', 'Running for')}
							<span class="word">{health ? duration(health.uptime_s) : '—'}</span>
							<span class="sm">{health ? `Version ${health.version}` : 'Could not read'}</span>
						</a>
					</div>
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
	/* High Contrast (forced colors) drops the box-shadow focus rings: draw a system outline instead. */
	@media (forced-colors: active) { .home :global(:focus-visible) { outline: 2px solid Highlight !important; outline-offset: 2px; } }
	.banner { margin-bottom: -16px; }
	.wide-row { display: contents; }
	@media (min-width: 768px) { .wide-row { --shelf-gap: 5px; } }

	/* The top: the greeting, then one line with the Home | System switch, the note beside it, and Search and Explore. */
	.top { display: flex; flex-direction: column; gap: 14px; margin-bottom: -8px; }
	.greet { font: var(--t-large-title); letter-spacing: var(--t-large-title-track); text-wrap: balance; }
	.top-row { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px; min-height: 44px; }
	.top-row :global(.ui-seg button) { padding: 0 16px; }
	.note { min-width: 0; flex: 1 1 240px; display: flex; align-items: center; gap: 8px; color: var(--text-muted); font: var(--t-callout); }
	.note .dot { width: 8px; height: 8px; flex: none; border-radius: 50%; background: var(--text-faint); }
	.note .dot.ok { background: var(--ok); }
	.note .dot.warn { background: var(--fail); }
	.acts { margin-left: auto; display: flex; }
	.wide-only { display: flex; gap: 8px; }
	.phone-only { display: none; gap: 8px; }

	/* Bento: one grid of tiles. Home: the big picture, Needs attention, approvals, a tall picture, Library, a wide picture, disk.
	   System: the same frame filled with how HolyArrs is running. Both grids fill the same rows, so switching moves nothing below. */
	.boards { position: relative; }
	.boards > .bento { transition: opacity var(--dur) var(--ease-out), translate var(--dur) var(--ease-out); }
	.bento.gone { position: absolute; inset: 0 0 auto; opacity: 0; translate: 0 6px; visibility: hidden; pointer-events: none; transition: opacity var(--dur) var(--ease-out), translate var(--dur) var(--ease-out), visibility 0s linear var(--dur); }
	.bento { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); grid-auto-rows: 150px; grid-auto-flow: row dense; gap: 14px; }
	.tile { position: relative; isolation: isolate; min-width: 0; min-height: 0; display: flex; flex-direction: column; padding: 16px 18px; border-radius: 22px; overflow: hidden; color: var(--text); background: var(--surface-1); box-shadow: var(--shadow-card), inset 0 1px 0 rgb(255 255 255 / 5%); outline: 1px solid var(--line); outline-offset: -1px; transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	a.tile:hover { transform: translateY(var(--lift)); box-shadow: var(--shadow-lift), inset 0 1px 0 rgb(255 255 255 / 6%); }
	a.tile:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tile.big { grid-column: span 2; grid-row: span 2; }
	.tile.wide { grid-column: span 2; }
	.tile.tall { grid-row: span 2; }
	/* Count tiles: each has its own colour (--kc) for the icon chip, a soft wash from the top corner and the arrow. */
	.stat { --kc: var(--text-muted); background: radial-gradient(140% 110% at 0% 0%, color-mix(in oklab, var(--kc) 17%, transparent), transparent 62%), color-mix(in oklab, var(--kc) 4%, var(--surface-1)); outline-color: color-mix(in oklab, var(--kc) 16%, var(--line)); }
	.head { display: flex; align-items: center; gap: 10px; min-width: 0; }
	.kchip { width: 34px; height: 34px; flex: none; display: grid; place-items: center; border-radius: 11px; background: color-mix(in oklab, var(--kc) 18%, var(--surface-2)); color: var(--kc); }
	.lab { flex: 1; min-width: 0; font: var(--t-callout); font-weight: 600; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.head em { font-style: normal; font: var(--t-footnote); color: var(--text-muted); white-space: nowrap; }
	.go { flex: none; display: inline-grid; color: var(--text-faint); transition: transform var(--dur) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	a.tile:hover .go { color: var(--kc); transform: translateX(2px); }
	.meter { display: block; height: 6px; margin-top: 10px; border-radius: 999px; background: color-mix(in oklab, var(--kc) 14%, var(--surface-3)); overflow: hidden; }
	.meter i { display: block; height: 100%; border-radius: inherit; background: var(--kc); }
	.tile .big, .tile .num, .tile .word { margin-top: auto; }
	.num { font: 700 36px / 1.05 var(--font-display); letter-spacing: -0.03em; font-variant-numeric: tabular-nums; }
	.num small { margin-left: 4px; font: var(--t-subhead); font-weight: 500; letter-spacing: 0; color: var(--text-muted); }
	.word { display: inline-flex; align-items: center; gap: 6px; font: 700 26px / 1.1 var(--font-display); letter-spacing: -0.02em; }
	.fail { color: var(--fail); }
	.ok { color: var(--ok); }
	.sm { margin-top: 2px; font: var(--t-footnote); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.calm { background: radial-gradient(120% 90% at 100% 0%, color-mix(in oklab, var(--ok) 14%, transparent), transparent 60%), var(--surface-1); }
	.hot { background: radial-gradient(120% 90% at 100% 0%, color-mix(in oklab, var(--fail) 14%, transparent), transparent 60%), var(--surface-1); }
	.wait { margin-top: auto; display: flex; flex-direction: column; gap: 8px; }
	.wait i { display: block; height: 12px; border-radius: 6px; background: var(--sk-base); }
	.wait i + i { width: 60%; }

	/* Picture tiles: the picture fills the tile, the words sit on a dark fade at the bottom (white in both themes). */
	.pic { justify-content: flex-end; color: #fff; background: var(--surface-3); outline: 0; }
	.pic > img:first-child { position: absolute; inset: 0; z-index: -2; width: 100%; height: 100%; object-fit: cover; }
	.pic::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(0deg, rgb(0 0 0 / 76%), transparent 58%); }
	.pic .lab { flex: none; font: var(--t-subhead); font-weight: 600; color: rgb(255 255 255 / 84%); }
	.pic .nm { font: 700 22px / 1.15 var(--font-display); letter-spacing: -0.01em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pic.big .nm { font-size: 30px; letter-spacing: -0.02em; }
	.pic .logo { width: auto; height: auto; max-width: 55%; max-height: 72px; margin: 4px 0 2px; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 2px 10px rgb(0 0 0 / 50%)); }
	.pic.big .logo { max-height: 104px; }
	.pic .sm { color: rgb(255 255 255 / 84%); }

	.ask-stack { margin-top: auto; display: flex; align-items: flex-end; }
	.ask-stack img { width: 44px; aspect-ratio: 2 / 3; object-fit: cover; border-radius: 7px; margin-right: -12px; box-shadow: 0 0 0 2px var(--surface-1); }
	.ask-stack b { margin-left: 20px; font: var(--t-footnote); font-weight: 600; color: var(--text-muted); }

	/* System tiles */
	.dl .num { margin-top: 10px; }
	.dl-list { list-style: none; margin: auto 0 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
	.dl-list li { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 2px 10px; font: var(--t-footnote); }
	.dl-list b { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.dl-list span { color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.pb { grid-column: 1 / -1; display: block; height: 4px; border-radius: 999px; background: var(--surface-3); overflow: hidden; }
	.pb i { display: block; height: 100%; border-radius: inherit; background: var(--info); }
	.quiet { margin: auto 0; align-self: center; display: flex; flex-direction: column; align-items: center; gap: 10px; text-align: center; font: var(--t-callout); color: var(--text-muted); }
	.quiet .kchip { width: 56px; height: 56px; border-radius: 18px; }
	.split { display: flex; gap: 2px; height: 8px; margin-top: 10px; border-radius: 999px; overflow: hidden; background: var(--surface-3); }
	.split i { display: block; min-width: 3px; flex-basis: 0; }
	.split .rest { min-width: 0; background: transparent; }
	.legend { display: flex; flex-wrap: wrap; gap: 2px 12px; margin-top: 8px; font: var(--t-footnote); color: var(--text-muted); }
	.legend span { display: inline-flex; align-items: center; gap: 5px; white-space: nowrap; }
	.legend i { width: 7px; height: 7px; border-radius: 50%; }
	.k-movie { background: var(--accent); }
	.k-show { background: var(--info); }
	.k-artist { background: var(--mark-dl); }
	.pips { display: flex; gap: 4px; margin-top: 8px; }
	.pips i { flex: 1; height: 6px; border-radius: 999px; background: var(--ok); }
	.stat .pips { margin-top: 10px; }
	.pips i.off { background: var(--fail); }
	.pips i.none { background: var(--surface-3); }

	/* Narrow: two columns; the big tiles keep their two-by-two, the tall one becomes one row. */
	@container home (max-width: 900px) {
		.bento { grid-template-columns: repeat(2, minmax(0, 1fr)); grid-auto-rows: 132px; gap: 10px; }
		.tile.tall { grid-row: span 1; }
		.tile.fill { grid-column: span 2; }
	}
	/* Phones: Search and Explore become two round buttons beside the switch; the note goes under them. */
	@media (max-width: 767px) {
		.home { --home-gap: 26px; --poster-item: 54vw; --title-item: calc((100cqi - var(--shelf-gap)) / 1.6 / .78); }
		.greet { font-size: 28px; }
		.wide-only { display: none; }
		.phone-only { display: flex; }
		.top-row :global(.ui-seg) { flex: 1; }
		.top-row :global(.ui-seg button) { min-height: 44px; }
		.note { order: 3; flex-basis: 100%; font: var(--t-subhead); }
		.note:empty:not(.pending) { display: none; }
		.note.pending { min-height: 1.3em; }
		.kchip { width: 30px; height: 30px; border-radius: 9px; }
		.head { gap: 8px; }
		.lab { font: var(--t-subhead); font-weight: 600; }
		.go { display: none; }
		.ask-stack img { width: 32px; margin-right: -10px; border-radius: 6px; }
		.ask-stack b { margin-left: 16px; }
		.tile { padding: 12px 14px; border-radius: 18px; }
		.num { font-size: 28px; }
		.word { font-size: 20px; }
		.pic .nm { font-size: 18px; }
		.pic.big .nm { font-size: 24px; }
		.pic.big .logo { max-height: 72px; }
		.lab em { display: none; }
		.sm { white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; line-height: 1.3; }
	}

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
	.steps .dot { width: 26px; height: 26px; flex: none; display: grid; place-items: center; border-radius: 50%; background: var(--surface-1); outline: 1px solid var(--line-strong); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.steps .done .dot { background: var(--ok); color: var(--on-art); outline: none; }
	.grow { flex: 1; min-width: 0; }

	.prompt { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 16px; padding: clamp(16px, 3vw, 24px); border-radius: var(--r-panel); background: var(--surface-2); }
	.prompt h2 { font: var(--t-title3); }
	.prompt p { color: var(--text-muted); max-width: 56ch; margin-top: 4px; }

	.welcome-acts { display: flex; flex-wrap: wrap; gap: 8px; }

	@media (max-width: 900px) { .setup { grid-template-columns: 1fr; } }
</style>

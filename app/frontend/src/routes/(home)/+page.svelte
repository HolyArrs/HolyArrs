<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import HomeOverview from './HomeOverview.svelte';
	import { createHomeAttention } from './home-attention.svelte.ts';
	import { createHomeOverview } from './home-overview.svelte.ts';
	import { createHomeDownloads } from './home-downloads.svelte.ts';
	import { createHomeDisk } from './home-disk.svelte.ts';
	import type { Fresh } from '#lib/ui/fresh.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import ArtPill from '#lib/ui/ArtPill.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { artGone, lightOnDark } from './art-check.ts';
	import { arrivedHead, attention, downloadMoved, downloadingNow, fallbackRows, homeDownloadsURL, homeStrip, n, type HomeAnswer, type HomeCard, type HomeRow } from './home.ts';
	import { MetadataNotice } from '#lib/metadata/index.ts';
	import { homeFacts, homeGreeting, homeNews } from './home-line.ts';
	import type { Health, HealthChecks } from '#lib/health/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { partOff } from '#lib/mediafiles/index.ts';
	import { api } from '#lib/core/api.ts';
	import { afterHandoff } from '#lib/core/loader.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { proxied, sized } from '#lib/core/tmdb-image.ts';
	import { inbox } from '#lib/core/inbox.svelte.ts';
	import type { DownloadRow } from '#lib/core/types.ts';
	import type { Ask, LibraryFolder } from '#lib/core/types.ts';

	// Home is "my stuff": the server sends the row order per person (GET /home); every row hides when it has nothing.
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
	// The Sources and Downloaders read (GET /health) failed: their tiles say so and do not show zeros.
	let uncheckedParts = $state(false);
	// The asks read failed: the asks tile says so and does not claim nothing is waiting.
	let uncheckedAsks = $state(false);
	// The Library folders read failed: the disk tile says it could not read the free room.
	let uncheckedFolders = $state(false);

	const admin = $derived(session.can('settings'));
	const manager = $derived(session.can('manage_library'));
	const approver = $derived(session.can('approve'));
	const owner = $derived(home ? home.owner : manager);

	const rows = $derived<HomeRow[]>(home?.rows ?? (loaded ? fallbackRows(owner) : []));
	// Sources and Downloaders, from GET /health and the health checks. A failing Source is paused by the server and its check warns; a Downloader carries its own state. Only "disabled" is turned off.
	const attentionCtx = {
		get checks() { return checks; },
		get health() { return health; },
		get settled() { return settled; },
		get uncheckedParts() { return uncheckedParts; },
		get folders() { return folders; },
		get owner() { return owner; },
		get rows() { return rows; },
		get admin() { return admin; },
		get downloads() { return downloads; },
		get manager() { return manager; },
		get uncheckedDl() { return uncheckedDl; },
		get uncheckedHealth() { return uncheckedHealth; },
		get home() { return home; },
		get asks() { return asks; },
	};
	const attentionView = createHomeAttention(attentionCtx);
	// A turned-down ask is not open: it is counted apart.
	const mineOpen = $derived(attentionView.mine.filter((a) => a.state !== 'turned_down').length);
	const mineDeclined = $derived(attentionView.mine.length - mineOpen);
	const asksLine = $derived([mineOpen ? t('home.stillOpen', { n: n(mineOpen) }) : '', mineDeclined ? t('home.turnedDown', { n: n(mineDeclined) }) : ''].filter(Boolean).join(' · '));
	const canAsk = $derived((session.can('ask') || approver) && !partOff('wishlist'));
	const progress = $derived(home?.ask_progress ?? {});
	// While one of this person's own asks is downloading, Home re-reads, so it sees the title arrive.
	const busy = $derived(attentionView.mineShown.some((a) => !!a.title_id && !!progress[a.title_id]));
	// Downloading (managers): what HolyArrs fetches now, from the downloads this page already read. It shows on the System side.
	const fetchingAll = $derived(manager ? downloadingNow(downloads, Infinity) : []);
	// The top of Home: a greeting that shows on the first frame, and a short note beside the switch once the data has answered.
	const kid = $derived(home?.kid ?? session.kid);
	const greeting = $derived(homeGreeting({ name: session.s.name, now }));
	const news = $derived(settled ? homeNews({ now, kid, facts: homeFacts({ home, asks, downloads, approver, manager, kid, now }) }) : '');

	// Bento: the Home grid (pictures and counts) and, for people who manage the Library, a System grid of the same size.
	const overviewCtx = {
		get manager() { return manager; },
		get kid() { return kid; },
		get home() { return home; },
		get now() { return now; },
		get rows() { return rows; },
		get owner() { return owner; },
		get canAsk() { return canAsk; },
		get libCards() { return attentionView.libCards; },
		get showDisk() { return attentionView.showDisk; },
		get fetchingAll() { return fetchingAll; },
		get waiting() { return attentionView.waiting; },
	};
	const overview = createHomeOverview(overviewCtx);
	const downloadingCtx = {
		get waiting() { return attentionView.waiting; },
		askPic: overview.askPic,
		get fetchingAll() { return fetchingAll; },
		get manager() { return manager; },
		get downloads() { return downloads; },
	};
	const downloading = createHomeDownloads(downloadingCtx);
	// Disk: how much room is left, and whether the disk space check is failing (then a reading of 0 is a full disk).
	const diskCtx = {
		get checks() { return checks; },
		get folders() { return folders; },
		get uncheckedFolders() { return uncheckedFolders; },
		get diskTotal() { return attentionView.diskTotal; },
		get settled() { return settled; },
		get worst() { return attentionView.worst; },
		get fixes() { return attentionView.fixes; },
		get unchecked() { return attentionView.unchecked; },
		get uncheckedDl() { return uncheckedDl; },
		get dlCount() { return downloading.dlCount; },
		get speed() { return downloading.speed; },
		stageWords: downloading.stageWords,
	};
	const disk = createHomeDisk(diskCtx);
	function strip(c: HomeCard, type: string): Fresh | null {
		const own = homeStrip(c);
		if (own || !c.line || type === 'home_arrived') return own;
		return { kind: type === 'home_soon' ? 'coming' : 'new_season', text: serverWords(c.line) };
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
		// The first screen (the Home answer) loads now; the bigger asks wait until the sign-in hand-off has left, so its frames stay smooth.
		const first = loadHome().then(() => {
			if (home) loaded = true;
		});
		await afterHandoff();
		const [, fo, dl, he, ch, as] = await Promise.allSettled([
			first,
			manager ? api.get<{ library_folders: LibraryFolder[] }>('/library-folders') : Promise.resolve(null),
			// Members may only read the Library and their own asks; the rest needs manage_library.
			manager ? api.get<{ downloads: DownloadRow[] }>(homeDownloadsURL) : Promise.resolve(null),
			manager ? api.get<Health>('/health') : Promise.resolve(null),
			// The checks need manage_library only; people without Settings get Status links instead of Fix.
			manager ? api.get<HealthChecks>('/health/checks') : Promise.resolve(null),
			canAsk ? api.get<{ asks: Ask[] }>('/asks') : Promise.resolve(null)
		]);
		if (fo.status === 'fulfilled' && fo.value) folders = fo.value.library_folders;
		if (dl.status === 'fulfilled' && dl.value) downloads = dl.value.downloads ?? [];
		if (he.status === 'fulfilled' && he.value) health = he.value;
		if (ch.status === 'fulfilled' && ch.value) checks = ch.value;
		if (as.status === 'fulfilled' && as.value) asks = as.value.asks ?? [];
		uncheckedDl = dl.status === 'rejected';
		uncheckedHealth = ch.status === 'rejected';
		uncheckedParts = he.status === 'rejected';
		uncheckedAsks = as.status === 'rejected';
		uncheckedFolders = fo.status === 'rejected';
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

	// Members get no live title or import news. When their download ends, Home reads everything once; while an ask of theirs is approved and waiting for a copy, it reads again once a minute (in view only).
	let wasBusy = false;
	$effect(() => {
		const b = busy;
		if (wasBusy && !b && !manager) void loadAll();
		wasBusy = b;
	});
	// Members get no live stream: when the bell's minute check brings a new message (approved, declined, ready), Home reads again.
	let lastMsg: number | undefined;
	$effect(() => {
		const top = inbox.messages.length ? Math.max(...inbox.messages.map((m) => m.id)) : undefined;
		if (!manager && lastMsg !== undefined && top !== undefined && top !== lastMsg) void loadAll();
		lastMsg = top;
	});
	const waitingCopy = $derived(!manager && !busy && attentionView.mine.some((a) => a.state === 'approved'));
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

<svelte:head><title>{t('home.home')} · HolyArrs</title></svelte:head>

<div class="home">
	<div class="top">
		<p class="greet" data-testid="home-greeting">{greeting}</p>
		<!-- The page's own heading, for screen readers and for the small title bar phones show once the greeting has scrolled away. -->
		<PageHeader title={t('home.home')} />
		<div class="top-row">
			{#if overview.showSystem && overview.showOverview && loaded && !(failed && !home)}
				<SegmentedControl options={[{ value: 'home', label: t('home.home'), icon: 'home' }, { value: 'system', label: t('home.system'), icon: 'gauge' }]} bind:value={overview.board} label={t('home.showOnHome')} testid="home-switch" announce={(l) => t('home.showing', { name: l })} />
			{/if}
			<p class="note" class:pending={!settled} class:hold={overview.showSystem && overview.showOverview}>
				{#if failed && !home}{:else if overview.shown === 'system'}{#if disk.sysNote}<span class="dot {attentionView.worst}" aria-hidden="true"></span>{disk.sysNote}{/if}{:else}{news}{/if}
			</p>
			<div class="acts">
				<span class="wide-only"><Button variant="secondary" icon="search" href={to('/search')}>{t('home.search')}</Button><Button variant="secondary" icon="compass" href={to('/explore')}>{t('home.explore')}</Button></span>
				<span class="phone-only"><IconButton icon="search" label={t('home.search')} variant="tinted" size="lg" href={to('/search')} tooltip={false} /><IconButton icon="compass" label={t('home.explore')} variant="tinted" size="lg" href={to('/explore')} tooltip={false} /></span>
			</div>
		</div>
	</div>

	{#if failed && !home && loaded}
		<LoadError hasData={false} message={t('home.couldNotLoadHome')} onretry={retry} busy={retrying} />
	{:else}
		{#if failed}<div class="banner"><LoadError hasData={true} message={t('home.couldNotRefreshHome')} onretry={retry} busy={retrying} /></div>{/if}
		<MetadataNotice />

		{#snippet wideRow(cards: HomeCard[], type: string, opts: { title: string; subtitle?: string; href?: string })}
			{#if cards.length}
				<div class="wide-row" data-testid={type}>
					<Shelf title={opts.title} subtitle={opts.subtitle} href={opts.href} item="var(--title-item)">
						{#each cards as c (c.key)}
							<!-- Every card is 16:9; an artist shows its wide picture with the new album's cover inset. -->
							<WideCard cover title={c.name} kind={c.kind} typed row={type} href={to(c.href)} image={c.backdrop || c.poster || undefined} logo={c.kind === 'artist' || !(c.backdrop || c.poster) ? null : c.logo} inset={c.kind === 'artist' ? c.albums?.[0]?.cover : undefined} subtitle={serverWords(c.line)} fresh={strip(c, type)} />
						{/each}
					</Shelf>
				</div>
			{/if}
		{/snippet}

		<!-- One picture tile: the big and wide ones use the wide picture (with the title logo), the tall one the poster. -->
		{#snippet picTile(p: { c: HomeCard; label: string }, cls: string, tall: boolean)}
			<a class="tile pic {cls}" href={to(p.c.href)}>
				<!-- Through HolyArrs' own artwork cache, like every other card: the browser never calls the picture site itself. -->
				<img src={tall ? sized(p.c.poster || p.c.backdrop, 'poster', 400) : sized(p.c.backdrop || p.c.poster, cls === 'big' ? 'large' : 'backdrop')} alt="" decoding="async" use:artGone={() => overview.badPic.add(p.c.title_id)} />
				<span class="pill"><ArtPill text={p.label} icon={downloading.pillIcon[p.label]} /></span>
				{#if !tall && p.c.backdrop && p.c.logo && p.c.kind !== 'artist' && !overview.badLogo.has(p.c.title_id)}
					<img class="logo" src={proxied(p.c.logo)} alt={p.c.name} decoding="async" use:lightOnDark use:artGone={() => overview.badLogo.add(p.c.title_id)} />
				{:else}
					<span class="nm" dir="auto">{p.c.name}</span>
				{/if}
				{#if p.c.line}<span class="sm">{serverWords(p.c.line)}</span>{/if}
			</a>
		{/snippet}

		<PageLoader loading={!loaded} label={t('home.loadingHome')} onretry={retry}>
		<HomeOverview {overview} {disk} {admin} {settled} {attentionView} {uncheckedParts} {retrying} {retry} {approver} {kid} {uncheckedAsks} {downloading} {mineOpen} {mineDeclined} {fetchingAll} {home} {uncheckedDl} {now} {uncheckedHealth} {health} {picTile} />
		{#each rows as row (row)}
			{#if row === 'welcome'}
				{#if owner}
					<section class="setup" aria-labelledby="h-setup" data-testid="home-welcome">
						<div class="setup-main">
							<p class="kicker">{#if settled}{t('home.ofDone', { done: attentionView.doneCount, total: attentionView.steps.length })}{:else}&nbsp;{/if}</p>
							<h2 id="h-setup">{t('home.welcomeToHolyarrs')}</h2>
							<div class="segs" aria-hidden="true">{#each attentionView.steps as s, i (i)}<i class:on={settled && s.done}></i>{/each}</div>
							{#if settled}
								<h3>{attentionView.nextStep.text}</h3>
								<p>{attentionView.nextStep.detail}</p>
							{:else}
								<span class="sk setup-wait" aria-hidden="true"><i></i><i></i></span><span class="sr-only">{t('home.loading')}</span>
							{/if}
							<div class="welcome-acts" class:hide={!settled}>
								<Button variant="primary" size="lg" href={to(attentionView.nextStep.href)} iconAfter="chevronRight">{attentionView.nextStep.cta}</Button>
								<Button variant="secondary" size="lg" href={to('/library/import')}>{t('home.importWhatYouHave')}</Button>
							</div>
						</div>
						<ol class="steps">
							{#each attentionView.steps as s, i (i)}
								<li class:done={settled && s.done}>
									<a href={to(s.href)}>
										<span class="dot">{#if settled && s.done}<Icon name="check" size={14} />{:else}{i + 1}{/if}</span>
										<span class="grow">{s.text}</span>
										<span class="sr-only">{!settled ? '' : s.done ? t('home.done') : t('home.toDo')}</span>
										<Icon name="chevronRight" size={15} />
									</a>
								</li>
							{/each}
						</ol>
					</section>
				{:else}
					<section class="prompt" aria-labelledby="h-welcome" data-testid="home-welcome">
						<div>
							<h2 id="h-welcome">{t('home.welcomeToHolyarrs')}</h2>
							<p>{t('home.nothingHereYetAsk')}</p>
						</div>
						<Button variant="primary" size="lg" icon="compass" href={to('/explore')}>{t('home.startAsking')}</Button>
					</section>
				{/if}
			{:else if row === 'arrived'}
				{@render wideRow(overview.below(home?.arrived), 'home_arrived', { ...arrivedHead(home?.arrived ?? [], now.getTime()), href: to(overview.kindHref(home?.arrived?.[0])) })}
			{:else if row === overview.upAt && overview.comingUp.length}
				<section class="up" aria-labelledby="h-up" data-testid="home_coming_up">
					<header class="up-head">
						<h2 id="h-up">{t('home.comingUp')}</h2>
						{#if owner}<a class="up-more" href={to('/calendar')}>{t('home.calendar')}<Icon name="chevronRight" size={14} /></a>{/if}
					</header>
					<ul class="up-list">
						{#each overview.comingUp as u (u.key)}
							{@const art = u.c.backdrop || u.c.poster}
							<li>
								<a class="up-line" href={to(u.c.href)}>
									<span class="up-th" aria-hidden="true">
										{#if art && !overview.badThumb.has(u.key)}<img src={sized(art, 'backdrop', 130)} alt="" loading="lazy" decoding="async" use:artGone={() => overview.badThumb.add(u.key)} />{:else}<Icon name={overview.kindIcon(u.c.kind)} size={16} />{/if}
									</span>
									<span class="up-t"><b dir="auto">{u.c.name}</b><span>{u.sub}</span></span>
									<span class="up-when" class:now={u.now}>{u.when}</span>
								</a>
							</li>
						{/each}
					</ul>
				</section>
			{/if}
		{/each}
		</PageLoader>
	{/if}
</div>

<style>
	/* The same sizes as Explore: 279px wide cards at a 1536px window, 5px gaps on wide rows, one 32px rhythm between blocks. The sheet colours live here so the tiles and the Welcome card share them. */
	.home {
		container: home / inline-size; --title-item: max(200px, calc((100vw - 96px) / 5.16)); --poster-item: 168px; --home-gap: 32px; display: flex; flex-direction: column; gap: var(--home-gap); padding-bottom: 48px;
		--sheet-a: light-dark(#ffffff, #212126); --sheet-b: light-dark(#f4f5f8, #151518);
		--edge: inset 0 1px 0 light-dark(rgb(255 255 255 / 95%), rgb(255 255 255 / 10%)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 9%), rgb(255 255 255 / 6%));
		--drop: 0 1px 2px light-dark(rgb(20 24 40 / 5%), rgb(0 0 0 / 40%)), 0 18px 32px -20px light-dark(rgb(20 24 40 / 30%), rgb(0 0 0 / 85%));
		--well: light-dark(rgb(20 24 40 / 4%), rgb(255 255 255 / 5%));
		--track: light-dark(rgb(20 24 40 / 9%), rgb(255 255 255 / 12%));
	}
	.banner { margin-bottom: -16px; }
	.wide-row { display: contents; }
	@media (min-width: 768px) { .wide-row { --shelf-gap: 5px; }
}

	.top { display: flex; flex-direction: column; gap: 14px; margin-bottom: -8px; }
	.top :global(.ui-head) { position: absolute; width: 1px; height: 1px; margin: 0; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
	.greet { font: var(--t-large-title); letter-spacing: var(--t-large-title-track); text-wrap: balance; overflow-wrap: anywhere; }
	.top-row { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px; min-height: 44px; }
	.top-row :global(.ui-seg button) { padding: 0 16px; }
	.note { min-width: 0; flex: 1 1 120px; color: var(--text-muted); font: var(--t-callout); line-height: 1.35; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
	.note .dot { display: inline-block; width: 8px; height: 8px; margin-right: 8px; vertical-align: 1px; border-radius: 50%; background: var(--text-faint); }
	.note .dot.ok { background: var(--ok); }
	.note .dot.wait { background: var(--wait); }
	.note .dot.fail { background: var(--fail); }
	.acts { margin-left: auto; display: flex; }
	.wide-only { display: flex; gap: 8px; }
	.phone-only { display: none; gap: 8px; }

	/* The tile is the Requests card's sheet on its own: lit from above, a hairline, and one long soft shadow. A whisper of grain keeps the flat colour from looking printed. */
	.tile {
		position: relative; isolation: isolate; min-width: 0; min-height: 0; display: flex; flex-direction: column; padding: 16px 18px; border: 0; border-radius: 20px; overflow: hidden;
		color: var(--text); text-align: left; font: inherit; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b)); box-shadow: var(--edge), var(--drop);
		transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out), opacity var(--dur-fast) var(--ease-out);
	}
	a.tile { cursor: pointer; }
	.tile.wide { grid-column: span 2; }
	.tile.tall { grid-row: span 2; }
	.tile.banner { grid-column: 1 / -1; }
	.sm { margin-top: 5px; font: var(--t-footnote); color: var(--text-muted); overflow: hidden; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
	.sm.two > span { display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sk { margin-top: auto; display: flex; flex-direction: column; gap: 8px; }
	.sk i { display: block; height: 12px; border-radius: 6px; background: var(--sk-base); }
	.sk i + i { width: 60%; }

	/* Picture tiles: the picture fills the tile; a glass pill says what it is and the title sits on a dark fade at the foot (white in both themes). */
	.pic { justify-content: flex-end; color: #fff; background: #17181d; box-shadow: var(--drop); }
	.pic > img:first-child { position: absolute; inset: 0; z-index: -2; width: 100%; height: 100%; object-fit: cover; object-position: center 28%; transition: transform var(--dur-slow) var(--ease-out); }
	a.pic:hover > img:first-child { transform: scale(1.03); }
	.pic::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(0deg, rgb(0 0 0 / 80%), rgb(0 0 0 / 30%) 38%, transparent 62%); }
	/* The bright edge sits over the picture. */
	.pic::before { content: ''; position: absolute; inset: 0; z-index: 1; border-radius: inherit; pointer-events: none; box-shadow: inset 0 1px 0 rgb(255 255 255 / 22%), inset 0 0 0 1px rgb(255 255 255 / 10%); }
	.pill { position: absolute; top: 12px; left: 12px; display: flex; }
	.pic .nm { font: 800 22px / 1.1 var(--font-display); letter-spacing: -0.02em; overflow: hidden; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
	.pic.big .nm { font-size: 32px; letter-spacing: -0.03em; }
	.pic .logo { width: auto; height: auto; max-width: 55%; max-height: 72px; margin: 4px 0 2px; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 2px 10px rgb(0 0 0 / 50%)); }
	.pic.big .logo { max-height: 104px; }
	/* A dark wordmark (black letters made for light posters) is drawn white, its shape unchanged, like on the title page. */
	.pic .logo:global(.dark) { filter: brightness(0) invert(1) drop-shadow(0 2px 10px rgb(0 0 0 / 50%)); }
	.pic .sm { color: rgb(255 255 255 / 86%); font: 600 12px / 1.3 var(--font-ui); -webkit-line-clamp: 1; line-clamp: 1; }
	.ask-stack img { width: 38px; height: auto; aspect-ratio: 2 / 3; object-fit: cover; border-radius: 7px; margin-left: -12px; background: var(--surface-3); box-shadow: 0 0 0 2.5px var(--sheet-b), 0 6px 14px -4px rgb(0 0 0 / 35%); }
	.ask-stack img:first-child { margin-left: 0; }
	.flow span { font: var(--t-footnote); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.dl-list span { color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.ft > span:last-child { font: var(--t-footnote); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	/* Two columns (a narrow window, a tablet, a phone): the big tiles keep their two-by-two, the tall one becomes one row, an odd last tile spans the row, and everything inside a tile takes its compact size (the rows are 132px here). */
	@container home (max-width: 900px) {
		.tile.tall { grid-row: span 1; }
		.tile.fill { grid-column: span 2; }
		.tile { padding: 12px 14px; border-radius: 18px; }
		.pill { top: 10px; left: 10px; }
		.pic .nm { font-size: 18px; }
		.pic.big .nm { font-size: 26px; }
		.pic.big .logo { max-height: 72px; }
		.sm { line-height: 1.3; }
		.ask-stack img { width: 32px; margin-left: -10px; border-radius: 6px; }
}
	/* Phones: the big tiles have less room, so two downloads show and the problem rows keep only their arrow. */
	@container home (max-width: 560px) {
}
	/* A small phone (360): the tile names still fit. */
	@container home (max-width: 340px) {
		.tile { padding-inline: 12px; }
}
	/* Phones: Search and Explore become two round buttons beside the switch; the note goes under them and keeps its line when there is a switch, so Home and System start at the same height. */
	@media (max-width: 767px) {
		.home { --home-gap: 26px; --poster-item: 54vw; --title-item: calc((100cqi - var(--shelf-gap)) / 1.6 / .78); }
		.greet { font-size: 28px; }
		.wide-only { display: none; }
		.phone-only { display: flex; }
		.top-row :global(.ui-seg) { flex: 1; }
		.top-row :global(.ui-seg button) { min-height: 44px; }
		.note { order: 3; flex-basis: 100%; font: var(--t-subhead); min-height: 1.35em; }
		.note:empty:not(.pending):not(.hold) { display: none; }
}

	.up { display: flex; flex-direction: column; gap: 12px; }
	.up-head { display: flex; align-items: baseline; gap: 12px; }
	.up-head h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.up-more { position: relative; margin-left: auto; display: inline-flex; align-items: center; gap: 2px; min-height: 32px; color: var(--accent-text); font: var(--t-subhead); font-weight: 600; border-radius: 8px; }
	.up-more::after { content: ''; position: absolute; inset: -6px 0; }
	.up-more:hover { text-decoration: underline; text-underline-offset: 3px; }
	.up-list { list-style: none; margin: 0; padding: 6px; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); column-gap: 6px; border-radius: 20px; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b)); box-shadow: var(--edge), var(--drop); }
	.up-list li { min-width: 0; }
	.up-list li:nth-child(n + 3) { box-shadow: inset 0 1px 0 var(--line); }
	/* An odd last line takes the whole width, so the sheet has no empty half. */
	.up-list li:last-child:nth-child(odd) { grid-column: 1 / -1; }
	.up-line { display: flex; align-items: center; gap: 12px; min-height: 58px; padding: 0 10px; border-radius: 14px; color: inherit; transition: background var(--dur-fast) var(--ease-out); }
	.up-line:hover { background: var(--well); }
	.up-line:focus-visible, .up-more:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.up-th { flex: none; width: 68px; aspect-ratio: 16 / 9; display: grid; place-items: center; border-radius: 8px; overflow: hidden; background: var(--surface-3); color: var(--text-faint); box-shadow: 0 0 0 1px var(--line); }
	.up-th img { width: 100%; height: 100%; object-fit: cover; object-position: center 28%; }
	.up-t { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
	.up-t b { font: 600 14px / 1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.up-t span { font: var(--t-footnote); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.up-when { flex: none; font: 600 12px / 1.2 var(--font-ui); color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.up-when.now { color: var(--accent-text); }
	@container home (max-width: 700px) {
		.up-list { grid-template-columns: minmax(0, 1fr); border-radius: 18px; }
		.up-list li:nth-child(n + 2) { box-shadow: inset 0 1px 0 var(--line); }
		.up-list li:nth-child(n + 5) { display: none; }
		.up-th { width: 60px; }
}

	/* The Welcome card and the first-ask prompt are the same sheet as the tiles. */
	.setup { display: grid; grid-template-columns: minmax(0, 1.3fr) minmax(260px, 1fr); gap: 0; border-radius: 20px; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b)); box-shadow: var(--edge), var(--drop); overflow: hidden; }
	.setup-main { padding: clamp(20px, 3vw, 32px); display: flex; flex-direction: column; align-items: flex-start; gap: 8px; }
	.kicker { color: var(--text-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.setup h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.setup h3 { font: var(--t-headline); margin-top: 8px; }
	.setup-main > p:not(.kicker) { color: var(--text-muted); max-width: 52ch; margin-bottom: 8px; }
	.setup-wait { margin: 12px 0 14px; width: min(320px, 100%); }
	.segs { display: grid; grid-auto-flow: column; grid-auto-columns: 1fr; gap: 4px; width: min(320px, 100%); margin: 4px 0; }
	.segs i { height: 4px; border-radius: 2px; background: var(--track); }
	.segs i.on { background: var(--ok); }
	.steps { list-style: none; margin: 0; padding: 12px; background: var(--well); box-shadow: inset 1px 0 0 var(--line); display: flex; flex-direction: column; justify-content: center; }
	.steps a { display: flex; align-items: center; gap: 12px; min-height: 48px; padding: 0 10px; border-radius: var(--r-btn); color: var(--text); font: var(--t-callout); font-weight: 500; }
	.steps a:hover { background: var(--well); }
	.steps a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.steps a > :global(.icon) { color: var(--text-muted); }
	.steps .done a { color: var(--text-muted); }
	.steps .dot { width: 26px; height: 26px; flex: none; display: grid; place-items: center; border-radius: 50%; background: var(--surface-1); outline: 1px solid var(--line-strong); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.steps .done .dot { background: var(--ok); color: var(--on-art); outline: none; }
	.grow { flex: 1; min-width: 0; }

	.prompt { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 16px; padding: clamp(18px, 3vw, 28px); border-radius: 20px; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b)); box-shadow: var(--edge), var(--drop); }
	.prompt h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	.prompt p { color: var(--text-muted); max-width: 56ch; margin-top: 4px; }

	.welcome-acts { display: flex; flex-wrap: wrap; gap: 8px; }
	.welcome-acts.hide { visibility: hidden; }

	@media (max-width: 900px) { .setup { grid-template-columns: 1fr; } .steps { box-shadow: inset 0 1px 0 var(--line); }
}

	/* High Contrast (forced colors) drops shadows and fills: tiles, the steps box and the ticks get a system border, focus gets a system outline, and the small coloured marks (dots, pips, the foot line, the count) keep their colours. */
	@media (forced-colors: active) {
		.home :global(:focus-visible) { outline: 2px solid Highlight !important; outline-offset: 2px; }
		.tile, .setup, .prompt, .up-list { border: 1px solid CanvasText; }
		.note .dot { forced-color-adjust: none; }
}
</style>

<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/api.ts';
	import { afterHandoff } from '#lib/loader.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { stageOf } from '#lib/downloads/status-tabs.ts';
	import { SvelteSet } from 'svelte/reactivity';
	import { artGone, lightOnDark } from '#lib/art-check.ts';
	import { proxied, sized } from '#lib/tmdb-image.ts';
	import { askArtist, askDetails, wantDetails } from '#lib/approvals/details.svelte.ts';
	import type { IconName } from '#lib/Icon.svelte';
	import { arrivedHead, askProgressLine, attention, downloadLine, downloadMoved, downloadingNow, fallbackRows, freeSpace, arrivedTitle, groupAttention, homeFailures, homeStrip, libraryCards, moreLink, n, partCount, shownState, uncheckedLine, type Attention, type AttentionGroup, type HomeAnswer, type HomeCard, type HomeRow } from '#lib/home.ts';
	import { decisions } from '#lib/decisions.svelte.ts';
	import { METADATA_DOWN, metadataOutage } from '#lib/metadata-health.svelte.ts';
	import { agoLong, bytes, duration, percent } from '#lib/format.ts';
	import { homeFacts, homeGreeting, homeNews, localDay } from '#lib/home-line.ts';
	import { personName } from '#lib/plain.ts';
	import type { Fresh } from '#lib/fresh.ts';
	import type { DownloadRow } from '#lib/downloaders.ts';
	import type { Ask, Health, HealthChecks, LibraryFolder } from '#lib/types.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import SettingsTile from '#lib/ui/SettingsTile.svelte';
	import ArtPill from '#lib/ui/ArtPill.svelte';
	import type { Tint } from '#lib/settings.ts';
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
	// The Sources and Downloaders read (GET /health) failed: their tiles say so and do not show zeros.
	let uncheckedParts = $state(false);
	// The asks read failed: the asks tile says so and does not claim nothing is waiting.
	let uncheckedAsks = $state(false);

	const admin = $derived(session.can('settings'));
	const manager = $derived(session.can('manage_library'));
	const approver = $derived(session.can('approve'));
	const owner = $derived(home ? home.owner : manager);

	const rows = $derived<HomeRow[]>(home?.rows ?? (loaded ? fallbackRows(owner) : []));
	// Sources and Downloaders, from GET /health and the health checks. A failing Source is paused by the server and its
	// check warns; a Downloader carries its own state. Only "disabled" is turned off.
	const failingChecks = (kind: string) => (checks?.checks ?? []).filter((c) => c.kind === kind && shownState(c) !== 'ok');
	const srcs = $derived(partCount(health?.sources, failingChecks('source').length));
	const dls = $derived(partCount(health?.downloaders, failingChecks('downloader').length));
	const partsRead = $derived(settled && !!health && !uncheckedParts);
	const steps = $derived([
		{ done: folders.length > 0, text: 'Choose a Library folder', detail: 'Tell HolyArrs where finished media should live.', href: '/settings/library-folders' },
		{ done: srcs.total > 0, text: 'Add a Source', detail: 'Connect a place where HolyArrs can find Releases.', href: '/settings/sources' },
		{ done: dls.total > 0, text: 'Add a Downloader', detail: 'Connect the app that fetches chosen Releases.', href: '/settings/downloaders' },
		{ done: false, text: 'Add your first title', detail: 'Search for a new movie, show or artist to fetch, or import the folders you already have on disk.', href: '/search' }
	]);
	const doneCount = $derived(steps.filter((s) => s.done).length);
	const nextStep = $derived(steps.find((s) => !s.done) ?? steps[3]);

	/** Which parts have a problem, in one short line: the name when it is one ("SABnzbd has a problem"), else how many. Needs attention names them all. */
	function troubled(kind: 'source' | 'downloader', parts: { name: string; state: string }[] | null | undefined): string {
		const names = new Set<string>();
		for (const p of parts ?? []) if (!['ok', 'working', 'enabled', 'disabled'].includes(p.state) && !/ \(removed \d+\)$/.test(p.name)) names.add(p.name);
		for (const c of failingChecks(kind)) if (c.subject) names.add(c.subject);
		const list = [...names];
		if (!list.length) return '';
		return list.length === 1 ? `${list[0]} has a problem` : `${n(list.length)} have a problem`;
	}
	const partLine = (p: { total: number; off: number }, trouble: string) => trouble || (p.off ? `${n(p.off)} turned off` : 'All working');
	// Nothing can be found or fetched without a Source and a Downloader that are on. The health checks only cover the
	// ones that exist, so Home adds these itself once setup is over (the Welcome card covers it before that).
	const setupGaps = $derived.by((): Attention[] => {
		if (!owner || !partsRead || rows.includes('welcome')) return [];
		const gap = (key: string, title: string, reason: string, path: string, action: string): Attention => ({ key, tone: 'wait', word: 'Warning', text: `${title}: ${reason}.`, title, reason, cause: `setup:${key}`, href: admin ? path : '/status', action: admin ? action : 'Look' });
		const out: Attention[] = [];
		if (!srcs.total) out.push(gap('no-sources', 'Sources', 'none added yet', '/settings/sources', 'Add'));
		else if (!srcs.on) out.push(gap('sources-off', 'Sources', 'all turned off', '/settings/sources', 'Fix'));
		if (!dls.total) out.push(gap('no-downloaders', 'Downloaders', 'none added yet', '/settings/downloaders', 'Add'));
		else if (!dls.on) out.push(gap('downloaders-off', 'Downloaders', 'all turned off', '/settings/downloaders', 'Fix'));
		// The health checks run every few minutes; /health is read now. A part it already reports as failing gets its row at once.
		if (srcs.failing && !failingChecks('source').length) out.push(gap('sources-trouble', srcs.failing === 1 ? 'Source' : `${n(srcs.failing)} Sources`, srcs.failing === 1 ? 'has a problem' : 'have a problem', '/settings/sources', 'Fix'));
		if (dls.failing && !failingChecks('downloader').length) out.push(gap('downloaders-trouble', dls.failing === 1 ? 'Downloader' : `${n(dls.failing)} Downloaders`, dls.failing === 1 ? 'has a problem' : 'have a problem', '/settings/downloaders', 'Fix'));
		return out;
	});
	// Needs attention is what needs you: failed downloads or imports (the same rule as Downloads), decisions,
	// health problems and a metadata outage while it lasts. Decisions come from the list the layout keeps fresh.
	const fixes = $derived(
		owner
			? [
					...attention({
						failed: homeFailures(downloads),
						decisions: manager ? decisions.items : [],
						health: checks?.checks ?? [],
						metadata: metadataOutage.s.down ? METADATA_DOWN : undefined,
						admin
					}),
					...setupGaps
				]
			: []
	);
	const unchecked = $derived(owner && (uncheckedDl || uncheckedHealth || uncheckedParts));
	// How bad it is, for the colour: a failure is red, a warning (an update, a low disk) is amber.
	const worst = $derived(fixes.some((f) => f.tone === 'fail') ? 'fail' : fixes.length || unchecked ? 'wait' : 'ok');
	const attTint = $derived<Tint>(!settled ? 'gray' : worst === 'fail' ? 'red' : worst === 'wait' ? 'amber' : 'green');
	// Items that share a cause are one row ("2 downloads failed").
	const fixRows = $derived(groupAttention(fixes));
	/** One row in a few words: "6 downloads failed", "2 Sources have a problem", "Downloader: SABnzbd not answering". */
	const rowLine = (g: AttentionGroup) => (g.items.length > 1 ? (/^(health|setup):/.test(g.items[0].cause) ? `${g.title} ${g.reason}` : g.title) : `${g.title}: ${g.reason}`);
	// The Home tile names the worst row and says how many more things there are, so the number adds up.
	const fixLine = $derived(fixRows.length ? rowLine(fixRows[0]) + (fixes.length > fixRows[0].items.length ? ` and ${n(fixes.length - fixRows[0].items.length)} more` : '') : '');
	// The Needs attention tile opens the page of the row it names (the worst one).
	const fixHref = $derived(fixRows[0]?.href ?? (admin ? '/settings/health' : '/status'));
	// System lists each cause as a row; four fit, the rest link to the page that holds most of them.
	const fixShown = $derived(fixRows.slice(0, 4));
	const fixHidden = $derived(moreLink(fixRows.slice(4), admin));
	// What passed, for the all-good state: each kind of check that has no problem, once.
	const CHECK_WORDS: Record<string, string> = { library_folder: 'Library folders', disk_space: 'Disk space', source: 'Sources', downloader: 'Downloaders', backups: 'Backups', update: 'Updates', config_disk: 'Config folder', config_folder: 'Config folder', waiting_downloads: 'Downloads', trailers: 'Trailers' };
	const passedCount = $derived((checks?.checks ?? []).filter((c) => shownState(c) === 'ok').length);
	const passed = $derived.by(() => {
		const bad = new Set((checks?.checks ?? []).filter((c) => shownState(c) !== 'ok').map((c) => CHECK_WORDS[c.kind]));
		return [...new Set((checks?.checks ?? []).map((c) => CHECK_WORDS[c.kind]).filter((w) => w && !bad.has(w)))];
	});
	// Your Library: one card per kind that is on, and for owners what it all takes on disk.
	const libCards = $derived(libraryCards(home?.library, manager ? '/downloads' : undefined));
	const diskTotal = $derived(libCards.reduce((t, c) => t + c.size, 0));
	const showDisk = $derived(owner && libCards.length > 0);
	const waiting = $derived(asks.filter((a) => a.state === 'asked'));
	const mine = $derived(asks.filter((a) => a.mine && a.state !== 'available'));
	const mineShown = $derived(mine.slice(0, 20));
	// A turned-down ask is not open: it is counted apart.
	const mineOpen = $derived(mine.filter((a) => a.state !== 'turned_down').length);
	const mineDeclined = $derived(mine.length - mineOpen);
	const asksLine = $derived([mineOpen ? `${n(mineOpen)} still open` : '', mineDeclined ? `${n(mineDeclined)} turned down` : ''].filter(Boolean).join(' · '));
	const canAsk = $derived(session.can('ask') || approver);
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
	// A picture that cannot load drops out, and the next one (or a count tile) takes its place.
	const badPic = new SvelteSet<number>();
	const badLogo = new SvelteSet<number>();
	const pics = $derived.by(() => {
		const lists: [HomeCard[], (c: HomeCard) => string][] = [
			[home?.new_seasons ?? [], () => 'New season'],
			// "Just arrived" only while it is: an older arrival says how old it is.
			[home?.arrived ?? [], (c) => arrivedTitle(c.at ? Date.parse(c.at) : undefined, now.getTime())],
			[home?.soon ?? [], () => 'Coming soon'],
			[home?.following ?? [], () => 'Following']
		];
		const seen = new Set<number>();
		const out: { c: HomeCard; label: string }[] = [];
		for (let i = 0; out.length < 3 && lists.some(([l]) => i < l.length); i++) {
			for (const [l, label] of lists) {
				const c = l[i];
				if (c && (c.backdrop || c.poster) && !seen.has(c.title_id) && !badPic.has(c.title_id) && out.length < 3) {
					seen.add(c.title_id);
					out.push({ c, label: label(c) });
				}
			}
		}
		return out;
	});
	const showOverview = $derived(!rows.includes('welcome'));
	// The count tiles, in order. The first four are always there for the person they are for (Needs attention, asks,
	// Library, disk), so they never change places while the page loads; the rest fill the picture places that have
	// no picture. Explore, New & Popular and Search always can, so the grid never has a hole.
	const recentNew = $derived((home?.arrived ?? []).filter((c) => c.at && now.getTime() - Date.parse(c.at) < 14 * 86400000).length + (home?.new_seasons?.length ?? 0));
	const statPool = $derived(
		[
			owner ? 'att' : '',
			canAsk ? 'ask' : '',
			libCards.length ? 'lib' : '',
			showDisk ? 'disk' : '',
			fetchingAll.length ? 'dl' : '',
			owner && home?.soon?.length && !pics.some((p) => p.label === 'Coming soon') ? 'soon' : '',
			recentNew ? 'new' : '',
			'explore',
			'popular',
			manager ? 'calendar' : '',
			'search'
		].filter(Boolean)
	);
	// The grid's shape for how many tiles there are: every shape fills whole rows at four columns and at two.
	const SHAPES: Record<number, string[]> = {
		7: ['big', '', '', 'tall', '', 'wide', 'fill'],
		6: ['big', '', '', 'wide', 'wide', 'wide'],
		5: ['big', 'wide', 'wide', 'wide', 'wide'],
		4: ['big', '', '', 'wide'],
		3: ['big', 'wide', 'wide'],
		2: ['big', 'big'],
		1: ['banner']
	};
	type Place = { key: string; shape: string; pic?: { c: HomeCard; label: string }; stat?: string };
	const places = $derived.by((): Place[] => {
		const shapes = SHAPES[Math.min(7, pics.length + statPool.length)] ?? [];
		const p = [...pics];
		const st = [...statPool];
		const out: Place[] = shapes.map((shape) => ({ key: '', shape }));
		const roomy = (shape: string) => shape === 'big' || shape === 'tall' || shape === 'wide' || shape === 'banner';
		// Pictures take the roomy places, the first count tiles take the small ones, and what is left fills the rest.
		for (const pl of out) if (roomy(pl.shape) && p.length) pl.pic = p.shift();
		for (const pl of out) if (!roomy(pl.shape) && st.length) pl.stat = st.shift();
		for (const pl of out) {
			if (!pl.pic && !pl.stat) {
				if (st.length) pl.stat = st.shift();
				else pl.pic = p.shift();
			}
			pl.key = pl.pic ? `p${pl.pic.c.title_id}` : `s${pl.stat}`;
		}
		return out;
	});
	const libTotal = $derived(libCards.reduce((t, c) => t + c.big, 0));
	// The Library is never a mixed list: every link goes to one kind. A card's own kind when the link is about a card;
	// for the Library total, the biggest part of it.
	const libHref = $derived([...libCards].sort((x, y) => y.big - x.big)[0]?.href ?? '/library?kind=movie');
	const kindHref = (c?: HomeCard) => (c ? `/library?kind=${c.kind}` : libHref);
	// An ask's picture. Its own poster first; when the ask was saved without one, or it cannot be shown, the picture
	// the Requests page uses: the title's poster from this install's own details, or the artist's wide photo.
	const badArt = new SvelteSet<number>();
	const noArt = new SvelteSet<number>();
	function askPic(a: Ask): string | undefined {
		if (noArt.has(a.id)) return undefined;
		if (a.poster && !badArt.has(a.id)) return a.poster;
		if (a.kind === 'artist') return askArtist(a)?.backgrounds[0] || undefined;
		wantDetails(a);
		return askDetails(a)?.poster || undefined;
	}
	const askKind = (a: Ask): 'movie' | 'show' | 'artist' => (a.kind === 'show' ? 'show' : a.kind === 'artist' ? 'artist' : 'movie');
	/** The ask's wide picture, for a wide card: the title's backdrop or the artist's wide photo (the same ones the Requests cards show). */
	function askWide(a: Ask): string | undefined {
		if (a.kind === 'artist') return askArtist(a)?.backgrounds[0] || undefined;
		wantDetails(a);
		return askDetails(a)?.backdrop || undefined;
	}
	function artFailed(a: Ask, pic: string) {
		if (pic === a.poster) badArt.add(a.id);
		else noArt.add(a.id);
	}
	// An artist's own photo can be an empty file: it is tried once here, so the row can fall back before it shows a gap.
	const tried = new Set<number>();
	$effect(() => {
		for (const a of [...waiting.slice(0, 20), ...mineShown]) {
			if (a.kind !== 'artist' || !a.poster || tried.has(a.id)) continue;
			tried.add(a.id);
			const probe = new Image();
			probe.onerror = () => badArt.add(a.id);
			probe.src = a.poster;
		}
	});
	const waitingPics = $derived(
		waiting
			.slice(0, 20)
			.map((a) => ({ a, pic: askPic(a) }))
			.filter((x): x is { a: Ask; pic: string } => !!x.pic)
			.slice(0, 3)
	);
	// Downloads, counted once and used everywhere: waiting (queued), downloading (bytes coming in, stalled and checking
	// included), importing, and paused apart (a paused download is not in the list or the row). The tile, the note
	// beside the switch and the row all read these.
	const speed = $derived(fetchingAll.reduce((t, d) => t + (d.speed || 0), 0));
	const pausedCount = $derived(manager ? downloads.filter((d) => !d.client_item && stageOf(d) === 'paused').length : 0);
	function countStages(rows: DownloadRow[], paused = 0) {
		const c = { waiting: 0, down: 0, imp: 0, paused };
		for (const d of rows) {
			const st = stageOf(d);
			if (st === 'queued') c.waiting++;
			else if (st === 'importing') c.imp++;
			else c.down++;
		}
		return c;
	}
	const dlCount = $derived(countStages(fetchingAll, pausedCount));
	const stageWords = (c: { waiting: number; down: number; imp: number; paused: number }) =>
		[c.down ? `${n(c.down)} downloading` : '', c.imp ? `${n(c.imp)} importing` : '', c.waiting ? `${n(c.waiting)} waiting` : '', c.paused ? `${n(c.paused)} paused` : ''].filter(Boolean).join(' · ');
	/** One running download in a few words: its percent and speed, or the step it is in. */
	function dlState(d: DownloadRow): string {
		const st = stageOf(d);
		if (st === 'queued') return 'Waiting';
		if (st === 'importing') return 'Importing';
		if (st === 'checking') return 'Checking';
		if (st === 'stalled') return `Stalled at ${Math.round(percent(d.progress))}%`;
		return `${Math.round(percent(d.progress))}%${d.speed ? ` · ${bytes(d.speed)}/s` : ''}`;
	}
	// The tile lists what is moving first: downloading, then stalled, checking, importing, and waiting last.
	const STEP_ORDER: Record<string, number> = { downloading: 0, stalled: 1, checking: 2, importing: 3, queued: 4 };
	const dlList = $derived([...fetchingAll].sort((x, y) => (STEP_ORDER[stageOf(x)] ?? 9) - (STEP_ORDER[stageOf(y)] ?? 9)).slice(0, 3));
	// When nothing runs, the tile shows what finished last.
	const recentDone = $derived(
		manager
			? downloads
					.filter((d) => !d.client_item && !!d.finished_at && d.phase === 'done')
					.sort((x, y) => Date.parse(y.finished_at ?? '') - Date.parse(x.finished_at ?? ''))
					.slice(0, 3)
			: []
	);
	const pillIcon: Record<string, IconName> = { 'New season': 'sparkle', 'New today': 'sparkle', 'New this week': 'sparkle', 'Just arrived': 'check', 'Recently added': 'check', 'Coming soon': 'calendar', Following: 'bell' };
	// Disk: how much room is left, and whether the disk space check is failing (then a reading of 0 is a full disk).
	const diskState = $derived.by(() => {
		const st = (checks?.checks ?? []).filter((c) => c.kind === 'disk_space').map((c) => shownState(c));
		return st.includes('error') ? 'error' : st.includes('warning') ? 'warning' : 'ok';
	});
	const free = $derived(freeSpace(folders, diskState !== 'ok'));
	const diskLine = $derived(
		!free
			? diskState === 'ok'
				? 'Across your Library folders'
				: 'A disk is running out of room'
			: free.free <= 0
				? free.many
					? 'A disk is full'
					: 'The disk is full'
				: `${diskState === 'ok' ? '' : 'Only '}${bytes(free.free)} free${free.many ? ' on the fullest disk' : ''}`
	);
	const diskTint = $derived<Tint>(diskState === 'error' ? 'red' : diskState === 'warning' ? 'amber' : 'teal');
	const diskUsed = $derived(!free ? 0 : free.free <= 0 ? 100 : Math.min(100, Math.max(2, (diskTotal / Math.max(1, diskTotal + free.free)) * 100)));
	const sysOk = $derived(settled && worst === 'ok');
	const sysNote = $derived(
		!settled
			? ''
			: (fixes.length ? `${n(fixes.length)} ${fixes.length === 1 ? 'thing needs' : 'things need'} a look` : unchecked ? 'Could not check everything' : 'All good') +
					(uncheckedDl ? '' : dlCount.down ? ` · ${n(dlCount.down)} downloading${speed ? ` at ${bytes(speed)}/s` : ''}` : dlCount.imp || dlCount.waiting || dlCount.paused ? ` · ${stageWords(dlCount)}` : '')
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
		uncheckedParts = he.status === 'rejected';
		uncheckedAsks = as.status === 'rejected';
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
	<div class="top">
		<p class="greet" data-testid="home-greeting">{greeting}</p>
		<!-- The page's own heading, for screen readers and for the small title bar phones show once the greeting has scrolled away. -->
		<PageHeader title="Home" />
		<div class="top-row">
			{#if showSystem && showOverview && loaded && !(failed && !home)}
				<SegmentedControl options={[{ value: 'home', label: 'Home', icon: 'home' }, { value: 'system', label: 'System', icon: 'gauge' }]} bind:value={board} label="Show on Home" testid="home-switch" announce={(l) => `Showing ${l}`} />
			{/if}
			<p class="note" class:pending={!settled} class:hold={showSystem && showOverview}>
				{#if failed && !home}{:else if shown === 'system'}{#if sysNote}<span class="dot {worst}" aria-hidden="true"></span>{sysNote}{/if}{:else}{news}{/if}
			</p>
			<div class="acts">
				<span class="wide-only"><Button variant="secondary" icon="search" href={to('/search')}>Search</Button><Button variant="secondary" icon="compass" href={to('/explore')}>Explore</Button></span>
				<span class="phone-only"><IconButton icon="search" label="Search" variant="tinted" size="lg" href={to('/search')} tooltip={false} /><IconButton icon="compass" label="Explore" variant="tinted" size="lg" href={to('/explore')} tooltip={false} /></span>
			</div>
		</div>
	</div>

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
							<WideCard cover title={c.name} kind={c.kind} typed row={type} href={to(c.href)} image={c.backdrop || c.poster || undefined} logo={c.kind === 'artist' || !(c.backdrop || c.poster) ? null : c.logo} inset={c.kind === 'artist' ? c.albums?.[0]?.cover : undefined} subtitle={c.line} fresh={strip(c, type)} />
						{/each}
					</Shelf>
				</div>
			{/if}
		{/snippet}

		<!-- One picture tile: the big and wide ones use the wide picture (with the title logo), the tall one the poster. -->
		{#snippet picTile(p: { c: HomeCard; label: string }, cls: string, tall: boolean)}
			<a class="tile pic {cls}" href={to(p.c.href)}>
				<!-- Through HolyArrs' own artwork cache, like every other card: the browser never calls the picture site itself. -->
				<img src={tall ? sized(p.c.poster || p.c.backdrop, 'poster', 400) : sized(p.c.backdrop || p.c.poster, cls === 'big' ? 'large' : 'backdrop')} alt="" decoding="async" use:artGone={() => badPic.add(p.c.title_id)} />
				<span class="pill"><ArtPill text={p.label} icon={pillIcon[p.label]} /></span>
				{#if !tall && p.c.backdrop && p.c.logo && p.c.kind !== 'artist' && !badLogo.has(p.c.title_id)}
					<img class="logo" src={proxied(p.c.logo)} alt={p.c.name} decoding="async" use:lightOnDark use:artGone={() => badLogo.add(p.c.title_id)} />
				{:else}
					<span class="nm" dir="auto">{p.c.name}</span>
				{/if}
				{#if p.c.line}<span class="sm">{p.c.line}</span>{/if}
			</a>
		{/snippet}

		<PageLoader loading={!loaded} label="Loading Home" onretry={retry}>
		{#if showOverview}
			<div class="boards" data-testid="home-overview">
				<!-- A tile's head: the same glossy icon square as the Settings pages, its name, and an arrow when it opens a page. -->
				{#snippet head(icon: IconName, tint: Tint, label: string, go = true)}
					<span class="head"><SettingsTile {icon} {tint} /><span class="lab">{label}</span>{#if go}<span class="go" aria-hidden="true"><Icon name="chevronRight" size={16} /></span>{/if}</span>
				{/snippet}
				<!-- A tile whose numbers have not arrived yet. -->
				{#snippet wait(lines = 2)}
					<span class="sk" aria-hidden="true">{#each Array(lines) as _, i (i)}<i></i>{/each}</span><span class="sr-only">Loading</span>
				{/snippet}
				<!-- Disk: what the Library takes, how much room is left, and one thin line along the foot for how full it is (only when the free room is known). -->
				{#snippet diskTile(cls: string)}
					<a class="tile stat {cls}" class:lined={!!free} href={to(admin ? '/settings/library-folders' : '/status')}>
						{@render head('archive', diskTint, 'On disk')}
						{#if !settled}
							{@render wait()}
						{:else}
							<span class="num">{bytes(diskTotal).split(' ')[0]}<small>{bytes(diskTotal).split(' ')[1]}</small></span>
							<span class="sm" class:fail={diskState === 'error'} class:warn={diskState === 'warning'}>{diskLine}</span>
							{#if free}
								<span class="life" role="img" aria-label="Your Library takes {bytes(diskTotal)}; {bytes(free.free)} free">
									<i class="used {diskState}" style="width: {diskUsed}%"></i>
								</span>
							{/if}
						{/if}
					</a>
				{/snippet}
				<!-- Sources or Downloaders: how many work, which have a problem, and one pip each along the foot. -->
				{#snippet partTile(label: string, icon: IconName, tint: Tint, p: { total: number; on: number; working: number; off: number; failing: number }, trouble: string, path: string, empty: string)}
					<a class="tile stat" class:lined={settled && !uncheckedParts && p.total > 0} href={to(admin ? path : '/status')}>
						{@render head(icon, tint, label)}
						{#if !settled}
							{@render wait()}
						{:else if uncheckedParts}
							<span class="word">Could not read</span>
							<span class="sm">Home could not read this right now.</span>
						{:else if !p.total}
							<span class="word">None added</span>
							<span class="sm">{empty}</span>
						{:else}
							<span class="num" class:warn={p.on > 0 && !p.working}>{n(p.working)}<small>of {n(p.total)} working</small></span>
							<span class="sm" class:warn={!!trouble}>{partLine(p, trouble)}</span>
							<span class="life pips" aria-hidden="true">
								{#if p.total <= 12}
									{#each Array(p.total) as _, i (i)}<i class:bad={i >= p.working && i < p.on} class:off={i >= p.on}></i>{/each}
								{:else}
									{#if p.working}<i style="flex-grow: {p.working}"></i>{/if}{#if p.failing}<i class="bad" style="flex-grow: {p.failing}"></i>{/if}{#if p.off}<i class="off" style="flex-grow: {p.off}"></i>{/if}
								{/if}
							</span>
						{/if}
					</a>
				{/snippet}
				<!-- One count tile. -->
				{#snippet stat(key: string, cls: string)}
					{#if key === 'att' && settled && unchecked && !fixes.length}
						<button type="button" class="tile stat {cls}" class:busy={retrying} disabled={retrying} onclick={retry} aria-busy={retrying || undefined} data-testid="home-attention">
							{@render head('refresh', 'amber', 'Needs attention', false)}
							<span class="word">Could not check</span>
							<span class="sm">Tap to try again</span>
						</button>
					{:else if key === 'att'}
						<a class="tile stat {cls}" href={to(fixHref)} data-testid="home-attention">
							{@render head(worst === 'ok' ? 'check' : 'alert', attTint, 'Needs attention')}
							{#if !settled}
								{@render wait()}
							{:else if fixes.length}
								<span class="num" class:fail={worst === 'fail'} class:warn={worst === 'wait'}>{n(fixes.length)}</span>
								<span class="sm">{fixLine}</span>
							{:else}
								<span class="word">All good</span>
								<span class="sm">Downloads, imports and checks</span>
							{/if}
						</a>
					{:else if key === 'ask'}
						{@const approving = approver && !kid && waiting.length > 0}
						<a class="tile stat {cls}" href={to('/requests')}>
							{@render head('requests', 'orange', approving ? 'To approve' : mine.length ? 'Your asks' : 'Requests')}
							{#if !settled}
								{@render wait()}
							{:else if uncheckedAsks}
								<span class="word">Could not read</span>
								<span class="sm">Home could not read asks right now.</span>
							{:else if approving && waitingPics.length}
								<span class="ask-stack" aria-hidden="true">{#each waitingPics as w (w.a.id + w.pic)}<img src={sized(w.pic, 'poster', 80)} alt="" decoding="async" use:artGone={() => artFailed(w.a, w.pic)} />{/each}{#if waiting.length > waitingPics.length}<b>+{n(waiting.length - waitingPics.length)}</b>{/if}</span>
								<span class="sm">{n(waiting.length)} {waiting.length === 1 ? 'ask' : 'asks'} to look at</span>
							{:else if approving}
								<span class="num">{n(waiting.length)}</span>
								<span class="sm">{waiting.length === 1 ? 'ask' : 'asks'} to look at</span>
							{:else if mineOpen}
								<span class="num">{n(mineOpen)}</span>
								<span class="sm">still open{mineDeclined ? ` · ${n(mineDeclined)} turned down` : ''}</span>
							{:else if mineDeclined}
								<span class="num">{n(mineDeclined)}</span>
								<span class="sm">turned down</span>
							{:else}
								<span class="word">Nothing waiting</span>
								<span class="sm">{approver && !kid ? 'No asks to approve' : 'Ask for something on Explore'}</span>
							{/if}
						</a>
					{:else if key === 'lib'}
						<a class="tile stat {cls}" href={to(libHref)} data-testid="home-library">
							{@render head('library', 'indigo', 'Library')}
							<span class="num">{n(libTotal)}</span>
							<span class="sm">{libCards.map((c) => `${n(c.big)}\u00a0${c.unit}`).join(' · ')}</span>
						</a>
					{:else if key === 'disk'}
						{@render diskTile(cls)}
					{:else if key === 'dl'}
						<a class="tile stat {cls}" href={to('/downloads')}>
							{@render head('download', 'blue', 'Downloads')}
							<span class="num">{n(fetchingAll.length)}</span>
							<span class="sm">{stageWords(countStages(fetchingAll))}</span>
						</a>
					{:else if key === 'soon'}
						<a class="tile stat {cls}" href={to('/calendar')}>
							{@render head('calendar', 'purple', 'Coming soon')}
							<span class="num">{n(home?.soon?.length ?? 0)}{(home?.soon?.length ?? 0) >= 20 ? '+' : ''}</span>
							<span class="sm">in the next 30 days</span>
						</a>
					{:else if key === 'new'}
						<a class="tile stat {cls}" href={to(kindHref(home?.arrived?.[0] ?? home?.new_seasons?.[0]))}>
							{@render head('sparkle', 'pink', 'New for you')}
							<span class="num">{n(recentNew)}</span>
							<span class="sm">new in the last two weeks</span>
						</a>
					{:else if key === 'explore'}
						<a class="tile stat {cls}" href={to('/explore')}>
							{@render head('compass', 'green', 'Explore')}
							<span class="word">Find something new</span>
							<span class="sm">Pick what to ask for next</span>
						</a>
					{:else if key === 'popular'}
						<a class="tile stat {cls}" href={to('/new')}>
							{@render head('star', 'pink', 'New & Popular')}
							<span class="word">What's trending</span>
							<span class="sm">Top 10s and coming soon</span>
						</a>
					{:else if key === 'calendar'}
						<a class="tile stat {cls}" href={to('/calendar')}>
							{@render head('calendarDay', 'purple', 'Calendar')}
							<span class="word">What's next</span>
							<span class="sm">Air dates and releases</span>
						</a>
					{:else}
						<a class="tile stat {cls}" href={to('/search')}>
							{@render head('search', 'graphite', 'Search')}
							<span class="word">Look anything up</span>
							<span class="sm">By name</span>
						</a>
					{/if}
				{/snippet}

				<div class="bento" class:gone={shown !== 'home'} inert={shown !== 'home'} data-testid="home-board">
					{#each places as pl (pl.key)}
						{#if pl.pic}{@render picTile(pl.pic, pl.shape, pl.shape === 'tall')}{:else if pl.stat}{@render stat(pl.stat, pl.shape)}{/if}
					{/each}
				</div>
				{#if showSystem}
					<div class="bento" class:gone={shown !== 'system'} inert={shown !== 'system'} data-testid="home-system">
						<a class="tile stat big dl" href={to('/downloads')}>
							{@render head('download', 'blue', 'Downloading')}
							{#if !settled}
								{@render wait(3)}
							{:else if uncheckedDl}
								<span class="word">Could not read</span>
								<span class="sm">Home could not read downloads right now.</span>
							{:else}
								{#if speed}
									<span class="num">{bytes(speed).split(' ')[0]}<small>{bytes(speed).split(' ')[1]}/s</small></span>
								{:else}
									<span class="num">{n(dlCount.down)}<small>downloading</small></span>
								{/if}
								<!-- The three steps a download goes through, in order. -->
								<ol class="flow" aria-label="Downloads by step">
									<li><b>{n(dlCount.waiting)}</b><span>Waiting</span></li>
									<li><b>{n(dlCount.down)}</b><span>Downloading</span></li>
									<li><b>{n(dlCount.imp)}</b><span>Importing</span></li>
								</ol>
								{#if fetchingAll.length}
									<ul class="dl-list">
										{#each dlList as d (d.id)}
											{@const moving = ['downloading', 'stalled'].includes(stageOf(d))}
											<li><b dir="auto">{d.title || d.name}</b><span>{dlState(d)}</span>{#if moving}<i class="pb"><i style="width: {percent(d.progress)}%"></i></i>{/if}</li>
										{/each}
									</ul>
								{:else if recentDone.length}
									<ul class="dl-list done">
										{#each recentDone as d (d.id)}
											<li><b dir="auto">{d.title || d.name}</b><span>Finished {agoLong(d.finished_at, now.getTime())}</span></li>
										{/each}
									</ul>
								{:else}
									<span class="quiet">Nothing is downloading right now.{pausedCount ? ` ${n(pausedCount)} paused.` : ''}</span>
								{/if}
							{/if}
						</a>
						<section class="tile stat big att" aria-labelledby="h-fix" data-testid="home-problems">
							<header class="head">
								<SettingsTile icon={worst === 'ok' ? 'check' : 'alert'} tint={attTint} />
								<h2 class="lab" id="h-fix">Needs attention</h2>
								{#if fixes.length}<b class="count {worst}">{n(fixes.length)}</b>{/if}
							</header>
							{#if !settled}
								{@render wait(3)}
							{:else if fixes.length}
								<ul class="fixes">
									{#each fixShown as g (g.key)}
										<li>
											<a class="fix" href={to(g.href)}>
												<i class="tone {g.tone}" aria-hidden="true"></i>
												<span class="ft"><span class="sr-only">{g.word}: </span><b dir="auto">{g.title}</b><span dir="auto">{g.reason}</span></span>
												<span class="fa"><span class="act">{g.action}</span><Icon name="chevronRight" size={14} /></span>
											</a>
										</li>
									{/each}
								</ul>
								{#if fixHidden}
									<a class="more" href={to(fixHidden.href)}>{fixHidden.text}<Icon name="chevronRight" size={13} /></a>
								{:else if unchecked}
									<span class="rest-ok">Some checks could not be read right now.</span>
								{:else if fixShown.length <= 2 && passedCount}
									<span class="rest-ok"><i class="tone ok" aria-hidden="true"></i>{n(passedCount)} other {passedCount === 1 ? 'check' : 'checks'} passed.</span>
								{/if}
							{:else if unchecked}
								<span class="word">Could not check</span>
								<span class="sm">{uncheckedLine(uncheckedDl, uncheckedHealth || uncheckedParts)}</span>
								<span class="again"><Button size="sm" variant="secondary" busy={retrying} onclick={retry}>Try again</Button></span>
							{:else}
								<!-- All good: what was checked, each with its tick. -->
								{#if passed.length}
									<ul class="passed">
										{#each passed.slice(0, 8) as w (w)}<li><Icon name="check" size={14} />{w}</li>{/each}
									</ul>
								{/if}
								<span class="word">All good</span>
								<span class="sm">Downloads, imports and checks are running fine.</span>
							{/if}
						</section>
						{@render diskTile('')}
						{@render partTile('Sources', 'source', 'purple', srcs, troubled('source', health?.sources), '/settings/sources', 'Add a Source to find releases')}
						{@render partTile('Downloaders', 'downloader', 'indigo', dls, troubled('downloader', health?.downloaders), '/settings/downloaders', 'Add a Downloader to fetch them')}
						<a class="tile stat" href={to('/status')}>
							{@render head('clock', 'graphite', 'Running for')}
							{#if !settled}
								{@render wait()}
							{:else}
								{#if health && !uncheckedParts}<span class="num">{duration(health.uptime_s)}</span>{:else}<span class="word">Could not read</span>{/if}
								{#if health && !uncheckedParts}<span class="sm">Version {health.version}</span>{/if}
								<span class="sm meta"><i class="tone" class:ok={!metadataOutage.s.down} class:wait={metadataOutage.s.down} aria-hidden="true"></i>Metadata server {metadataOutage.s.down ? 'down' : 'online'}</span>
							{/if}
						</a>
					</div>
				{/if}
			</div>
		{/if}
		{#if fetchingShown.length}
			<Shelf title="Downloading" subtitle={stageWords(countStages(fetchingAll))} href={to('/downloads')} item="var(--poster-item)">
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
							<p class="kicker">{#if settled}{doneCount} of {steps.length} done{:else}&nbsp;{/if}</p>
							<h2 id="h-setup">Welcome to HolyArrs</h2>
							<div class="segs" aria-hidden="true">{#each steps as s, i (i)}<i class:on={settled && s.done}></i>{/each}</div>
							{#if settled}
								<h3>{nextStep.text}</h3>
								<p>{nextStep.detail}</p>
							{:else}
								<span class="sk setup-wait" aria-hidden="true"><i></i><i></i></span><span class="sr-only">Loading</span>
							{/if}
							<div class="welcome-acts" class:hide={!settled}>
								<Button variant="primary" size="lg" href={to(nextStep.href)} iconAfter="chevronRight">Continue setup</Button>
								<Button variant="secondary" size="lg" href={to('/library/import')}>Import what you have</Button>
							</div>
						</div>
						<ol class="steps">
							{#each steps as s, i (i)}
								<li class:done={settled && s.done}>
									<a href={to(s.href)}>
										<span class="dot">{#if settled && s.done}<Icon name="check" size={14} />{:else}{i + 1}{/if}</span>
										<span class="grow">{s.text}</span>
										<span class="sr-only">{!settled ? '' : s.done ? '(done)' : '(to do)'}</span>
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
				{@render wideRow(home?.arrived ?? [], 'home_arrived', { ...arrivedHead(home?.arrived ?? [], now.getTime()), href: to(kindHref(home?.arrived?.[0])) })}
			{:else if row === 'approval' && waiting.length}
				<!-- The asks rows use the same wide cards as the rows around them: the title's wide picture, who asked under it. -->
				<div class="wide-row" data-testid="home_approval">
					<Shelf title="Waiting for approval" subtitle="{waiting.length} {waiting.length === 1 ? 'ask' : 'asks'} on the Requests page" href={to('/requests')} item="var(--title-item)">
						{#each waiting.slice(0, 20) as a (a.id)}
							{@const wide = askWide(a)}
							{@const pic = askPic(a)}
							{#key `${wide}|${pic}`}
								<WideCard cover title={a.name} kind={askKind(a)} typed href={to(`/requests?ask=${a.id}`)} image={wide} poster={pic} subtitle="Asked by {personName(a.asked_by?.name, 'someone in your household')}" owned={askMark(a)} />
							{/key}
						{/each}
					</Shelf>
				</div>
			{:else if row === 'asks' && mine.length}
				<div class="wide-row" data-testid="home_asks">
					<Shelf title="Your asks" subtitle={asksLine} href={to('/requests')} item="var(--title-item)">
						{#each mineShown as a (a.id)}
							{@const p = a.title_id ? progress[a.title_id] : undefined}
							{@const wide = askWide(a)}
							{@const pic = askPic(a)}
							{#key `${wide}|${pic}`}
								<WideCard
									cover
									title={a.name}
									kind={askKind(a)}
									typed
									href={a.title_id ? to(`/library/${a.title_id}`) : to(`/requests?ask=${a.id}`)}
									image={wide}
									poster={pic}
									subtitle={p ? askProgressLine(p).replace(/^Downloading /, '') : a.season_words || a.album_words || (a.year ? String(a.year) : undefined)}
									progress={p && p.state === 'downloading' ? Math.round(p.progress * 100) : undefined}
									owned={askMark(a)}
								/>
							{/key}
						{/each}
					</Shelf>
				</div>
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
	/* The same sizes as Explore: 279px wide cards at a 1536px window (Netflix), 5px gaps on wide rows, one 32px rhythm between blocks.
	   The sheet colours live here so the tiles and the Welcome card share them. */
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
	@media (min-width: 768px) { .wide-row { --shelf-gap: 5px; } }

	/* The top: the greeting, then one line with the Home | System switch, the note beside it, and Search and Explore. */
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

	/* Bento: one grid of tiles. Home: pictures in the roomy places, counts in the small ones; the shape follows how many
	   tiles there are, and every shape fills whole rows. System: the same frame filled with how HolyArrs is running.
	   The hidden grid is lifted out of the flow. */
	.boards { position: relative; }
	.boards > .bento { transition: opacity var(--dur) var(--ease-out), translate var(--dur) var(--ease-out); }
	.bento.gone { position: absolute; inset: 0 0 auto; opacity: 0; translate: 0 6px; visibility: hidden; pointer-events: none; transition: opacity var(--dur) var(--ease-out), translate var(--dur) var(--ease-out), visibility 0s linear var(--dur); }
	.bento { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); grid-auto-rows: 150px; grid-auto-flow: row dense; gap: 16px; }

	/* The tile is the Requests card's sheet, on its own: lit from above (a paler top, a bright top edge), a hairline,
	   and one long soft shadow that sits it on the page. A whisper of grain keeps the flat colour from looking printed. */
	.tile {
		position: relative; isolation: isolate; min-width: 0; min-height: 0; display: flex; flex-direction: column; padding: 16px 18px; border: 0; border-radius: 20px; overflow: hidden;
		color: var(--text); text-align: left; font: inherit; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b)); box-shadow: var(--edge), var(--drop);
		transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out), opacity var(--dur-fast) var(--ease-out);
	}
	.stat::before { content: ''; position: absolute; inset: 0; z-index: -1; border-radius: inherit; pointer-events: none; opacity: .05; background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='180' height='180'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix values='0 0 0 0 .5 0 0 0 0 .5 0 0 0 0 .5 0 0 0 1 0'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E"); }
	a.tile, button.tile { cursor: pointer; }
	a.tile:hover, button.tile:hover { transform: translateY(var(--lift)); box-shadow: var(--edge), 0 2px 4px light-dark(rgb(20 24 40 / 6%), rgb(0 0 0 / 40%)), 0 26px 40px -22px light-dark(rgb(20 24 40 / 38%), rgb(0 0 0 / 90%)); }
	a.tile:focus-visible, button.tile:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tile.busy { opacity: .6; }
	.tile.big { grid-column: span 2; grid-row: span 2; }
	.tile.wide { grid-column: span 2; }
	.tile.tall { grid-row: span 2; }
	.tile.banner { grid-column: 1 / -1; }

	/* A count tile: the icon square and name on top, the number low on the left, one quiet line under it. */
	.head { display: flex; align-items: center; gap: 10px; min-width: 0; }
	.lab { flex: 1; min-width: 0; margin: 0; font: 600 14px / 1.3 var(--font-ui); letter-spacing: -0.005em; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.go { flex: none; display: inline-grid; color: var(--text-faint); transition: transform var(--dur) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	a.tile:hover .go { color: var(--text); transform: translateX(2px); }
	.num, .word { margin-top: auto; }
	.num { font: 800 40px / 1 var(--font-display); letter-spacing: -0.04em; font-variant-numeric: tabular-nums; white-space: nowrap; }
	.num small { margin-left: 5px; font: 500 13px / 1 var(--font-ui); letter-spacing: 0; color: var(--text-muted); }
	.word { font: 700 24px / 1.1 var(--font-display); letter-spacing: -0.025em; }
	.sm { margin-top: 5px; font: var(--t-footnote); color: var(--text-muted); overflow: hidden; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
	.stat > .num + .sm:not(:has(+ .sm)), .stat > .word + .sm:not(:has(+ .sm)) { min-height: 2.6em; }
	.fail, .sm.fail { color: var(--fail); }
	.warn, .sm.warn { color: var(--wait); }
	.sk { margin-top: auto; display: flex; flex-direction: column; gap: 8px; }
	.sk i { display: block; height: 12px; border-radius: 6px; background: var(--sk-base); }
	.sk i + i { width: 60%; }
	.sk i + i + i { width: 40%; }

	/* The thin line along a tile's foot, the same as the life line on a Requests card: how full the disk is, or one pip per part. */
	.life { position: absolute; left: 18px; right: 18px; bottom: 6px; display: flex; gap: 2px; height: 4px; border-radius: 4px; overflow: hidden; background: var(--track); }
	.life i { display: block; min-width: 2px; flex-basis: 0; }
	.life .used { flex: none; border-radius: inherit; background: #1ea9be; }
	.life .used.warning { background: var(--wait); }
	.life .used.error { background: var(--fail); }
	.life.pips { gap: 3px; background: none; }
	.life.pips i { flex: 1; border-radius: 4px; background: var(--ok); }
	.life.pips i.bad { background: var(--wait); }
	.life.pips i.off { background: var(--track); }

	/* Picture tiles: the picture fills the tile; a glass pill says what it is, the title sits on a dark fade at the foot (white in both themes). */
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

	/* Waiting asks: up to three posters overlapping left to right, each ringed in the sheet's own colour, then a round +N. */
	.ask-stack { margin-top: auto; display: flex; align-items: center; }
	.ask-stack img { width: 38px; height: auto; aspect-ratio: 2 / 3; object-fit: cover; border-radius: 7px; margin-left: -12px; background: var(--surface-3); box-shadow: 0 0 0 2.5px var(--sheet-b), 0 6px 14px -4px rgb(0 0 0 / 35%); }
	.ask-stack img:first-child { margin-left: 0; }
	.ask-stack b { min-width: 30px; height: 30px; margin-left: 10px; padding: 0 8px; display: grid; place-items: center; border-radius: 999px; background: var(--well); box-shadow: inset 0 0 0 1px var(--line); color: var(--text); font: var(--t-footnote); font-weight: 700; font-variant-numeric: tabular-nums; }

	/* System: downloads. The number, the three steps a download goes through, then what is running (or what finished last). */
	.dl .num, .dl > .word { margin-top: 14px; }
	.flow { list-style: none; margin: 14px 0 0; padding: 0; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); border-radius: 12px; background: var(--well); box-shadow: inset 0 0 0 1px var(--line); }
	.flow li { position: relative; min-width: 0; padding: 9px 12px; display: flex; flex-direction: column; gap: 2px; }
	.flow li + li::before { content: ''; position: absolute; left: 0; top: 10px; bottom: 10px; width: 1px; background: var(--line); }
	.flow b { font: 700 18px / 1.1 var(--font-display); letter-spacing: -0.02em; font-variant-numeric: tabular-nums; }
	.flow span { font: var(--t-footnote); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.dl-list { list-style: none; margin: auto 0 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
	.dl-list li { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 3px 10px; font: var(--t-footnote); }
	.dl-list b { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.dl-list span { color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.pb { grid-column: 1 / -1; display: block; height: 4px; border-radius: 4px; background: var(--track); overflow: hidden; }
	.pb i { display: block; height: 100%; border-radius: inherit; background: var(--info); }
	.quiet { margin-top: auto; font: var(--t-callout); color: var(--text-muted); }

	/* System: Needs attention. Each cause is one row that opens where it can be fixed; when all is well, what was checked. */
	.count { flex: none; min-width: 24px; height: 24px; padding: 0 7px; display: grid; place-items: center; border-radius: 999px; background: var(--fail); color: #fff; font: 700 12px / 1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.count.wait { background: var(--wait); }
	.fixes { list-style: none; margin: 10px -8px 0; padding: 0; display: flex; flex-direction: column; min-height: 0; }
	.fixes li + li { box-shadow: inset 0 1px 0 var(--line); }
	.fix { display: flex; align-items: center; gap: 12px; min-height: 50px; padding: 0 8px; border-radius: 10px; color: inherit; transition: background var(--dur-fast) var(--ease-out); }
	.fix:hover { background: var(--well); }
	.fix:focus-visible, .more:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tone { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--fail); }
	.tone.wait { background: var(--wait); }
	.tone.ok { background: var(--ok); }
	.ft { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
	.ft b { font: 600 14px / 1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ft > span:last-child { font: var(--t-footnote); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.fa { flex: none; display: inline-flex; align-items: center; gap: 2px; color: var(--accent-text); font: 600 13px / 1 var(--font-ui); white-space: nowrap; }
	.more { position: relative; margin-top: auto; align-self: flex-start; display: inline-flex; align-items: center; gap: 2px; min-height: 28px; border-radius: 8px; color: var(--accent-text); font: 600 13px / 1 var(--font-ui); }
	.more::after { content: ''; position: absolute; inset: -8px 0; }
	.more:hover { text-decoration: underline; text-underline-offset: 3px; }
	.rest-ok { margin-top: auto; display: flex; align-items: center; gap: 8px; font: var(--t-footnote); color: var(--text-muted); }
	.passed { list-style: none; margin: 14px 0 0; padding: 0; display: flex; flex-wrap: wrap; gap: 6px; overflow: hidden; }
	.passed li { display: inline-flex; align-items: center; gap: 5px; height: 28px; padding: 0 10px 0 8px; border-radius: 999px; background: var(--well); box-shadow: inset 0 0 0 1px var(--line); font: var(--t-footnote); color: var(--text-muted); white-space: nowrap; }
	.passed li :global(.icon) { color: var(--ok); }
	.again { margin-top: 12px; }
	.meta { display: flex; align-items: center; gap: 6px; margin-top: 3px; }

	/* Two columns (a narrow window, a tablet, a phone): the big tiles keep their two-by-two, the tall one becomes one row,
	   an odd last tile spans the row, and everything inside a tile takes its compact size (the rows are 132px here). */
	@container home (max-width: 900px) {
		.bento { grid-template-columns: repeat(2, minmax(0, 1fr)); grid-auto-rows: 132px; gap: 10px; }
		.tile.tall { grid-row: span 1; }
		.tile.fill { grid-column: span 2; }
		.tile { padding: 12px 14px; border-radius: 18px; }
		.head { gap: 8px; }
		.lab { font-size: 13px; }
		.num { font-size: 30px; }
		.num small { font-size: 12px; margin-left: 4px; }
		.word { font-size: 19px; }
		.life { left: 14px; right: 14px; bottom: 5px; height: 3px; }
		.pill { top: 10px; left: 10px; }
		.pic .nm { font-size: 18px; }
		.pic.big .nm { font-size: 26px; }
		.pic.big .logo { max-height: 72px; }
		.sm { line-height: 1.3; }
		.ask-stack img { width: 32px; margin-left: -10px; border-radius: 6px; }
		.ask-stack b { min-width: 26px; height: 26px; margin-left: 8px; }
		.dl .num, .dl > .word { margin-top: 10px; }
		.flow { margin-top: 10px; }
		.flow li { padding: 7px 10px; }
		.fixes { margin-top: 6px; }
		.fix { min-height: 44px; }
		.passed { margin-top: 8px; max-height: 130px; }
	}
	/* Phones: the big tiles have less room, so two downloads show and the problem rows keep only their arrow. */
	@container home (max-width: 560px) {
		.dl-list li:nth-child(n + 3) { display: none; }
		.act { display: none; }
	}
	/* Two columns on a wide screen (a tablet, a narrow desktop window): a little taller, so pictures are not thin strips. */
	@container home (min-width: 560px) and (max-width: 900px) {
		.bento { grid-auto-rows: 150px; }
	}
	/* A small phone (360): the tile names still fit. */
	@container home (max-width: 340px) {
		.tile { padding-inline: 12px; }
		.life { left: 12px; right: 12px; }
		.head { gap: 7px; }
		.lab { font-size: 12.5px; letter-spacing: -0.012em; }
	}
	/* Phones: Search and Explore become two round buttons beside the switch; the note goes under them and keeps its
	   line when there is a switch, so Home and System start at the same height. */
	@media (max-width: 767px) {
		.home { --home-gap: 26px; --poster-item: 54vw; --title-item: calc((100cqi - var(--shelf-gap)) / 1.6 / .78); }
		.greet { font-size: 28px; }
		.wide-only { display: none; }
		.phone-only { display: flex; }
		.top-row :global(.ui-seg) { flex: 1; }
		.top-row :global(.ui-seg button) { min-height: 44px; }
		.note { order: 3; flex-basis: 100%; font: var(--t-subhead); min-height: 1.35em; }
		.note:empty:not(.pending):not(.hold) { display: none; }
		.go { display: none; }
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

	@media (max-width: 900px) { .setup { grid-template-columns: 1fr; } .steps { box-shadow: inset 0 1px 0 var(--line); } }

	/* High Contrast (forced colors) drops shadows and fills: tiles, the steps box and the ticks get a system border,
	   focus gets a system outline, and the small coloured marks (dots, pips, the foot line, the count) keep their colours. */
	@media (forced-colors: active) {
		.home :global(:focus-visible) { outline: 2px solid Highlight !important; outline-offset: 2px; }
		.tile, .setup, .prompt, .flow, .passed li { border: 1px solid CanvasText; }
		.life, .pb, .tone, .note .dot, .count, .ask-stack b { forced-color-adjust: none; }
		.life:not(.pips), .pb { outline: 1px solid CanvasText; }
	}
</style>

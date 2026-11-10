<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	// Show detail page: episodes by season, staff tools, requests and history.
	import Button from '#lib/ui/Button.svelte';
	import AskedByLine from '#lib/ui/AskedByLine.svelte';
	import { loadAskedBy, type AskedByAnswer } from '#lib/ui/asked-by.ts';
	import TitleHero, { richMetadata, shortFolder, type RichMeta } from '#lib/ui/TitleHero.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import FollowButton from '#lib/ui/FollowButton.svelte';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import ActionMenu from '#lib/ui/ActionMenu.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import TitleCast from '#lib/ui/TitleCast.svelte';
	import TitleRows from '#lib/ui/TitleRows.svelte';
	import type { QuickAskSheet } from '#lib/ui/title-cards.ts';
	import LibrarySection from '#lib/ui/LibrarySection.svelte';
	import { audienceStatus } from '#lib/ui/title-status.ts';
	import TitleSection from '#lib/ui/TitleSection.svelte';
	import '#lib/ui/title-page.css';
	import { RenamePreview, ShowDeleteDialog, ShowReleases, CopyControls, artName, crumbFor, qualityBadges, slotKind, type ReleaseScope } from '../mediafiles/index.ts';
	import { SubtitlePanel } from '../subtitles/index.ts';
	import { groupDownloads, HistoryLine as CompactRow, isRoutine } from '../downloads/index.ts';
	import ShowVersions from './ShowVersions.svelte';
	import EpisodeList, { type EpisodeRow } from './EpisodeList.svelte';
	import SeasonPicker from './SeasonPicker.svelte';
	import {
		detailsSources,
		episodeLabel,
		episodeOrders,
		episodeWhy,
		keepOffNote,
		label,
		mainSlot,
		newSeason,
		newestSeason,
		seasonChoices,
		seasonLine,
		seasonLooking,
		seasonName,
		seasonWords,
		searchWord,
		showProgress,
	} from './labels.ts';
	import type { AskShowSheet } from './types.ts';
	import { tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { peopleNames, decided } from '../core/plain.ts';
	import { to } from '../core/paths.ts';
	import { scrollToStart } from '../core/motion.ts';
	import { api, message } from '../core/api.ts';
	import { live } from '../core/live.svelte.ts';
	import { ago, date, dateTime, tone } from '../core/format.ts';
	import { session } from '../core/session.svelte.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { Ask, EpisodeView, EventPage, EventRecord, PickRulesView, TitleView } from '../core/types.ts';
	import type { LookupShow } from '../core/types.ts';

	let {
		title,
		onchange,
		error = '',
		onretry,
		askSheet: AskDialog,
		quickAsk
	}: { title: TitleView; onchange: (t: TitleView) => void; error?: string; onretry?: () => Promise<void> | void; askSheet: AskShowSheet; quickAsk: QuickAskSheet } = $props();

	let askedBy = $state<AskedByAnswer>({ titles: {} });
	// A failed refresh keeps the old data on screen and shows this beside it.
	let loadErr = $state('');
	let retrying = $state(false);
	let asker: ReturnType<AskShowSheet> | undefined = $state();
	let episodes = $state<Record<number, EpisodeView[]>>({});
	let open = $state<Record<number, boolean>>({});
	let busy = $state('');
	let history = $state<EventRecord[]>([]);
	let historyTotal = $state(0);
	let refreshing = $state(false);
	let renameDialog: HTMLDialogElement | undefined = $state();
	let renameOpen = $state(false);
	let deleter: ReturnType<typeof ShowDeleteDialog> | undefined = $state();
	let subDialog: HTMLDialogElement | undefined = $state();
	let subEpisode = $state<EpisodeView | null>(null);
	let scope = $state<ReleaseScope | null>(null);
	let scopeJobs = $state<number[]>([]);
	let searching = $state('');
	// A season in the address bar wins until one is picked here.
	const askedSeason = Number(page.url.searchParams.get('season'));
	let picked = $state<number | null>(Number.isInteger(askedSeason) && askedSeason > 0 ? askedSeason : null);
	let synopses = $state<Record<number, Record<number, string>>>({});
	let rich = $state<RichMeta | null>(null);
	let editDialog: HTMLDialogElement | undefined = $state();
	let rules = $state<PickRulesView[]>([]);
	let eRules = $state(0);
	let eOrder = $state('aired');
	let eNew = $state('all');
	let eSource = $state('tmdb');
	let editError = $state('');
	let saving = $state(false);
	let again: ReturnType<typeof setTimeout> | undefined;

	const askers = $derived(askedBy.titles[String(title.id)]?.askers ?? []);
	const staff = $derived(session.can('manage_library'));
	const show = $derived(title.show);
	const order = $derived(show?.episode_order ?? 'aired');
	// Routine events stay off this card; the History page lists them all.
	const historyRows = $derived(groupDownloads(history.filter((e) => !isRoutine(e.Kind))).slice(0, 8));
	const historyTitles = $derived(new Map([[title.id, title]]));
	const cur = $derived((picked !== null && show?.seasons.some((x) => x.number === picked) ? picked : null) ?? newestSeason(show?.seasons ?? []) ?? 0);
	const freshSeason = $derived(newSeason(show?.seasons ?? []));
	const curSeason = $derived(show?.seasons.find((x) => x.number === cur));
	const statusWord = (s: string): string => ({ running: t('shows.page.running'), ended: t('shows.page.ended'), announced: t('shows.page.announced') })[s] ?? s;
	const heroMeta = $derived([title.year ? String(title.year) : '', show?.seasons.length ? t(show.seasons.length === 1 ? 'shows.page.seasonCountOne' : 'shows.page.seasonCountOther', { n: show.seasons.length }) : '', show?.network ?? '', show ? statusWord(show.status) : '', ...(title.genres?.length ? title.genres : (rich?.genres ?? [])).slice(0, 2)].filter(Boolean));
	const heroBadges = $derived(qualityBadges(title.slots.map((s) => s.file)));
	const nextWords = $derived(show?.next_air_date ? date(show.next_air_date) : '');
	const heroKind = $derived(show?.missing ? 'missing' : show?.in_library ? 'in_library' : show?.not_out_yet ? 'coming_soon' : 'missing');
	// Staff also see how many episodes are missing.
	const heroWords = $derived(audienceStatus(!staff && show?.in_library ? 'in_library' : heroKind, staff ? 'staff' : 'member', { tracked: true, titleKind: 'show' }));
	const heroItems = $derived<MenuItem[]>([
		{ label: t('shows.search.now'), icon: 'search', onselect: searchShow, disabled: searching === 'show' },
		{ label: t('shows.sheet.keep'), checked: title.keep_up_to_date, disabled: busy === 'show', onselect: () => setKeep(!title.keep_up_to_date) },
		{ kind: 'separator' },
		{ label: t('shows.page.edit'), icon: 'tools', onselect: openEdit },
		{ label: t('shows.page.renameFiles'), icon: 'copy', onselect: openRename },
		{ label: t('shows.page.refreshMenu'), icon: 'refresh', onselect: refresh, disabled: refreshing },
		{ label: t('shows.page.allHistory'), icon: 'clock', href: to(`/downloads/history?title=${title.id}`) },
		{ kind: 'separator' },
		{ label: t('shows.page.deleteShow'), icon: 'trash', destructive: true, onselect: () => deleter?.open() }
	]);

	$effect(() => {
		const id = title?.id;
		if (!id) return;
		loadAskedBy(api.get, [id]).then((x) => (askedBy = x), () => {});
	});
	// Episode states change while searches run, so the open seasons are read again.
	$effect(() => {
		const soon = () => {
			clearTimeout(again);
			again = setTimeout(() => void reload(), 400);
		};
		const offs = [live.on('slot', soon), live.on('download', soon)];
		return () => {
			clearTimeout(again);
			offs.forEach((off) => off());
		};
	});
	$effect(() => {
		void title.id;
		void loadHistory();
	});
	$effect(() => {
		const n = cur;
		const tmdb = title.ids?.tmdb;
		if (!tmdb || synopses[n] || !show?.seasons.some((x) => x.number === n)) return;
		const ctl = new AbortController();
		api.get<{ episodes?: { number: number; overview?: string }[] }>(`/metadata/show/${encodeURIComponent(tmdb)}/seasons/${n}`, ctl.signal)
			.then((r) => (synopses[n] = Object.fromEntries((r.episodes ?? []).filter((e) => e.overview).map((e) => [e.number, e.overview as string]))))
			.catch(() => {
				// Synopses are optional, so the list works without them.
			});
		return () => ctl.abort();
	});
	$effect(() => {
		const n = cur;
		if (!curSeason) return;
		open[n] = true;
		if (!episodes[n]) void loadSeason(n);
	});
	// Only fetches when the title lacks art or genres; the flag drops a late reply after a change.
	$effect(() => {
		const tmdb = title.ids?.tmdb;
		rich = null;
		if (!tmdb || (title?.backdrop && title?.genres?.length)) return;
		let live = true;
		void richMetadata(api.get, 'show', tmdb).then((m) => live && (rich = m));
		return () => (live = false);
	});

	async function retry() {
		retrying = true;
		try {
			await onretry?.();
			await reload();
		} finally {
			retrying = false;
		}
	}

	function askMore() {
		const s: LookupShow = {
			source: title.ids?.tmdb ? 'tmdb' : Object.keys(title.ids ?? {})[0] ?? 'tmdb',
			name: title.name,
			year: title.year || undefined,
			status: title.show?.status ?? '',
			ids: title.ids ?? {},
			seasons: (title.show?.seasons ?? []).map((x) => ({ number: x.number, name: x.name })),
			title_id: title.id
		};
		asker?.open(s);
	}
	function asked(_s: LookupShow, r: { ask: Ask; joined: boolean }) {
		if (r.ask.state === 'approved') toasts.push(t('shows.page.approved', { name: title.name }), 'ok', { body: t('shows.page.searchingRelease') });
		else if (r.joined) toasts.push(t('shows.page.alreadyRequested', { name: title.name }), 'ok', { body: t('shows.page.onRequestToo') });
		else toasts.push(t('shows.page.requested', { name: title.name }), 'ok', { body: t('shows.page.waitingApproval') });
	}

	function openSubtitles(ep: EpisodeView) {
		subEpisode = ep;
		subDialog?.showModal();
	}

	function openRename() {
		renameOpen = true;
		renameDialog?.showModal();
	}

	async function deleted(p: { scope?: string }) {
		if (!p.scope || p.scope === 'title') {
			await goto(to(crumbFor('show').path));
			return;
		}
		episodes = {};
		await reload();
		void loadHistory();
	}

	async function startSearch(key: string, body: Record<string, unknown>, done: string, list?: ReleaseScope) {
		searching = key;
		try {
			if (list) body.choose = false;
			const r = await api.post<{ job_ids: number[] }>(`/titles/${title.id}/search`, body);
			if (list) {
				scope = list;
				scopeJobs = r.job_ids;
				// The release list sits in the Library section, so scroll to it.
				await tick();
				scrollToStart(document.querySelector('[data-testid="show-releases"]'));
			}
			toasts.push(r.job_ids.length ? done : t('shows.page.nothingToSearch'), r.job_ids.length ? 'info' : 'ok');
		} catch (e) {
			toasts.push(t('shows.sheet.startSearchFailed'), 'fail', { body: message(e) });
		} finally {
			// Keeps the busy state visible long enough to read.
			setTimeout(() => (searching = ''), 1200);
		}
	}

	function searchShow() {
		void startSearch('show', {}, t('shows.page.searchingAll'));
	}

	function searchSeason(n: number, list = false) {
		const name = seasonName(n);
		void startSearch(
			`s${n}${list ? 'l' : ''}`,
			{ season: n },
			t('shows.page.searchingNamed', { name }),
			list ? { label: name, query: `season=${n}` } : undefined
		);
	}

	function searchEpisode(ep: EpisodeView, list = false) {
		const sl = mainSlot(ep);
		if (!sl) return;
		const code = episodeLabel(ep, order);
		void startSearch(
			`e${ep.id}${list ? 'l' : ''}`,
			// A release list is only for the main Version.
			!list && ep.slots.length > 1 ? { item_id: ep.id } : { slot_id: sl.id },
			t('shows.page.searchingNamed', { name: code }),
			list ? { label: code, query: `item_id=${ep.id}`, slotId: sl.id, slotState: sl.state } : undefined
		);
	}

	async function reload() {
		try {
			onchange(await api.get<TitleView>(`/titles/${title.id}`));
		} catch (e) {
			loadErr = message(e);
			return;
		}
		loadErr = '';
		for (const n of Object.keys(open).map(Number)) if (open[n]) await loadSeason(n);
	}

	async function loadHistory() {
		if (!staff) return;
		try {
			const h = await api.get<EventPage>(`/events?title=${title.id}&limit=30`);
			history = h.events;
			historyTotal = h.total;
		} catch {
			// The page works without the history card.
		}
	}

	async function loadSeason(n: number) {
		try {
			const r = await api.get<{ episodes: EpisodeView[] }>(`/titles/${title.id}/episodes?season=${n}`);
			episodes[n] = r.episodes;
		} catch (e) {
			loadErr = message(e);
		}
	}

	function pickSeason(n: number) {
		picked = n;
	}

	function episodeItems(ep: EpisodeView): MenuItem[] {
		const code = episodeLabel(ep, order);
		const word = searchWord(mainSlot(ep)?.state);
		const items: MenuItem[] = [];
		if (word) items.push({ label: word, icon: 'search', onselect: () => searchEpisode(ep), disabled: searching === `e${ep.id}` });
		if (mainSlot(ep)) items.push({ label: t('shows.page.chooseRelease'), icon: 'grid', onselect: () => searchEpisode(ep, true), disabled: searching === `e${ep.id}l` });
		if (ep.slots.some((x) => x.file_id)) items.push({ label: t('shows.sheet.subtitles'), icon: 'subtitle', onselect: () => openSubtitles(ep) });
		items.push({ label: t('shows.tools.history'), icon: 'clock', href: to(`/downloads/history?title=${title.id}&episode=${ep.id}`) });
		items.push({ label: t('shows.page.keepNamed', { name: code }), checked: ep.keep_up_to_date, disabled: busy === `e${ep.id}`, onselect: () => setEpisode(ep, !ep.keep_up_to_date) });
		items.push({ kind: 'separator' }, { label: t('shows.tools.delete', { season: code }), icon: 'trash', destructive: true, onselect: () => deleter?.open({ episodeIds: [ep.id], label: code }) });
		return items;
	}

	function toLibrary() {
		const h = document.getElementById('h-library');
		scrollToStart(h?.closest('section'));
	}

	async function setSeason(n: number, keep: boolean) {
		busy = `s${n}`;
		try {
			await api.put(`/titles/${title.id}/seasons/${n}`, { keep_up_to_date: keep });
			toasts.push(t(keep ? 'shows.page.seasonKept' : 'shows.page.seasonNotKept', { name: seasonName(n) }), 'ok');
			await reload();
		} catch (e) {
			toasts.push(t('shows.page.updateSeasonFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	async function setEpisode(ep: EpisodeView, keep: boolean) {
		busy = `e${ep.id}`;
		try {
			const r = await api.put<EpisodeView>(`/episodes/${ep.id}`, { keep_up_to_date: keep });
			const list = episodes[ep.season];
			const i = list?.findIndex((x) => x.id === ep.id) ?? -1;
			if (list && i >= 0) list[i] = { ...list[i], keep_up_to_date: r.keep_up_to_date };
		} catch (e) {
			toasts.push(t('shows.sheet.updateFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	async function setKeep(v: boolean) {
		busy = 'show';
		try {
			onchange((await api.put<{ title: TitleView }>(`/titles/${title.id}`, { keep_up_to_date: v })).title);
			toasts.push(t(v ? 'shows.page.showKept' : 'shows.page.showNotKept'), 'ok');
		} catch (e) {
			toasts.push(t('shows.page.updateShowFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	async function refresh() {
		refreshing = true;
		try {
			await api.post(`/titles/${title.id}/refresh`);
			toasts.push(t('shows.page.refreshing', { name: title.name }), 'info', { body: t('shows.page.refreshingBody') });
		} catch (e) {
			toasts.push(t('shows.page.refreshFailed'), 'fail', { body: message(e) });
		} finally {
			// Keeps the menu item disabled while the refresh job starts.
			setTimeout(() => (refreshing = false), 1500);
		}
	}

	async function openEdit() {
		editError = '';
		eRules = title.pick_rules_id ?? 0;
		eOrder = show?.episode_order ?? 'aired';
		eNew = show?.new_seasons ?? 'all';
		eSource = show?.details_source ?? 'tmdb';
		editDialog?.showModal();
		if (!rules.length) {
			try {
				rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules.filter((r) => !r.kinds.length || r.kinds.includes('show'));
			} catch (e) {
				editError = message(e);
			}
		}
	}

	// Cached episodes are dropped because a new order renumbers them.
	async function saveEdit(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		editError = '';
		try {
			const body: Record<string, unknown> = { episode_order: eOrder, new_seasons: eNew, details_source: eSource };
			if (eRules) body.pick_rules_id = eRules;
			onchange((await api.put<{ title: TitleView }>(`/titles/${title.id}`, body)).title);
			editDialog?.close();
			episodes = {};
			for (const n of Object.keys(open).map(Number)) if (open[n]) void loadSeason(n);
			toasts.push(t('shows.page.saved'), 'ok');
		} catch (err) {
			editError = message(err);
		} finally {
			saving = false;
		}
	}

	const still = (ep: EpisodeView) => ep.still;
	// Members only see a mark on episodes already in the Library.
	const rowsOf = (n: number): EpisodeRow[] =>
		(episodes[n] ?? []).map((ep) => ({
			key: ep.id,
			number: order === 'aired' ? String(ep.episode) : episodeLabel(ep, order),
			name: ep.name || t('shows.page.nameUnknown'),
			still: still(ep),
			runtime: ep.runtime_minutes ?? ep.runtime,
			when: ep.air_date ? date(ep.air_date) : t('shows.why.noAirDateShort'),
			overview: synopses[n]?.[ep.episode],
			here: !staff && ep.slots.some((x) => x.file_id)
		}));
	const seasonMenu = (n: number, keep: boolean): MenuItem[] => [
		{ label: t('shows.page.searchSeason'), icon: 'search', onselect: () => searchSeason(n), disabled: searching === `s${n}` },
		{ label: t('shows.page.chooseRelease'), icon: 'grid', onselect: () => searchSeason(n, true), disabled: searching === `s${n}l` },
		{ label: t('shows.page.keepNamed', { name: seasonName(n) }), checked: keep, disabled: busy === `s${n}`, onselect: () => setSeason(n, !keep) },
		{ kind: 'separator' },
		{ label: t('shows.tools.delete', { season: seasonName(n) }), icon: 'trash', destructive: true, onselect: () => deleter?.open({ season: n, label: seasonName(n) }) }
	];
</script>

<div class="tp">
<TitleHero
	title={title.name}
	kind="show"
	tmdb={title.ids?.tmdb}
	poster={title.artwork[0] ?? rich?.poster}
	backdrop={title.backdrop ?? rich?.backdrop}
	backdrops={rich?.backdrops}
	logo={title.logo ?? rich?.logo}
	crumbIcon="tv"
	breadcrumb={[{ href: to(crumbFor('show').path), label: crumbFor('show').label }, { href: to(`/library/${title.id}`), label: title.year ? `${title.name} (${title.year})` : title.name }]}
	meta={heroMeta}
	certification={title.rating ?? rich?.certification}
	score={rich?.score}
	badges={heroBadges}
	status={heroWords.kind}
	statusLabel={staff && show?.missing ? `${show.missing} missing` : heroWords.label}
	description={title.overview || rich?.overview}
	vtName={artName(title.id)}
>
	{#snippet byline()}
		<AskedByLine {askers} />
		{#if show?.next_air_date}<span data-testid="next-episode"><b>{t('shows.page.nextEpisode')}</b> {nextWords}</span>{/if}
		{#if staff}<span class="tp-progress" data-testid="show-progress">{showProgress(show)}</span>{/if}
	{/snippet}
	{#snippet actions()}
		{#if staff}
			<ActionButton icon="library" label={t('shows.page.goLibrary')} caption={t('shows.page.inLibraryCaption')} primary onclick={toLibrary} data-testid="hero-library" />
			<ActionMenu label={t('shows.page.moreActions')} caption={t('shows.page.more')} icon="more" items={heroItems} />
		{:else if session.can('ask')}
			<ActionButton icon="wish" label={t('shows.page.askSeasons')} primary onclick={askMore} data-testid="ask-seasons-more" />
		{/if}
		<FollowButton titleId={title.id} kind="show" />
	{/snippet}
</TitleHero>
{#if !staff && session.can('ask')}<AskDialog bind:this={asker} onasked={asked} />{/if}

{#if loadErr || error}<LoadError hasData message={loadErr || error} onretry={() => void retry()} busy={retrying} />{/if}

<section class="tp-section" aria-labelledby="h-seasons">
	<div class="tp-head">
		<h2 id="h-seasons" class="tp-h2">{t('shows.list.episodes')}</h2>
		{#if show?.seasons.length}
			<div class="tp-head-end">
				<SeasonPicker seasons={show.seasons} value={cur} fresh={freshSeason} onpick={pickSeason} />
				{#if staff && curSeason}
					<span data-testid="season-menu"><Menu label={t('shows.page.seasonActions', { name: seasonName(curSeason.number) })} icon="more" items={seasonMenu(curSeason.number, curSeason.keep_up_to_date)} width={260} align="end" /></span>
				{/if}
			</div>
		{/if}
	</div>
	{#if !show?.seasons.length}
		<p class="small muted">{t('shows.page.noSeasons')}</p>
	{:else if curSeason}
		{@const s = curSeason}
		{@const off = staff ? keepOffNote(s.keep_up_to_date, title.keep_up_to_date) : ''}
		<div class="season" data-testid="season">
			<p class="xs muted" data-testid="season-line">{seasonLine(s)}{s.dropped ? ` · ${t('shows.page.noLongerListed')}` : ''}{#if staff && episodes[s.number] && seasonLooking(episodes[s.number])}{` · ${t('shows.page.looking', { n: seasonLooking(episodes[s.number]) })}`}{/if}{#if off}{` · ${off}`}{/if}</p>
			{#if episodes[s.number]}
				{@const eps = episodes[s.number]}
				<EpisodeList rows={rowsOf(s.number)} fallback={title.backdrop ?? rich?.backdrop}>
					{#snippet status(i: number)}
						{@const main = mainSlot(eps[i])}
						{#if staff && main}<StatusPill kind={slotKind(main.state)} label={eps[i].slots.length > 1 ? eps[i].slots.map((x) => `${x.version}: ${serverWords(x.state_label)}`).join(' · ') : serverWords(main.state_label)} testid="episode-state" />{/if}
					{/snippet}
					{#snippet action(i: number)}
						{#if staff}<Menu label={t('shows.page.episodeActions', { name: episodeLabel(eps[i], order) })} items={episodeItems(eps[i])} width={250} />{/if}
					{/snippet}
					{#snippet after(i: number)}
						{@const ep = eps[i]}
						{@const why = staff ? episodeWhy(ep, s.keep_up_to_date, title.keep_up_to_date) : ''}
						{#if why && why !== off}<p class="xs muted ep-why" data-testid="episode-why">{why}</p>{/if}
						{#if staff}
							{#each ep.slots.filter((x) => x.reason && ep.slots.length > 1) as sl (sl.id)}
								<p class="xs muted ep-why">{sl.version}: {sl.reason}</p>
							{/each}
						{/if}
					{/snippet}
				</EpisodeList>
			{:else}
				<div class="episodes" aria-busy="true">{#each [0, 1, 2] as i (i)}<Skeleton lines={2} />{/each}</div>
			{/if}
		</div>
	{/if}
</section>

<TitleRows kind="show" tmdb={title.ids?.tmdb} {quickAsk}>
	{#snippet middle()}{#key title.id}<TitleCast titleId={title.id} />{/key}{/snippet}
</TitleRows>

<TitleSection id="h-about" title={t('shows.page.about')}>
	<dl class="tp-about">
		<dt>{t('shows.list.episodes')}</dt>
		<dd>{t('shows.page.episodesIn', { episodes: show?.episodes ?? 0, seasons: show?.seasons.length ?? 0 })}</dd>
		{#if title.year}<dt>{t('shows.page.firstAired')}</dt><dd>{title.year}</dd>{/if}
		{#if show?.next_air_date}<dt>{t('shows.page.nextEpisodeRow')}</dt><dd>{date(show.next_air_date)}</dd>{/if}
		{#if show?.air_time}<dt>{t('shows.page.airsAt')}</dt><dd>{show.air_time}{show.time_zone ? ` (${show.time_zone})` : ''}</dd>{/if}
		{#if show?.network}<dt>{t('shows.page.network')}</dt><dd>{show.network}</dd>{/if}
		{#if show?.status}<dt>{t('shows.page.status')}</dt><dd>{statusWord(show.status)}</dd>{/if}
		{#if title.rating ?? rich?.certification}<dt>{t('shows.page.rated')}</dt><dd>{title.rating ?? rich?.certification}</dd>{/if}
		{#if title.genres?.length}<dt>{t('shows.page.genres')}</dt><dd>{title.genres.join(', ')}</dd>{/if}
	</dl>
</TitleSection>

{#if title.asks?.length}
	<TitleSection id="h-show-asks" title={t('shows.page.requests')} testid="title-asks">
		{#each title.asks as a (a.id)}
			<dl class="facts">
				<dt>{t('shows.page.askedBy')}</dt>
				<dd>{peopleNames(a.people.map((p) => p.name))} · {date(a.asked_at)}</dd>
				<dt>{t('shows.page.for')}</dt>
				<dd>{a.season_words ?? seasonWords(a.seasons)}</dd>
				{#if decided(a)}<dt>{t('shows.page.decided')}</dt>
					<dd>{decided(a)}{a.decided_at ? ` · ${date(a.decided_at)}` : ''}</dd>{/if}
				<dt>{t('shows.page.now')}</dt>
				<dd>{serverWords(a.state_label)}</dd>
				{#if a.note}<dt>{t('shows.page.note')}</dt>
					<dd>{a.note}</dd>{/if}
			</dl>
		{/each}
	</TitleSection>
{/if}

{#if staff}
	<LibrarySection note={t('shows.page.librarySectionNote')}>
		{#if scope}
			<ShowReleases titleId={title.id} {scope} jobs={scopeJobs} onclose={() => (scope = null)} onchange={() => void reload()} />
		{/if}
		<ShowVersions {title} {staff} onchange={reload} />
		<CopyControls {title} onchange={reload} />
		<div id="title-subtitles">{#key title.id}<SubtitlePanel titleId={title.id} movie={false} files={false} />{/key}</div>
		<TitleSection id="h-show-facts" title={t('shows.page.keeps')}>
			<dl class="tp-about">
				<dt>{t('shows.sheet.keep')}</dt>
				<dd>{title.keep_up_to_date ? t('shows.page.on') : t('shows.sheet.off')}</dd>
				<dt>{t('shows.page.episodeOrder')}</dt>
				<dd>{label(episodeOrders, show?.episode_order)}</dd>
				<dt>{t('shows.page.detailsFrom')}</dt>
				<dd>{label(detailsSources, show?.details_source)}</dd>
				{#if show?.season_choice}<dt>{t('shows.page.pickedWhenAdded')}</dt>
					<dd>{label(seasonChoices, show.season_choice)}</dd>{/if}
				<dt>{t('shows.page.newSeasons')}</dt>
				<dd>{show?.new_seasons === 'none' ? t('shows.page.leftOff') : t('shows.page.keptUp')}</dd>
				{#if title.pick_rules_name}<dt>{t('shows.page.pickRules')}</dt>
					<dd>{title.pick_rules_name}</dd>{/if}
				{#if title.refreshed_at}<dt>{t('shows.page.refreshed')}</dt>
					<dd>{date(title.refreshed_at)}</dd>{/if}
			</dl>
			<p class="tp-why" data-testid="gate-reason">{title.gate_reason}</p>
			{#if title.folder_path}<details class="tp-details"><summary>{t('shows.versions.details')}</summary><ul><li>{t('shows.page.folder', { path: shortFolder(title.folder_path) })}</li></ul></details>{/if}
		</TitleSection>
		<TitleSection id="h-show-history" title={t('shows.tools.history')} testid="title-history">
			{#snippet head()}
				{#if historyTotal > 0}<Button variant="plain" href={to(`/downloads/history?title=${title.id}`)}>{t('shows.page.allRows', { n: historyTotal })}</Button>{/if}
			{/snippet}
			{#if historyRows.length === 0}
				<p class="small muted">{t('shows.sheet.nothingYet')}</p>
			{:else}
				<div class="hist">
					{#each historyRows as r (r.key)}
						<CompactRow row={r} ctx={{ titles: historyTitles }} titleById={historyTitles} onclick={() => goto(to(`/downloads/history?title=${title.id}`))} />
					{/each}
				</div>
			{/if}
		</TitleSection>
	</LibrarySection>
{/if}
</div>

{#if staff}
	<ShowDeleteDialog bind:this={deleter} titleId={title.id} name={title.name} ondone={deleted} />
{/if}

<dialog bind:this={subDialog} class="modal wide" aria-label={t('shows.page.episodeSubsLabel')} onclose={() => (subEpisode = null)} data-testid="episode-subtitles-dialog">
	{#if subEpisode}
		<SubtitlePanel titleId={title.id} movie={false} itemId={subEpisode.id} label={episodeLabel(subEpisode, order)} />
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => subDialog?.close()}>{t('shows.page.close')}</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={renameDialog} aria-labelledby="rename-show-title" class="modal wide" onclose={() => (renameOpen = false)}>
	{#if renameOpen}
		<h2 id="rename-show-title" class="h2">{t('shows.page.renameTitle', { name: title.name })}</h2>
		<p class="small muted">{t('shows.page.renameNote')}</p>
		<RenamePreview titleId={title.id} onDone={() => void reload()} />
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => renameDialog?.close()}>{t('shows.page.close')}</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={editDialog} aria-labelledby="edit-show-title" class="modal">
	<form onsubmit={saveEdit}>
		<h2 id="edit-show-title" class="h2">{t('shows.page.editTitle', { name: title.name })}</h2>
		<div class="field" style="margin-top: var(--s-5)">
			<label for="e-rules">{t('shows.page.pickRules')}{title.versions.length > 1 ? ` ${t('shows.page.mainVersionParen')}` : ''}</label>
			<select id="e-rules" class="input" bind:value={eRules}>
				{#each rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
			</select>
		</div>
		<div class="field">
			<label for="e-order">{t('shows.page.episodeOrder')}</label>
			<select id="e-order" class="input" bind:value={eOrder}>
				{#each episodeOrders as [v, l] (v)}<option value={v}>{l}</option>{/each}
			</select>
			<span class="hint">{episodeOrders.find(([v]) => v === eOrder)?.[2]}</span>
		</div>
		<div class="field">
			<label for="e-new">{t('shows.page.newSeasons')}</label>
			<select id="e-new" class="input" bind:value={eNew}>
				<option value="all">{t('shows.page.keepThem')}</option>
				<option value="none">{t('shows.page.leaveThem')}</option>
			</select>
		</div>
		<div class="field">
			<label for="e-source">{t('shows.page.detailsFrom')}</label>
			<select id="e-source" class="input" bind:value={eSource}>
				{#each detailsSources as [v, l] (v)}<option value={v}>{l}</option>{/each}
			</select>
			<span class="hint">{t('shows.page.filesStay')}</span>
		</div>
		{#if editError}<p class="form-error" role="alert">{editError}</p>{/if}
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => editDialog?.close()}>{t('shows.versions.cancel')}</Button>
			<Button variant="primary" type="submit" disabled={saving} busy={saving}>{t('shows.versions.save')}</Button>
		</div>
	</form>
</dialog>

<style>
	.tp-progress { color: var(--text-muted); }
	.episodes { display: flex; flex-direction: column; gap: 12px; }
	.ep-why { margin: 2px 0 0; }
	/* Native dialog styled as a modal card. */
	.modal { width: min(480px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: var(--s-6); border: 0; border-radius: var(--r-modal); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); }
	.modal.wide { width: min(640px, calc(100vw - 32px)); }
	.modal::backdrop { background: var(--scrim); }
	.modal-actions { justify-content: flex-end; margin-top: var(--s-6); }
	.hist { display: grid; gap: 2px; }
</style>

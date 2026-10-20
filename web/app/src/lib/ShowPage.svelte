<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { peopleNames } from './plain.ts';
	import { to } from './paths.ts';
	// One show: its seasons and episodes, Keep up to date per show, season
	// and episode, air dates and why each episode is or is not looked for;
	// Search now and Choose a release for the show, a season or an episode.
	import { goto } from '$app/navigation';
	import { tick } from 'svelte';
	import { reducedMotion } from './motion.ts';
	import { api, message } from './api.ts';
	import RenamePreview from './RenamePreview.svelte';
	import ShowDeleteDialog from './ShowDeleteDialog.svelte';
	import { live } from './live.svelte.ts';
	import ShowReleases from './ShowReleases.svelte';
	import ShowVersions from './ShowVersions.svelte';
	import CopyControls from './CopyControls.svelte';
	import SubtitlePanel from './SubtitlePanel.svelte';
	import { ago, date, dateTime, tone } from './format.ts';
	import AskedByLine from './ui/AskedByLine.svelte';
	import { loadAskedBy, type AskedByAnswer } from './askedby.ts';
	import TitleHero, { richMetadata, shortFolder, type RichMeta } from './ui/TitleHero.svelte';
	import LoadError from './ui/LoadError.svelte';
	import ActionButton from './ui/ActionButton.svelte';
	import ActionMenu from './ui/ActionMenu.svelte';
	import Menu, { type MenuItem } from './ui/Menu.svelte';
	import SegmentedControl from './ui/SegmentedControl.svelte';
	import WideCard from './ui/WideCard.svelte';
	import Skeleton from './ui/Skeleton.svelte';
	import TitleCast from './TitleCast.svelte';
	import TitleRows from './TitleRows.svelte';
	import LibrarySection from './ui/LibrarySection.svelte';
	import type { CardOwned } from './ui/CardBadges.svelte';
	import { artName, crumbFor, pageSections, qualityBadges, slotKind } from './title-page.ts';
	import { audienceStatus } from './title-status.ts';
	import SectionNav from './ui/SectionNav.svelte';
	import TitleSection from './ui/TitleSection.svelte';
	import './title-page.css';
	import { session } from './session.svelte.ts';
	import { toasts } from './toasts.svelte.ts';
	import { decided } from './wishlist.ts';
	import { groupLabel, kindTone } from './history.ts';
	import {
		detailsSources,
		episodeLabel,
		episodeOrders,
		episodeWhy,
		label,
		mainSlot,
		seasonChoices,
		seasonLine,
		seasonLooking,
		seasonName,
		seasonWords,
		searchWord,
		showProgress,
		type ReleaseScope
	} from './shows.ts';
	import type { EpisodeView, EventPage, EventRecord, PickRulesView, TitleView } from './types.ts';

	let {
		title,
		onchange,
		error = '',
		onretry
	}: { title: TitleView; onchange: (t: TitleView) => void; error?: string; onretry?: () => Promise<void> | void } = $props();

	// Who asked, as the server lets this reader see it.
	let askedBy = $state<AskedByAnswer>({ titles: {}, sees_all: false });
	$effect(() => {
		const id = title?.id;
		if (!id) return;
		loadAskedBy(api.get, [id]).then((x) => (askedBy = x), () => {});
	});
	const askers = $derived(askedBy.titles[String(title.id)]?.askers ?? []);

	// A failed refresh keeps what is shown and says so above it.
	let loadErr = $state('');
	let retrying = $state(false);
	async function retry() {
		retrying = true;
		try {
			await onretry?.();
			await reload();
		} finally {
			retrying = false;
		}
	}

	const staff = $derived(session.can('manage_library'));
	const show = $derived(title.show);
	const order = $derived(show?.episode_order ?? 'aired');
	let episodes = $state<Record<number, EpisodeView[]>>({});
	let open = $state<Record<number, boolean>>({});
	let busy = $state('');
	let history = $state<EventRecord[]>([]);
	let historyTotal = $state(0);
	let refreshing = $state(false);

	const statusWord: Record<string, string> = { running: 'Still running', ended: 'Ended', announced: 'Announced' };

	// Rename files and delete (5.2.4): the rename preview for this show, and
	// the delete dialog for the show, a season or an episode.
	let renameDialog: HTMLDialogElement | undefined = $state();
	let renameOpen = $state(false);
	let deleter: ReturnType<typeof ShowDeleteDialog> | undefined = $state();

	// Subtitles of one episode, in a dialog from its row.
	let subDialog: HTMLDialogElement | undefined = $state();
	let subEpisode = $state<EpisodeView | null>(null);

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
			await goto(to('/library'));
			return;
		}
		episodes = {};
		await reload();
		void loadHistory();
	}

	// Searching (stage 3.2): Search now picks by itself; Choose a release
	// lists every Release for an episode or a season.
	let scope = $state<ReleaseScope | null>(null);
	let scopeJobs = $state<number[]>([]);
	let searching = $state('');

	async function startSearch(key: string, body: Record<string, unknown>, done: string, list?: ReleaseScope) {
		searching = key;
		try {
			if (list) body.choose = false;
			const r = await api.post<{ job_ids: number[] }>(`/titles/${title.id}/search`, body);
			if (list) {
				scope = list;
				scopeJobs = r.job_ids;
				// The list opens at the top of the Library section: bring it into view.
				await tick();
				document.querySelector('[data-testid="show-releases"]')?.scrollIntoView({ behavior: reducedMotion() ? 'auto' : 'smooth', block: 'start' });
			}
			toasts.push(r.job_ids.length ? done : 'Nothing here is looking for a copy right now.', r.job_ids.length ? 'ok' : 'info');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			setTimeout(() => (searching = ''), 1200);
		}
	}

	function searchShow() {
		void startSearch('show', {}, 'Searching every season with an episode still looking. HolyArrs picks the best Releases by itself.');
	}

	function searchSeason(n: number, list = false) {
		const name = seasonName(n);
		void startSearch(
			`s${n}${list ? 'l' : ''}`,
			{ season: n },
			list ? `Searching ${name}. Pick a Release from the list once your Sources answer.` : `Searching ${name}. A season pack is taken when the whole season is missing.`,
			list ? { label: name, query: `season=${n}` } : undefined
		);
	}

	function searchEpisode(ep: EpisodeView, list = false) {
		const sl = mainSlot(ep);
		if (!sl) return;
		const code = episodeLabel(ep, order);
		void startSearch(
			`e${ep.id}${list ? 'l' : ''}`,
			// Search now looks for every Version's copy; a list is for the main Version.
			!list && ep.slots.length > 1 ? { item_id: ep.id } : { slot_id: sl.id },
			list ? `Searching ${code}. Pick a Release from the list once your Sources answer.` : `Searching ${code}. HolyArrs picks the best Release by itself.`,
			list ? { label: code, query: `item_id=${ep.id}`, slotId: sl.id, slotState: sl.state } : undefined
		);
	}

	// Episode states move while searches run: read the open seasons again.
	let again: ReturnType<typeof setTimeout> | undefined;
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
			const h = await api.get<EventPage>(`/events?title=${title.id}&limit=8`);
			history = h.events;
			historyTotal = h.total;
		} catch {
			// History is a side card; the page works without it.
		}
	}

	$effect(() => {
		void title.id;
		void loadHistory();
	});

	async function loadSeason(n: number) {
		try {
			const r = await api.get<{ episodes: EpisodeView[] }>(`/titles/${title.id}/episodes?season=${n}`);
			episodes[n] = r.episodes;
		} catch (e) {
			loadErr = message(e);
		}
	}

	// The season on screen: the first regular season until one is picked.
	let picked = $state<number | null>(null);
	const cur = $derived(picked ?? show?.seasons.find((x) => x.number > 0)?.number ?? show?.seasons[0]?.number ?? 0);
	const curSeason = $derived(show?.seasons.find((x) => x.number === cur));
	$effect(() => {
		const n = cur;
		if (!curSeason) return;
		open[n] = true;
		if (!episodes[n]) void loadSeason(n);
	});
	function pickSeason(n: number) {
		picked = n;
	}
	const seasonItems = $derived<MenuItem[]>((show?.seasons ?? []).map((x) => ({ label: x.name || seasonName(x.number), checked: x.number === cur, onselect: () => pickSeason(x.number) })));

	function episodeItems(ep: EpisodeView): MenuItem[] {
		const code = episodeLabel(ep, order);
		const word = searchWord(mainSlot(ep)?.state);
		const items: MenuItem[] = [];
		if (word) items.push({ label: word, icon: 'search', onselect: () => searchEpisode(ep), disabled: searching === `e${ep.id}` });
		if (mainSlot(ep)) items.push({ label: 'Choose a release', icon: 'grid', onselect: () => searchEpisode(ep, true), disabled: searching === `e${ep.id}l` });
		if (ep.slots.some((x) => x.file_id)) items.push({ label: 'Subtitles', icon: 'subtitle', onselect: () => openSubtitles(ep) });
		items.push({ label: 'History', icon: 'clock', href: to(`/downloads/history?title=${title.id}&episode=${ep.id}`) });
		items.push({ label: `Keep ${code} up to date`, checked: ep.keep_up_to_date, disabled: busy === `e${ep.id}`, onselect: () => setEpisode(ep, !ep.keep_up_to_date) });
		items.push({ kind: 'separator' }, { label: `Delete ${code}`, icon: 'trash', destructive: true, onselect: () => deleter?.open({ episodeIds: [ep.id], label: code }) });
		return items;
	}

	// Richer art and facts from the metadata endpoint, when the backend has it.
	let rich = $state<RichMeta | null>(null);
	$effect(() => {
		const tmdb = title.ids?.tmdb;
		rich = null;
		if (!tmdb || (title?.backdrop && title?.genres?.length)) return;
		let live = true;
		void richMetadata(api.get, 'show', tmdb).then((m) => live && (rich = m));
		return () => (live = false);
	});
	const heroMeta = $derived([title.year ? String(title.year) : '', show?.seasons.length ? `${show.seasons.length} ${show.seasons.length === 1 ? 'season' : 'seasons'}` : '', show?.network ?? '', show ? (statusWord[show.status] ?? show.status) : '', ...(title.genres?.length ? title.genres : (rich?.genres ?? [])).slice(0, 2)].filter(Boolean));
	const heroBadges = $derived(qualityBadges(title.slots.map((s) => s.file)));
	// Per season (managers): search it, pick a Release, keep it up to date, delete it.
	const seasonMenu = (n: number, keep: boolean): MenuItem[] => [
		{ label: 'Search season', icon: 'search', onselect: () => searchSeason(n), disabled: searching === `s${n}` },
		{ label: 'Choose a release', icon: 'grid', onselect: () => searchSeason(n, true), disabled: searching === `s${n}l` },
		{ label: `Keep ${seasonName(n)} up to date`, checked: keep, disabled: busy === `s${n}`, onselect: () => setSeason(n, !keep) },
		{ kind: 'separator' },
		{ label: `Delete ${seasonName(n)}`, icon: 'trash', destructive: true, onselect: () => deleter?.open({ season: n, label: seasonName(n) }) }
	];
	/** A requester's mark on an episode card: only whether it is here to watch. */
	const episodeMark = (ep: EpisodeView): CardOwned | null => (ep.slots.some((x) => x.file_id) ? { state: 'available', label: 'In your Library' } : null);
	const nextWords = $derived(show?.next_air_date ? date(show.next_air_date) : '');
	function toLibrary() {
		const h = document.getElementById('h-library');
		h?.closest('section')?.scrollIntoView({ behavior: reducedMotion() ? 'auto' : 'smooth', block: 'start' });
	}
	/** An episode's still, when the backend sends one. */
	const still = (ep: EpisodeView) => ep.still;
	const heroKind = $derived(show?.missing ? 'missing' : show?.in_library ? 'in_library' : show?.not_out_yet ? 'coming_soon' : 'missing');
	// One status vocabulary (title-status.ts); staff also see how many episodes are missing.
	const heroWords = $derived(audienceStatus(!staff && show?.in_library ? 'in_library' : heroKind, staff ? 'staff' : 'member', { tracked: true, titleKind: 'show' }));
	// Manager actions live in the hero's ⋯ menu; the Library section holds the rest.
	const heroItems = $derived<MenuItem[]>([
		{ label: 'Search now', icon: 'search', onselect: searchShow, disabled: searching === 'show' },
		{ label: 'Keep up to date', checked: title.keep_up_to_date, disabled: busy === 'show', onselect: () => setKeep(!title.keep_up_to_date) },
		{ kind: 'separator' },
		{ label: 'Edit', icon: 'tools', onselect: openEdit },
		{ label: 'Rename files', icon: 'copy', onselect: openRename },
		{ label: 'Refresh seasons and episodes', icon: 'refresh', onselect: refresh, disabled: refreshing },
		{ label: 'All history', icon: 'clock', href: to(`/downloads/history?title=${title.id}`) },
		{ kind: 'separator' },
		{ label: 'Delete show', icon: 'trash', destructive: true, onselect: () => deleter?.open() }
	]);

	async function setSeason(n: number, keep: boolean) {
		busy = `s${n}`;
		try {
			await api.put(`/titles/${title.id}/seasons/${n}`, { keep_up_to_date: keep });
			toasts.push(`${seasonName(n)}: ${keep ? 'HolyArrs keeps it up to date.' : 'HolyArrs stops looking for it.'}`, 'ok');
			await reload();
		} catch (e) {
			toasts.push(message(e), 'fail');
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
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}

	async function setKeep(v: boolean) {
		busy = 'show';
		try {
			onchange((await api.put<{ title: TitleView }>(`/titles/${title.id}`, { keep_up_to_date: v })).title);
			toasts.push(v ? 'HolyArrs keeps this show up to date.' : 'HolyArrs stops looking for this show.', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}

	async function refresh() {
		refreshing = true;
		try {
			await api.post(`/titles/${title.id}/refresh`);
			toasts.push('Refreshing seasons, episodes and air dates.', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			setTimeout(() => (refreshing = false), 1500);
		}
	}

	// The edit dialog
	let editDialog: HTMLDialogElement | undefined = $state();
	let rules = $state<PickRulesView[]>([]);
	let eRules = $state(0);
	let eOrder = $state('aired');
	let eNew = $state('all');
	let eSource = $state('tmdb');
	let editError = $state('');
	let saving = $state(false);

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
			toasts.push('Saved.', 'ok');
		} catch (err) {
			editError = message(err);
		} finally {
			saving = false;
		}
	}
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
		<AskedByLine {askers} seesAll={askedBy.sees_all} />
		{#if show?.next_air_date}<span data-testid="next-episode"><b>Next episode:</b> {nextWords}</span>{/if}
		{#if staff}<span class="tp-progress" data-testid="show-progress">{showProgress(show)}</span>{/if}
	{/snippet}
	{#snippet actions()}
		{#if staff}
			<ActionButton icon="library" label="In Library: go to the Library section" caption="In Library" primary onclick={toLibrary} data-testid="hero-library" />
			<ActionMenu label="More actions" caption="More" icon="more" items={heroItems} />
		{/if}
	{/snippet}
</TitleHero>

{#if loadErr || error}<LoadError hasData message={loadErr || error} onretry={() => void retry()} busy={retrying} />{/if}

<SectionNav items={pageSections('show', { manage: staff })} />

<section class="tp-section" aria-labelledby="h-seasons">
	<div class="tp-head">
		<h2 id="h-seasons" class="tp-h2">Episodes</h2>
		{#if show?.seasons.length}
			<div class="tp-head-end">
				{#if show.seasons.length <= 6}
					<SegmentedControl label="Season" options={show.seasons.map((s) => ({ value: String(s.number), label: s.name || seasonName(s.number) }))} value={String(cur)} onchange={(v) => pickSeason(Number(v))} />
				{:else}
					<Menu label="Pick a season" variant="button" icon="tv" text={curSeason ? curSeason.name || seasonName(curSeason.number) : 'Season'} align="end" items={seasonItems} />
				{/if}
				{#if staff && curSeason}
					<span data-testid="season-menu"><Menu label="Season actions for {seasonName(curSeason.number)}" icon="more" items={seasonMenu(curSeason.number, curSeason.keep_up_to_date)} width={260} align="end" /></span>
				{/if}
			</div>
		{/if}
	</div>
	{#if !show?.seasons.length}
		<p class="small muted">No seasons yet. A refresh adds them once the details source lists them.</p>
	{:else if curSeason}
		{@const s = curSeason}
		<div class="season" data-testid="season">
			<p class="xs muted" data-testid="season-line">{seasonLine(s)}{s.dropped ? ' · no longer listed' : ''}{#if staff && episodes[s.number] && seasonLooking(episodes[s.number])} · {seasonLooking(episodes[s.number])} looking for a copy{/if}</p>
			{#if episodes[s.number]}
				<ul class="episodes">
					{#each episodes[s.number] as ep (ep.id)}
						{@const why = staff ? episodeWhy(ep, s.keep_up_to_date, title.keep_up_to_date) : ''}
						{@const main = mainSlot(ep)}
						<li class="episode" data-testid="episode">
							<WideCard
								title={ep.name || 'Name not known yet'}
								image={still(ep) ?? title.backdrop ?? rich?.backdrop}
								kind="episode"
								subtitle="{episodeLabel(ep, order)} · {ep.air_date ? date(ep.air_date) : 'No air date yet'}"
								status={staff && main ? slotKind(main.state) : undefined}
								statusLabel={staff ? (ep.slots.length > 1 ? ep.slots.map((x) => `${x.version}: ${x.state_label}`).join(' · ') : main?.state_label) : undefined}
								statusTestid="episode-state"
								owned={staff ? null : episodeMark(ep)}
							>
								{#snippet action()}
									{#if staff}<span class="ep-menu"><Menu label="Episode actions for {episodeLabel(ep, order)}" items={episodeItems(ep)} width={250} /></span>{/if}
								{/snippet}
							</WideCard>
							{#if why}<p class="xs muted ep-why" data-testid="episode-why">{why}</p>{/if}
							{#if staff}
								{#each ep.slots.filter((x) => x.reason && ep.slots.length > 1) as sl (sl.id)}
									<p class="xs muted ep-why">{sl.version}: {sl.reason}</p>
								{/each}
							{/if}
						</li>
					{:else}
						<li class="small muted">No episodes listed yet.</li>
					{/each}
				</ul>
			{:else}
				<div class="episodes" aria-busy="true">{#each [0, 1, 2] as i (i)}<Skeleton lines={2} />{/each}</div>
			{/if}
		</div>
	{/if}
</section>

<TitleRows kind="show" tmdb={title.ids?.tmdb} dates={[['Next episode', show?.next_air_date ? date(show.next_air_date) : ''], ['Airs at', show?.air_time ? `${show.air_time}${show.time_zone ? ` (${show.time_zone})` : ''}` : ''], ['First aired', title.year ? String(title.year) : '']]}>
	{#snippet middle()}{#key title.id}<TitleCast titleId={title.id} />{/key}{/snippet}
</TitleRows>

<TitleSection id="h-about" title="About">
	<dl class="tp-about">
		<dt>Episodes</dt>
		<dd>{show?.episodes ?? 0} in {show?.seasons.length ?? 0} seasons</dd>
		{#if show?.network}<dt>Network</dt><dd>{show.network}</dd>{/if}
		{#if show?.status}<dt>Status</dt><dd>{statusWord[show.status] ?? show.status}</dd>{/if}
		{#if title.rating ?? rich?.certification}<dt>Rated</dt><dd>{title.rating ?? rich?.certification}</dd>{/if}
		{#if title.genres?.length}<dt>Genres</dt><dd>{title.genres.join(', ')}</dd>{/if}
	</dl>
</TitleSection>

{#if title.asks?.length}
	<TitleSection id="h-show-asks" title="Wishlist" testid="title-asks">
		{#each title.asks as a (a.id)}
			<dl class="facts">
				<dt>Asked by</dt>
				<dd>{peopleNames(a.people.map((p) => p.name))} · {date(a.asked_at)}</dd>
				<dt>For</dt>
				<dd>{a.season_words ?? seasonWords(a.seasons)}</dd>
				{#if decided(a)}<dt>Decided</dt>
					<dd>{decided(a)}{a.decided_at ? ` · ${date(a.decided_at)}` : ''}</dd>{/if}
				<dt>Now</dt>
				<dd>{a.state_label}</dd>
				{#if a.note}<dt>Note</dt>
					<dd>{a.note}</dd>{/if}
			</dl>
		{/each}
	</TitleSection>
{/if}

{#if staff}
	<LibrarySection note="Releases, Versions and files, subtitles, settings and history">
		{#if scope}
			<ShowReleases titleId={title.id} {scope} jobs={scopeJobs} onclose={() => (scope = null)} onchange={() => void reload()} />
		{/if}
		<ShowVersions {title} {staff} onchange={reload} />
		<CopyControls {title} onchange={reload} />
		<div id="title-subtitles">{#key title.id}<SubtitlePanel titleId={title.id} movie={false} files={false} />{/key}</div>
		<TitleSection id="h-show-facts" title="How HolyArrs keeps it">
			<dl class="tp-about">
				<dt>Keep up to date</dt>
				<dd>{title.keep_up_to_date ? 'On' : 'Off'}</dd>
				<dt>Episode order</dt>
				<dd>{label(episodeOrders, show?.episode_order)}</dd>
				<dt>Details from</dt>
				<dd>{label(detailsSources, show?.details_source)}</dd>
				{#if show?.season_choice}<dt>Picked when added</dt>
					<dd>{label(seasonChoices, show.season_choice)}</dd>{/if}
				<dt>New seasons</dt>
				<dd>{show?.new_seasons === 'none' ? 'Left off' : 'Kept up to date'}</dd>
				{#if title.pick_rules_name}<dt>Pick rules</dt>
					<dd>{title.pick_rules_name}</dd>{/if}
				{#if title.refreshed_at}<dt>Refreshed</dt>
					<dd>{date(title.refreshed_at)}</dd>{/if}
			</dl>
			<p class="tp-why" data-testid="gate-reason">{title.gate_reason}</p>
			{#if title.folder_path}<details class="tp-details"><summary>Details</summary><ul><li>Folder: {shortFolder(title.folder_path)}</li></ul></details>{/if}
		</TitleSection>
		<TitleSection id="h-show-history" title="History" testid="title-history">
			{#snippet head()}
				{#if historyTotal > 0}<Button variant="plain" href={to(`/downloads/history?title=${title.id}`)}>All {historyTotal} rows</Button>{/if}
			{/snippet}
			{#if history.length === 0}
				<p class="small muted">Nothing yet.</p>
			{:else}
				<ol class="rows hist">
					{#each history as e (e.ID)}
						<li>
							<span class="chip {kindTone(e.Kind)}">{groupLabel(e.Kind)}</span>
							<span class="grow small">{#if e.episode}<span class="mono xs muted">{e.episode}</span>{' '}{/if}{#if e.version}<span class="chip" data-testid="event-version">{e.version}</span>{' '}{/if}{e.Summary}</span>
							<time class="mono xs muted" datetime={e.At} title={dateTime(e.At)}>{ago(e.At)}</time>
						</li>
					{/each}
				</ol>
			{/if}
		</TitleSection>
	</LibrarySection>
{/if}
</div>

{#if staff}
	<ShowDeleteDialog bind:this={deleter} titleId={title.id} name={title.name} ondone={deleted} />
{/if}

<dialog bind:this={subDialog} class="modal wide" aria-label="Subtitles of an episode" onclose={() => (subEpisode = null)} data-testid="episode-subtitles-dialog">
	{#if subEpisode}
		<SubtitlePanel titleId={title.id} movie={false} itemId={subEpisode.id} label={episodeLabel(subEpisode, order)} />
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => subDialog?.close()}>Close</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={renameDialog} aria-labelledby="rename-show-title" class="modal wide" onclose={() => (renameOpen = false)}>
	{#if renameOpen}
		<h2 id="rename-show-title" class="h2">Rename the files of {title.name}</h2>
		<p class="small muted">The names now and the names they get from the show, season and episode patterns. Nothing changes until you press Rename.</p>
		<RenamePreview titleId={title.id} onDone={() => void reload()} />
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => renameDialog?.close()}>Close</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={editDialog} aria-labelledby="edit-show-title" class="modal">
	<form onsubmit={saveEdit}>
		<h2 id="edit-show-title" class="h2">Edit {title.name}</h2>
		<div class="field" style="margin-top: var(--s-5)">
			<label for="e-rules">Pick rules{title.versions.length > 1 ? ' (main Version)' : ''}</label>
			<select id="e-rules" class="input" bind:value={eRules}>
				{#each rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
			</select>
		</div>
		<div class="field">
			<label for="e-order">Episode order</label>
			<select id="e-order" class="input" bind:value={eOrder}>
				{#each episodeOrders as [v, l] (v)}<option value={v}>{l}</option>{/each}
			</select>
			<span class="hint">{episodeOrders.find(([v]) => v === eOrder)?.[2]}</span>
		</div>
		<div class="field">
			<label for="e-new">New seasons</label>
			<select id="e-new" class="input" bind:value={eNew}>
				<option value="all">Keep them up to date</option>
				<option value="none">Leave them off</option>
			</select>
		</div>
		<div class="field">
			<label for="e-source">Details from</label>
			<select id="e-source" class="input" bind:value={eSource}>
				{#each detailsSources as [v, l] (v)}<option value={v}>{l}</option>{/each}
			</select>
			<span class="hint">Switching finds the show in the other source; episodes and their files stay.</span>
		</div>
		{#if editError}<p class="form-error" role="alert">{editError}</p>{/if}
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => editDialog?.close()}>Cancel</Button>
			<Button variant="primary" type="submit" disabled={saving} busy={saving}>Save</Button>
		</div>
	</form>
</dialog>

<style>
	.path { overflow-wrap: anywhere; }
	.season-tools { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 10px; margin: 12px 0 6px; }
	.season-keep { margin: 0 0 14px; }
	.tp-progress { color: var(--text-muted); }
	.season-line { margin: 0 0 12px; }
	.episodes { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--title-item), 1fr)); gap: 20px var(--shelf-gap); }
	.episode { min-width: 0; }
	@media (max-width: 767px) { .episodes { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px 10px; } }
	.ep-why { margin: 4px 0 0; }
	.ep-menu :global(.ui-pop > .trigger) { background: var(--on-art-scrim-mid); color: var(--on-art); backdrop-filter: blur(12px); -webkit-backdrop-filter: blur(12px); }
	.ep-menu :global(.ui-pop > .trigger:hover), .ep-menu :global(.ui-pop > .trigger[aria-expanded='true']) { background: rgb(0 0 0 / 65%); color: var(--on-art); }
	.modal { width: min(480px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: var(--s-6); border: 0; border-radius: var(--r-modal); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); }
	.modal.wide { width: min(640px, calc(100vw - 32px)); }
	.modal::backdrop { background: var(--scrim); }
	.modal-actions { justify-content: flex-end; margin-top: var(--s-6); }
	.hist li { flex-wrap: wrap; gap: var(--s-2) var(--s-3); }
	.hist .grow { min-width: 0; overflow-wrap: anywhere; }
</style>

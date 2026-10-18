<script lang="ts">
	import { to } from './paths.ts';
	// One show: its seasons and episodes, Keep up to date per show, season
	// and episode, air dates and why each episode is or is not looked for;
	// Search now and Choose a release for the show, a season or an episode.
	import { goto } from '$app/navigation';
	import { api, message } from './api.ts';
	import RenamePreview from './RenamePreview.svelte';
	import ShowDeleteDialog from './ShowDeleteDialog.svelte';
	import { live } from './live.svelte.ts';
	import ShowReleases from './ShowReleases.svelte';
	import ShowVersions from './ShowVersions.svelte';
	import CopyControls from './CopyControls.svelte';
	import SubtitlePanel from './SubtitlePanel.svelte';
	import { ago, date, dateTime, tone } from './format.ts';
	import Icon from './Icon.svelte';
	import TitleHero from './ui/TitleHero.svelte';
	import ActionButton from './ui/ActionButton.svelte';
	import ActionMenu from './ui/ActionMenu.svelte';
	import Menu, { type MenuItem } from './ui/Menu.svelte';
	import SegmentedControl from './ui/SegmentedControl.svelte';
	import StatusPill from './ui/StatusPill.svelte';
	import WideCard from './ui/WideCard.svelte';
	import Skeleton from './ui/Skeleton.svelte';
	import TitleCast from './TitleCast.svelte';
	import SimilarShelf from './SimilarShelf.svelte';
	import { artName, crumbFor, slotKind } from './title-page.ts';
	import './title-page.css';
	import { session } from './session.svelte.ts';
	import { toasts } from './toasts.svelte.ts';
	import { decided, names } from './wishlist.ts';
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

	let { title, onchange }: { title: TitleView; onchange: (t: TitleView) => void } = $props();

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
		onchange(await api.get<TitleView>(`/titles/${title.id}`));
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
			toasts.push(message(e), 'fail');
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

	const heroMeta = $derived([title.year ? String(title.year) : '', show ? (statusWord[show.status] ?? show.status) : '', show?.network ?? '', show?.seasons.length ? `${show.seasons.length} ${show.seasons.length === 1 ? 'season' : 'seasons'}` : ''].filter(Boolean));
	const heroKind = $derived(show?.missing ? 'missing' : show?.in_library ? 'in_library' : show?.not_out_yet ? 'coming_soon' : 'missing');
	const libraryItems = $derived<MenuItem[]>([
		{ label: 'Rename files', icon: 'copy', onselect: openRename },
		{ kind: 'separator' },
		{ label: 'Delete show', icon: 'trash', destructive: true, onselect: () => deleter?.open() }
	]);
	const moreItems = $derived<MenuItem[]>([
		{ label: 'Refresh seasons and episodes', icon: 'refresh', onselect: refresh, disabled: refreshing },
		{ label: 'All history', icon: 'clock', href: to(`/downloads/history?title=${title.id}`) }
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
	poster={title.artwork[0]}
	backdrop={title.artwork[1]}
	breadcrumb={[{ href: to(crumbFor('show').path), label: crumbFor('show').label }, { href: to(`/library/${title.id}`), label: title.year ? `${title.name} (${title.year})` : title.name }]}
	meta={heroMeta}
	status={heroKind}
	statusLabel={show?.missing ? `${show.missing} missing` : undefined}
	description={title.overview}
	vtName={artName(title.id)}
>
	{#snippet actions()}
		{#if staff}
			<ActionButton icon="search" label="Search now" primary onclick={searchShow} disabled={searching === 'show'} busy={searching === 'show'} data-testid="search-show" />
			<ActionButton
				icon={title.keep_up_to_date ? 'check' : 'plus'}
				label="Keep up to date"
				caption={title.keep_up_to_date ? 'Kept up to date' : 'Keep up to date'}
				pressed={title.keep_up_to_date}
				disabled={busy === 'show'}
				onclick={() => setKeep(!title.keep_up_to_date)}
			/>
			<ActionButton icon="tools" label="Edit" onclick={openEdit} />
			<ActionMenu label="Library" icon="library" items={libraryItems} />
			<ActionMenu label="More actions" caption="More" icon="more" items={moreItems} />
		{:else}
			<div class="tp-member"><StatusPill kind={heroKind} label={heroKind === 'in_library' ? 'Ready to watch' : undefined} on="art" /></div>
		{/if}
	{/snippet}
	{#snippet extra()}
		<p class="small" data-testid="show-progress">{showProgress(show)}</p>
	{/snippet}
</TitleHero>

<p class="tp-why open" data-testid="gate-reason"><b>Why:</b> {title.gate_reason}</p>

{#if scope && staff}
	<ShowReleases titleId={title.id} {scope} jobs={scopeJobs} onclose={() => (scope = null)} onchange={() => void reload()} />
{/if}

<section class="tp-section" aria-labelledby="h-seasons">
	<div class="tp-head">
		<h2 id="h-seasons" class="tp-h2">Seasons</h2>
		{#if show?.seasons.length}
			{#if show.seasons.length <= 4}
				<SegmentedControl label="Season" options={show.seasons.map((s) => ({ value: String(s.number), label: s.name || seasonName(s.number) }))} value={String(cur)} onchange={(v) => pickSeason(Number(v))} />
			{:else}
				<Menu label="Pick a season" variant="button" icon="tv" text={curSeason ? curSeason.name || seasonName(curSeason.number) : 'Season'} align="end" items={seasonItems} />
			{/if}
		{/if}
	</div>
	{#if !show?.seasons.length}
		<p class="small muted">No seasons yet. A refresh adds them once the details source lists them.</p>
	{:else if curSeason}
		{@const s = curSeason}
		<div class="season" data-testid="season">
			<p class="xs muted" data-testid="season-line">{seasonLine(s)}{s.dropped ? ' · no longer listed' : ''}</p>
			{#if staff}
				<div class="row season-actions">
					<button class="btn btn-ghost" type="button" onclick={() => searchSeason(s.number)} disabled={searching === `s${s.number}`} aria-label="Search now: {seasonName(s.number)}" data-testid="search-season"><Icon name="search" size={16} /> Search now</button>
					<button class="btn btn-quiet" type="button" onclick={() => searchSeason(s.number, true)} disabled={searching === `s${s.number}l`} aria-label="Choose a release for {seasonName(s.number)}" data-testid="choose-season">Choose a release</button>
					<button class="btn btn-quiet" type="button" onclick={() => deleter?.open({ season: s.number, label: seasonName(s.number) })} aria-label="Delete {seasonName(s.number)}" data-testid="delete-season">Delete season</button>
					<label class="switch">
						<input type="checkbox" checked={s.keep_up_to_date} disabled={busy === `s${s.number}`} onchange={(e) => setSeason(s.number, e.currentTarget.checked)} />
						<span class="sw" aria-hidden="true"></span>
						<span class="small">Keep {seasonName(s.number)} up to date</span>
					</label>
					{#if episodes[s.number] && seasonLooking(episodes[s.number])}
						<span class="xs muted">{seasonLooking(episodes[s.number])} looking for a copy</span>
					{/if}
				</div>
			{:else if !s.keep_up_to_date}
				<p class="xs muted">Not kept</p>
			{/if}
			{#if episodes[s.number]}
				<ul class="episodes">
					{#each episodes[s.number] as ep (ep.id)}
						{@const why = episodeWhy(ep, s.keep_up_to_date, title.keep_up_to_date)}
						{@const main = mainSlot(ep)}
						<li class="episode" data-testid="episode">
							<WideCard
								title="{episodeLabel(ep, order)} · {ep.name || 'Name not known yet'}"
								image={title.artwork[1] ?? title.artwork[0]}
								subtitle={ep.air_date ? date(ep.air_date) : 'No air date yet'}
								status={main ? slotKind(main.state) : undefined}
								statusLabel={ep.slots.length > 1 ? ep.slots.map((x) => `${x.version}: ${x.state_label}`).join(' · ') : main?.state_label}
							>
								{#snippet action()}
									{#if staff}<span class="ep-menu"><Menu label="Actions for {episodeLabel(ep, order)}" items={episodeItems(ep)} width={250} /></span>{/if}
								{/snippet}
							</WideCard>
							{#if why}<p class="xs muted ep-why" data-testid="episode-why">{why}</p>{/if}
							{#each ep.slots.filter((x) => x.reason && ep.slots.length > 1) as sl (sl.id)}
								<p class="xs muted ep-why">{sl.version}: {sl.reason}</p>
							{/each}
						</li>
					{:else}
						<li class="small muted">No episodes listed yet.</li>
					{/each}
				</ul>
			{:else}
				<div class="episodes" aria-busy="true">{#each [0, 1, 2] as i (i)}<Skeleton shape="wide" />{/each}</div>
			{/if}
		</div>
	{/if}
</section>

{#key title.id}<TitleCast titleId={title.id} />{/key}

<div class="tp-body">
	<div class="tp-main">
		<ShowVersions {title} {staff} onchange={reload} />
		<CopyControls {title} onchange={reload} />
		{#if staff}
			{#key title.id}<SubtitlePanel titleId={title.id} movie={false} files={false} />{/key}
		{/if}
	</div>
	<aside class="tp-side" aria-label="About this show">

		<section class="card" aria-labelledby="h-show-facts">
			<h2 id="h-show-facts" class="h2 card-title">About</h2>
			<dl class="facts">
				<dt>Episodes</dt>
				<dd>{show?.episodes ?? 0} in {show?.seasons.length ?? 0} seasons</dd>
				{#if show?.next_air_date}<dt>Next episode</dt>
					<dd>{date(show.next_air_date)}</dd>{/if}
				{#if show?.air_time}<dt>Airs at</dt>
					<dd>{show.air_time}{show.time_zone ? ` (${show.time_zone})` : ''}</dd>{/if}
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
			{#if staff && title.folder_path}<p class="xs muted path">In {title.folder_path}</p>{/if}
		</section>

		{#if title.asks?.length}
			<section class="card" aria-labelledby="h-show-asks" data-testid="title-asks">
				<h2 id="h-show-asks" class="h2 card-title">Wishlist</h2>
				{#each title.asks as a (a.id)}
					<dl class="facts">
						<dt>Asked by</dt>
						<dd>{names(a.people.map((p) => p.name))} · {date(a.asked_at)}</dd>
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
			</section>
		{/if}

		{#if staff}
			<section class="card" aria-labelledby="h-show-history" data-testid="title-history">
				<div class="card-head">
					<h2 id="h-show-history" class="h2">History</h2>
					{#if historyTotal > 0}<a class="btn btn-quiet" href={to(`/downloads/history?title=${title.id}`)}>All {historyTotal} rows</a>{/if}
				</div>
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
			</section>
		{/if}
	</aside>
</div>

<SimilarShelf {title} />
</div>

{#if staff}
	<ShowDeleteDialog bind:this={deleter} titleId={title.id} name={title.name} ondone={deleted} />
{/if}

<dialog bind:this={subDialog} class="modal wide" aria-label="Subtitles of an episode" onclose={() => (subEpisode = null)} data-testid="episode-subtitles-dialog">
	{#if subEpisode}
		<SubtitlePanel titleId={title.id} movie={false} itemId={subEpisode.id} label={episodeLabel(subEpisode, order)} />
		<div class="row modal-actions">
			<button class="btn btn-quiet" type="button" onclick={() => subDialog?.close()}>Close</button>
		</div>
	{/if}
</dialog>

<dialog bind:this={renameDialog} aria-labelledby="rename-show-title" class="modal wide" onclose={() => (renameOpen = false)}>
	{#if renameOpen}
		<h2 id="rename-show-title" class="h2">Rename the files of {title.name}</h2>
		<p class="small muted">The names now and the names they get from the show, season and episode patterns. Nothing changes until you press Rename.</p>
		<RenamePreview titleId={title.id} onDone={() => void reload()} />
		<div class="row modal-actions">
			<button class="btn btn-quiet" type="button" onclick={() => renameDialog?.close()}>Close</button>
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
			<button class="btn btn-quiet" type="button" onclick={() => editDialog?.close()}>Cancel</button>
			<button class="btn btn-red" type="submit" disabled={saving} aria-busy={saving}>Save</button>
		</div>
	</form>
</dialog>

<style>
	.path { overflow-wrap: anywhere; }
	.season-actions { gap: var(--s-2); flex-wrap: wrap; align-items: center; margin: var(--s-3) 0 var(--s-4); }
	.episodes { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 240px), 1fr)); gap: 20px 16px; }
	.episode { min-width: 0; }
	.ep-why { margin-top: 2px; }
	.ep-menu :global(.ui-pop > .trigger) { background: rgb(0 0 0 / 45%); color: #fff; backdrop-filter: blur(12px); -webkit-backdrop-filter: blur(12px); }
	.ep-menu :global(.ui-pop > .trigger:hover), .ep-menu :global(.ui-pop > .trigger[aria-expanded='true']) { background: rgb(0 0 0 / 65%); color: #fff; }
	.modal { width: min(480px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: var(--s-6); border: 0; border-radius: var(--r-modal); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); }
	.modal.wide { width: min(640px, calc(100vw - 32px)); }
	.modal::backdrop { background: var(--scrim); }
	.modal-actions { justify-content: flex-end; margin-top: var(--s-6); }
	.hist li { flex-wrap: wrap; gap: var(--s-2) var(--s-3); }
	.hist .grow { min-width: 0; overflow-wrap: anywhere; }
</style>

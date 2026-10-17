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
	import Poster from './Poster.svelte';
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

	function toggle(n: number, e: Event) {
		open[n] = (e.currentTarget as HTMLDetailsElement).open;
		if (open[n] && !episodes[n]) void loadSeason(n);
	}

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

<section class="hero">
	<div class="art"><Poster src={title.artwork[0]} name={title.name} /></div>
	<div class="info">
		<h1 class="title">{title.name}{#if title.year}{' '}<span class="muted">({title.year})</span>{/if}</h1>
		{#if title.overview}<p class="overview">{title.overview}</p>{/if}
		<div class="row chips">
			<span class="chip">Show</span>
			{#if show}<span class="chip {show.status === 'ended' ? '' : 'ok'}">{statusWord[show.status] ?? show.status}</span>{/if}
			{#if show?.network}<span class="chip">{show.network}</span>{/if}
			{#if show?.missing}<span class="chip {tone('wanted')}">{show.missing} missing</span>{/if}
		</div>
		<p class="small" data-testid="show-progress">{showProgress(show)}</p>
		<p class="why small open" data-testid="gate-reason"><b>Why:</b> {title.gate_reason}</p>
		{#if staff}
			<div class="row actions">
				<button class="btn btn-red" type="button" onclick={searchShow} disabled={searching === 'show'} aria-busy={searching === 'show'} data-testid="search-show"
					><Icon name="search" size={16} /> Search now</button
				>
				<button class="btn btn-ghost" type="button" onclick={refresh} disabled={refreshing} aria-busy={refreshing}><Icon name="refresh" size={16} /> Refresh</button>
				<button class="btn btn-ghost" type="button" onclick={openEdit}>Edit</button>
				<button class="btn btn-ghost" type="button" onclick={openRename} data-testid="rename-show">Rename files</button>
				<button class="btn btn-quiet" type="button" onclick={() => deleter?.open()} data-testid="delete-show"><Icon name="trash" size={16} /> Delete</button>
				<label class="switch">
					<input type="checkbox" checked={title.keep_up_to_date} disabled={busy === 'show'} onchange={(e) => setKeep(e.currentTarget.checked)} />
					<span class="sw" aria-hidden="true"></span>
					<span class="small">Keep up to date</span>
				</label>
			</div>
		{/if}
	</div>
</section>

<div class="cols">
	<div class="side">
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

		<ShowVersions {title} {staff} onchange={reload} />
		<CopyControls {title} onchange={reload} />

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
			{#key title.id}<SubtitlePanel titleId={title.id} movie={false} files={false} />{/key}
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
	</div>

	<div class="main">
	{#if scope && staff}
		<ShowReleases titleId={title.id} {scope} jobs={scopeJobs} onclose={() => (scope = null)} onchange={() => void reload()} />
	{/if}
	<section class="card seasons-card" aria-labelledby="h-seasons">
		<h2 id="h-seasons" class="h2 card-title">Seasons</h2>
		{#if !show?.seasons.length}
			<p class="small muted">No seasons yet. A refresh adds them once the details source lists them.</p>
		{:else}
			<ul class="seasons">
				{#each show.seasons as s (s.number)}
					<li class="season" data-testid="season">
						<div class="season-head">
							<details open={open[s.number] ?? false} ontoggle={(e) => toggle(s.number, e)}>
								<summary>
									<b>{s.name || seasonName(s.number)}</b>
									<span class="xs muted" data-testid="season-line">{seasonLine(s)}{s.dropped ? ' · no longer listed' : ''}</span>
								</summary>
								{#if staff}
									<div class="row season-actions">
										<button
											class="btn btn-ghost btn-sm"
											type="button"
											onclick={() => searchSeason(s.number)}
											disabled={searching === `s${s.number}`}
											aria-label="Search now: {seasonName(s.number)}"
											data-testid="search-season">Search now</button
										>
										<button
											class="btn btn-quiet btn-sm"
											type="button"
											onclick={() => searchSeason(s.number, true)}
											disabled={searching === `s${s.number}l`}
											aria-label="Choose a release for {seasonName(s.number)}"
											data-testid="choose-season">Choose a release</button
										>
										<button
											class="btn btn-quiet btn-sm"
											type="button"
											onclick={() => deleter?.open({ season: s.number, label: seasonName(s.number) })}
											aria-label="Delete {seasonName(s.number)}"
											data-testid="delete-season">Delete season</button
										>
										{#if episodes[s.number] && seasonLooking(episodes[s.number])}
											<span class="xs muted">{seasonLooking(episodes[s.number])} looking for a copy</span>
										{/if}
									</div>
								{/if}
								{#if episodes[s.number]}
									<ul class="episodes">
										{#each episodes[s.number] as ep (ep.id)}
											{@const why = episodeWhy(ep, s.keep_up_to_date, title.keep_up_to_date)}
											<li class="episode" data-testid="episode">
												<div class="ep-main">
													<span class="mono xs code">{episodeLabel(ep, order)}</span>
													<span class="ep-name small">{ep.name || 'Name not known yet'}</span>
													<span class="xs muted">{ep.air_date ? date(ep.air_date) : 'No air date yet'}</span>
												</div>
												<div class="ep-state">
													{#each ep.slots as sl (sl.id)}
														<span class="chip {tone(sl.state)}" data-testid="episode-state">{ep.slots.length > 1 ? `${sl.version}: ` : ''}{sl.state_label}</span>
													{/each}
													{#if staff && searchWord(mainSlot(ep)?.state)}
														<button
															class="btn btn-quiet btn-sm"
															type="button"
															onclick={() => searchEpisode(ep)}
															disabled={searching === `e${ep.id}`}
															aria-label="{searchWord(mainSlot(ep)?.state)}: {episodeLabel(ep, order)}"
															data-testid="search-episode">{searchWord(mainSlot(ep)?.state)}</button
														>
													{/if}
													{#if staff && mainSlot(ep)}
														<button
															class="btn btn-quiet btn-sm"
															type="button"
															onclick={() => searchEpisode(ep, true)}
															disabled={searching === `e${ep.id}l`}
															aria-label="Choose a release for {episodeLabel(ep, order)}"
															data-testid="choose-episode">Choose a release</button
														>
													{/if}
													{#if staff}
														{#if ep.slots.some((x) => x.file_id)}
															<button
																class="btn btn-quiet btn-sm"
																type="button"
																onclick={() => openSubtitles(ep)}
																aria-label="Subtitles of {episodeLabel(ep, order)}"
																data-testid="episode-subtitles">Subtitles</button
															>
														{/if}
														<a class="btn btn-quiet btn-sm" href={to(`/downloads/history?title=${title.id}&episode=${ep.id}`)} aria-label="History of {episodeLabel(ep, order)}"
															data-testid="episode-history">History</a
														>
														<button
															class="btn btn-quiet btn-sm"
															type="button"
															onclick={() => deleter?.open({ episodeIds: [ep.id], label: episodeLabel(ep, order) })}
															aria-label="Delete {episodeLabel(ep, order)}"
															data-testid="delete-episode">Delete</button
														>
														<label class="switch">
															<input type="checkbox" checked={ep.keep_up_to_date} disabled={busy === `e${ep.id}`} onchange={(e) => setEpisode(ep, e.currentTarget.checked)} />
															<span class="sw" aria-hidden="true"></span>
															<span class="sr-only">Keep {episodeLabel(ep, order)} up to date</span>
														</label>
													{/if}
												</div>
												{#if why}<p class="xs muted ep-why" data-testid="episode-why">{why}</p>{/if}
												{#each ep.slots.filter((x) => x.reason && ep.slots.length > 1) as sl (sl.id)}
													<p class="xs muted ep-why">{sl.version}: {sl.reason}</p>
												{/each}
											</li>
										{:else}
											<li class="small muted">No episodes listed yet.</li>
										{/each}
									</ul>
								{:else if open[s.number]}
									<div class="sk" style="height: 80px; margin-top: 8px"></div>
								{/if}
							</details>
							{#if staff}
								<label class="switch season-switch">
									<input type="checkbox" checked={s.keep_up_to_date} disabled={busy === `s${s.number}`} onchange={(e) => setSeason(s.number, e.currentTarget.checked)} />
									<span class="sw" aria-hidden="true"></span>
									<span class="sr-only">Keep {seasonName(s.number)} up to date</span>
								</label>
							{:else if !s.keep_up_to_date}
								<span class="xs muted">Not kept</span>
							{/if}
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
	</div>
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
	.hero {
		display: grid;
		grid-template-columns: 168px minmax(0, 1fr);
		gap: var(--s-5);
		margin-bottom: var(--s-5);
		padding: var(--s-4);
		border: 1px solid var(--line);
		border-radius: 6px;
		background: var(--surface-1);
	}
	.title {
		font: var(--weight-semibold) var(--text-2xl) / var(--leading-tight) var(--font-display);
		letter-spacing: var(--tracking-display);
	}
	.overview {
		margin-top: var(--s-3);
		color: var(--text-muted);
		max-width: 78ch;
		line-height: 1.6;
		text-wrap: pretty;
	}
	.chips {
		margin: var(--s-4) 0 var(--s-2);
		flex-wrap: wrap;
	}
	.why {
		margin: var(--s-2) 0;
		padding: var(--s-2) var(--s-3);
		border-left: 3px solid var(--ok);
		background: var(--surface-1);
		border-radius: var(--r-sm);
		max-width: 70ch;
	}
	.actions {
		margin-top: var(--s-4);
		gap: var(--s-4);
		flex-wrap: wrap;
	}
	.cols {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 320px;
		gap: var(--s-4);
		align-items: start;
	}
	.side {
		display: flex;
		flex-direction: column;
		gap: var(--s-4);
		grid-column: 2;
		position: sticky;
		top: var(--s-3);
	}
	.side .card + .card {
		margin-top: 0;
	}
	.card-title {
		margin-bottom: var(--s-4);
	}
	.path {
		overflow-wrap: anywhere;
	}
	.seasons {
		display: flex;
		flex-direction: column;
	}
	.season + .season {
		border-top: 1px solid var(--line);
	}
	.season-head {
		display: flex;
		align-items: flex-start;
		gap: var(--s-3);
		padding: var(--s-2) 0;
	}
	.season-head details {
		flex: 1;
		min-width: 0;
	}
	summary {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 4px var(--s-3);
		min-height: var(--tap);
		padding: var(--s-2) 0;
		cursor: pointer;
		border-radius: var(--r-sm);
	}
	.season-switch {
		flex: none;
	}
	.episodes {
		display: flex;
		flex-direction: column;
		margin: var(--s-1) 0 var(--s-2);
	}
	.episode {
		padding: var(--s-2) 0 var(--s-2) var(--s-3);
		border-left: 2px solid var(--line);
	}
	.ep-main {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 2px var(--s-3);
	}
	.ep-name {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.code {
		color: var(--text-muted);
	}
	.ep-state {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--s-2);
	}
	.ep-why {
		margin-top: 2px;
	}
	.main {
		display: flex;
		flex-direction: column;
		gap: var(--s-4);
		min-width: 0;
		grid-column: 1;
		grid-row: 1;
	}
	.main :global(.card + .card) {
		margin-top: 0;
	}
	.season-actions {
		gap: var(--s-2);
		flex-wrap: wrap;
		align-items: center;
		margin: var(--s-1) 0 var(--s-2);
	}
	.btn-sm {
		min-height: var(--tap);
		padding-inline: var(--s-3);
		font-size: var(--text-xs);
	}
	.modal {
		width: min(480px, calc(100vw - 32px));
		max-height: calc(100dvh - 32px);
		overflow: auto;
		padding: var(--s-6);
		border: 0;
		border-radius: var(--r-modal);
		background: var(--surface-1);
		color: var(--text);
		box-shadow: var(--shadow-modal);
	}
	.modal::backdrop {
		background: var(--scrim);
	}
	.modal.wide {
		width: min(720px, calc(100vw - 32px));
	}
	.modal-actions {
		justify-content: flex-end;
		margin-top: var(--s-6);
	}
	@media (max-width: 900px) {
		.cols {
			grid-template-columns: minmax(0, 1fr);
		}
		.main {
			grid-row: 1;
		}
		.side { grid-column: auto; position: static; }
	}
	@media (max-width: 600px) {
		.hero {
			grid-template-columns: 92px minmax(0, 1fr);
			gap: var(--s-3);
			padding: var(--s-3);
		}
		.title {
			font-size: var(--text-xl);
		}
	}
</style>

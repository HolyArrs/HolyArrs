<script lang="ts">
	import { to } from '#lib/paths.ts';
	// One album: its tracks with their state, every Pressing MusicBrainz
	// lists, its History, and for the staff Keep up to date, which Pressing
	// is used, Rename files for the album and Delete for the album or a track.
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import Icon from '#lib/Icon.svelte';
	import { ago, dateTime, tone } from '#lib/format.ts';
	import { groupLabel, kindTone } from '#lib/history.ts';
	import RenamePreview from '#lib/RenamePreview.svelte';
	import { session } from '#lib/session.svelte.ts';
	import ShowDeleteDialog from '#lib/ShowDeleteDialog.svelte';
	import ShowReleases from '#lib/ShowReleases.svelte';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { albumKind, albumLine, albumStateOf, albumYear, pressingLabel, pressingStatuses, trackLength, word } from '#lib/music.ts';
	import type { AlbumSummary, EventPage, EventRecord, PressingView, TitleView, TrackView } from '#lib/types.ts';

	const titleId = $derived(Number(page.params.id));
	const albumId = $derived(Number(page.params.album));
	const staff = $derived(session.can('manage_library'));
	let album = $state<AlbumSummary | null>(null);
	let pressings = $state<PressingView[]>([]);
	let tracks = $state<TrackView[] | null>(null);
	let artist = $state<TitleView | null>(null);
	let error = $state('');
	let notFound = $state(false);
	let chosen = $state(0);
	let busy = $state('');

	const discs = $derived(new Set((tracks ?? []).map((t) => t.disc)).size);
	const current = $derived(pressings.find((p) => p.chosen));

	// History of this album: its tracks' Events and those naming the album.
	let history = $state<EventRecord[]>([]);
	let historyTotal = $state(0);
	async function loadHistory() {
		if (!staff) return;
		try {
			const h = await api.get<EventPage>(`/events?album=${albumId}&limit=8`);
			history = h.events;
			historyTotal = h.total;
		} catch {
			// History is a side card; the page works without it.
		}
	}

	// Rename files of this album, and delete the album or one track.
	let renameDialog: HTMLDialogElement | undefined = $state();
	let renameOpen = $state(false);
	let deleter: ReturnType<typeof ShowDeleteDialog> | undefined = $state();
	function openRename() {
		renameOpen = true;
		renameDialog?.showModal();
	}
	function trackLabel(t: TrackView): string {
		const n = String(t.number).padStart(2, '0');
		return `${discs > 1 ? `disc ${t.disc} track ${n}` : `track ${n}`}${t.name ? ` ${t.name}` : ''}`;
	}
	async function afterDelete() {
		await load();
		void loadHistory();
	}

	async function load() {
		try {
			const r = await api.get<{ album: AlbumSummary; title_id: number; pressings: PressingView[] }>(`/albums/${albumId}`);
			album = r.album;
			pressings = r.pressings;
			chosen = r.pressings.find((p) => p.chosen)?.id ?? 0;
			tracks = (await api.get<{ tracks: TrackView[] }>(`/albums/${albumId}/tracks`)).tracks;
			artist = await api.get<TitleView>(`/titles/${r.title_id}`);
			error = '';
		} catch (e) {
			const m = message(e);
			if (/no such album/i.test(m)) notFound = true;
			else error = m;
		}
	}

	$effect(() => {
		void albumId;
		void load();
		void loadHistory();
	});

	// Search now lets HolyArrs pick; Choose a release lists every Release.
	let listing = $state(false);
	let listJobs = $state<number[]>([]);
	const scope = $derived({ label: `${artist?.name ?? 'Artist'} – ${album?.name ?? 'Album'}`, query: `album_id=${albumId}` });

	async function search(choose: boolean) {
		busy = choose ? 'search' : 'list';
		try {
			const r = await api.post<{ job_ids: number[] }>(`/titles/${titleId}/search`, choose ? { album_id: albumId } : { album_id: albumId, choose: false });
			if (choose) toasts.push(`Searching for ${album?.name}. HolyArrs picks the best Release for every track.`, 'ok');
			else {
				listJobs = r.job_ids;
				listing = true;
			}
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}

	async function setKeep(keep: boolean) {
		busy = 'keep';
		try {
			album = await api.put<AlbumSummary>(`/albums/${albumId}`, { keep_up_to_date: keep });
			toasts.push(keep ? 'HolyArrs keeps this album up to date.' : 'HolyArrs stops looking after this album.', 'ok');
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}

	async function usePressing(e: SubmitEvent) {
		e.preventDefault();
		busy = 'pressing';
		try {
			album = await api.put<AlbumSummary>(`/albums/${albumId}`, { pressing_id: chosen });
			toasts.push(`The tracks now follow ${pressings.find((p) => p.id === chosen)?.name ?? 'that Pressing'}.`, 'ok');
			await load();
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busy = '';
		}
	}
</script>

<svelte:head><title>{album?.name ?? 'Album'} · HolyArrs</title></svelte:head>

<a class="back small muted" href={to(`/library/${titleId}`)}><Icon name="back" size={16} /> {artist?.name ?? 'Artist'}</a>

{#if notFound}
	<h1 class="sr-only">Album</h1>
	<EmptyState icon="music" title="This album is not in your Library">
		<p>MusicBrainz may no longer list it.</p>
		{#snippet actions()}<a class="btn btn-ghost" href={to(`/library/${titleId}`)}>Back to the artist</a>{/snippet}
	</EmptyState>
{:else if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if !album}
	<div class="sk" style="height: 220px"></div>
{:else}
	{@const st = albumStateOf(album)}
	<div class="detail-workspace">
	<header class="page-head">
		<div>
			<h1 class="name">{album.name}{#if album.disambiguation}{' '}<span class="muted small">({album.disambiguation})</span>{/if}</h1>
			<p>{[artist?.name, albumYear(album.release_date), albumKind(album)].filter(Boolean).join(' · ')}</p>
			<div class="row chips">
				<span class="chip {st.tone}" data-testid="album-state">{st.label}</span>
				{#if album.dropped}<span class="chip">No longer listed</span>{/if}
			</div>
			<p class="small" data-testid="album-line">{albumLine(album)}</p>
		</div>
		{#if staff}
			<div class="head-actions">
				<div class="row">
					<button class="btn btn-red" type="button" onclick={() => search(true)} disabled={busy === 'search'} aria-busy={busy === 'search'} data-testid="album-search-now"
						><Icon name="search" size={16} /> {album.state === 'better_wanted' ? 'Find a better copy' : 'Search now'}</button
					>
					<button class="btn btn-ghost" type="button" onclick={() => search(false)} disabled={busy === 'list'} aria-busy={busy === 'list'} data-testid="album-choose"
						>Choose a release</button
					>
				</div>
				<div class="row">
					<button class="btn btn-ghost" type="button" onclick={openRename} data-testid="rename-album">Rename files</button>
					<button class="btn btn-quiet" type="button" onclick={() => deleter?.open({ albumId, label: album?.name ?? 'Album' })} data-testid="delete-album"
						><Icon name="trash" size={16} /> Delete album</button
					>
				</div>
				<label class="switch">
					<input type="checkbox" checked={album.keep_up_to_date} disabled={busy === 'keep' || !album.included} onchange={(e) => setKeep(e.currentTarget.checked)} />
					<span class="sw" aria-hidden="true"></span>
					<span class="small">Keep up to date</span>
				</label>
			</div>
		{/if}
	</header>

	{#if listing}
		<ShowReleases titleId={artist?.id ?? titleId} {scope} jobs={listJobs} music onclose={() => (listing = false)} onchange={load} />
	{/if}

	<div class="cols">
		<section class="card" aria-labelledby="h-tracks">
			<h2 id="h-tracks" class="h2 card-title">Tracks</h2>
			{#if !tracks?.length}
				<p class="small muted">{album.included ? 'Tracks come with the next refresh.' : 'This album is left out by the album kinds, so its tracks are not listed.'}</p>
			{:else}
				<ol class="tracks">
					{#each tracks as t (t.id)}
						<li class="track" data-testid="track">
							<span class="mono xs muted no">{discs > 1 ? `${t.disc}-` : ''}{t.number}</span>
							<span class="grow small">{t.name || 'Name not known yet'}</span>
							<span class="mono xs muted">{trackLength(t.length_ms)}</span>
							{#each t.slots as sl (sl.id)}
								<span class="chip {tone(sl.state)}" data-testid="track-state">{t.slots.length > 1 ? `${sl.version}: ` : ''}{sl.state_label}</span>
							{/each}
							{#if staff}
								<button
									class="btn btn-quiet icon-btn"
									type="button"
									onclick={() => deleter?.open({ trackIds: [t.id], label: trackLabel(t) })}
									aria-label="Delete {trackLabel(t)}"
									data-testid="delete-track"><Icon name="trash" size={16} /></button
								>
							{/if}
						</li>
					{/each}
				</ol>
			{/if}
		</section>

		<section class="card" aria-labelledby="h-pressings">
			<h2 id="h-pressings" class="h2 card-title">Pressings</h2>
			{#if pressings.length === 0}
				<p class="small muted">{album.included ? 'Pressings come with the next refresh.' : 'Pressings are read only for included albums.'}</p>
			{:else}
				{#if current}
					<p class="small" data-testid="chosen-pressing">
						The tracks follow <b>{pressingLabel(current)}</b> ({current.chosen_by === 'person' ? 'picked by hand' : 'picked by the rule in Settings > Music'}).
					</p>
				{/if}
				{#if staff}
					<form class="row pick" onsubmit={usePressing}>
						<label class="sr-only" for="pressing">Pressing to follow</label>
						<select id="pressing" class="input" bind:value={chosen}>
							{#each pressings as p (p.id)}
								<option value={p.id}>{pressingLabel(p)}</option>
							{/each}
						</select>
						<button class="btn btn-ghost" type="submit" disabled={busy === 'pressing' || chosen === current?.id} aria-busy={busy === 'pressing'}>Use this Pressing</button>
					</form>
					<p class="hint">Tracks on both Pressings keep their files; tracks the new one lacks are removed unless a file holds them.</p>
				{/if}
				<ul class="pressings">
					{#each pressings as p (p.id)}
						<li data-testid="pressing">
							<span class="grow small">{pressingLabel(p)}</span>
							<span class="chip">{word(pressingStatuses, p.status)}</span>
							{#if p.chosen}<span class="chip ok">Used</span>{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	</div>

	{#if staff}
		<section class="card hist" aria-labelledby="h-album-history" data-testid="album-history">
			<div class="row between">
				<h2 id="h-album-history" class="h2">History</h2>
				{#if historyTotal > 0}<a class="btn btn-quiet" href={to(`/downloads/history?title=${titleId}`)}>All rows for the artist</a>{/if}
			</div>
			{#if history.length === 0}
				<p class="small muted">Nothing has happened to this album yet.</p>
			{:else}
				<ul class="events">
					{#each history as e (e.ID)}
						<li class="ev" data-testid="album-event">
							<span class="chip {kindTone(e.Kind)}">{groupLabel(e.Kind)}</span>
							<span class="small grow">{e.Summary}</span>
							<time class="xs muted" datetime={e.At} title={dateTime(e.At)}>{ago(e.At)}</time>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<ShowDeleteDialog bind:this={deleter} titleId={titleId} name={artist?.name ?? 'Artist'} kind="artist" ondone={afterDelete} />

		<dialog bind:this={renameDialog} aria-labelledby="rename-album-title" class="modal wide" onclose={() => (renameOpen = false)}>
			{#if renameOpen}
				<h2 id="rename-album-title" class="h2">Rename the files of {album.name}</h2>
				<p class="small muted">The names now and the names they get from the album and track patterns. Nothing changes until you press Rename.</p>
				<RenamePreview {titleId} {albumId} onDone={() => void afterDelete()} />
				<div class="row modal-actions">
					<button class="btn btn-quiet" type="button" onclick={() => renameDialog?.close()}>Close</button>
				</div>
			{/if}
		</dialog>
	{/if}
	</div>
{/if}

<style>
	.detail-workspace { overflow: hidden; border: 1px solid var(--line); border-radius: 16px; background: var(--surface-1); }
	.detail-workspace .page-head { margin: 0; padding: var(--s-5); border-bottom: 1px solid var(--line); }
	.back {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		min-height: var(--tap);
		margin-bottom: var(--s-3);
	}
	.name {
		overflow-wrap: anywhere;
	}
	.chips {
		margin: var(--s-3) 0 var(--s-2);
		flex-wrap: wrap;
	}
	.cols {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(300px, 340px);
		gap: 0;
		align-items: start;
	}
	.cols > .card { margin: 0; border: 0; border-radius: 0; box-shadow: none; }
	.cols > .card + .card { border-left: 1px solid var(--line); }
	.cols :global(.card + .card) {
		margin-top: 0;
	}
	.card-title {
		margin-bottom: var(--s-4);
	}
	.tracks,
	.pressings {
		display: flex;
		flex-direction: column;
	}
	.track,
	.pressings li {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--s-2) var(--s-3);
		padding: var(--s-3) 0;
	}
	.track:hover, .pressings li:hover { background: var(--surface-2); }
	.track + .track,
	.pressings li + li {
		border-top: 1px solid var(--line);
	}
	.no {
		min-width: 2.5em;
	}
	.grow {
		flex: 1;
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.pick {
		gap: var(--s-2);
		flex-wrap: wrap;
		margin: var(--s-3) 0 var(--s-2);
	}
	.pick select {
		flex: 1;
		min-width: 0;
	}
	.pressings {
		margin-top: var(--s-3);
	}
	@media (max-width: 900px) {
		.cols {
			grid-template-columns: minmax(0, 1fr);
		}
		.cols > .card + .card { border-left: 0; border-top: 1px solid var(--line); }
	}
	.head-actions {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: var(--s-3);
	}
	.head-actions .row {
		gap: var(--s-2);
		flex-wrap: wrap;
	}
	.hist {
		margin: 0;
		border: 0;
		border-top: 1px solid var(--line);
		border-radius: 0;
		box-shadow: none;
	}
	.between {
		justify-content: space-between;
		align-items: center;
	}
	.events {
		list-style: none;
		padding: 0;
		margin: var(--s-3) 0 0;
	}
	.ev {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--s-2);
		padding: var(--s-2) 0;
	}
	.ev + .ev {
		border-top: 1px solid var(--line);
	}
	.modal {
		width: min(720px, calc(100vw - 32px));
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
	.modal-actions {
		justify-content: flex-end;
		margin-top: var(--s-6);
	}
	@media (max-width: 600px) {
		.head-actions {
			align-items: flex-start;
		}
	}
</style>

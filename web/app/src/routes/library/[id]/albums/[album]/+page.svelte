<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { to } from '#lib/paths.ts';
	// One album: its tracks with their state, every Pressing MusicBrainz
	// lists, its History, and for the staff Keep up to date, which Pressing
	// is used, Rename files for the album and Delete for the album or a track.
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import { ago, date, dateTime, tone } from '#lib/format.ts';
	import TitleHero from '#lib/ui/TitleHero.svelte';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import ActionMenu from '#lib/ui/ActionMenu.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import TitleSection from '#lib/ui/TitleSection.svelte';
	import LibrarySection from '#lib/ui/LibrarySection.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import { crumbFor } from '#lib/title-page.ts';
	import { audienceStatus } from '#lib/title-status.ts';
	import { reducedMotion } from '#lib/motion.ts';
	import '#lib/title-page.css';
	import { groupLabel, kindTone } from '#lib/history.ts';
	import RenamePreview from '#lib/RenamePreview.svelte';
	import { session } from '#lib/session.svelte.ts';
	import ShowDeleteDialog from '#lib/ShowDeleteDialog.svelte';
	import ShowReleases from '#lib/ShowReleases.svelte';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { albumCover, albumKind, albumLine, albumStateOf, albumYear, pressingLabel, pressingStatuses, trackLength, word } from '#lib/music/music.ts';
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

	// The other albums of the artist (More by the artist), newest first.
	let siblings = $state<AlbumSummary[]>([]);
	$effect(() => {
		const t = titleId;
		const a = albumId;
		let live = true;
		api.get<{ albums: AlbumSummary[] }>(`/titles/${t}/albums`)
			.then((r) => live && (siblings = (r.albums ?? []).filter((x) => x.id !== a)))
			.catch(() => {
				/* An extra row: the page works without it. */
			});
		return () => (live = false);
	});
	const more = $derived([...siblings].sort((x, y) => (y.release_date ?? '').localeCompare(x.release_date ?? '')).slice(0, 18));
	// The cover, once known to load; else the artist's picture fills the frame.
	let coverOk = $state(false);
	$effect(() => {
		const id = album?.id;
		coverOk = false;
		if (!id || typeof Image === 'undefined') return;
		const img = new Image();
		img.onload = () => album?.id === id && (coverOk = true);
		img.src = albumCover(id);
	});
	const totalLength = $derived.by(() => {
		const ms = (tracks ?? []).reduce((n, t) => n + (t.length_ms ?? 0), 0);
		if (!ms) return '';
		const m = Math.round(ms / 60000);
		return m >= 60 ? `${Math.floor(m / 60)} hr ${m % 60} min` : `${m} min`;
	});
	const audience = $derived(audienceStatus(album && album.in_library > 0 && !album.missing ? 'in_library' : 'missing', staff ? 'staff' : 'member', { tracked: true, titleKind: 'artist' }));
	const heroItems = $derived<MenuItem[]>([
		{ label: album?.state === 'better_wanted' ? 'Find a better copy' : 'Search now', icon: 'search', onselect: () => search(true), disabled: busy === 'search' },
		{ label: 'Choose a release', icon: 'grid', onselect: () => search(false), disabled: busy === 'list' },
		{ label: 'Keep up to date', checked: !!album?.keep_up_to_date, disabled: busy === 'keep' || !album?.included, onselect: () => album && setKeep(!album.keep_up_to_date) },
		{ kind: 'separator' },
		{ label: 'Rename files', icon: 'copy', onselect: openRename },
		{ kind: 'separator' },
		{ label: 'Delete album', icon: 'trash', destructive: true, onselect: () => deleter?.open({ albumId, label: album?.name ?? 'Album' }) }
	]);
	function toLibrary() {
		const h = document.getElementById('h-library');
		h?.closest('section')?.scrollIntoView({ behavior: reducedMotion() ? 'auto' : 'smooth', block: 'start' });
	}

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
				toLibrary();
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

{#if notFound}
	<h1 class="sr-only">Album</h1>
	<EmptyState icon="music" title="This album is not in your Library">
		<p>MusicBrainz may no longer list it.</p>
		{#snippet actions()}<Button href={to(`/library/${titleId}`)}>Back to the artist</Button>{/snippet}
	</EmptyState>
{:else if error}
	<p class="notice bad" role="alert">{error}</p>
{:else}
<PageLoader loading={!album} label="Loading the album">
{#if album}
	{@const st = albumStateOf(album)}
	<div class="tp">
		<TitleHero
			title={album.name}
			cover={coverOk ? albumCover(album.id) : null}
			poster={coverOk ? null : artist?.artwork[0]}
			square
			crumbIcon="music"
			breadcrumb={[{ href: to(crumbFor('artist').path), label: crumbFor('artist').label }, { href: to(`/library/${titleId}`), label: artist?.name ?? 'Artist' }, { href: to(`/library/${titleId}/albums/${albumId}`), label: album.name }]}
			meta={[albumYear(album.release_date), albumKind(album), tracks?.length ? `${tracks.length} ${tracks.length === 1 ? 'track' : 'tracks'}` : '', totalLength].filter(Boolean)}
			status={audience.kind}
			statusLabel={staff ? st.label : audience.label}
			description={album.disambiguation}
		>
			{#snippet byline()}
				{#if artist}<a class="artist-link" href={to(`/library/${titleId}`)}>{artist.name}</a>{/if}
				{#if staff}<span data-testid="album-line">{albumLine(album!)}</span>{/if}
				{#if album!.dropped}<span>No longer listed</span>{/if}
			{/snippet}
			{#snippet actions()}
				{#if staff}
					<ActionButton icon="library" label="In Library: go to the Library section" caption="In Library" primary onclick={toLibrary} data-testid="hero-library" />
					<ActionMenu label="More actions" caption="More" icon="more" items={heroItems} />
				{/if}
			{/snippet}
		</TitleHero>

		<TitleSection id="h-tracks" title="Tracks" primary>
			{#if !tracks?.length}
				<p class="small muted">{album.included ? 'Tracks come with the next refresh.' : 'This album is left out by the album kinds, so its tracks are not listed.'}</p>
			{:else}
				<ol class="tracks">
					{#each tracks as t (t.id)}
						{@const here = t.slots.some((sl) => tone(sl.state) === 'ok')}
						<li class="track" data-testid="track">
							<span class="no" aria-hidden="true">{discs > 1 ? `${t.disc}-` : ''}{t.number}</span>
							<span class="grow">{t.name || 'Name not known yet'}</span>
							<span class="len">{trackLength(t.length_ms)}</span>
							{#if staff}
								{#each t.slots as sl (sl.id)}
									<span class="chip {tone(sl.state)}" data-testid="track-state">{t.slots.length > 1 ? `${sl.version}: ` : ''}{sl.state_label}</span>
								{/each}
								<IconButton icon="trash" size="sm" label="Delete {trackLabel(t)}" onclick={() => deleter?.open({ trackIds: [t.id], label: trackLabel(t) })} data-testid="delete-track" />
							{:else}
								<span class="mark" class:here data-testid="track-state">{here ? 'In your Library' : 'Not here yet'}</span>
							{/if}
						</li>
					{/each}
				</ol>
			{/if}
		</TitleSection>

		{#if more.length}
			<Shelf title="More by {artist?.name ?? 'the artist'}" item="var(--title-item)" anchor="title-similar" placeholder="wide" lines={1}>
				{#each more as a (a.id)}
					<WideCard title={a.name} href={to(`/library/${titleId}/albums/${a.id}`)} image={albumCover(a.id)} kind="artist" subtitle={[albumYear(a.release_date), albumKind(a)].filter(Boolean).join(' · ')} owned={a.in_library > 0 ? { state: a.missing ? 'partial' : 'available', label: a.missing ? `${a.in_library} of ${a.tracks} tracks here` : 'In your Library' } : null} />
				{/each}
			</Shelf>
		{/if}

		{#if pressings.length > 1}
			<Shelf title="Other editions" item="var(--title-item)" anchor="title-editions" placeholder="wide" lines={1}>
				{#each pressings as p (p.id)}
					<WideCard title={p.name} image={coverOk ? albumCover(album.id) : null} kind="artist" subtitle={pressingLabel(p)} owned={p.chosen ? { state: 'available', label: 'The edition HolyArrs follows' } : null} />
				{/each}
			</Shelf>
		{/if}

		<TitleSection id="h-about" title="About">
			<dl class="tp-about">
				{#if artist}<dt>Artist</dt><dd><a href={to(`/library/${titleId}`)}>{artist.name}</a></dd>{/if}
				{#if album.release_date}<dt>Released</dt><dd>{date(album.release_date)}</dd>{/if}
				<dt>Kind</dt>
				<dd>{albumKind(album)}</dd>
				{#if pressings.length}<dt>Editions</dt><dd>{pressings.length}</dd>{/if}
				{#if current}<dt>Edition followed</dt><dd>{pressingLabel(current)}</dd>{/if}
			</dl>
		</TitleSection>

		{#if staff}
			<LibrarySection note="Releases, the edition followed, files and history">
				{#if listing}
					<ShowReleases titleId={artist?.id ?? titleId} {scope} jobs={listJobs} music onclose={() => (listing = false)} onchange={load} />
				{/if}
				<TitleSection id="h-album-search" title="Search">
					<div class="row tools">
						<Button icon="search" onclick={() => search(true)} disabled={busy === 'search'} busy={busy === 'search'} data-testid="album-search-now">{album.state === 'better_wanted' ? 'Find a better copy' : 'Search now'}</Button>
						<Button onclick={() => search(false)} disabled={busy === 'list'} busy={busy === 'list'} data-testid="album-choose">Choose a release</Button>
						<label class="switch">
							<input type="checkbox" checked={album.keep_up_to_date} disabled={busy === 'keep' || !album.included} onchange={(e) => setKeep(e.currentTarget.checked)} />
							<span class="sw" aria-hidden="true"></span>
							<span class="small">Keep up to date</span>
						</label>
					</div>
					<p class="small muted"><span class="chip {st.tone}" data-testid="album-state">{st.label}</span></p>
				</TitleSection>

				<TitleSection id="h-pressings" title="Pressings">
					{#if pressings.length === 0}
						<p class="small muted">{album.included ? 'Pressings come with the next refresh.' : 'Pressings are read only for included albums.'}</p>
					{:else}
						{#if current}
							<p class="small" data-testid="chosen-pressing">
								The tracks follow <b>{pressingLabel(current)}</b> ({current.chosen_by === 'person' ? 'picked by hand' : 'picked by the rule in Settings > Music'}).
							</p>
						{/if}
						<form class="row pick" onsubmit={usePressing}>
							<label class="sr-only" for="pressing">Pressing to follow</label>
							<select id="pressing" class="input" bind:value={chosen}>
								{#each pressings as p (p.id)}
									<option value={p.id}>{pressingLabel(p)}</option>
								{/each}
							</select>
							<Button type="submit" disabled={busy === 'pressing' || chosen === current?.id} busy={busy === 'pressing'}>Use this Pressing</Button>
						</form>
						<p class="hint">Tracks on both Pressings keep their files; tracks the new one lacks are removed unless a file holds them.</p>
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
				</TitleSection>

				<TitleSection id="h-album-history" title="History" testid="album-history">
					{#snippet head()}
						{#if historyTotal > 0}<Button variant="plain" href={to(`/downloads/history?title=${titleId}`)}>All rows for the artist</Button>{/if}
					{/snippet}
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
				</TitleSection>
			</LibrarySection>

			<ShowDeleteDialog bind:this={deleter} titleId={titleId} name={artist?.name ?? 'Artist'} kind="artist" ondone={afterDelete} />

			<dialog bind:this={renameDialog} aria-labelledby="rename-album-title" class="modal wide" onclose={() => (renameOpen = false)}>
				{#if renameOpen}
					<h2 id="rename-album-title" class="h2">Rename the files of {album.name}</h2>
					<p class="small muted">The names now and the names they get from the album and track patterns. Nothing changes until you press Rename.</p>
					<RenamePreview {titleId} {albumId} onDone={() => void afterDelete()} />
					<div class="row modal-actions">
						<Button variant="plain" onclick={() => renameDialog?.close()}>Close</Button>
					</div>
				{/if}
			</dialog>
		{/if}
	</div>
{/if}
</PageLoader>
{/if}

<style>
	.artist-link { color: var(--text); font-weight: 600; }
	.artist-link:hover { text-decoration: underline; text-underline-offset: 3px; }
	.tracks { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; }
	.track { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 14px; min-height: 52px; padding: 8px 10px; border-radius: 10px; font: var(--t-body); }
	.track:hover { background: var(--surface-1); }
	.no { min-width: 2.2em; color: var(--text-muted); font-variant-numeric: tabular-nums; text-align: right; }
	.len { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.mark { font: var(--t-footnote); color: var(--text-muted); }
	.mark.here { color: var(--ok, var(--text)); font-weight: 600; }
	.grow { flex: 1; min-width: 0; overflow-wrap: anywhere; }
	.tools { gap: var(--s-2) var(--s-3); flex-wrap: wrap; align-items: center; }
	.pick { gap: var(--s-2); flex-wrap: wrap; margin: var(--s-3) 0 var(--s-2); }
	.pick select { flex: 1; min-width: 0; }
	.pressings { list-style: none; margin: var(--s-3) 0 0; padding: 0; display: flex; flex-direction: column; }
	.pressings li, .ev { display: flex; align-items: center; flex-wrap: wrap; gap: var(--s-2) var(--s-3); padding: var(--s-3) 0; }
	.pressings li + li, .ev + .ev { border-top: 1px solid var(--line); }
	.events { list-style: none; padding: 0; margin: 0; }
	.modal { width: min(720px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: var(--s-6); border: 0; border-radius: var(--r-modal); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); }
	.modal::backdrop { background: var(--scrim); }
	.modal-actions { justify-content: flex-end; margin-top: var(--s-6); }
	@media (max-width: 600px) { .track { padding-inline: 4px; } }
</style>

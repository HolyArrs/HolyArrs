<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { peopleNames } from './plain.ts';
	import { to } from './paths.ts';
	// One artist: its albums with their kinds, dates and state, Keep up to
	// date per artist and album, the album kinds it includes, and who asked.
	// Search now looks for every missing album; each album has its own
	// Search now, and its page a Choose a release list.
	import { api, message } from './api.ts';
	import { live } from './live.svelte.ts';
	import { ago, date, dateTime } from './format.ts';
	import Icon from './Icon.svelte';
	import TitleHero, { shortFolder } from './ui/TitleHero.svelte';
	import LoadError from './ui/LoadError.svelte';
	import FollowButton from './ui/FollowButton.svelte';
	import ActionButton from './ui/ActionButton.svelte';
	import ActionMenu from './ui/ActionMenu.svelte';
	import type { MenuItem } from './ui/Menu.svelte';
	import SimilarShelf from './SimilarShelf.svelte';
	import LibrarySection from './ui/LibrarySection.svelte';
	import TitleSection from './ui/TitleSection.svelte';
	import SectionNav from './ui/SectionNav.svelte';
	import Shelf from './ui/Shelf.svelte';
	import WideCard from './ui/WideCard.svelte';
	import { reducedMotion } from './motion.ts';
	import { artName, crumbFor, pageSections } from './title-page.ts';
	import './title-page.css';
	import { session } from './session.svelte.ts';
	import { audienceStatus } from './title-status.ts';
	import { toasts } from './toasts.svelte.ts';
	import { decided } from './wishlist.ts';
	import { groupLabel, kindTone } from './history.ts';
	import AskArtistDialog from './AskArtistDialog.svelte';
	import RenamePreview from './RenamePreview.svelte';
	import ShowDeleteDialog from './ShowDeleteDialog.svelte';
	import { goto } from '$app/navigation';
	import {
		albumChoices,
		albumKind,
		albumLine,
		albumStateOf,
		albumWords,
		albumYear,
		artistProgress,
		albumCover,
		artistSearchToast,
		discography,
		latestRelease,
		extraKinds,
		mainKinds,
		newAlbumChoices,
		pressingStatuses,
		toggle,
		word
	} from './music.ts';
	import type { AlbumSummary, Ask, EventPage, EventRecord, LookupArtist, TitleView } from './types.ts';

	let {
		title,
		onchange,
		error = '',
		onretry
	}: { title: TitleView; onchange: (t: TitleView) => void; error?: string; onretry?: () => Promise<void> | void } = $props();

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
	const artist = $derived(title.artist);
	let albums = $state<AlbumSummary[] | null>(null);
	let show = $state<'included' | 'left_out' | 'all'>('included');
	let busy = $state('');
	let refreshing = $state(false);
	let history = $state<EventRecord[]>([]);
	let historyTotal = $state(0);
	let asker: ReturnType<typeof AskArtistDialog> | undefined = $state();
	const visibleDisambiguation = $derived(artist?.disambiguation?.trim() ?? '');
	const heroMeta = $derived(['Artist', artist?.type ? artist.type.charAt(0).toUpperCase() + artist.type.slice(1) : '', artist ? (artist.status === 'ended' ? 'No longer active' : 'Still active') : ''].filter(Boolean));
	const heroKind = $derived(artist?.missing ? 'missing' : 'in_library');
	// One status vocabulary (title-status.ts); staff also see how many are missing.
	const heroWords = $derived(audienceStatus(heroKind, staff ? 'staff' : 'member', { tracked: true, titleKind: 'artist' }));
	// Manager actions live in the hero's ⋯ menu; the Library section holds the rest.
	const heroItems = $derived<MenuItem[]>([
		{ label: 'Search now', icon: 'search', onselect: () => search(), disabled: busy === 'search' },
		{ label: 'Keep up to date', checked: title.keep_up_to_date, disabled: busy === 'artist', onselect: () => setKeep(!title.keep_up_to_date) },
		{ kind: 'separator' },
		{ label: 'Edit album kinds', icon: 'tools', onselect: openEdit },
		{ label: 'Rename files', icon: 'copy', onselect: () => openRename() },
		{ label: 'Refresh albums', icon: 'refresh', onselect: () => refresh(), disabled: refreshing },
		{ label: 'All history', icon: 'clock', href: to(`/downloads/history?title=${title.id}`) },
		{ kind: 'separator' },
		{ label: 'Delete artist', icon: 'trash', destructive: true, onselect: () => deleter?.open() }
	]);
	function toLibrary() {
		const h = document.getElementById('h-library');
		h?.closest('section')?.scrollIntoView({ behavior: reducedMotion() ? 'auto' : 'smooth', block: 'start' });
	}
	// The discography: one wide-card row per kind; Missing only keeps what is not all here yet.
	let missingOnly = $state(false);
	const notHere = (a: AlbumSummary) => a.in_library === 0 || a.missing > 0;
	const missingCount = $derived((albums ?? []).filter(notHere).length);
	const rows = $derived(discography((albums ?? []).filter((a) => !missingOnly || notHere(a))));
	const latest = $derived(missingOnly ? null : latestRelease(albums ?? []));

	// Rename files and delete (4.15.4): the rename preview for this artist,
	// and the delete dialog for the artist.
	let renameDialog: HTMLDialogElement | undefined = $state();
	let renameOpen = $state(false);
	let deleter: ReturnType<typeof ShowDeleteDialog> | undefined = $state();

	function openRename() {
		renameOpen = true;
		renameDialog?.showModal();
	}

	async function deleted() {
		await goto(to('/library'));
	}

	const shown = $derived(
		(albums ?? []).filter((a) => (show === 'all' ? true : show === 'included' ? a.included : !a.included))
	);
	const leftOut = $derived((albums ?? []).filter((a) => !a.included).length);

	async function loadAlbums() {
		try {
			albums = (await api.get<{ albums: AlbumSummary[] }>(`/titles/${title.id}/albums`)).albums;
			loadErr = '';
		} catch (e) {
			albums ??= [];
			loadErr = message(e);
		}
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

	async function reload() {
		try {
			onchange(await api.get<TitleView>(`/titles/${title.id}`));
		} catch (e) {
			loadErr = message(e);
			return;
		}
		await loadAlbums();
		void loadHistory();
	}

	$effect(() => {
		void title.id;
		void loadAlbums();
		void loadHistory();
	});

	// Albums, Pressings and tracks arrive with the refresh after adding:
	// read again when this artist's Events come in, and every few seconds
	// while included albums have no tracks yet.
	let again: ReturnType<typeof setTimeout> | undefined;
	$effect(() => {
		const soon = (e?: unknown) => {
			const ev = e as { TitleID?: number } | undefined;
			if (ev && typeof ev === 'object' && 'TitleID' in ev && ev.TitleID !== title.id) return;
			clearTimeout(again);
			again = setTimeout(() => void reload(), 400);
		};
		const offs = [live.on('event', soon), live.on('slot', () => soon())];
		return () => {
			clearTimeout(again);
			offs.forEach((off) => off());
		};
	});
	let polls = 0;
	$effect(() => {
		const waiting = !!artist && artist.included > 0 && artist.tracks === 0 && polls < 30;
		if (!waiting) return;
		const t = setTimeout(() => {
			polls++;
			void reload().catch(() => {});
		}, 3000);
		return () => clearTimeout(t);
	});

	async function setAlbum(a: AlbumSummary, keep: boolean) {
		busy = `a${a.id}`;
		try {
			const r = await api.put<AlbumSummary>(`/albums/${a.id}`, { keep_up_to_date: keep });
			if (albums) {
				const i = albums.findIndex((x) => x.id === a.id);
				if (i >= 0) albums[i] = r;
			}
			toasts.push(`${a.name}: ${keep ? 'HolyArrs keeps it up to date.' : 'HolyArrs stops looking after it.'}`, 'ok');
			onchange(await api.get<TitleView>(`/titles/${title.id}`));
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}

	// Search now: every album with a track kept up to date and missing, or one album.
	async function search(a?: AlbumSummary) {
		busy = a ? `s${a.id}` : 'search';
		try {
			const r = await api.post<{ job_ids: number[]; left_out?: number }>(`/titles/${title.id}/search`, a ? { album_id: a.id } : {});
			const n = r.job_ids.length;
			toasts.push(a ? `Searching for ${a.name}. Home shows what it finds.` : artistSearchToast(n, r.left_out), n ? 'ok' : 'info');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}

	async function setKeep(v: boolean) {
		busy = 'artist';
		try {
			onchange((await api.put<{ title: TitleView }>(`/titles/${title.id}`, { keep_up_to_date: v })).title);
			toasts.push(v ? 'HolyArrs keeps this artist up to date.' : 'HolyArrs stops looking after this artist.', 'ok');
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
			polls = 0;
			toasts.push('Refreshing albums, Pressings and tracks from MusicBrainz.', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			setTimeout(() => (refreshing = false), 1500);
		}
	}

	// The edit dialog: new albums and album kinds.
	let editDialog: HTMLDialogElement | undefined = $state();
	let eNew = $state('all');
	let eTypes = $state<string[]>([]);
	let eExtras = $state<string[]>([]);
	let eStatuses = $state<string[]>([]);
	let editError = $state('');
	let saving = $state(false);

	function openEdit() {
		editError = '';
		eNew = artist?.new_albums ?? 'all';
		eTypes = [...(artist?.album_types ?? [])];
		eExtras = [...(artist?.secondary_types ?? [])];
		eStatuses = [...(artist?.release_statuses ?? [])];
		editDialog?.showModal();
	}

	async function saveEdit(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		editError = '';
		try {
			onchange(
				(
					await api.put<{ title: TitleView }>(`/titles/${title.id}/artist`, {
						new_albums: eNew,
						album_types: eTypes,
						secondary_types: eExtras,
						release_statuses: eStatuses
					})
				).title
			);
			editDialog?.close();
			polls = 0;
			toasts.push('Saved. A refresh brings in the albums the new kinds include.', 'ok');
		} catch (err) {
			editError = message(err);
		} finally {
			saving = false;
		}
	}

	function askMore() {
		if (!artist) return;
		const a: LookupArtist = { mbid: artist.mbid, name: title.name, status: artist.status, title_id: title.id, disambiguation: artist.disambiguation };
		void asker?.open(a);
	}

	function asked(_a: LookupArtist, r: { ask: Ask; joined: boolean }) {
		const what = `${title.name} (${albumWords(r.ask.albums)})`;
		if (r.ask.state === 'approved') toasts.push(`${what} is approved; HolyArrs keeps it up to date.`, 'ok');
		else if (r.joined) toasts.push(`Someone asked for ${what} already; you are on the ask too.`, 'ok');
		else toasts.push(`Asked for ${what}. You will hear when it is approved.`, 'ok');
		void reload();
	}
</script>

<div class="tp">
<TitleHero
	title={title.name}
	poster={title.artwork[0]}
	backdrop={title.backdrop}
	logo={title.logo}
	mbid={artist?.mbid}
	square
	crumbIcon="music"
	breadcrumb={[{ href: to(crumbFor('artist').path), label: crumbFor('artist').label }, { href: to(`/library/${title.id}`), label: title.name }]}
	meta={heroMeta}
	status={heroWords.kind}
	statusLabel={staff && artist?.missing ? `${artist.missing} missing` : heroWords.label}
	description={visibleDisambiguation}
	vtName={artName(title.id)}
>
	{#snippet byline()}
		{#if staff}<span class="tp-progress" data-testid="artist-progress">{artistProgress(artist)}</span>{/if}
		{#if artist?.next_release}<span><b>Next album:</b> {date(artist.next_release)}</span>{/if}
	{/snippet}
	{#snippet actions()}
		{#if staff}
			<ActionButton icon="library" label="In Library: go to the Library section" caption="In Library" primary onclick={toLibrary} data-testid="hero-library" />
			<ActionMenu label="More actions" caption="More" icon="more" items={heroItems} />
		{:else if session.can('ask')}
			<ActionButton icon="wish" label="Ask for albums" primary onclick={askMore} data-testid="ask-albums" />
		{/if}
		<FollowButton titleId={title.id} kind="artist" />
	{/snippet}
</TitleHero>

{#if loadErr || error}<LoadError hasData message={loadErr || error} onretry={() => void retry()} busy={retrying} />{/if}

<SectionNav items={pageSections('artist', { manage: staff })} />

{#snippet albumCard(a: AlbumSummary)}
	{@const st = albumStateOf(a)}
	<WideCard
		title={a.name}
		href={to(`/library/${title.id}/albums/${a.id}`)}
		image={albumCover(a.id)}
		kind="artist"
		subtitle={[albumYear(a.release_date), albumKind(a)].filter(Boolean).join(' · ')}
		owned={a.in_library > 0 ? { state: a.missing ? 'partial' : 'available', label: a.missing ? `${a.in_library} of ${a.tracks} tracks here` : 'In your Library' } : staff && a.included ? { state: 'requested', label: st.label } : null}
	/>
{/snippet}

{#if albums === null}
	<div class="sk" style="height: 160px" aria-busy="true"></div>
{:else if albums.length === 0}
	<p class="small muted">No albums yet. A refresh adds them once MusicBrainz lists them.</p>
{:else}
	{#if latest}
		<section class="tp-section latest" aria-labelledby="h-latest">
			<h2 id="h-latest" class="tp-h2">Latest release</h2>
			<div class="latest-card">{@render albumCard(latest)}</div>
		</section>
	{/if}
	<div class="disc" id="h-discography">
		{#if missingCount}
			<div class="disc-tools">
				<button type="button" class="pill" aria-pressed={missingOnly} onclick={() => (missingOnly = !missingOnly)} data-testid="missing-only">Missing only <span class="n">{missingCount}</span></button>
			</div>
		{/if}
		{#each rows as g (g.row)}
			<Shelf title={g.label} item="var(--title-item)" placeholder="wide" lines={1} anchor="disc-{g.row}">
				{#each g.albums as a (a.id)}{@render albumCard(a)}{/each}
			</Shelf>
		{:else}
			<p class="small muted">Nothing missing.</p>
		{/each}
	</div>
{/if}

<SimilarShelf {title} />

<TitleSection id="h-about" title="About">
	<dl class="tp-about">
		{#if artist?.type}<dt>Kind</dt><dd>{artist.type.charAt(0).toUpperCase() + artist.type.slice(1)}</dd>{/if}
		<dt>Active</dt>
		<dd>{artist?.status === 'ended' ? 'No longer active' : 'Still active'}</dd>
		<dt>Albums</dt>
		<dd>{albums?.length ?? artist?.albums ?? 0}</dd>
		{#if artist?.next_release}<dt>Next album</dt><dd>{date(artist.next_release)}</dd>{/if}
	</dl>
</TitleSection>

{#if title.asks?.length}
	<TitleSection id="h-artist-asks" title="Wishlist" testid="title-asks">
		{#each title.asks as a (a.id)}
			<dl class="facts">
				<dt>Asked by</dt>
				<dd>{peopleNames(a.people.map((p) => p.name))} · {date(a.asked_at)}</dd>
				<dt>For</dt>
				<dd>{a.album_words ?? albumWords(a.albums)}</dd>
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
	<LibrarySection note="Album choices, files, kinds and history">
		<TitleSection id="h-albums" title="Albums" primary>
			{#snippet head()}
				{#if leftOut}
					<div class="pills" role="group" aria-label="Albums shown">
						<button type="button" class="pill" aria-pressed={show === 'included'} onclick={() => (show = 'included')}>Included</button>
						<button type="button" class="pill" aria-pressed={show === 'left_out'} onclick={() => (show = 'left_out')}>Not included <span class="n">{leftOut}</span></button>
						<button type="button" class="pill" aria-pressed={show === 'all'} onclick={() => (show = 'all')}>All</button>
					</div>
				{/if}
			{/snippet}
			{#if albums === null}
				<div class="sk" style="height: 120px"></div>
			{:else if shown.length === 0}
				<p class="small muted">Nothing here.</p>
			{:else}
				<ul class="albums">
					{#each shown as a (a.id)}
						{@const st = albumStateOf(a)}
						<li class="album" data-testid="album">
							<div class="grow album-copy">
								<a class="album-name" href={to(`/library/${title.id}/albums/${a.id}`)}><b>{a.name}</b></a>
								{#if a.disambiguation}<span class="xs muted"> ({a.disambiguation})</span>{/if}
								<p class="xs muted album-meta">{[albumYear(a.release_date), albumKind(a), a.dropped ? 'no longer listed' : ''].filter(Boolean).join(' · ')}</p>
								<p class="xs muted album-availability" data-testid="album-line">{albumLine(a)}</p>
							</div>
							<span class="chip {st.tone}" data-testid="album-state">{st.label}</span>
							<div class="album-actions">
								<Button variant="plain" size="sm" icon="search" onclick={() => search(a)} disabled={busy === `s${a.id}`} aria-label="Search now for {a.name}" data-testid="album-search"><span class="album-action-label" aria-hidden="true">Search</span></Button>
								<label class="switch" title={a.included ? '' : 'Not included by the album kinds'}>
									<input type="checkbox" checked={a.keep_up_to_date} disabled={busy === `a${a.id}` || !a.included} onchange={(e) => setAlbum(a, e.currentTarget.checked)} />
									<span class="sw" aria-hidden="true"></span>
									<span class="sr-only">Keep {a.name} up to date</span>
									<span class="album-action-label" aria-hidden="true">Keep up to date</span>
								</label>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</TitleSection>

		<TitleSection id="h-artist-facts" title="How HolyArrs keeps it">
			<dl class="tp-about">
				<dt>Keep up to date</dt>
				<dd>{title.keep_up_to_date ? 'On' : 'Off'}</dd>
				<dt>Albums</dt>
				<dd>{artist?.included ?? 0} included of {artist?.albums ?? 0}</dd>
				{#if artist?.album_choice}<dt>Picked when added</dt>
					<dd>{word(albumChoices, artist.album_choice)}</dd>{/if}
				<dt>New albums</dt>
				<dd>{word(newAlbumChoices, artist?.new_albums)}</dd>
				<dt>Album kinds</dt>
				<dd>{(artist?.album_types ?? []).map((k) => word(mainKinds, k)).join(', ')}</dd>
				<dt>Extra kinds</dt>
				<dd>{(artist?.secondary_types ?? []).map((k) => word(extraKinds, k)).join(', ')}</dd>
				<dt>Pressings that count</dt>
				<dd>{(artist?.release_statuses ?? []).map((k) => word(pressingStatuses, k)).join(', ')}</dd>
				{#if title.pick_rules_name}<dt>Pick rules</dt>
					<dd>{title.pick_rules_name}</dd>{/if}
				{#if title.refreshed_at}<dt>Refreshed</dt>
					<dd>{date(title.refreshed_at)}</dd>{/if}
			</dl>
			<p class="tp-why" data-testid="gate-reason">{title.gate_reason}</p>
			{#if title.folder_path}<details class="tp-details"><summary>Details</summary><ul><li>Folder: {shortFolder(title.folder_path)}</li></ul></details>{/if}
		</TitleSection>

		<TitleSection id="h-artist-history" title="History" testid="title-history">
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
							<span class="grow small">{e.Summary}</span>
							<time class="mono xs muted" datetime={e.At} title={dateTime(e.At)}>{ago(e.At)}</time>
						</li>
					{/each}
				</ol>
			{/if}
		</TitleSection>
	</LibrarySection>
{/if}
</div>

{#if !staff && session.can('ask')}
	<AskArtistDialog bind:this={asker} onasked={asked} />
{/if}

{#if staff}
	<ShowDeleteDialog bind:this={deleter} titleId={title.id} name={title.name} kind="artist" ondone={deleted} />
{/if}

<dialog bind:this={renameDialog} aria-labelledby="rename-artist-title" class="modal wide" onclose={() => (renameOpen = false)}>
	{#if renameOpen}
		<h2 id="rename-artist-title" class="h2">Rename the files of {title.name}</h2>
		<p class="small muted">The names now and the names they get from the artist, album and track patterns. Nothing changes until you press Rename.</p>
		<RenamePreview titleId={title.id} onDone={() => void reload()} />
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => renameDialog?.close()}>Close</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={editDialog} aria-labelledby="edit-artist-title" class="modal">
	<form onsubmit={saveEdit}>
		<h2 id="edit-artist-title" class="h2">Edit {title.name}</h2>
		<div class="field" style="margin-top: var(--s-5)">
			<label for="e-new">New albums</label>
			<select id="e-new" class="input" bind:value={eNew}>
				{#each newAlbumChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
			</select>
		</div>
		<fieldset class="kinds">
			<legend class="small"><b>Album kinds</b></legend>
			<div class="boxes">
				{#each mainKinds as [v, l] (v)}
					<label class="box"><input type="checkbox" checked={eTypes.includes(v)} onchange={(e) => (eTypes = toggle(eTypes, v, e.currentTarget.checked, mainKinds))} /> {l}</label>
				{/each}
			</div>
		</fieldset>
		<fieldset class="kinds">
			<legend class="small"><b>Extra kinds</b></legend>
			<div class="boxes">
				{#each extraKinds as [v, l] (v)}
					<label class="box"><input type="checkbox" checked={eExtras.includes(v)} onchange={(e) => (eExtras = toggle(eExtras, v, e.currentTarget.checked, extraKinds))} /> {l}</label>
				{/each}
			</div>
		</fieldset>
		<fieldset class="kinds">
			<legend class="small"><b>Pressings that count</b></legend>
			<div class="boxes">
				{#each pressingStatuses as [v, l] (v)}
					<label class="box"><input type="checkbox" checked={eStatuses.includes(v)} onchange={(e) => (eStatuses = toggle(eStatuses, v, e.currentTarget.checked, pressingStatuses))} /> {l}</label>
				{/each}
			</div>
		</fieldset>
		<p class="hint" style="margin-top: var(--s-3)">Albums the new kinds leave out stay listed under Not included; their files stay where they are.</p>
		{#if editError}<p class="form-error" role="alert">{editError}</p>{/if}
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => editDialog?.close()}>Cancel</Button>
			<Button variant="primary" type="submit" disabled={saving || !eTypes.length || !eExtras.length || !eStatuses.length} busy={saving}>Save</Button>
		</div>
	</form>
</dialog>

<style>
	.tp-progress { color: var(--text-muted); }
	.card-title {
		margin-bottom: var(--s-4);
	}
	.card-head {
		flex-wrap: wrap;
		gap: var(--s-3);
		margin-bottom: var(--s-3);
	}
	.path {
		overflow-wrap: anywhere;
	}
	.pills {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-2);
	}
	.pill {
		min-height: 36px;
		padding: 0 var(--s-3);
		border-radius: var(--r-pill);
		border: 1px solid var(--line-strong);
		color: var(--text-muted);
		font-size: var(--text-sm);
	}
	.pill[aria-pressed='true'] {
		background: var(--surface-1);
		border-color: var(--line-strong);
		color: var(--text);
		box-shadow: var(--shadow-panel);
	}
	.n {
		margin-left: 4px;
		font-variant-numeric: tabular-nums;
	}
	.albums {
		display: flex;
		flex-direction: column;
	}
	.album {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		padding: var(--s-4) 0;
	}
	.album + .album {
		border-top: 1px solid var(--line);
	}
	.album .grow {
		min-width: 0;
		flex: 1;
		overflow-wrap: anywhere;
	}
	.album-actions {
		display: flex;
		align-items: center;
		gap: var(--s-2);
	}
	/* An album's own buttons are scrolled to (a click, a focus) from far
	   below the artist's buttons. The margin keeps the artist's hero
	   buttons clear of the sticky top bar when that scroll centres the row. */
	.album-actions :global(.ui-btn),
	.album-actions .switch {
		scroll-margin-top: 160px;
	}
	.album-action-label {
		display: none;
	}
	.album-name {
		display: inline-flex;
		align-items: center;
		min-height: var(--tap);
		text-decoration: underline;
		text-decoration-color: var(--line-strong);
	}
	.modal {
		width: min(520px, calc(100vw - 32px));
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
	.kinds {
		border: 0;
		padding: 0;
		margin: var(--s-4) 0 0;
	}
	.kinds legend {
		margin-bottom: var(--s-2);
	}
	.boxes {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
		gap: var(--s-2);
	}
	.box {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		min-height: var(--tap);
		font-size: var(--text-sm);
	}
	@media (max-width: 600px) {
		.album {
			display: grid;
			grid-template-columns: minmax(0, 1fr) auto;
			align-items: center;
			gap: var(--s-2) var(--s-3);
			padding: var(--s-4) 0;
		}
		.album-copy {
			grid-column: 1 / -1;
		}
		.album-name {
			min-height: 36px;
		}
		.album-meta,
		.album-availability {
			max-width: 42ch;
			line-height: 1.45;
		}
		.album-availability {
			margin-top: 2px;
		}
		.album > .chip {
			grid-column: 1;
			justify-self: start;
		}
		.album-actions {
			grid-column: 1 / -1;
			display: grid;
			grid-template-columns: repeat(2, minmax(0, 1fr));
			justify-self: stretch;
		}
		.album-actions :global(.ui-btn),
		.album-actions .switch {
			width: 100%;
			min-width: 44px;
			min-height: 44px;
		}
		.album-actions :global(.ui-btn) {
			justify-content: center;
			gap: var(--s-2);
			padding: 0 var(--s-3);
		}
		.album-actions .switch {
			justify-content: center;
			gap: var(--s-2);
		}
		.album-action-label {
			display: inline;
			font-size: var(--text-sm);
			font-weight: var(--weight-medium);
		}
	}
	.latest-card { width: min(100%, calc(var(--title-item) * 1.0)); }
	.disc { display: flex; flex-direction: column; gap: clamp(28px, 4vw, 44px); }
	.disc-tools { display: flex; gap: 8px; margin-bottom: calc(clamp(28px, 4vw, 44px) * -0.6); }
</style>

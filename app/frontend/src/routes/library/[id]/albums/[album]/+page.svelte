<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import '#lib/ui/title-page.css';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { TitleEmpty as EmptyState } from '#lib/mediafiles/index.ts';
	import TitleHero from '#lib/ui/TitleHero.svelte';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import ActionMenu from '#lib/ui/ActionMenu.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import TitleSection from '#lib/ui/TitleSection.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import { audienceStatus } from '#lib/ui/title-status.ts';
	import { SkTrackRow } from '#lib/ui/sk/index.ts';
	import { HistoryLine as CompactRow, isRoutine, groupDownloads } from '#lib/downloads/index.ts';
	import { crumbFor, RenamePreview, ShowDeleteDialog, ShowReleases } from '#lib/mediafiles/index.ts';
	import { albumCover, albumKind, albumStateOf, albumYear, pressingLabel, trackLength, noSongsLine, TRACK_REASK_MS, sleep, trackListView } from '#lib/music/index.ts';
	import type { PressingView, TrackView } from '#lib/music/index.ts';
	import { ReportSheet, hasFile } from '#lib/requests/index.ts';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { tone } from '#lib/core/format.ts';
	import { scrollToStart } from '#lib/core/motion.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import type { AlbumSummary, EventPage, EventRecord, TitleView } from '#lib/core/types.ts';
	import { t as tr } from '#lib/core/i18n/i18n.svelte.ts';

	// One album: its tracks with their state, every Pressing MusicBrainz lists, its History, and for the staff Keep up to date, which Pressing is used, Rename files for the album and Delete for the album or a track.

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
	let renameOpen = $state(false);
	let deleter: ReturnType<typeof ShowDeleteDialog> | undefined = $state();
	function openRename() {
		renameOpen = true;
	}
	function trackLabel(t: TrackView): string {
		const n = String(t.number).padStart(2, '0');
		return `${discs > 1 ? tr('library.album.discTrack', { disc: t.disc, n }) : tr('library.album.track', { n })}${t.name ? ` ${t.name}` : ''}`;
	}
	async function afterDelete() {
		await load();
		void loadHistory();
	}

	// No songs yet: ask again quietly with growing pauses, about a minute, then show a neutral line.
	let askingSongs = $state(false);
	let songsStop: AbortController | null = null;
	async function askSongs(id: number) {
		songsStop?.abort();
		const stop = (songsStop = new AbortController());
		askingSongs = true;
		try {
			for (const ms of TRACK_REASK_MS) {
				await sleep(ms, stop.signal);
				try {
					const got = (await api.get<{ tracks: TrackView[] }>(`/albums/${id}/tracks`)).tracks;
					if (id !== albumId || stop.signal.aborted) return;
					if (got.length) {
						tracks = got;
						return;
					}
				} catch {
					/* Keep what is shown and try again. */
				}
			}
		} catch {
			return;
		} finally {
			if (songsStop === stop) askingSongs = false;
		}
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
			if (!tracks.length && r.album.included) void askSongs(albumId);
		} catch (e) {
			const m = message(e);
			if (/no such album/i.test(m)) notFound = true;
			else error = m;
		}
	}

	$effect(() => {
		void albumId;
		askingSongs = false;
		void load();
		void loadHistory();
		return () => songsStop?.abort();
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
		return m >= 60 ? tr('library.album.hrMin', { h: Math.floor(m / 60), m: m % 60 }) : tr('shows.sheet.minutes', { n: m });
	});
	const audience = $derived(audienceStatus(album && album.in_library > 0 && !album.missing ? 'in_library' : 'missing', staff ? 'staff' : 'member', { tracked: true, titleKind: 'artist' }));
	const heroItems = $derived<MenuItem[]>([
		{ label: album?.state === 'better_wanted' ? tr('shows.search.better') : tr('mediafiles.select.searchNow'), icon: 'search', onselect: () => search(true), disabled: busy === 'search' },
		{ label: tr('mediafiles.releaseSheet.choose'), icon: 'grid', onselect: () => search(false), disabled: busy === 'list' },
		{ label: tr('mediafiles.select.keep'), checked: !!album?.keep_up_to_date, disabled: busy === 'keep' || !album?.included, onselect: () => album && setKeep(!album.keep_up_to_date) },
		{ kind: 'separator' },
		{ label: tr('shows.page.renameFiles'), icon: 'copy', onselect: openRename },
		{ kind: 'separator' },
		{ label: tr('library.album.deleteAlbum'), icon: 'trash', destructive: true, onselect: () => deleter?.open({ albumId, label: album?.name ?? tr('library.album.album') }) }
	]);
	// Report a problem: anyone, once a track of the album has a file; members get it as their only More item.
	let reportOpen = $state(false);
	const mayReport = $derived(!!artist && (tracks ?? []).some(hasFile));
	const moreItems = $derived<MenuItem[]>([...(staff ? heroItems : []), ...(mayReport ? [...(staff ? [{ kind: 'separator' as const }] : []), { label: tr('library.title.report'), icon: 'alert' as const, onselect: () => (reportOpen = true) }] : [])]);
	function toLibrary() {
		scrollToStart(document.getElementById('album-library'));
	}

	// Search now lets HolyArrs pick; Choose a release lists every Release.
	let listing = $state(false);
	let listJobs = $state<number[]>([]);
	const scope = $derived({ label: `${artist?.name ?? tr('mediafiles.kindCap.artist')} – ${album?.name ?? tr('library.album.album')}`, query: `album_id=${albumId}` });

	async function search(choose: boolean) {
		busy = choose ? 'search' : 'list';
		try {
			const r = await api.post<{ job_ids: number[] }>(`/titles/${titleId}/search`, choose ? { album_id: albumId } : { album_id: albumId, choose: false });
			if (choose) toasts.push(tr('mediafiles.showReleases.searchingFor', { name: album?.name ?? '' }), 'info');
			else {
				listJobs = r.job_ids;
				listing = true;
			}
		} catch (e) {
			toasts.push(tr('mediafiles.title.startFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	async function setKeep(keep: boolean) {
		busy = 'keep';
		try {
			album = await api.put<AlbumSummary>(`/albums/${albumId}`, { keep_up_to_date: keep });
			toasts.push(tr(keep ? 'library.album.kept' : 'library.album.notKept'), 'ok');
			await load();
		} catch (e) {
			toasts.push(tr('library.album.updateFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	// History as compact rows like the movie and artist pages; Edition as a menu that asks before it changes.
	const histRows = $derived(groupDownloads(history.filter((e) => !isRoutine(e.Kind))));
	const histTitles = $derived(new Map(artist ? [[artist.id, artist]] : []));
	let editionOpen = $state(false);
	const editionMenu = $derived<MenuItem[]>(pressings.map((p) => ({ label: pressingLabel(p), checked: !!p.chosen, onselect: () => { if (!p.chosen) { chosen = p.id; editionOpen = true; } } })));
	async function usePressing(e?: Event) {
		e?.preventDefault();
		editionOpen = false;
		busy = 'pressing';
		try {
			album = await api.put<AlbumSummary>(`/albums/${albumId}`, { pressing_id: chosen });
			toasts.push(tr('library.album.pressingChanged'), 'ok', { body: pressings.find((p) => p.id === chosen)?.name ?? tr('library.album.tracksFollow') });
			await load();
		} catch (err) {
			toasts.push(tr('library.album.pressingFailed'), 'fail', { body: message(err) });
		} finally {
			busy = '';
		}
	}
</script>

<svelte:head><title>{album?.name ?? tr('library.album.album')} · HolyArrs</title></svelte:head>

{#if notFound}
	<h1 class="sr-only">{tr('library.album.album')}</h1>
	<EmptyState icon="music" title={tr('library.album.notInLibrary')}>
		<p>{tr('library.album.noLonger')}</p>
		{#snippet actions()}<Button href={to(`/library/${titleId}`)}>{tr('library.album.backArtist')}</Button>{/snippet}
	</EmptyState>
{:else if error}
	<p class="notice bad" role="alert">{error}</p>
{:else}
<PageLoader loading={!album} label={tr('library.album.loading')}>
{#if album}
	{@const st = albumStateOf(album)}
	<div class="tp">
		<TitleHero
			title={album.name}
			cover={coverOk ? albumCover(album.id) : null}
			poster={coverOk ? null : artist?.artwork[0]}
			square
			crumbIcon="music"
			breadcrumb={[{ href: to(crumbFor('artist').path), label: crumbFor('artist').label }, { href: to(`/library/${titleId}`), label: artist?.name ?? tr('mediafiles.kindCap.artist') }, { href: to(`/library/${titleId}/albums/${albumId}`), label: album.name }]}
			meta={[albumYear(album.release_date), albumKind(album), tracks?.length ? tr('mediafiles.unit.track.' + (tracks.length === 1 ? 'one' : 'other'), { n: tracks.length }) : '', totalLength].filter(Boolean)}
			status={audience.kind}
			statusLabel={staff ? st.label : audience.label}
			description={album.disambiguation}
		>
			{#snippet byline()}
				{#if artist}<a class="artist-link" href={to(`/library/${titleId}`)}>{artist.name}</a>{/if}
				{#if album!.dropped}<span>{tr('library.album.noLongerListed')}</span>{/if}
			{/snippet}
			{#snippet actions()}
				{#if staff}
					<ActionButton icon="library" label={tr('shows.page.goLibrary')} caption={tr('shows.page.inLibraryCaption')} primary onclick={toLibrary} data-testid="hero-library" />
					<ActionMenu label={tr('shows.page.moreActions')} caption={tr('shows.page.more')} icon="more" items={moreItems} />
				{:else if mayReport}
					<ActionMenu label={tr('shows.page.moreActions')} caption={tr('shows.page.more')} icon="more" items={moreItems} />
				{/if}
			{/snippet}
		</TitleHero>

		<div class="alb" class:staff>
		<div class="alb-main">
		<TitleSection id="h-tracks" title={tr('library.album.tracks')} primary>
			{#if !tracks?.length && album.included && trackListView(0, askingSongs || tracks === null) === 'skeleton'}
				<ol class="tracks" aria-busy="true" aria-label={tr('library.album.loadingTracks')}>{#each Array.from({ length: 8 }, (_, i) => i) as i (i)}<li><SkTrackRow height={52} /></li>{/each}</ol>
			{:else if !tracks?.length}
				<p class="small muted">{album.included ? noSongsLine() : tr('library.album.leftOut')}</p>
			{:else}
				<ol class="tracks">
					{#each tracks as t (t.id)}
						{@const here = t.slots.some((sl) => tone(sl.state) === 'ok')}
						<li class="track" data-testid="track">
							<span class="no" aria-hidden="true">{discs > 1 ? `${t.disc}-` : ''}{t.number}</span>
							<span class="grow">{t.name || tr('shows.page.nameUnknown')}</span>
							<span class="len">{trackLength(t.length_ms)}</span>
							{#if staff}
								{#each t.slots as sl (sl.id)}
									<span class="chip {tone(sl.state)}" data-testid="track-state">{t.slots.length > 1 ? `${sl.version}: ` : ''}{serverWords(sl.state_label)}</span>
								{/each}
								<IconButton icon="trash" size="sm" tipSide="below-end" label={tr('mediafiles.showDelete.deleteLabel', { what: trackLabel(t) })} onclick={() => deleter?.open({ trackIds: [t.id], label: trackLabel(t) })} data-testid="delete-track" />
							{:else}
								<span class="mark" class:here data-testid="track-state">{here ? tr('shows.list.inLibrary') : tr('library.album.notHere')}</span>
							{/if}
						</li>
					{/each}
				</ol>
			{/if}
		</TitleSection>
		</div>
		{#if staff}
			<aside class="alb-side" id="album-library">
				<section class="alb-card">
					<p class="alb-state"><span class="dot {st.tone}" aria-hidden="true"></span><span data-testid="album-state">{st.label}</span></p>
					<p class="alb-sub">{tr('library.album.haveOf', { have: album.in_library, total: album.tracks })}</p>
					<div class="alb-acts">
						<button type="button" class="alb-btn primary" disabled={busy === 'search'} onclick={() => search(true)} data-testid="album-search-now"><Icon name="search" size={15} />{album.state === 'better_wanted' ? tr('shows.search.better') : tr('mediafiles.select.searchNow')}</button>
						<button type="button" class="alb-btn" disabled={busy === 'list'} onclick={() => search(false)} data-testid="album-choose"><Icon name="grid" size={15} />{tr('mediafiles.releaseSheet.choose')}</button>
					</div>
				</section>

				<h3 class="alb-h">{tr('shows.tools.history')}</h3>
				<div class="alb-hist" data-testid="album-history">
					{#each histRows as r (r.key)}
						<div data-testid="album-event"><CompactRow row={r} ctx={{ titles: histTitles }} titleById={histTitles} onclick={() => goto(to(`/downloads/history?title=${titleId}`))} /><span class="sr-only">{r.events?.[0]?.Summary ?? ''}</span></div>
					{:else}<p class="small muted">{tr('library.album.nothingYet')}</p>{/each}
				</div>

				<h3 class="alb-h">{tr('library.album.settings')}</h3>
				<div class="alb-rows">
					<button type="button" class="alb-row" role="switch" aria-checked={album.keep_up_to_date} disabled={busy === 'keep' || !album.included} onclick={() => setKeep(!album!.keep_up_to_date)}>
						<span><b>{tr('shows.sheet.keep')}</b><small>{album.included ? tr('library.album.keepHint') : tr('library.album.keepOut')}</small></span>
						<span class="alb-switch" class:on={album.keep_up_to_date} aria-hidden="true"></span>
					</button>
					{#if pressings.length > 1 && current}
						<div class="alb-row static" data-testid="chosen-pressing">
							<span><b>{tr('library.album.edition')}</b><small>{current.chosen_by === 'person' ? tr('library.album.byHand') : tr('library.album.byRule')}</small></span>
							<div class="alb-pick"><Menu label={tr('library.album.edition')} variant="button" width={340} items={editionMenu}>{#snippet trigger()}<i>{pressingLabel(current!).replace(album!.name + ' · ', '')}</i><Icon name="chevronDown" size={14} />{/snippet}</Menu></div>
						</div>
					{/if}
				</div>
			</aside>
		{/if}
		</div>

		{#if more.length}
			<Shelf title={tr('library.album.moreBy', { name: artist?.name ?? tr('library.album.theArtist') })} item="var(--title-item)" anchor="title-similar" placeholder="wide" lines={1}>
				{#each more as a (a.id)}
					<WideCard title={a.name} href={to(`/library/${titleId}/albums/${a.id}`)} image={albumCover(a.id)} needsArt kind="artist" subtitle={[albumYear(a.release_date), albumKind(a)].filter(Boolean).join(' · ')} owned={a.in_library > 0 ? { state: a.missing ? 'partial' : 'available', label: a.missing ? tr('library.album.tracksHere', { have: a.in_library, total: a.tracks }) : tr('shows.list.inLibrary') } : null} />
				{/each}
			</Shelf>
		{/if}

		{#if staff}
			<Sheet bind:open={listing} title={tr('mediafiles.releaseSheet.choose')} description={scope.label} size="lg">
				{#if listing}<ShowReleases titleId={artist?.id ?? titleId} {scope} jobs={listJobs} music bare onclose={() => (listing = false)} onchange={load} />{/if}
			</Sheet>
			<Sheet bind:open={editionOpen} title={tr('library.album.followEdition')} description={pressings.find((p) => p.id === chosen) ? pressingLabel(pressings.find((p) => p.id === chosen)!) : ''} size="sm">
				<p class="small muted">{tr('library.album.followNote')}</p>
				<div class="row modal-actions">
					<Button variant="plain" onclick={() => (editionOpen = false)}>{tr('mediafiles.dialogs.cancel')}</Button>
					<Button variant="primary" disabled={busy === 'pressing'} busy={busy === 'pressing'} onclick={() => usePressing()}>{tr('library.album.useEdition')}</Button>
				</div>
			</Sheet>

			<ShowDeleteDialog bind:this={deleter} titleId={titleId} name={artist?.name ?? tr('mediafiles.kindCap.artist')} kind="artist" ondone={afterDelete} />

			<Sheet bind:open={renameOpen} title={tr('shows.page.renameFiles')} description={album.name} size="md">
				{#if renameOpen}<RenamePreview {titleId} {albumId} onDone={() => { renameOpen = false; void afterDelete(); }} />{/if}
			</Sheet>
		{/if}
		<!-- Last in the page, never right after the hero: title-page.css gives the hero's next sibling its top margin, which would pull the sheet up. -->
		{#if artist && mayReport}<ReportSheet bind:open={reportOpen} title={artist} album={{ id: album.id, name: album.name, tracks: tracks ?? [] }} />{/if}
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
	/* Tracks beside the album's Library column, like a movie's Library tab. */
	.alb.staff { display: grid; grid-template-columns: minmax(0, 1fr) 360px; gap: 40px; align-items: start; }
	.alb-main { min-width: 0; }
	.alb-side { display: grid; gap: 12px; min-width: 0; padding-top: 8px; scroll-margin-top: 90px; }
	.alb-card { display: grid; gap: 6px; padding: 18px 20px; border-radius: 16px; background: var(--surface-2); }
	.alb-card p { margin: 0; }
	.alb-state { display: flex; align-items: center; gap: 8px; font: 700 18px/1.2 var(--font-ui); }
	.alb-state .dot { width: 8px; height: 8px; border-radius: 99px; background: var(--text-muted); }
	.alb-state .dot.ok { background: var(--ok, #30d158); }
	.alb-state .dot.warn, .alb-state .dot.part { background: var(--wait); }
	.alb-state .dot.bad { background: var(--fail, #ff453a); }
	.alb-sub { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.alb-acts { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
	.alb-btn { display: inline-flex; align-items: center; gap: 8px; height: 40px; padding: 0 18px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	.alb-btn.primary { background: var(--text); color: var(--bg); }
	.alb-btn:disabled { opacity: var(--disabled-opacity); }
	.alb-h { margin: 16px 0 0; font: 700 20px/1.2 var(--font-ui); }
	.alb-hist { display: grid; gap: 2px; margin: 0 -12px; }
	.alb-rows { display: grid; grid-template-columns: minmax(0, 1fr); border-radius: 16px; background: var(--surface-2); overflow: hidden; }
	.alb-row { display: flex; align-items: center; justify-content: space-between; gap: 14px; width: 100%; padding: 14px 18px; border: 0; background: none; color: var(--text); text-align: left; font: inherit; cursor: pointer; }
	.alb-row:not(.static):hover { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.alb-row:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.alb-row + .alb-row { box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.alb-row.static { cursor: default; }
	.alb-row > span { display: grid; gap: 3px; min-width: 0; }
	.alb-row b { font: 600 15px/1.2 var(--font-ui); }
	.alb-row small { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.alb-switch { position: relative; flex: none; width: 44px; height: 26px; border-radius: 99px; background: color-mix(in srgb, var(--text) 18%, transparent); }
	.alb-switch::after { content: ''; position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 99px; background: #fff; transition: transform var(--dur-fast) var(--ease-out); }
	.alb-switch.on { background: var(--ok); }
	.alb-switch.on::after { transform: translateX(18px); }
	.alb-pick { flex: none; max-width: 60%; min-width: 0; }
	.alb-pick :global(.ui-pop) { display: flex; max-width: 100%; }
	.alb-pick :global(.trigger) { max-width: 100%; min-width: 0; overflow: hidden; height: 34px; padding: 0 10px; border-radius: 9px; color: var(--text-muted); font: 500 14px/1 var(--font-ui); }
	.alb-pick i { font-style: normal; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	@media (max-width: 1099px) { .alb.staff { grid-template-columns: minmax(0, 1fr); gap: 8px; } }
	.ev { display: flex; align-items: center; flex-wrap: wrap; gap: var(--s-2) var(--s-3); padding: var(--s-3) 0; }
	.ev + .ev { border-top: 1px solid var(--line); }
	.events { list-style: none; padding: 0; margin: 0; }
	.modal { width: min(720px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: var(--s-6); border: 0; border-radius: var(--r-modal); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); }
	.modal::backdrop { background: var(--scrim); }
	.modal-actions { justify-content: flex-end; margin-top: var(--s-6); }
	@media (max-width: 600px) { .track { padding-inline: 4px; } }
</style>

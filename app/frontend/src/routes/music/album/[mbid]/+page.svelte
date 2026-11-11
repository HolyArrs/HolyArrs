<script lang="ts">
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import { dragScroll } from '#lib/ui/drag-scroll.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import PersonCircle from '#lib/ui/PersonCircle.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { SkRail, SkTrackRow } from '#lib/ui/sk/index.ts';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import { followHint, followLabel, followPath, setFollow } from '#lib/ui/follow.ts';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import { AskArtistDialog, AddArtistDialog } from '#lib/requests/index.ts';
	import { albumYear, GapNotice, gapWaits, keepShown, noSongsLine, reaskWhileIncomplete, sameLists, sameTracks, TRACK_REASK_MS, trackListView, albumArtistFix, albumHref, appPicture, artistHref, musicLoadError, askAgainOnce, coreIfSlow, coverFor, withCover, kindWord, length, owned, type AlbumCoreData, type AlbumPageData, type PageAlbum, type PageArtist, type PageCredit, type PageTrack, paintWindow, sampleCover, tintOf, unpaintWindow } from '#lib/music/index.ts';
	import type { LookupArtist, MusicSettings } from '#lib/music/index.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { beforeNavigate, goto } from '$app/navigation';
	import { api, message } from '#lib/core/api.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { refreshInbox } from '#lib/core/inbox.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import { pictureOf } from '#lib/core/pictures.ts';
	import { minimalLook } from '#lib/core/look.svelte.ts';
	import type { Ask, LibraryFolder, PickRulesView } from '#lib/core/types.ts';

	// An album's page: the cover beside its title and actions, the tracklist with what the Library has, its other editions, and where to go next.
	const mbid = $derived((page.params.mbid ?? '').toLowerCase());
	const artist = $derived((page.url.searchParams.get('artist') ?? '').toLowerCase());
	// Hero first: the core answer draws the cover, name and artist; the full answer (tracks, editions, more by) and the similar artists follow.
	let core = $state<AlbumCoreData | null>(null);
	let data = $state<AlbumPageData | null>(null);
	let similar = $state<PageArtist[] | null>(null);
	let error = $state('');
	let errorNet = $state(false);
	let busy = $state(false);
	let lookup = $state<LookupArtist | null>(null);
	let coverIn = $state(false);
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let defaults = $state<MusicSettings | null>(null);
	let askDialog: ReturnType<typeof AskArtistDialog> | undefined = $state();
	let addDialog: ReturnType<typeof AddArtistDialog> | undefined = $state();
	let askedHere = $state(false);
	let near = $state<PageCredit[] | null>(null);
	const moreBy = $derived(withCover(data?.more_by));
	let edition = $state('');
	let asking = $state(false);
	// A slow source: the answer came incomplete and is asked again quietly (lib/music/reask.ts).
	let reasking = $state(false);
	// The answer was already kept by the app (fetched ahead or seen within a minute), so no loader hold.
	let fromCache = $state(false);
	let leave: AbortController | null = null;

	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask'));

	function load() {
		const want = `${mbid}|${artist}`;
		const still = () => want === `${mbid}|${artist}`;
		busy = true;
		error = '';
		leave?.abort();
		const stop = (leave = new AbortController());
		const full = `/music/albums/${mbid}?artist=${artist}&similar=0`;
		fromCache = api.has(full) || (!!artist && api.has(`/music/albums/${mbid}?part=core&artist=${artist}`));
		// All at once: the hero, the tracks, the similar artists and the ask state.
		const fullRead = askAgainOnce(() => api.get<AlbumPageData>(full));
		void fullRead.then((d) => {
			if (!still()) return;
			// An incomplete answer is shown, then asked again quietly and merged in, keeping what is on screen.
			void reaskWhileIncomplete(d, () => api.get<AlbumPageData>(full), (v, pending) => {
				if (!still() || stop.signal.aborted) return;
				data = keepShown(data, v, ['tracks', 'editions', 'more_by']);
				reasking = pending;
			}, stop.signal, gapWaits(d, TRACK_REASK_MS), undefined, (a, b) => (a.tracks.length ? sameLists<AlbumPageData>(['tracks', 'editions', 'more_by'])(a, b) : sameTracks(a, b)));
			// A link with a missing or wrong artist is mended quietly: the address takes the album's real artist.
			const fix = albumArtistFix(artist, d.artist?.mbid);
			if (fix && artist && typeof history !== 'undefined') {
				const u = new URL(location.href);
				u.searchParams.set('artist', fix);
				try { history.replaceState(history.state, '', u); } catch { /* the address stays as it was */ }
			}
			// A bare album link learns its artist from the full answer: ask the nearby albums now.
			const found = d.artist?.mbid;
			if (!artist && found) void api.get<{ near: PageCredit[] }>(`/music/albums/${mbid}?part=near&artist=${found}`).then((r) => still() && (near = withCover(r.near))).catch(() => still() && (near = []));
		}).catch((e) => {
			if (!still()) return;
			({ text: error, network: errorNet } = musicLoadError(e, 'album'));
		}).finally(() => {
			if (still()) busy = false;
		});
		// The quick core and nearby parts need the artist; a bare album link waits for the full answer instead. The full answer holds the core, so the quick core is asked only when the full one is slow.
		if (artist) void coreIfSlow(fullRead, () => api.get<AlbumCoreData>(`/music/albums/${mbid}?part=core&artist=${artist}`)).then((c) => c && still() && !data && (core = c)).catch(() => {});
		// A bare album link has no artist: skip the artist calls instead of asking for /music/artists/.
		if (!artist) similar = [];
		else void api.get<{ similar: PageArtist[] }>(`/music/artists/${artist}?part=similar&n=14`).then((r) => still() && (similar = withCover(r.similar))).catch(() => still() && (similar = []));
		if (artist) void api.get<{ near: PageCredit[] }>(`/music/albums/${mbid}?part=near&artist=${artist}`).then((r) => still() && (near = withCover(r.near))).catch(() => still() && (near = []));
		if (artist) void api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(`mbid:${artist}`)}`).then((r) => {
			if (still()) lookup = r.artists[0] ?? null;
		}).catch(() => {});
	}

	$effect(() => {
		void mbid;
		void artist;
		core = null;
		data = null;
		similar = null;
		near = null;
		edition = '';
		lookup = null;
		coverIn = false;
		askedHere = false;
		reasking = false;
		sampled = null;
		showEditions = false;
		void load();
		return () => leave?.abort();
	});

	onMount(() => {
		if (session.can('manage_library')) {
			void api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
			void api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
			void api.get<MusicSettings>('/settings/music').then((r) => (defaults = r)).catch(() => {});
		}
	});

	const hero = $derived<AlbumCoreData | null>(data ?? core);
	// A cover the server has not stored yet is asked for again while the page is open (20 s, 70 s, 150 s) and swaps in once stored; until then the placeholder shows.
	const COVER_RETRY = [20000, 70000, 150000];
	let coverTry = $state(0);
	let coverFailed = $state(false);
	const cover = $derived.by(() => {
		const p = appPicture(hero?.image);
		return p && coverTry ? `${p}${p.includes('?') ? '&' : '?'}fresh=${coverTry}` : p;
	});
	// The hero cover at its slot (the big size only on sharp screens), on its main colour until it arrives; the preload asks the same address.
	const heroCover = $derived(coverFor(cover, 260, undefined, true).src);
	const coverTint = $derived(pictureOf(cover)?.color || null);
	// The page colour: sampled from the cover once it shows (truer than the stored colour, which can miss a blue cover), else the stored one (lib/music/album-tint.ts).
	let sampled = $state<string | null>(null);
	// The minimal look keeps the cover but not its colour: the page follows the theme.
	const minimal = $derived(minimalLook());
	const tint = $derived(!minimal && (sampled || coverTint) ? tintOf(sampled || coverTint) : null);
	const tintStyle = $derived(tint ? `--al-bg:${tint.bg};--al-band:${tint.band};--al-soft:${tint.soft}` : '');
	// The whole window (side menu, top bar, page) takes the colour while the album shows. Between two albums it keeps the last colour until the next is known, and fades back to the normal look on leaving.
	$effect(() => {
		if (tint) paintWindow(tint);
		else if (minimal) unpaintWindow();
	});
	// Going on to another album keeps the colour until that album's is known (no flash between two).
	let toAlbum = false;
	beforeNavigate((nav) => (toAlbum = nav.to?.route.id === page.route.id));
	onMount(() => () => {
		if (!toAlbum) unpaintWindow();
	});
	function coverLoaded(e: Event) {
		coverIn = true;
		if (!minimal) sampled = sampleCover(e.currentTarget as HTMLImageElement);
	}
	const songs = $derived.by(() => {
		const n = list.length;
		const ms = picked?.list?.length ? list.reduce((s, t) => s + (t.length_ms ?? 0), 0) : (data?.length_ms ?? 0);
		const min = Math.round(ms / 60000);
		return [n ? (n === 1 ? t('music.song', { n }) : t('music.songs', { n })) : '', min ? (min === 1 ? t('music.minute', { n: min }) : t('music.minutes', { n: min })) : ''].filter(Boolean).join(', ');
	});
	const released = $derived.by(() => {
		const d = data?.release_date;
		if (!d || !/^\d{4}-\d{2}-\d{2}$/.test(d)) return d ?? '';
		return new Date(`${d}T12:00:00`).toLocaleDateString(appLocale(), { day: 'numeric', month: 'long', year: 'numeric' });
	});
	let showEditions = $state(false);
	// Follow the artist (in the Library): a round bell beside the main action, so both sides match.
	let following = $state(false);
	let followBusy = $state(false);
	$effect(() => {
		const id = titleId;
		following = false;
		if (id) api.get<{ following: boolean }>(followPath({ titleId: id })).then((r) => (following = r.following), () => {});
	});
	async function flipFollow() {
		if (!titleId || followBusy) return;
		followBusy = true;
		try {
			following = await setFollow(api, { titleId }, !following);
		} catch (e) {
			toasts.push(t('music.couldnTUpdateFollow'), 'fail', { body: message(e) });
		} finally {
			followBusy = false;
		}
	}
	function copy(text: string, done: string, failed: string) {
		void navigator.clipboard?.writeText(text).then(() => toasts.push(done, 'ok'), () => toasts.push(failed, 'fail', { body: t('music.selectItInstead') }));
	}
	const albumLink = $derived(typeof location === 'undefined' ? '' : location.href);
	function trackItems(tr: PageTrack): MenuItem[] {
		const items: MenuItem[] = [
			{ label: t('music.copySongName'), icon: 'copy', onselect: () => copy(`${tr.name} · ${hero?.artist.name ?? ''}`, t('music.songNameCopied'), t('music.couldnTCopySong')) },
			{ label: t('music.copyAlbumLink'), icon: 'copy', onselect: () => copy(albumLink, t('music.albumLinkCopied'), t('music.couldnTCopyAlbum')) }
		];
		if (titleId) items.push({ label: t('music.openInLibrary'), icon: 'library', onselect: () => void goto(to(data?.part_id ? `/library/${titleId}/albums/${data.part_id}` : `/library/${titleId}`)) });
		return items;
	}
	$effect(() => {
		void hero?.mbid;
		coverTry = 0;
	});
	$effect(() => {
		void cover;
		coverFailed = false;
	});
	$effect(() => {
		if (!coverFailed || coverTry >= COVER_RETRY.length) return;
		const t = setTimeout(() => (coverTry += 1), COVER_RETRY[coverTry]);
		return () => clearTimeout(t);
	});
	const titleId = $derived(data?.title_id || lookup?.title_id || 0);
	const asked = $derived(askedHere || !!lookup?.ask?.albums?.some((a) => a.mbid === mbid) || (!!lookup?.ask && !lookup.ask.albums?.length));
	const status = $derived(data ? owned(data, asked) : null);
	// The editions switcher: the picked edition's own list, else the main one.
	const picked = $derived(data?.editions.find((e) => e.mbid === edition) ?? null);
	const list = $derived<PageTrack[]>(picked?.list?.length ? picked.list : (data?.tracks ?? []));
	const discs = $derived.by(() => {
		const m = new Map<number, PageTrack[]>();
		for (const t of list) m.set(t.disc, [...(m.get(t.disc) ?? []), t]);
		return [...m.entries()];
	});
	const haveCount = $derived(list.filter((t) => t.have === 'in_library').length);
	const missingCount = $derived(list.filter((t) => t.have === 'missing').length);
	const meta = $derived.by(() => {
		if (!data) return [];
		const n = data.tracks.length;
		return [kindWord(data), albumYear(data.release_date), data.label, n ? (n === 1 ? t('music.track', { n }) : t('music.tracks', { n })) : '', length(data.length_ms, true)].filter(Boolean) as string[];
	});

	function act() {
		if (titleId) return void goto(to(data?.part_id ? `/library/${titleId}/albums/${data.part_id}` : `/library/${titleId}`));
		if (!lookup) return void toasts.push(t('music.stillReadingArtist'), 'fail', { body: t('music.tryAgainInA') });
		if (manage) addDialog?.open(lookup);
		else if (edition) void askEdition();
		else askDialog?.open(lookup, mbid);
	}

	/** A member who picked an edition asks for this album in that edition. */
	async function askEdition() {
		if (asking || !lookup) return;
		asking = true;
		try {
			const r = await api.post<{ ask: Ask; joined: boolean }>('/asks', { kind: 'artist', id: `mbid:${artist}`, albums: [mbid], editions: { [mbid]: edition } });
			onAsked(lookup, r);
		} catch (e) {
			toasts.push(t('music.couldnTSendRequest'), 'fail', { body: message(e) });
		} finally {
			asking = false;
		}
	}

	function onAsked(_a: LookupArtist, r: { ask: { name: string }; joined: boolean }) {
		askedHere = true;
		toasts.push(r.joined ? t('music.alreadyRequested', { what: hero?.name ?? '' }) : t('music.requested2', { what: hero?.name ?? '' }), 'ok', { body: r.joined ? t('music.youReOnThe') : t('music.waitingForApproval') });
		void refreshInbox();
	}

	const cta = $derived(titleId ? (data?.state && data.state !== 'not_included' ? t('music.openInLibrary') : t('music.manageInLibrary')) : asked ? t('music.asked') : manage ? t('music.addArtist') : t('music.askForThisAlbum'));

</script>

<svelte:head><title>{hero?.name ?? t('music.album')} · HolyArrs</title>{#if heroCover}<link rel="preload" as="image" href={heroCover} />{/if}</svelte:head>

{#snippet albumCard(a: PageAlbum)}
	{@const c = coverFor(appPicture(a.image), 200)}
	<PosterCard title={a.name} href={albumHref(a.mbid, data?.artist?.mbid || artist)} image={c.src} kind="artist" shape="square" music needsArt
		subtitle={[albumYear(a.release_date), kindWord(a)].filter(Boolean).join(' · ')} badges={a.explicit ? ['E'] : undefined} owned={owned(a)} />
{/snippet}

<div class="album" class:tinted={!!tint} class:minimal style={tintStyle} data-tint={tint?.bg ?? ''}>
	{#if error && !data}<LoadError hasData={false} message={error} network={errorNet} onretry={load} {busy} />{/if}
	<PageLoader loading={!hero && !error} cached={fromCache} label={t('music.loadingThisAlbum')} onretry={load}>
		{#if hero && !(error && !data)}
			<header class="hero" data-hero>
				<div class="cover" class:in={coverIn} style:background-color={coverTint}>
					{#if cover && !coverFailed}<img src={heroCover} alt={t('music.cover', { name: hero.name })} width="320" height="320" decoding="async" fetchpriority="high" onload={coverLoaded} onerror={() => { coverIn = false; coverFailed = true; }} />{:else}<ArtPlaceholder name={hero.name} shape="square" color={coverTint} />{/if}
				</div>
				<div class="words">
					<h1>{hero.name}{#if data?.explicit}<span class="e big" aria-label={t('music.explicit')}>E</span>{/if}</h1>
					<a class="by" href={artistHref(hero.artist.mbid)}>{hero.artist.name}</a>
					<p class="meta">{[kindWord(hero), albumYear(hero.release_date)].filter(Boolean).join(' · ')}</p>
					{#if hero.disambiguation}<p class="blurb">{hero.disambiguation}</p>{/if}
					<div class="actions">
						<ActionButton icon="user" label={t('music.goTo', { name: hero.artist.name })} caption={t('music.artist')} href={artistHref(hero.artist.mbid)} />
						{#if titleId || manage || mayAsk}
							<ActionButton primary icon={titleId ? 'library' : asked ? 'check' : 'plus'} label={cta} disabled={!titleId && (asked || !lookup || asking)} busy={asking} onclick={act} data-testid="album-ask" />
						{:else}
							<ActionButton primary icon="user" label={t('music.openArtist')} href={artistHref(hero.artist.mbid)} />
						{/if}
						{#if titleId}<ActionButton icon="bell" label={followLabel(following)} title={followHint('artist', following, false)} pressed={following} busy={followBusy} onclick={flipFollow} data-testid="follow" />{/if}
					</div>
					{#if status && data}<span class="pill {status.state}" data-testid="album-state">{status.label}{#if data.part_id && data.tracks.length}{t('music.ofTracks2', { have: haveCount, total: data.tracks.length })}{/if}</span>{/if}
				</div>
			</header>

			{#if !data}
				<section class="block" aria-busy="true" aria-label={t('music.loadingTheTracks')}>
					<div class="tracks">{#each Array.from({ length: 10 }, (_, i) => i) as i (i)}<SkTrackRow height={48} />{/each}</div>
				</section>
			{:else}
				<section class="block" aria-label={t('music.tracks2')}>
					{#if !data.tracks.length && data.gap}<GapNotice gap={data.gap} />
					{:else if trackListView(data.tracks.length, reasking) === 'skeleton'}<div class="tracks" aria-busy="true" aria-label={t('music.loadingTheTracks')}>{#each Array.from({ length: 10 }, (_, i) => i) as i (i)}<SkTrackRow height={48} />{/each}</div>
					{:else if trackListView(data.tracks.length, reasking) === 'gave-up'}<p class="note">{noSongsLine()}</p>{/if}
					{#if data.part_id && list.length}<p class="count" data-testid="album-have">{t('music.haveMissing', { have: haveCount, missing: missingCount })}</p>{/if}
					{#each discs as [disc, tracks] (disc)}
						{#if discs.length > 1}<h3 class="disc">{t('music.disc', { n: disc })}</h3>{/if}
						<ol class="tracks">
							{#each tracks as tr (tr.disc * 1000 + tr.number)}
								<li class:have={tr.have === 'in_library'}>
									<span class="n">{tr.number}</span>
									<span class="t-name"><span class="t-title">{tr.name}</span>{#if tr.explicit}<span class="e" aria-label={t('music.explicit')}>E</span>{/if}</span>
									{#if tr.have}
										<span class="t-have {tr.have}" title={tr.have === 'in_library' ? t('music.inYourLibrary') : tr.have === 'downloading' ? t('music.downloading') : t('music.missing')}>
											<Icon name={tr.have === 'in_library' ? 'check' : tr.have === 'downloading' ? 'download' : 'dash'} size={14} />
											<span class="sr-only">{tr.have === 'in_library' ? t('music.inYourLibrary') : tr.have === 'downloading' ? t('music.downloading') : t('music.missing')}</span>
										</span>
									{:else}<span></span>{/if}
									<span class="t-len">{length(tr.length_ms)}</span>
									<span class="t-more"><Menu label={t('music.moreFor2', { name: tr.name })} items={trackItems(tr)} width={220} /></span>
								</li>
							{/each}
						</ol>
					{/each}
				</section>

				<footer class="foot" data-testid="album-foot">
					{#if released}<p>{released}</p>{/if}
					{#if songs}<p>{songs}</p>{/if}
					{#if data.label}<p>℗ {albumYear(data.release_date) ? `${albumYear(data.release_date)} ` : ''}{data.label}</p>{/if}
					{#if data.editions.length > 1}
						<button type="button" class="ed-link" aria-expanded={showEditions} onclick={() => (showEditions = !showEditions)} data-testid="album-editions-link">{t('music.editions', { n: data.editions.length })}{edition ? ` · ${picked?.label ?? ''}` : ''}</button>
						{#if showEditions}
							<div class="chips" role="toolbar" aria-label={t('music.editions2')} data-testid="album-editions">
								<Chip label={t('music.usual')} selected={!edition} onclick={() => (edition = '')} />
								{#each data.editions as ed (ed.mbid)}<Chip label={`${ed.label} · ${ed.tracks}`} selected={edition === ed.mbid} onclick={() => (edition = ed.mbid)} />{/each}
							</div>
							{#if edition && !titleId && mayAsk && !manage}<p>{t('music.askingNowAsksFor')}</p>{/if}
						{/if}
					{/if}
				</footer>
			{/if}

			<div class="band">
				{#if !data}
					<div class="sq-rail"><SkRail item="var(--music-item)" titleWidth="200px" /></div>
				{:else if moreBy.length}
					<Shelf title={t('music.moreBy', { name: data.artist.name })} href={artistHref(data.artist.mbid)} item="var(--music-item)" items={moreBy} key={(a) => a.mbid} placeholder="square" lines={1}>
						{#snippet card(a: PageAlbum)}{@render albumCard(a)}{/snippet}
					</Shelf>
				{/if}

				{#if near?.length}
					<Shelf title={t('music.fansAlsoLike')} item="var(--music-item)" items={near} key={(a) => a.mbid} placeholder="square" lines={2}>
						{#snippet card(a: PageCredit)}
							{@const c = coverFor(appPicture(a.image), 200)}
							<PosterCard title={a.name} href={albumHref(a.mbid, a.artist_mbid)} image={c.src} kind="artist" shape="square" music needsArt subtitle={a.artist_name} badges={a.explicit ? ['E'] : undefined} />
						{/snippet}
					</Shelf>
				{/if}

				{#if similar === null}
					<SkRail kind="person" item="168px" titleWidth="200px" />
				{:else if similar.length}
					<section class="block">
						<h2>{t('music.ifYouLike', { name: hero.artist.name })}</h2>
						<ul class="circles" use:dragScroll>{#each similar as p (p.mbid)}<li><PersonCircle name={p.name} image={appPicture(p.image)} href={artistHref(p.mbid)} size={152} needsArt /></li>{/each}</ul>
					</section>
				{/if}
			</div>
		{/if}
	</PageLoader>
</div>

{#if manage}
	<AddArtistDialog bind:this={addDialog} {folders} {rules} {defaults} onadded={(_a, added) => { toasts.push(t('music.added', { what: added.name }), 'ok', { body: t('music.fillingInAlbumsAnd') }); void load(); }} />
{:else if mayAsk}
	<AskArtistDialog bind:this={askDialog} onasked={onAsked} />
{/if}

<style>
	/* The page sits on its cover's colour, edge to edge, in both app themes. */
	.album {
		--al-bg: #1d1d22; --al-band: #19191d; --al-soft: #d2d2d3;
		--music-item: max(168px, calc((100vw - 96px) / 7.2));
		--text: #fff; --text-muted: var(--al-soft); --surface-1: rgb(255 255 255 / 8%); --surface-2: rgb(255 255 255 / 12%); --accent: #fff; --border: rgb(255 255 255 / 14%);
		display: flex; flex-direction: column; gap: 28px; min-width: 0;
		margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) calc(var(--s-12) * -1); padding: 40px var(--gutter) 0;
		background: var(--al-bg); color: #fff; color-scheme: dark;
		transition: background-color var(--dur-slow) var(--ease-out);
	}
	.hero { display: grid; grid-template-columns: 270px minmax(0, 1fr); gap: 36px; align-items: end; margin: 0; max-width: 1180px; }
	.cover { position: relative; width: 270px; aspect-ratio: 1; display: grid; place-items: center; border-radius: 10px; overflow: hidden; background: rgb(255 255 255 / 8%); box-shadow: 0 18px 44px rgb(0 0 0 / .38), 0 3px 10px rgb(0 0 0 / .22); }
	.cover img { width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); }
	.cover.in img { opacity: 1; }
	.words { min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 6px; padding-bottom: 4px; }
	h1 { margin: 0; font: 700 34px/1.12 var(--font-display); letter-spacing: -.015em; text-wrap: balance; overflow-wrap: anywhere; }
	.by { color: var(--al-soft); font: 400 24px/1.2 var(--font-display); letter-spacing: -.01em; }
	.by:hover { color: #fff; text-decoration: underline; }
	.meta { margin: 2px 0 0; color: var(--al-soft); font: 600 13px/1.4 var(--font-ui); text-transform: uppercase; letter-spacing: .04em; }
	.blurb { margin: 6px 0 0; max-width: 62ch; color: var(--al-soft); font: var(--t-subhead); }
	.actions { display: flex; align-items: center; gap: 12px; margin-top: 16px; }
	.pill { margin-top: 6px; padding: 6px 12px; border-radius: 999px; background: rgb(255 255 255 / 10%); color: #fff; font: var(--t-footnote); font-weight: 600; font-variant-numeric: tabular-nums; }
	.e { display: inline-grid; place-items: center; flex: none; width: 15px; height: 15px; margin-left: 8px; border-radius: 3px; background: var(--al-soft); color: var(--al-bg); font: 700 10px/1 var(--font-ui); vertical-align: 2px; }
	.e.big { width: 24px; height: 24px; font-size: 14px; border-radius: 5px; vertical-align: 5px; margin-left: 10px; }
	.block { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
	h2 { margin: 0; font: var(--t-title2); }
	.disc { margin: 12px 0 0; font: var(--t-headline); color: var(--al-soft); }
	.count, .note { margin: 0; color: var(--al-soft); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.tracks { margin: 0; padding: 0; list-style: none; max-width: 1180px; }
	.tracks li { position: relative; display: grid; grid-template-columns: 28px minmax(0, 1fr) 20px 52px 36px; align-items: center; gap: 12px; min-height: 48px; padding: 0 4px 0 12px; border-radius: 8px; transition: background-color var(--dur-fast) var(--ease-out); }
	/* A thin line between rows, from the title column on, hidden around a highlighted row. */
	.tracks li + li::before { content: ''; position: absolute; top: 0; left: 52px; right: 12px; height: 1px; background: rgb(255 255 255 / 14%); }
	.tracks li:hover, .tracks li:focus-within { background: rgb(255 255 255 / 9%); }
	.tracks li:hover::before, .tracks li:hover + li::before, .tracks li:focus-within::before, .tracks li:focus-within + li::before { opacity: 0; }
	.n { color: var(--al-soft); font: var(--t-subhead); font-variant-numeric: tabular-nums; text-align: right; }
	.t-name { display: flex; align-items: center; min-width: 0; font: var(--t-callout); }
	.t-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.t-have { display: grid; place-items: center; color: var(--al-soft); }
	.t-have.in_library { color: #fff; }
	.t-len { color: var(--al-soft); font: var(--t-footnote); font-variant-numeric: tabular-nums; text-align: right; }
	.t-more { display: grid; place-items: center; color: var(--al-soft); }
	.foot { display: flex; flex-direction: column; gap: 2px; max-width: 1180px; padding: 4px 12px 8px; color: var(--al-soft); font: var(--t-footnote); }
	.foot p { margin: 0; }
	.ed-link { align-self: flex-start; margin-top: 8px; padding: 0; border: 0; background: none; color: #fff; font: 600 13px/1.4 var(--font-ui); text-decoration: underline; text-underline-offset: 3px; cursor: pointer; }
	.chips { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 8px; }
	.band { display: flex; flex-direction: column; gap: 28px; margin: 4px calc(var(--gutter) * -1) 0; padding: 28px var(--gutter) 56px; background: var(--al-band); border-top: 1px solid rgb(255 255 255 / 8%); transition: background-color var(--dur-slow) var(--ease-out); }
	.circles { display: flex; gap: 18px; overflow-x: auto; margin: 0; padding: 4px 2px 8px; list-style: none; scrollbar-width: none; }
	.sq-rail { --poster-ratio: 1; }
	/* Minimal: the theme's own colours instead of the dark cover palette, and a smaller cover. */
	.album.minimal { --al-bg: var(--bg); --al-band: var(--surface-2); --al-soft: var(--text-muted); --text: inherit; --text-muted: inherit; --surface-1: inherit; --surface-2: inherit; --accent: inherit; --border: inherit; color: var(--text); color-scheme: inherit; }
	.album.minimal .hero { grid-template-columns: 200px minmax(0, 1fr); gap: 28px; }
	.album.minimal .cover { width: 200px; background: var(--surface-2); box-shadow: 0 4px 14px rgb(0 0 0 / .16); }
	.album.minimal .by:hover, .album.minimal .t-have.in_library, .album.minimal .ed-link { color: var(--text); }
	.album.minimal .pill { background: var(--surface-2); color: var(--text); }
	.album.minimal .tracks li + li::before { background: var(--line); }
	.album.minimal .tracks li:hover, .album.minimal .tracks li:focus-within { background: var(--surface-2); }
	.album.minimal .band { border-top-color: var(--line); }
	:global(html:not([data-theme='dark'])) .album.minimal .actions :global(.ui-act) { background: color-mix(in srgb, var(--text) 7%, transparent); border-color: var(--line); color: var(--text); backdrop-filter: none; -webkit-backdrop-filter: none; }
	:global(html:not([data-theme='dark'])) .album.minimal .actions :global(.ui-act.primary), :global(html:not([data-theme='dark'])) .album.minimal .actions :global(.ui-act[aria-pressed='true']) { background: var(--text); border-color: var(--text); color: var(--bg); box-shadow: none; }
	:global(html:not([data-theme='dark'])) .album.minimal .actions :global(.ui-act:focus-visible) { box-shadow: 0 0 0 2px var(--bg), 0 0 0 4px var(--text); }
	@media (max-width: 767px) {
		.album.minimal .hero { grid-template-columns: minmax(0, 1fr); gap: 20px; }
		.album.minimal .cover { width: min(200px, 60vw); }
	}
	@media (max-width: 767px) {
		.album { --music-item: calc((100cqi - var(--shelf-gap, 12px)) / 2.3); gap: 24px; padding-top: 24px; }
		.hero { grid-template-columns: minmax(0, 1fr); justify-items: center; text-align: center; gap: 20px; }
		.cover { width: min(270px, 72vw); }
		.words { align-items: center; width: 100%; }
		h1 { font-size: 26px; }
		.by { font-size: 20px; }
		.actions { justify-content: center; width: 100%; }
		.tracks li { grid-template-columns: 24px minmax(0, 1fr) 16px 44px 32px; gap: 10px; padding: 0 0 0 8px; }
		.tracks li + li::before { left: 42px; right: 8px; }
		.foot { padding-inline: 8px; }
	}
</style>

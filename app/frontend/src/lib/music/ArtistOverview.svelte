<script module lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	/** The Library's own parts when this artist is in the Library (/library/{id}). */
	export type ArtistLibrary = {
		titleId: number;
		staff: boolean;
		progress: string;
		status: string;
		note: string;
		nextRelease?: string;
		/** Changes when the page hears the artist changed; the Library tab reads again. */
		stamp?: number;
		onAsk?: () => void;
	};
</script>

<script lang="ts">
	import { dragScroll } from '#lib/ui/drag-scroll.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import PersonCircle from '#lib/ui/PersonCircle.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import { SkRail, SkRowTitle, SkTrackRow } from '#lib/ui/sk/index.ts';
	import { paintWindow, sampleCover, tintOf, unpaintWindow } from './album-tint.ts';
	import { gapWaits, keepShown, reaskWhileIncomplete } from './reask.ts';
	import GapNotice from './GapNotice.svelte';
	import { albumYear } from './labels.ts';
	import { releaseWords } from './music-trending.ts';
	import {  musicLoadError, albumHref, appPicture, artistHref,   coverFor, kindWord,  owned, listening,   shortCount, type Catalog, askAgainOnce, coreIfSlow, withCover, shelves as shelvesOf, type ArtistCoreData, type ArtistMoreData, type ArtistPageData, type PageAlbum, type PageArtist, type PageCredit, type PageSong, type Shelf as ShelfKind } from './music-pages.ts';
	import type { ArtistSheets, LookupArtist, MusicSettings } from './types.ts';
	import ArtistStats from './ArtistStats.svelte';
	import ArtistTopSongs from './ArtistTopSongs.svelte';
	import ArtistAbout from './ArtistAbout.svelte';
	import ArtistHeroBanner from './ArtistHeroBanner.svelte';
	import ArtistVideos from './ArtistVideos.svelte';
	import ArtistLibraryTab from './ArtistLibrary.svelte';
	import { onMount, tick } from 'svelte';
	import { beforeNavigate, goto } from '$app/navigation';
	import { page } from '$app/state';
	import type { Snippet } from 'svelte';
	import { api, message } from '#lib/core/api.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { pictureFor, pictureOf } from '#lib/core/pictures.ts';
	import { refreshInbox } from '#lib/core/inbox.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import { minimalLook } from '#lib/core/look.svelte.ts';
	import type { Ask, LibraryFolder, PickRulesView } from '#lib/core/types.ts';

	// The one artist page: any MusicBrainz artist, in the Library or not. The Library route passes `library` (its menu, progress and ask) and `below` (albums to keep, how HolyArrs keeps it, history, requests).
	const ART_RETRY = [1500, 3000, 5000, 10000];
	let { mbid: rawId, library: lib, below, sheets }: { mbid: string; library?: ArtistLibrary; below?: Snippet; sheets: ArtistSheets } = $props();
	const mbid = $derived((rawId ?? '').toLowerCase());
	// Hero first: the core answer (name, picture, bio) draws the hero; the full answer (albums, songs, links) and the similar artists fill the shelves.
	let core = $state<ArtistCoreData | null>(null);
	let data = $state<ArtistPageData | null>(null);
	let similar = $state<PageArtist[] | null>(null);
	let more = $state<ArtistMoreData | null>(null);
	const related = $derived(withCover(data?.related));
	let confirmAll = $state(false);
	let askingAll = $state(false);
	let aboutOpen = $state(false);
	let error = $state('');
	let errorNet = $state(false);
	let busy = $state(false);
	// The answer was already kept by the app (fetched ahead or seen within a minute), so no loader hold.
	let fromCache = $state(false);
	let leave: AbortController | null = null;
	let lookup = $state<LookupArtist | null>(null);
	let art = $state<{ logo?: string; backgrounds?: string[]; thumb?: string }>({});
	let following = $state<boolean | null>(null);
	// Covers that failed to load show the shared placeholder.
	let badArt = $state(new Set<string>());
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let defaults = $state<MusicSettings | null>(null);
	let askDialog: ReturnType<ArtistSheets['ask']> | undefined = $state();
	let addDialog: ReturnType<ArtistSheets['add']> | undefined = $state();
	let asked = $state(new Set<string>());
	let follows = $state(new Map<string, boolean>());

	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask'));
	const hero = $derived<ArtistCoreData | null>(data ?? core);
	const titleId = $derived(lib?.titleId || hero?.title_id || lookup?.title_id || 0);
	// Overview or Library, for an artist in the Library (?tab=library opens it there).
	let artistTab = $state<'overview' | 'library'>(typeof location !== 'undefined' && new URLSearchParams(location.search).get('tab') === 'library' ? 'library' : 'overview');
	/** The hero's In Library button: the Library tab, scrolled to. */
	async function toLibraryTab() {
		artistTab = 'library';
		await tick();
		document.querySelector('[data-testid="artist-tabs"]')?.scrollIntoView({ behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth', block: 'start' });
	}

	function load() {
		const want = mbid;
		busy = true;
		error = '';
		leave?.abort();
		const stop = (leave = new AbortController());
		const full = `/music/artists/${want}?similar=0`;
		fromCache = api.has(`/music/artists/${want}?part=core`) || api.has(full);
		let followAsked = false;
		const askFollow = (id?: number) => {
			if (!id || followAsked) return;
			followAsked = true;
			void api.get<{ following: boolean }>(`/titles/${id}/follow`).then((r) => want === mbid && (following = r.following)).catch(() => {});
		};
		// All at once: the hero, the shelves, the similar artists, the artwork and the ask state.
		const fullRead = askAgainOnce(() => api.get<ArtistPageData>(full));
		// The full answer holds the core, so the quick core part is asked only when the full one is slow.
		const coreAsk = coreIfSlow(fullRead, () => api.get<ArtistCoreData>(`/music/artists/${want}?part=core`)).then((c) => {
			if (!c || want !== mbid) return;
			if (!data) core = c;
			askFollow(c.title_id);
		});
		const fullAsk = fullRead.then((d) => {
			if (want !== mbid) return;
			// An incomplete answer (slow source) is shown, then asked again quietly and merged in, keeping what is on screen.
			void reaskWhileIncomplete(d, () => api.get<ArtistPageData>(full), (v) => {
				if (want !== mbid || stop.signal.aborted) return;
				data = keepShown(data, v, ['albums', 'top', 'related']);
				follows = new Map(data.albums.filter((a) => a.upcoming).map((a) => [a.mbid, !!a.following]));
				askFollow(data.title_id);
			}, stop.signal, gapWaits(d));
		});
		// The server may still be getting the similar artists: the row keeps its placeholder and is asked again, then hides if none came.
		const simPath = `/music/artists/${want}?part=similar`;
		void api.get<{ similar: PageArtist[]; incomplete?: boolean }>(simPath).then((r) => {
			if (want !== mbid) return;
			void reaskWhileIncomplete(r, () => api.get<{ similar: PageArtist[]; incomplete?: boolean }>(simPath), (v, pending) => {
				if (want !== mbid || stop.signal.aborted) return;
				similar = pending && !v.similar.length ? null : withCover(v.similar);
			}, stop.signal);
		}).catch(() => want === mbid && (similar = []));
		void api.get<ArtistMoreData>(`/music/artists/${want}?part=more`).then((r) => want === mbid && (more = { ...r, appears_on: withCover(r.appears_on) })).catch(() => want === mbid && (more = { essential: [], appears_on: [] }));
		// A cold artist's art may still be on its way, since the server fetches it right then. An answer with no wide photo is asked again over about 20 s, and the photo fades in when it lands.
		const askArt = (left: number[]) => {
			void api.get<typeof art>(`/extras/artist/${want}`).then((x) => {
				if (want !== mbid) return;
				// The server's own picture routes sit under the app's base path, at their stored version.
				const local = (u?: string) => (u && u.startsWith('/') ? (appPicture(u) ?? undefined) : u);
				x = { ...x, logo: local(x.logo), thumb: local(x.thumb), backgrounds: x.backgrounds?.map((u) => local(u) as string) };
				if ((x.backgrounds?.length ?? 0) > 0 || x.logo || !art.logo) art = x;
				if (!(x.backgrounds?.length ?? 0) && left.length) setTimeout(() => want === mbid && askArt(left.slice(1)), left[0]);
			}).catch(() => want === mbid && left.length && setTimeout(() => want === mbid && askArt(left.slice(1)), left[0]));
		};
		askArt(ART_RETRY);
		void api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(`mbid:${want}`)}`).then((r) => {
			if (want !== mbid) return;
			lookup = r.artists[0] ?? null;
			asked = new Set((lookup?.ask?.albums ?? []).map((a) => a.mbid));
		}).catch(() => {});
		void Promise.allSettled([coreAsk, fullAsk]).then(([c, f]) => {
			if (want !== mbid) return;
			busy = false;
			// The shelves failing shows the error; the core failing alone is quiet, because the full answer has the hero too.
			const bad = f.status === 'rejected' ? f.reason : c.status === 'rejected' && !data ? c.reason : null;
			if (bad) ({ text: error, network: errorNet } = musicLoadError(bad, 'artist'));
		});
	}

	$effect(() => {
		void mbid;
		core = null;
		data = null;
		similar = null;
		more = null;
		confirmAll = false;
		aboutOpen = false;
		lookup = null;
		art = {};
		following = null;
		void load();
		return () => leave?.abort();
	});

	// Phones get their own sizes.
	let phone = $state(false);
	// How far the banner has scrolled away (0 to 1), and whether the name has left the screen.
	let gone = $state(0);
	let slimOn = $state(false);
	let nameEl: HTMLElement | undefined = $state();
	function measure() {
		const box = heroBox;
		if (box) {
			const r = box.getBoundingClientRect();
			gone = Math.min(1, Math.max(0, -r.top / Math.max(1, r.height)));
		}
		const bar = document.querySelector('.ui-topbar');
		const barBottom = bar && !document.documentElement.classList.contains('bar-hidden') ? Math.max(0, bar.getBoundingClientRect().bottom) : 0;
		slimOn = !!nameEl && nameEl.getBoundingClientRect().bottom < barBottom + 4;
	}
	onMount(() => {
		if (session.can('manage_library')) {
			void api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
			void api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
			void api.get<MusicSettings>('/settings/music').then((r) => (defaults = r)).catch(() => {});
		}
		const pq = matchMedia('(max-width: 767px)');
		const sizes = () => (phone = pq.matches);
		sizes();
		pq.addEventListener('change', sizes);
		let frame = 0;
		const onScroll = () => {
			if (frame) return;
			frame = requestAnimationFrame(() => {
				frame = 0;
				measure();
			});
		};
		addEventListener('scroll', onScroll, { passive: true });
		addEventListener('resize', onScroll, { passive: true });
		return () => {
			pq.removeEventListener('change', sizes);
			removeEventListener('scroll', onScroll);
			removeEventListener('resize', onScroll);
			cancelAnimationFrame(frame);
		};
	});
	// A new artist: measure again.
	$effect(() => {
		void nameEl;
		void heroBox;
		measure();
	});

	// The artist's photo. One the server has not stored yet is asked for again while the page is open (the server fetches it right away) and swaps in once stored, with no reload.
	let photoTry = $state(0);
	const PHOTO_RETRY = [20000, 70000, 150000];
	const picture = $derived.by(() => {
		const p = appPicture(hero?.image);
		return p && photoTry ? `${p}${p.includes('?') ? '&' : '?'}fresh=${photoTry}` : p;
	});
	// The banner never stays empty: each wide photo in turn, then a soft crop of the artist's photo.
	let heroBox: HTMLElement | undefined = $state();
	const heroList = $derived([...(art.backgrounds ?? []), ...(picture ? [picture] : [])]);
	// A picture that fails (none by the artist's id) shows initials, never a stranger.
	let photoFailed = $state(false);
	$effect(() => {
		void picture;
		photoFailed = false;
	});
	$effect(() => {
		void mbid;
		photoTry = 0;
	});
	$effect(() => {
		if (!photoFailed || photoTry >= PHOTO_RETRY.length) return;
		const t = setTimeout(() => (photoTry += 1), PHOTO_RETRY[photoTry]);
		return () => clearTimeout(t);
	});
	// With no round photo, crop the wide hero photo into the circle (the face sits upper middle); initials only with no image at all.
	const wideFace = $derived((art.backgrounds ?? [])[0] ?? null);
	let wideFailed = $state(false);
	$effect(() => {
		void wideFace;
		wideFailed = false;
	});
	// The banner's main colour tints the slim bar.
	const tint = $derived(pictureFor('wide', mbid)?.color || pictureFor('artist', mbid)?.color || pictureOf(hero?.image)?.color || '');
	// The whole window takes the artist's colour, as an album does: the stored banner sampled once it shows (truer than the stored colour), else the stored one. The banner fades into it at the bottom.
	let sampledTint = $state<string | null>(null);
	// The colour worked out for an artist is kept per browser, so a return visit opens in it at once.
	const tintKey = (id: string) => `holyarrs.artist-tint.${id}`;
	$effect(() => {
		const id = mbid;
		let kept: string | null = null;
		try { kept = localStorage.getItem(tintKey(id)); } catch {}
		sampledTint = kept;
	});
	// The minimal look has no banner and no colour from the art: the page keeps the theme.
	const minimal = $derived(minimalLook());
	const pageTint = $derived(!minimal && (sampledTint || tint) ? tintOf(sampledTint || tint) : null);
	$effect(() => {
		if (pageTint) paintWindow(pageTint);
		else if (minimal) unpaintWindow();
	});
	let toTinted = false;
	beforeNavigate((nav) => (toTinted = nav.to?.route.id === page.route.id || !!nav.to?.route.id?.startsWith('/music/album')));
	onMount(() => () => {
		if (!toTinted) unpaintWindow();
	});
	// The colour is read from the artist's round photo (one picture per artist, so every visitor gets the same colour); the banner only when there is no photo.
	$effect(() => {
		const src = picture, id = mbid;
		if (!src || sampledTint || minimal) return;
		const img = new Image();
		img.onload = () => {
			if (id !== mbid || sampledTint) return;
			const c = sampleCover(img);
			if (c) { sampledTint = c; try { localStorage.setItem(tintKey(id), c); } catch {} }
		};
		img.src = src;
	});
	function heroLoaded(img: HTMLImageElement) {
		if (sampledTint || picture) return;
		sampledTint = sampleCover(img);
		if (sampledTint) try { localStorage.setItem(tintKey(mbid), sampledTint); } catch {}
	}

	// The full answer's albums, else the core answer's stored list (drawn with the hero).
	const released = $derived((data?.albums ?? core?.albums ?? []).filter((a) => !a.upcoming));
	// The figures of the stats band, the About card and its sheet.
	const listen = $derived(listening(data?.popularity));
	const catalog = $derived.by((): Catalog | null => {
		if (!data) return null;
		const plain = (t: string) => released.filter((a) => a.type?.toLowerCase() === t && !a.secondary_types?.length).length;
		const years = released.map((a) => a.release_date?.slice(0, 4)).filter((y): y is string => !!y).sort();
		const since = years[0] ?? '';
		const last = years[years.length - 1] ?? '';
		return { albums: plain('album'), eps: plain('ep'), singles: plain('single'), since, span: since && last !== since ? `${since}–${last}` : since };
	});
	const listenWords = $derived(listen ? `${shortCount(listen.n)} ${listen.unit}` : '');
	const genres = $derived((data?.genres ?? []).slice(0, 4).join(', '));
	// "0 of 113 tracks · 48 missing" said once, even when progress and status repeat a part.
	const libLine = $derived(lib?.staff ? [...new Set([lib.progress, lib.status].filter(Boolean).flatMap((x) => String(x).split(' · ')))].join(' · ') : '');
	const haveShare = $derived(data?.total ? Math.round((100 * (data.have ?? 0)) / data.total) : 0);
	// The discography: one row per kind of release.
	const rowWords = (): Record<ShelfKind, string> => ({ albums: t('music.albums'), singles: t('music.singlesEps'), compilations: t('music.compilations'), live: t('music.live'), other: t('music.soundtracksRemixesAndMore') });
	const rows = $derived(shelvesOf(released).map((s) => ({ ...s, title: rowWords()[s.shelf] ?? s.title })));
	const essential = $derived.by(() => {
		if (!data || !more) return [];
		const by = new Map(data.albums.map((a) => [a.mbid, a]));
		return withCover(more.essential.map((id) => by.get(id)).filter((a): a is PageAlbum => !!a));
	});
	const missing = $derived(data?.missing);
	const allAsked = $derived(!!lookup?.ask && !lookup.ask.albums?.length);
	const soon = $derived(data?.albums.filter((a) => a.upcoming) ?? []);
	// Top songs: ten, in two columns of five.
	const songs = $derived((data?.top ?? []).slice(0, 10));
	// Similar artists: the 200 px artist circle, smaller on phones so more than one shows.
	const circle = $derived(phone ? 136 : 200);

	/** The person's ask, or what they may do: ask (members) or add (managers). */
	function askAlbum(a?: PageAlbum) {
		if (!lookup) return void toasts.push(t('music.stillReadingArtist'), 'fail', { body: t('music.tryAgainInA') });
		if (titleId) {
			if (a?.part_id) return void goto(to(`/library/${titleId}/albums/${a.part_id}`));
			return void goto(to(`/library/${titleId}`));
		}
		if (manage) addDialog?.open(lookup);
		else askDialog?.open(lookup, a?.mbid);
	}

	function onAsked(_a: LookupArtist, r: { ask: Ask; joined: boolean }) {
		asked = new Set([...asked, ...(r.ask.albums ?? []).map((x) => x.mbid)]);
		lookup = lookup ? { ...lookup, ask: { id: r.ask.id, state: r.ask.state, state_label: r.ask.state_label, mine: true, people: 1, albums: r.ask.albums } } : lookup;
		toasts.push(r.joined ? t('music.alreadyRequested', { what: r.ask.name }) : t('music.requested2', { what: r.ask.name }), 'ok', { body: r.joined ? t('music.youReOnThe') : t('music.waitingForApproval') });
		void refreshInbox();
	}

	async function askAllMissing() {
		if (!missing?.albums.length || askingAll) return;
		askingAll = true;
		try {
			const r = await api.post<{ ask: Ask; joined: boolean }>('/asks', { kind: 'artist', id: `mbid:${mbid}`, albums: missing.albums });
			if (lookup) onAsked(lookup, r);
			else asked = new Set([...asked, ...missing.albums]);
			confirmAll = false;
		} catch (e) {
			toasts.push(t('music.couldnTSendRequest'), 'fail', { body: message(e) });
		} finally {
			askingAll = false;
		}
	}

	async function toggleFollow() {
		if (!titleId) return;
		const on = !following;
		following = on;
		try {
			await (on ? api.put(`/titles/${titleId}/follow`, {}) : api.del(`/titles/${titleId}/follow`));
		} catch (e) {
			following = !on;
			toasts.push(t('music.couldnTUpdateFollow'), 'fail', { body: message(e) });
		}
	}

	async function followAlbum(a: PageAlbum) {
		const on = !follows.get(a.mbid);
		follows = new Map([...follows, [a.mbid, on]]);
		try {
			if (on) await api.put(`/metadata/album/${a.mbid}/follow`, { name: a.name, artist: hero?.name ?? '', release_date: a.release_date ?? '' });
			else await api.del(`/metadata/album/${a.mbid}/follow`);
			// The browser keeps the page answer a minute (max-age=60), so fetch it afresh so a reload or return visit shows the bell as it is now.
			await fetch(to(`/api/v1/music/artists/${mbid}?similar=0`), { cache: 'reload', headers: { Accept: 'application/json' }, credentials: 'same-origin' }).catch(() => {});
		} catch (e) {
			follows = new Map([...follows, [a.mbid, !on]]);
			toasts.push(t('music.couldnTUpdateFollow'), 'fail', { body: message(e) });
		}
	}

	const albumAsked = (a: PageAlbum) => asked.has(a.mbid) || (!!lookup?.ask && !lookup.ask.albums?.length);
	const cta = $derived(titleId ? t('music.openInLibrary') : lookup?.ask ? t('music.asked') : manage ? t('music.addArtist') : t('music.askForDiscography'));

	async function copyLink(href: string) {
		try {
			await navigator.clipboard.writeText(new URL(href, location.href).href);
			toasts.push(t('music.linkCopied'), 'ok');
		} catch {
			toasts.push(t('music.couldnTCopyLink'), 'fail', { body: t('music.selectItInstead') });
		}
	}
	/** Menu of a top song: open its album, ask for it (add, for managers), copy its link. */
	function songItems(s: PageSong): MenuItem[] {
		if (!s.album_mbid) return [];
		const href = albumHref(s.album_mbid, mbid);
		const al = data?.albums.find((x) => x.mbid === s.album_mbid);
		const items: MenuItem[] = [{ label: t('music.openAlbum'), icon: 'music', href }];
		if (titleId) items.push({ label: t('music.openInLibrary'), icon: 'library', onselect: () => askAlbum(al) });
		else if (al && manage) items.push({ label: hero?.name ? t('music.add', { name: hero.name }) : t('music.addArtist'), icon: 'plus', onselect: () => askAlbum(al) });
		else if (al && mayAsk) items.push(albumAsked(al) ? { label: t('music.asked'), icon: 'check', disabled: true } : { label: t('music.askForTheAlbum'), icon: 'plus', onselect: () => askAlbum(al) });
		items.push({ label: t('music.copyLink'), icon: 'copy', onselect: () => void copyLink(href) });
		return items;
	}
	const yearOf = (s: PageSong) => {
		const a = s.album_mbid ? data?.albums.find((x) => x.mbid === s.album_mbid) : undefined;
		return a ? albumYear(a.release_date) : '';
	};
</script>

<svelte:head><title>{hero?.name ?? t('music.artist')} · HolyArrs</title></svelte:head>

{#snippet albumCard(a: PageAlbum, eager = false, row = false)}
	{@const c = coverFor(appPicture(a.image), 200)}
	<PosterCard title={a.name} href={albumHref(a.mbid, mbid)} image={c.src} {eager} kind="artist" shape="square" music needsArt={row} actionOnHover
		subtitle={[albumYear(a.release_date), a.shelf === 'albums' ? '' : kindWord(a)].filter(Boolean).join(' · ') || undefined}
		badges={a.explicit ? ['E'] : undefined} owned={owned(a, albumAsked(a))}>
		{#snippet action()}
			{#if !titleId && !albumAsked(a) && (manage || mayAsk)}
				<IconButton icon="plus" label={manage ? t('music.add', { name: hero?.name ?? '' }) : t('music.askFor', { name: a.name })} variant="glass" size="lg" aria-haspopup="dialog" onclick={() => askAlbum(a)} />
			{/if}
		{/snippet}
	</PosterCard>
{/snippet}

{#snippet creditCard(a: PageCredit)}
	{@const c = coverFor(appPicture(a.image), 200)}
	<PosterCard title={a.name} href={albumHref(a.mbid, a.artist_mbid || mbid)} image={c.src} kind="artist" shape="square" music needsArt
		subtitle={a.artist_name || undefined} badges={a.explicit ? ['E'] : undefined} />
{/snippet}

{#snippet mainAct()}
	<!-- The main button sits where a player puts play: In Library, Ask or Add. -->
	{#if lib?.staff}
		<ActionButton icon="library" label={t('music.inLibraryOpenThe')} caption={t('music.inLibrary2')} primary onclick={toLibraryTab} data-testid="hero-library" />
	{:else if lib?.onAsk}
		<ActionButton icon="plus" label={t('music.askForAlbums')} primary onclick={lib.onAsk} data-testid="ask-albums" />
	{:else if lib}
		<!-- In the Library, nothing to ask: no main button. -->
	{:else if titleId}
		<!-- Everyone goes to the one artist page in the Library, like a title page. -->
		<ActionButton icon="library" label={t('music.openInLibrary')} primary href={to(`/library/${titleId}`)} />

	{:else if manage || mayAsk}
		<ActionButton icon={lookup?.ask ? 'check' : 'plus'} label={cta} primary disabled={!!lookup?.ask || !lookup} onclick={() => askAlbum()} data-testid="artist-ask" />
	{/if}
{/snippet}

{#snippet aboutAct()}
	<ActionButton icon="info" label={hero?.name ? t('music.about2', { name: hero.name }) : t('music.aboutThisArtist')} caption={t('music.about')} aria-haspopup="dialog" onclick={() => (aboutOpen = true)} data-testid="artist-about-open" />
{/snippet}

{#snippet followAct()}
	{#if titleId && following !== null}
		<ActionButton icon={following ? 'bellOn' : 'bell'} label={following ? t('music.following2') : t('music.follow')} pressed={!!following} onclick={toggleFollow} />
	{/if}
{/snippet}

{#snippet acts()}
	<div class="acts">
		{@render mainAct()}{@render aboutAct()}
		{@render followAct()}
	</div>
{/snippet}

{#snippet photo(size: number)}
	{#if picture && !photoFailed}<img class="photo" src={picture} alt="" width={size} height={size} fetchpriority="high" onerror={() => (photoFailed = true)} />{:else if wideFace && !wideFailed}<img class="photo crop" src={wideFace} alt="" width={size} height={size} onerror={() => (wideFailed = true)} />{:else}<div class="photo none" aria-hidden="true" data-testid="artist-photo-placeholder"><ArtPlaceholder name={hero?.name ?? ''} shape="circle" /></div>{/if}
{/snippet}

{#snippet compactLine()}
	<div class="cline">
		<span class="c-photo">{@render photo(136)}</span>
		<div class="c-words">
			<h1 bind:this={nameEl}>{hero?.name}</h1>
			{#if listenWords}<p class="c-line">{listenWords}</p>{/if}
		</div>
		{@render acts()}
	</div>
{/snippet}

{#snippet slimBar()}
	<!-- The slim bar: the main button and the name, tinted from the banner, once the name has scrolled away. -->
	<div class="slimwrap">
		<div class="slim" class:on={slimOn} style:--tint={(!minimal && tint) || undefined} data-testid="artist-slim-bar" aria-hidden={!slimOn} inert={!slimOn}>
			{@render mainAct()}
			<span class="slim-name">{hero?.name}</span>
		</div>
	</div>
{/snippet}

{#snippet libBits()}
	{#if lib?.note || libLine || lib?.nextRelease}
		<div class="libbits">
			{#if lib?.note}<p class="quiet">{lib.note}</p>{/if}
			{#if libLine}
				<div class="libline" data-testid="artist-progress">
					<span class="libword">{t('music.yourLibrary')}</span>
					<span class="bar" aria-hidden="true"><i style="width: {haveShare}%"></i></span>
					<span>{libLine}</span>
				</div>
			{/if}
			{#if lib?.nextRelease}<p class="quiet" data-testid="artist-next">{t('music.nextAlbum', { date: lib.nextRelease })}</p>{/if}
		</div>
	{/if}
{/snippet}

{#snippet kindRows()}
	{#if !rows.length && data?.gap}
		<GapNotice gap={data.gap} />
	{:else if rows.length}
		<div class="rows" data-testid="discography">
			{#each rows as r (r.shelf)}
				<Shelf title={r.title} item="var(--music-item)" items={r.albums} key={(a) => a.mbid} placeholder="square" lines={2}>
					{#snippet card(a: PageAlbum, i: number)}{@render albumCard(a, i < 6)}{/snippet}
				</Shelf>
			{/each}
		</div>
	{/if}
{/snippet}

{#snippet storeDisco()}
	{#if !released.length && data?.store_albums?.length}
		<!-- MusicBrainz lists nothing for this artist, so show what a store knows to keep the page from being empty. No ask control, because a request needs a MusicBrainz id. -->
		<section class="block" data-testid="store-discography">
			<h2>{t('music.discography')}</h2>
			<ul class="grid">
				{#each data.store_albums as a, i (a.title + i)}
					<li>
						<PosterCard title={a.title} image={a.image ? coverFor(appPicture(a.image), 200).src : null} eager={i < 6} kind="artist" shape="square" music
							subtitle={[albumYear(a.release_date), a.type === 'album' ? '' : ({ single: t('music.single'), ep: t('music.ep'), compilation: t('music.compilation') } as Record<string, string>)[a.type] ?? '', a.track_count ? (a.track_count === 1 ? t('music.track', { n: a.track_count }) : t('music.tracks', { n: a.track_count })) : ''].filter(Boolean).join(' · ') || undefined}
							badges={a.explicit ? ['E'] : undefined} />
					</li>
				{/each}
			</ul>
		</section>
	{/if}
{/snippet}

{#snippet essentialRow()}
	{#if essential.length}
		<section class="block">
			<h2>{t('music.essentialAlbums')}</h2>
			<ul class="grid">{#each essential as a (a.mbid)}<li>{@render albumCard(a, false, true)}</li>{/each}</ul>
		</section>
	{/if}
{/snippet}

{#snippet soonRow()}
	{#if soon.length}
		<section class="block">
			<h2>{t('music.comingSoon')}</h2>
			<ul class="soon">
				{#each soon as a (a.mbid)}
					<li>
						<!-- The cover repeats the name link beside it, so screen readers and Tab skip it. -->
						<a href={albumHref(a.mbid, mbid)} tabindex="-1" aria-hidden="true"><span class="cov small">{#if a.image && !badArt.has(a.mbid)}<img src={coverFor(appPicture(a.image), 72).src} alt="" width="72" height="72" loading="lazy" decoding="async" onerror={() => (badArt = new Set([...badArt, a.mbid]))} />{:else}<ArtPlaceholder name={a.name} shape="square" />{/if}</span></a>
						<span class="soon-words"><a class="s-name" href={albumHref(a.mbid, mbid)}>{a.name}</a><span class="s-album">{kindWord(a)} · {releaseWords(a.release_date ?? '')}</span></span>
						{#if !data?.kid}<!-- The bell sits at the row's right edge: its tip lines up with it, so a long name never pushes the page sideways. --><IconButton icon={follows.get(a.mbid) ? 'bellOn' : 'bell'} label={follows.get(a.mbid) ? t('music.stopFollowing', { name: a.name }) : t('music.tellMeWhenIs', { name: a.name })} tipSide="below-end" aria-pressed={!!follows.get(a.mbid)} onclick={() => followAlbum(a)} />{/if}
					</li>
				{/each}
			</ul>
		</section>
	{/if}
{/snippet}

{#snippet videos()}
	{#if hero}<ArtistVideos {mbid} artist={hero.name} {picture} />{/if}
{/snippet}

{#snippet appearsOn()}
	{#if more?.appears_on.length}
		<Shelf title={t('music.appearsOn')} item="var(--music-item)" items={more.appears_on} key={(a) => a.mbid} placeholder="square" lines={2}>
			{#snippet card(a: PageCredit)}{@render creditCard(a)}{/snippet}
		</Shelf>
	{/if}
{/snippet}

{#snippet relatedRow()}
	{#if related.length}
		<section class="block">
			<h2>{t('music.bandsAndCollaborators')}</h2>
			<ul class="circles" use:dragScroll>{#each related as p (p.mbid)}<li><PersonCircle name={p.name} role={p.note} image={appPicture(p.image)} href={artistHref(p.mbid)} size={circle} needsArt /></li>{/each}</ul>
		</section>
	{/if}
{/snippet}

{#snippet similarRow()}
	{#if similar === null}
		<SkRail kind="person" item="{circle + 16}px" titleWidth="150px" />
	{:else if similar.length}
		<section class="block" data-testid="similar-artists">
			<h2>{t('music.similarArtists')}</h2>
			<ul class="circles" use:dragScroll>{#each similar as p (p.mbid)}<li><PersonCircle name={p.name} image={appPicture(p.image)} href={artistHref(p.mbid)} size={circle} needsArt /></li>{/each}</ul>
		</section>
	{/if}
{/snippet}

{#snippet loading()}
	{#if !error}
		<section class="block" aria-busy="true" aria-label={t('music.loadingAlbumsAndSongs')}>
			<SkRowTitle width="120px" /><div class="sk-songs">{#each Array.from({ length: 10 }, (_, i) => i) as i (i)}<SkTrackRow height={64} />{/each}</div>
		</section>
		<div class="sq-rail"><SkRail item="var(--music-item)" titleWidth="160px" /></div>
	{:else}
		<LoadError hasData={true} message={error} network={errorNet} onretry={load} {busy} />
	{/if}
{/snippet}

{#snippet sections()}
	{#if !data}
		{@render loading()}
	{:else}
		{#if data?.top.length}<ArtistTopSongs {songs} {mbid} items={songItems} year={yearOf} bind:badArt />{/if}
		{@render essentialRow()}
		{@render kindRows()}
		{@render storeDisco()}
		{@render soonRow()}
		{@render videos()}
		{#if data}<ArtistStats {listen} {missing} {mayAsk} {manage} {titleId} {allAsked} bind:confirmAll {askingAll} onask={askAllMissing} />{/if}
		{@render appearsOn()}
		{@render relatedRow()}
		{#if hero}<ArtistAbout bind:open={aboutOpen} {hero} {picture} {wideFace} {wideFailed} {photoFailed} {listen} {listenWords} {catalog} {genres} />{/if}
	{/if}
	{@render similarRow()}
{/snippet}

<div class="artist" class:tinted={!!pageTint} class:minimal style={pageTint ? `--ar-bg:${pageTint.bg};--ar-soft:${pageTint.soft}` : undefined}>
	{#if error && !hero}<LoadError hasData={false} message={error} network={errorNet} onretry={load} {busy} />{/if}
	<PageLoader loading={!hero && !error} cached={fromCache} label={t('music.loadingThisArtist')} onretry={load}>
		{#if hero}
			{#if !minimal}<ArtistHeroBanner {mbid} ready={!!hero && hero.mbid === mbid} list={heroList} {picture} backgrounds={art.backgrounds ?? []} {gone} bind:box={heroBox} onimage={heroLoaded} />{/if}
			{@render slimBar()}
			{@render compactLine()}
			{@render libBits()}

			{#if titleId && manage}
				<div class="ar-tabs" role="tablist" data-testid="artist-tabs">
					<button type="button" role="tab" class:on={artistTab === 'overview'} aria-selected={artistTab === 'overview'} onclick={() => (artistTab = 'overview')}>{t('music.overview')}</button>
					<button type="button" role="tab" class:on={artistTab === 'library'} aria-selected={artistTab === 'library'} onclick={() => (artistTab = 'library')}>{t('music.library')}</button>
				</div>
			{/if}
			{#if titleId && manage && artistTab === 'library'}
				<ArtistLibraryTab {titleId} {manage} stamp={lib?.stamp} ondeleted={() => (lib ? void goto(to('/library?kind=artist')) : location.reload())} />
			{:else}
				{@render sections()}
			{/if}

			{#if below}{@render below()}{/if}
		{/if}
	</PageLoader>
</div>

{#if manage}
	<sheets.add bind:this={addDialog} {folders} {rules} {defaults} onadded={(_a, added) => { toasts.push(t('music.added', { what: added.name }), 'ok', { body: t('music.fillingInAlbumsAnd') }); void load(); }} />
{:else if mayAsk}
	<sheets.ask bind:this={askDialog} onasked={onAsked} />
{/if}

<style>
	/* Windows opened on a tinted artist page get a solid tinted background; the page's see-through surfaces stay inside them. */
	.artist.tinted :global(.ui-sheet) { background: radial-gradient(120% 70% at 50% 0%, color-mix(in srgb, var(--ar-bg, #2a2c33) 85%, #fff 6%), color-mix(in srgb, var(--ar-bg, #1b1c20) 55%, #050506) 70%); }
	.artist.tinted { --ok: #30d158; --fail: #ff6961; --fail-text: #ff6961; --accent-text: #fff; --text: #fff; --bg: var(--ar-bg); --text-muted: var(--ar-soft); --surface-1: rgb(255 255 255 / 8%); --surface-2: rgb(255 255 255 / 12%); --border: rgb(255 255 255 / 14%); --line: rgb(255 255 255 / 14%); color: #fff; color-scheme: dark; }
	.artist { --music-item: max(168px, calc((100vw - 96px) / 7.2)); --gap: 40px; display: flex; flex-direction: column; gap: var(--gap); padding-bottom: 48px; min-width: 0; }
	h1 { margin: 0; font-family: var(--font-display); font-weight: 800; line-height: 1; letter-spacing: -.03em; text-wrap: balance; overflow-wrap: anywhere; }
	h2 { margin: 0; font: var(--t-title2); }
	.quiet { margin: 0; color: var(--text-muted); font: var(--t-subhead); }

	.cline { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: end; gap: 20px; margin-top: calc(var(--gap) * -1 - 52px); position: relative; z-index: 1; }
	.c-photo { --photo: 136px; display: block; }
	.photo { width: var(--photo); height: var(--photo); border-radius: 50%; object-fit: cover; flex: none; background: #1c1d21; box-shadow: 0 12px 32px rgb(0 0 0 / .35), 0 0 0 4px var(--bg); }
	.photo.crop { object-position: 50% 22%; }
	.photo.none { position: relative; overflow: hidden; }
	.c-words { display: flex; flex-direction: column; gap: 4px; min-width: 0; padding-bottom: 8px; }
	.c-words h1 { font-size: clamp(30px, 3vw, 40px); color: var(--text); }
	.c-line { margin: 0; color: var(--text-muted); font: 600 15px/1.3 var(--font-ui); font-variant-numeric: tabular-nums; }
	.cline .acts { padding-bottom: 8px; }

	.acts { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
	:global(html:not([data-theme='dark'])) .cline .acts :global(.ui-act), :global(html:not([data-theme='dark'])) .cline .acts :global(.ui-pop > .trigger) { background: color-mix(in srgb, var(--text) 7%, transparent); border-color: var(--line); color: var(--text); backdrop-filter: none; -webkit-backdrop-filter: none; }
	:global(html:not([data-theme='dark'])) .cline .acts :global(.ui-act.primary), :global(html:not([data-theme='dark'])) .cline .acts :global(.ui-act[aria-pressed='true']) { background: var(--text); border-color: var(--text); color: var(--bg); }
	:global(html:not([data-theme='dark'])) .cline .acts :global(.ui-act:focus-visible) { box-shadow: 0 0 0 2px var(--bg), 0 0 0 4px var(--text); }

	.slimwrap { position: sticky; top: var(--topbar); z-index: 25; height: 0; margin-bottom: calc(var(--gap) * -1); transition: transform var(--bar-glide); }
	:global(html.bar-hidden) .slimwrap { transform: translateY(calc(-1 * var(--topbar))); }
	.slim { --tint: #3a3d44; position: absolute; top: 0; left: calc(var(--gutter) * -1); right: calc(var(--gutter) * -1); display: flex; align-items: center; gap: 14px; height: 64px; padding: 0 var(--gutter); box-sizing: border-box; background: color-mix(in srgb, var(--tint) 45%, #0b0c0f); color: #fff; box-shadow: 0 8px 24px rgb(0 0 0 / .22); opacity: 0; translate: 0 -8px; pointer-events: none; transition: opacity var(--dur) var(--ease-out), translate var(--dur) var(--ease-out); }
	/* Minimal: no banner, so the row is a plain header under the top bar, and the slim bar takes the theme. */
	.artist.minimal .cline { margin-top: 0; padding-top: 8px; }
	.artist.minimal .photo { box-shadow: 0 4px 12px rgb(0 0 0 / .18), 0 0 0 4px var(--bg); }
	.artist.minimal .slim { background: var(--bg); color: var(--text); border-bottom: 1px solid var(--line); box-shadow: none; }
	:global(html:not([data-theme='dark'])) .artist.minimal .slim :global(.ui-act.primary) { background: var(--text); border-color: var(--text); color: var(--bg); box-shadow: none; }
	.slim.on { opacity: 1; translate: 0 0; pointer-events: auto; }
	.slim-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 700 22px/1.1 var(--font-display); letter-spacing: -.01em; }
	@media (prefers-reduced-motion: reduce) { .slim, .slimwrap { transition: none; translate: none; } }

	.libbits { display: flex; flex-direction: column; gap: 8px; margin-top: calc(var(--gap) * -1 + 16px); }
	.libline { display: inline-flex; align-items: center; gap: 10px; flex-wrap: wrap; color: var(--text-muted); font: var(--t-footnote); }
	.libword { color: var(--text); font-weight: 600; }
	.bar { width: 120px; height: 4px; border-radius: 99px; background: var(--line); overflow: hidden; }
	.bar i { display: block; height: 100%; background: var(--text); border-radius: inherit; }

	.block { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
	.cov { position: relative; display: block; flex: none; width: 48px; height: 48px; overflow: hidden; border-radius: 6px; background: var(--surface-2); }
	.cov img { width: 100%; height: 100%; object-fit: cover; }
	.cov.small { width: 72px; height: 72px; border-radius: 8px; }
	.soon-words { display: flex; flex-direction: column; gap: 2px; min-width: 0; }

	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--music-item), 1fr)); gap: 24px 16px; margin: 0; padding: 0; list-style: none; --poster-ratio: 1; }
	.rows { display: flex; flex-direction: column; gap: var(--gap); }
	.soon { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr)); gap: 12px; margin: 0; padding: 0; list-style: none; }
	.soon li { display: grid; grid-template-columns: 72px minmax(0, 1fr) auto; gap: 14px; align-items: center; padding: 10px; border-radius: var(--r-card); background: var(--surface-1, var(--surface-2)); }
	.circles { display: flex; gap: 20px; overflow-x: auto; margin: 0; padding: 4px 2px 8px; list-style: none; scrollbar-width: none; }

	.sk-songs { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 24px; }
	.sq-rail { --poster-ratio: 1; }

	.ar-tabs { display: flex; gap: 6px; margin-top: calc(var(--gap) * -1 + 16px); border-bottom: 1px solid var(--line); scroll-margin-top: calc(var(--topbar, 56px) + 16px); }
	.ar-tabs button { height: 40px; padding: 0 14px; border: 0; border-bottom: 2px solid transparent; background: none; color: var(--text-muted); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	@media (max-width: 767px) and (pointer: coarse) { .ar-tabs button { height: 44px; } }
	.ar-tabs button.on { color: var(--text); border-bottom-color: var(--text); }

	@media (max-width: 767px) {
		.artist { --music-item: calc((100cqi - var(--shelf-gap, 12px)) / 2.3); --gap: 32px; }
		.cline { grid-template-columns: minmax(0, 1fr); justify-items: center; text-align: center; gap: 12px; margin-top: calc(var(--gap) * -1 - 56px); }
		.c-photo { --photo: 112px; }
		.c-words { align-items: center; padding: 0; }
		.c-words h1 { font-size: clamp(28px, 8.4vw, 36px); }
		.cline .acts { justify-content: center; padding: 0; }
		.slimwrap { top: 0; }
		:global(html.bar-hidden) .slimwrap { transform: none; }
		.slim { height: 56px; gap: 12px; }
		.slim-name { font-size: 18px; }
		.slim :global(.ui-act.primary) { height: 38px; padding: 0 14px 0 12px; font-size: 14px; }
		.grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.sk-songs { grid-template-columns: minmax(0, 1fr); }
	}
</style>

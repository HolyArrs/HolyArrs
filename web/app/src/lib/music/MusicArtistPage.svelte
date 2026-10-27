<script module lang="ts">
	import type { MenuItem } from '#lib/ui/Menu.svelte';

	/** The Library's own parts when this artist is in the Library (/library/{id}). */
	export type ArtistLibrary = {
		titleId: number;
		staff: boolean;
		menu: MenuItem[];
		progress: string;
		status: string;
		note: string;
		nextRelease?: string;
		onLibrary: () => void;
		onAsk?: () => void;
	};
</script>

<script lang="ts">
	import { dragScroll } from '#lib/drag-scroll.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import PersonCircle from '#lib/ui/PersonCircle.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import Icon from '#lib/Icon.svelte';
	import AskArtistDialog from '#lib/AskArtistDialog.svelte';
	import AddArtistDialog from '#lib/AddArtistDialog.svelte';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import { to } from '#lib/paths.ts';
	import { plainReason } from '#lib/plain.ts';
	import { cameFrom, fromIcon } from '#lib/came-from.svelte.ts';
	import { albumYear } from '#lib/music/music.ts';
	import { releaseWords } from '#lib/music/music-trending.ts';
	import { SkRail, SkRowTitle, SkTrackRow } from '#lib/ui/sk/index.ts';
	import Chip from '#lib/ui/Chip.svelte';
	import { attachHLS, screenTarget } from '#lib/hls-player.ts';
	import { HERO_DELAY, setSoundOn, soundOn } from '#lib/trailer.ts';
	import { artistVideo, autoplayHere, loadUISettings, nativeHLS } from '#lib/autoplay.svelte.ts';
	import { heroVideoStart, type HeroVideo } from '#lib/music/hero-video.ts';
	import { aboutSize, albumHref, appPicture, artistHref, artistKind, countryName, initialsOf, coverFor, haveLine, kindWord, length, owned, SHELVES, type ArtistCoreData, type ArtistMoreData, type ArtistPageData, type PageAlbum, type PageArtist, type PageCredit, type Shelf as ShelfKind } from '#lib/music/music-pages.ts';
	import type { Ask, LibraryFolder, LookupArtist, MusicSettings, PickRulesView } from '#lib/types.ts';
	import ActionMenu from '#lib/ui/ActionMenu.svelte';
	import ArtistVideos from '#lib/ArtistVideos.svelte';
	import ArtistLibraryTab from '#lib/music/ArtistLibrary.svelte';
	import type { Snippet } from 'svelte';


	// The one artist page: any MusicBrainz artist, in the Library or not. The
	// Library route passes `library` (its menu, progress and ask) and `below`
	// (albums to keep, how HolyArrs keeps it, history, wishlist). HolyArrs asks
	// for music; it does not play it, so songs lead to asking for their album.
	// Waits between re-asks of an artist's art when it has no wide photo yet (about 20 s in all).
	const ART_RETRY = [1500, 3000, 5000, 10000];
	let { mbid: rawId, library: lib, below }: { mbid: string; library?: ArtistLibrary; below?: Snippet } = $props();
	const mbid = $derived((rawId ?? '').toLowerCase());
	// Hero first: the core answer (name, picture, bio) draws the hero; the full
	// answer (albums, songs, links) and the similar artists fill the shelves.
	let core = $state<ArtistCoreData | null>(null);
	let data = $state<ArtistPageData | null>(null);
	let similar = $state<PageArtist[] | null>(null);
	let more = $state<ArtistMoreData | null>(null);
	let shelfPick = $state<ShelfKind | 'all'>('albums');
	let havePick = $state<'all' | 'have' | 'missing'>('all');
	let confirmAll = $state(false);
	let discoAll = $state(false);
	let askingAll = $state(false);
	let aboutEl: HTMLElement | undefined = $state();
	let error = $state('');
	let busy = $state(false);
	let lookup = $state<LookupArtist | null>(null);
	let art = $state<{ logo?: string; backgrounds?: string[]; thumb?: string }>({});
	let following = $state<boolean | null>(null);
	let bioOpen = $state(false);
	let heroIn = $state(false);
	// The hero's music video (muted, looping, relayed); none keeps the photo.
	let heroVideo = $state<HeroVideo | null>(null);
	let videoEl: HTMLVideoElement | undefined = $state();
	let videoMuted = $state(true);
	// Covers that failed to load show the shared placeholder instead.
	let badArt = $state(new Set<string>());
	let videoPaused = $state(false);
	// Sound is one choice for the whole app (shared with trailers): turning it
	// on or off here carries to every later video and trailer.
	function toggleMute() {
		if (!videoEl) return;
		videoEl.muted = !videoEl.muted;
		videoMuted = videoEl.muted;
		setSoundOn(!videoEl.muted);
	}
	// When a video ends: the artist's next unplayed video, else the same one again.
	let playedVideos: string[] = [];
	$effect(() => {
		void mbid;
		playedVideos = [];
	});
	async function nextVideo() {
		const cur = heroVideo;
		const el = videoEl;
		if (!cur || !el) return;
		if (!playedVideos.includes(cur.key)) playedVideos = [...playedVideos, cur.key];
		const want = mbid;
		const next = await api.get<HeroVideo>(`/music/artists/${want}?part=video&skip=${encodeURIComponent(playedVideos.join(','))}`).catch(() => null);
		if (want !== mbid || heroVideo !== cur) return;
		if (next?.proxy_url && next.key !== cur.key) {
			heroVideo = next;
			return;
		}
		// None left: start over from the first, repeating this one now.
		playedVideos = [cur.key];
		el.currentTime = heroVideoStart(cur);
		void el.play().catch(() => {});
	}
	function togglePause() {
		if (!videoEl) return;
		if (videoEl.paused) void videoEl.play().catch(() => {});
		else videoEl.pause();
	}
	function fullScreen() {
		const el = videoEl as (HTMLVideoElement & { webkitEnterFullscreen?: () => void }) | undefined;
		if (!el) return;
		if (el.requestFullscreen) void el.requestFullscreen().catch(() => {});
		else el.webkitEnterFullscreen?.();
	}
	let videoIn = $state(false);
	const videoOn = $derived(videoIn && !videoPaused);
	let logoFailed = $state(false);
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let defaults = $state<MusicSettings | null>(null);
	let askDialog: ReturnType<typeof AskArtistDialog> | undefined = $state();
	let addDialog: ReturnType<typeof AddArtistDialog> | undefined = $state();
	let asked = $state(new Set<string>());
	let follows = $state(new Map<string, boolean>());

	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask'));
	const hero = $derived<ArtistCoreData | null>(data ?? core);
	const titleId = $derived(lib?.titleId || hero?.title_id || lookup?.title_id || 0);
	// Overview or Library, for an artist in the Library (?tab=library opens it there).
	let artistTab = $state<'overview' | 'library'>(typeof location !== 'undefined' && new URLSearchParams(location.search).get('tab') === 'library' ? 'library' : 'overview');

	function load() {
		const want = mbid;
		busy = true;
		error = '';
		let followAsked = false;
		const askFollow = (id?: number) => {
			if (!id || followAsked) return;
			followAsked = true;
			void api.get<{ following: boolean }>(`/titles/${id}/follow`).then((r) => want === mbid && (following = r.following)).catch(() => {});
		};
		// All at once: the hero, the shelves, the similar artists, the artwork and the ask state.
		const coreAsk = api.get<ArtistCoreData>(`/music/artists/${want}?part=core`).then((c) => {
			if (want !== mbid) return;
			if (!data) core = c;
			askFollow(c.title_id);
		});
		const fullAsk = api.get<ArtistPageData>(`/music/artists/${want}?similar=0`).then((d) => {
			if (want !== mbid) return;
			data = d;
			follows = new Map(d.albums.filter((a) => a.upcoming).map((a) => [a.mbid, !!a.following]));
			askFollow(d.title_id);
		});
		void api.get<{ similar: PageArtist[] }>(`/music/artists/${want}?part=similar`).then((r) => want === mbid && (similar = r.similar)).catch(() => want === mbid && (similar = []));
		void api.get<ArtistMoreData>(`/music/artists/${want}?part=more`).then((r) => want === mbid && (more = r)).catch(() => want === mbid && (more = { essential: [], appears_on: [] }));
		// A cold artist's art may still be on its way (the server fetches it
		// right then): an answer with no wide photo is asked again over about
		// 20 s, and the photo fades in when it lands.
		const askArt = (left: number[]) => {
			void api.get<typeof art>(`/extras/artist/${want}`).then((x) => {
				if (want !== mbid) return;
				// The server's own picture routes sit under the app's base path.
				const local = (u?: string) => (u && u.startsWith('/') ? to(u) : u);
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
			// The shelves failing shows the error; the core failing alone is quiet (the full answer has the hero too).
			if (f.status === 'rejected') error = message(f.reason);
			else if (c.status === 'rejected' && !data) error = message(c.reason);
		});
	}

	$effect(() => {
		void mbid;
		core = null;
		data = null;
		similar = null;
		more = null;
		shelfPick = 'albums';
		discoAll = false;
		havePick = 'all';
		confirmAll = false;
		lookup = null;
		art = {};
		following = null;
		heroIn = false;
		heroVideo = null;
		videoIn = false;
		logoFailed = false;
		bioOpen = false;
		void load();
	});

	// As with a trailer hero: the beat (HERO_DELAY) starts when the hero
	// shows, and the video (looked up ahead of time when it could be) is
	// asked for at the same moment, only where trailers may autoplay.
	let beatDone = $state(false);
	$effect(() => {
		const want = mbid;
		if (!hero || hero.mbid !== want) return;
		beatDone = false;
		const beat = setTimeout(() => (beatDone = true), HERO_DELAY);
		return () => clearTimeout(beat);
	});
	$effect(() => {
		const want = mbid;
		if (!hero || hero.mbid !== want || heroVideo) return;
		const ctl = new AbortController();
		void loadUISettings().then(() => {
			if (ctl.signal.aborted || want !== mbid || !autoplayHere()) return;
			void artistVideo(want, ctl.signal).then((v) => !ctl.signal.aborted && want === mbid && v?.proxy_url && (heroVideo = v));
		});
		return () => ctl.abort();
	});

	// Play it on the stage; an error drops back to the photo.
	$effect(() => {
		const v = heroVideo;
		const el = videoEl;
		if (!v || !el) return;
		let detach: (() => void) | null = null;
		let stopped = false;
		const fail = () => {
			videoIn = false;
			heroVideo = null;
		};
		el.muted = true;
		videoMuted = true;
		el.addEventListener('error', fail);
		void attachHLS(el, v.proxy_url, nativeHLS(), heroVideoStart(v), screenTarget('cover'))
			.then((d) => {
				if (stopped) return d();
				detach = d;
				attached = true;
			})
			.catch(fail);
		return () => {
			stopped = true;
			attached = false;
			el.removeEventListener('error', fail);
			detach?.();
		};
	});

	// The photo and name hold for the beat; then the video takes over.
	let attached = $state(false);
	$effect(() => {
		if (!(attached && beatDone && videoEl && heroVideo)) return;
		const el = videoEl;
		void el
			.play()
			.then(() => {
				// Sound was left on elsewhere: unmute. A browser that pauses an
				// unmuted video without a recent click keeps it muted, and the
				// button shows the truth either way.
				if (!soundOn()) return;
				el.muted = false;
				if (el.paused) {
					el.muted = true;
					void el.play().catch(() => {});
				}
				videoMuted = el.muted;
			})
			.catch(() => {
				videoIn = false;
				heroVideo = null;
			});
	});

	onMount(() => {
		if (session.can('manage_library')) {
			void api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
			void api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
			void api.get<MusicSettings>('/settings/music').then((r) => (defaults = r)).catch(() => {});
		}
	});

	// The artist's photo. One the server has not stored yet is asked for
	// again while the page is open (it fetches it right away, a user is
	// waiting), and swaps in once stored: no reload needed.
	let photoTry = $state(0);
	const PHOTO_RETRY = [20000, 70000, 150000];
	const picture = $derived.by(() => {
		const p = appPicture(hero?.image);
		return p && photoTry ? `${p}${p.includes('?') ? '&' : '?'}fresh=${photoTry}` : p;
	});
	// The hero never stays empty: each wide photo in turn, then a soft crop of the artist's photo.
	const heroList = $derived([...(art.backgrounds ?? []), ...(picture ? [picture] : [])]);
	let heroTry = $state(0);
	$effect(() => {
		void heroList.length;
		void mbid;
		heroTry = 0;
	});
	const heroSrc = $derived(heroList[heroTry] ?? null);
	// A new hero picture (a late wide photo) fades in from dark, never pops.
	$effect(() => {
		void heroSrc;
		heroIn = false;
	});
	const heroSoft = $derived(!!heroSrc && heroSrc === picture && !(art.backgrounds ?? []).includes(heroSrc));
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
	// No round photo: crop the wide hero photo into the circle (face sits upper middle); initials only with no image at all.
	const wideFace = $derived((art.backgrounds ?? [])[0] ?? null);
	let wideFailed = $state(false);
	$effect(() => {
		void wideFace;
		wideFailed = false;
	});
	const facts = $derived.by(() => {
		if (!hero) return [];
		return [artistKind(hero.type), countryName(hero.country), hero.status === 'ended' ? 'Disbanded or retired' : '', (data?.genres ?? []).slice(0, 3).join(', ')].filter(Boolean) as string[];
	});
	const released = $derived(data?.albums.filter((a) => !a.upcoming) ?? []);
	// The band under the hero: what this artist has put out, in big numbers.
	const stats = $derived.by(() => {
		if (!data) return [];
		const plain = (t: string) => released.filter((a) => a.type?.toLowerCase() === t && !a.secondary_types?.length).length;
		const years = released.map((a) => a.release_date?.slice(0, 4)).filter((y): y is string => !!y).sort();
		const span = years.length ? (years[0] === years[years.length - 1] ? years[0] : `${years[0]}–${years[years.length - 1]}`) : '';
		return [
			{ v: String(released.length), l: released.length === 1 ? 'Release' : 'Releases' },
			{ v: String(plain('album')), l: 'Albums' },
			{ v: String(plain('ep')), l: 'EPs' },
			{ v: String(plain('single')), l: 'Singles' },
			{ v: span, l: 'Years active' }
		].filter((x) => x.v && x.v !== '0');
	});
	// One sentence or two of the bio, under the facts; the full text is in About.
	const bioLine = $derived(hero?.bio?.extract ? hero.bio.extract.split(/(?<=[.!?])\s+/).slice(0, 2).join(' ') : '');
	// "0 of 113 tracks · 48 missing" said once, even when progress and status repeat a part.
	const libLine = $derived(lib?.staff ? [...new Set([lib.progress, lib.status].filter(Boolean).flatMap((x) => String(x).split(' · ')))].join(' · ') : '');
	const haveShare = $derived(data?.total ? Math.round((100 * (data.have ?? 0)) / data.total) : 0);
	// The discography chips: only kinds this artist has; have/missing by Library state.
	const kinds = $derived(SHELVES.filter((s) => released.some((a) => a.shelf === s.shelf)));
	const disco = $derived(released.filter((a) => (shelfPick === 'all' || a.shelf === shelfPick) && (havePick === 'all' || (havePick === 'have' ? a.state === 'complete' || a.state === 'partial' : !(a.state === 'complete' || a.state === 'partial')))));
	// Albums first when the artist has any; a long list shows 18 until Show all.
	const shown = $derived(discoAll ? disco : disco.slice(0, 18));
	$effect(() => {
		if (data && shelfPick !== 'all' && !kinds.some((k) => k.shelf === shelfPick)) shelfPick = 'all';
	});
	const essential = $derived.by(() => {
		if (!data || !more) return [];
		const by = new Map(data.albums.map((a) => [a.mbid, a]));
		return more.essential.map((id) => by.get(id)).filter((a): a is PageAlbum => !!a);
	});
	const missing = $derived(data?.missing);
	const allAsked = $derived(!!lookup?.ask && !lookup.ask.albums?.length);
	const soon = $derived(data?.albums.filter((a) => a.upcoming) ?? []);

	/** The person's ask, or what they may do: ask (members) or add (managers). */
	function askAlbum(a?: PageAlbum) {
		if (!lookup) return void toasts.push('Still reading this artist; try again in a moment.', 'fail');
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
		toasts.push(r.joined ? `Someone asked for ${r.ask.name} already; you are on the ask too.` : `Asked for ${r.ask.name}. You will hear when it is approved.`, 'ok');
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
			toasts.push(message(e), 'fail');
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
			toasts.push(message(e), 'fail');
		}
	}

	async function followAlbum(a: PageAlbum) {
		const on = !follows.get(a.mbid);
		follows = new Map([...follows, [a.mbid, on]]);
		try {
			if (on) await api.put(`/metadata/album/${a.mbid}/follow`, { name: a.name, artist: hero?.name ?? '', release_date: a.release_date ?? '' });
			else await api.del(`/metadata/album/${a.mbid}/follow`);
			toasts.push(on ? `You will hear when ${a.name} is out.` : `Stopped following ${a.name}.`, 'ok');
		} catch (e) {
			follows = new Map([...follows, [a.mbid, !on]]);
			toasts.push(message(e), 'fail');
		}
	}

	const albumAsked = (a: PageAlbum) => asked.has(a.mbid) || (!!lookup?.ask && !lookup.ask.albums?.length);
	const cta = $derived(titleId ? 'Open in Library' : lookup?.ask ? 'Asked' : manage ? 'Add artist' : 'Ask for discography');

	const from = $derived.by(() => (void page.url.pathname, cameFrom()));
	function stepBack(e: MouseEvent) {
		if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
		e.preventDefault();
		history.back();
	}
</script>

<svelte:head><title>{hero?.name ?? 'Artist'} · HolyArrs</title></svelte:head>

{#snippet albumCard(a: PageAlbum, eager = false)}
	{@const c = coverFor(appPicture(a.image), 200)}
	<PosterCard title={a.name} href={albumHref(a.mbid, mbid)} image={c.src} srcset={c.srcset} {eager} kind="artist" shape="square" music actionOnHover
		subtitle={[albumYear(a.release_date), a.shelf === 'albums' ? '' : kindWord(a)].filter(Boolean).join(' · ') || undefined}
		badges={a.explicit ? ['E'] : undefined} owned={owned(a, albumAsked(a))}>
		{#snippet action()}
			{#if !titleId && !albumAsked(a) && (manage || mayAsk)}
				<IconButton icon="plus" label={manage ? `Add ${hero?.name}` : `Ask for ${a.name}`} variant="glass" size="lg" aria-haspopup="dialog" onclick={() => askAlbum(a)} />
			{/if}
		{/snippet}
	</PosterCard>
{/snippet}

{#snippet creditCard(a: PageCredit)}
	{@const c = coverFor(appPicture(a.image), 200)}
	<PosterCard title={a.name} href={albumHref(a.mbid, a.artist_mbid || mbid)} image={c.src} srcset={c.srcset} kind="artist" shape="square" music
		subtitle={a.artist_name || undefined} badges={a.explicit ? ['E'] : undefined} />
{/snippet}

<div class="artist">
	{#if error && !hero}<LoadError hasData={false} message={plainReason(error).text} onretry={load} {busy} />{/if}
	<PageLoader loading={!hero && !error} label="Loading this artist" onretry={load}>
		{#if hero}
			<header class="hero" class:vid={videoOn} class:bare={!heroSrc && !heroVideo} data-hero>
				<div class="stage" aria-hidden="true">
					{#if heroSrc}
						{#key heroSrc}
							<img class="art" class:soft={heroSoft} class:in={heroIn} class:under={videoOn} src={heroSrc} alt="" decoding="async" fetchpriority="high" onload={() => (heroIn = true)} onerror={() => { heroIn = false; heroTry++; }} />
						{/key}
					{/if}
					{#if heroVideo}
						<video class="clip" class:in={videoOn} bind:this={videoEl} muted playsinline disablepictureinpicture preload="auto" onplaying={() => { videoIn = true; videoPaused = false; }} onpause={() => (videoPaused = true)} onended={() => void nextVideo()} onplay={() => (videoPaused = false)} onvolumechange={() => videoEl && (videoMuted = videoEl.muted)} data-testid="artist-hero-video"></video>
					{/if}
				</div>
				<div class="shade" aria-hidden="true"></div>
				{#if heroVideo && videoIn}
					<div class="vctl" data-testid="artist-hero-video-controls">
						<button type="button" onclick={togglePause} aria-label={videoPaused ? 'Play video' : 'Pause video'}><Icon name={videoPaused ? 'play' : 'pause'} size={16} /></button>
						<button type="button" onclick={toggleMute} aria-label={videoMuted ? 'Turn sound on' : 'Turn sound off'}><Icon name={videoMuted ? 'soundOff' : 'sound'} size={16} /></button>
						<button type="button" onclick={fullScreen} aria-label="Full screen"><Icon name="expand" size={16} /></button>
					</div>
				{/if}
				{#if from?.label}<nav class="crumbs" aria-label="Breadcrumb"><a class="chip" href={from.path} onclick={stepBack} data-testid="artist-back"><Icon name={fromIcon(from.path) ?? 'chevronLeft'} size={14} />{from.label}</a></nav>{/if}
				<div class="hero-in">
					{#if picture && !photoFailed}<img class="photo" src={picture} alt="" width="184" height="184" onerror={() => (photoFailed = true)} />{:else if wideFace && !wideFailed}<img class="photo crop" src={wideFace} alt="" width="184" height="184" onerror={() => (wideFailed = true)} />{:else}<div class="photo initials" aria-hidden="true">{initialsOf(hero.name)}</div>{/if}
					{#if art.logo && !logoFailed}
						<h1 class="logo"><img src={art.logo} alt={hero.name} onerror={() => (logoFailed = true)} /></h1>
					{:else}
						<h1>{hero.name}</h1>
					{/if}
				</div>
			</header>
			<div class="under-hero">
					{#if facts.length}<p class="facts">{facts.join(' · ')}</p>{/if}
					{#if bioLine}<p class="bioline">{bioLine} <button type="button" class="more" onclick={() => aboutEl?.scrollIntoView({ behavior: 'smooth', block: 'start' })}>More</button></p>{/if}
					{#if stats.length}
						<dl class="stats" data-testid="artist-stats">
							{#each stats as x (x.l)}<div><dt>{x.l}</dt><dd>{x.v}</dd></div>{/each}
						</dl>
					{/if}
					<div class="round">
						<button class="rb" type="button" aria-label="About {hero.name}" title="About" onclick={() => aboutEl?.scrollIntoView({ behavior: 'smooth', block: 'start' })}><Icon name="info" size={20} /></button>
						{#if lib?.staff}
							<button class="rb main pill" type="button" onclick={lib.onLibrary} aria-label="In Library: go to the Library section" data-testid="hero-library"><Icon name="library" size={18} /><span>In Library</span></button>
						{:else if lib?.onAsk}
							<button class="rb main pill" type="button" onclick={lib.onAsk} data-testid="ask-albums"><Icon name="plus" size={18} /><span>Ask for albums</span></button>
						{:else if lib}
							<span></span>
						{:else if titleId}
							{#if manage}<button class="rb main pill" type="button" onclick={() => (artistTab = 'library')}><Icon name="library" size={18} /><span>In Library</span></button>{:else}<a class="rb main pill" href={to(`/library/${titleId}`)}><Icon name="library" size={18} /><span>Open in Library</span></a>{/if}
						{:else if manage || mayAsk}
							<button class="rb main pill" type="button" disabled={!!lookup?.ask || !lookup} onclick={() => askAlbum()} data-testid="artist-ask"><Icon name={lookup?.ask ? 'check' : 'plus'} size={18} /><span>{cta}</span></button>
						{/if}
						{#if titleId && following !== null}
							<button class="rb" type="button" class:on={following} aria-pressed={following} aria-label={following ? 'Following' : 'Follow'} title={following ? 'Following' : 'Follow'} onclick={toggleFollow}><Icon name={following ? 'bellOn' : 'bell'} size={20} /></button>
						{/if}
						{#if lib?.staff}<ActionMenu label="More actions" caption="More" icon="more" items={lib.menu} />{/if}
					</div>
					{#if lib?.note}<p class="facts">{lib.note}</p>{/if}
					{#if libLine}
						<div class="libline" data-testid="artist-progress">
							<span class="libword">Your Library</span>
							<span class="bar" aria-hidden="true"><i style="width: {haveShare}%"></i></span>
							<span>{libLine}</span>
						</div>
					{/if}
					{#if lib?.nextRelease}<p class="have" data-testid="artist-next">Next album: {lib.nextRelease}</p>{/if}
					{#if data?.total}<p class="have" data-testid="artist-have"><Icon name="music" size={14} />{haveLine(data.have, data.total)}</p>{/if}
			</div>

			{#if titleId && manage}
				<!-- An artist in the Library: Overview (this page) and Library (releases, tracks, settings). -->
				<div class="ar-tabs" role="tablist" data-testid="artist-tabs">
					<button type="button" role="tab" class:on={artistTab === 'overview'} aria-selected={artistTab === 'overview'} onclick={() => (artistTab = 'overview')}>Overview</button>
					<button type="button" role="tab" class:on={artistTab === 'library'} aria-selected={artistTab === 'library'} onclick={() => (artistTab = 'library')}>Library</button>
				</div>
			{/if}
			{#if titleId && manage && artistTab === 'library'}
				<ArtistLibraryTab {titleId} {manage} ondeleted={() => location.reload()} />
			{:else}
			<!-- Music videos sit under the artist's facts, where a title page has its trailers. -->
			{#if hero}<ArtistVideos {mbid} artist={hero.name} {picture} />{/if}

			{#if !data}
				{#if !error}
					<section class="lead" aria-busy="true" aria-label="Loading albums and songs">
						<div class="latest"><SkRowTitle width="140px" /><div class="latest-card sk-latest"><span class="sk sq"></span><span class="sk-lines"><span class="sk sk-line"></span><span class="sk sk-line"></span><span class="sk sk-line short"></span></span></div></div>
						<div class="top"><SkRowTitle width="120px" /><div class="sk-songs">{#each Array.from({ length: 9 }, (_, i) => i) as i (i)}<SkTrackRow height={60} />{/each}</div></div>
					</section>
					<div class="sq-rail"><SkRail item="var(--music-item)" titleWidth="160px" /></div>
				{:else}
					<LoadError hasData={true} message={plainReason(error).text} onretry={load} {busy} />
				{/if}
			{:else}
				{#if data.latest || data.top.length}
					<section class="lead">
						{#if data.latest}
							{@const l = data.latest}
							<div class="latest">
								<h2>Latest release</h2>
								<a class="latest-card" href={albumHref(l.mbid, mbid)}>
									<span class="cov" style="width:160px;height:160px">{#if l.image && !badArt.has(l.mbid)}<img src={coverFor(appPicture(l.image), 200).src} srcset={coverFor(appPicture(l.image), 200).srcset} sizes="160px" alt="" width="160" height="160" loading="lazy" decoding="async" onerror={() => (badArt = new Set([...badArt, l.mbid]))} />{:else}<ArtPlaceholder name={l.name} shape="square" />{/if}</span>
									<span class="l-meta">
										<span class="l-date">{l.release_date ? new Date(`${l.release_date.padEnd(10, '-01').slice(0, 10)}T00:00:00Z`).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: l.release_date.length === 10 ? 'numeric' : undefined, timeZone: 'UTC' }) : ''}</span>
										<span class="l-name">{#if l.explicit}<span class="e" aria-label="Explicit">E</span>{/if}{l.name}</span>
										<span class="l-kind">{kindWord(l)}</span>
										{#if owned(l, albumAsked(l))}{@const o = owned(l, albumAsked(l))}<span class="pill {o?.state}">{o?.label}</span>{/if}
									</span>
								</a>
							</div>
						{/if}
						{#if data.top.length}
							<div class="top">
								<h2>Top songs</h2>
								<ol class="songs">
									{#each data.top.slice(0, 9) as s, i (s.title + i)}
										<li>
											<span class="cov" style="width:40px;height:40px">{#if s.image && !badArt.has('top:' + s.title)}<img src={coverFor(appPicture(s.image), 40).src} alt="" width="40" height="40" loading="lazy" decoding="async" onerror={() => (badArt = new Set([...badArt, 'top:' + s.title]))} />{:else}<ArtPlaceholder name={s.title} shape="square" />{/if}</span>
											<span class="s-words">
												<span class="s-name">{#if s.explicit}<span class="e" aria-label="Explicit">E</span>{/if}{s.title}</span>
												{#if s.album_mbid}<a class="s-album" href={albumHref(s.album_mbid, mbid)}>{s.album} · {length(s.length_ms)}</a>{:else}<span class="s-album">{s.album}</span>{/if}
											</span>
											{#if s.album_mbid && !titleId && (manage || mayAsk)}
												{@const al = data.albums.find((x) => x.mbid === s.album_mbid)}
												{#if al && !albumAsked(al)}
													<IconButton icon="plus" size="sm" label={`Ask for the album ${s.album}`} onclick={() => askAlbum(al)} />
												{:else}
													<span class="s-done" title="Asked"><Icon name="check" size={14} /></span>
												{/if}
											{:else}
												<span></span>
											{/if}
										</li>
									{/each}
								</ol>
							</div>
						{/if}
					</section>
				{/if}

				{#if essential.length}
					<section class="block">
						<h2>Essential albums</h2>
						<ul class="essential">{#each essential as a (a.mbid)}<li>{@render albumCard(a, true)}</li>{/each}</ul>
					</section>
				{/if}

				{#if released.length}
					<section class="block" data-testid="discography">
						<div class="row-head">
							<h2>Discography</h2>
							{#if missing?.count && mayAsk && !manage && !titleId && !allAsked}
								{#if confirmAll}
									<span class="confirm" role="group" aria-label="Confirm asking for all missing">
										<span>Ask for {missing.count} {missing.count === 1 ? 'release' : 'releases'}, {aboutSize(missing.size_bytes)}?</span>
										<Button variant="primary" onclick={askAllMissing} disabled={askingAll}>Ask</Button>
										<Button onclick={() => (confirmAll = false)}>Cancel</Button>
									</span>
								{:else}
									<Button icon="plus" onclick={() => (confirmAll = true)} data-testid="ask-all-missing">Ask for all missing ({missing.count})</Button>
								{/if}
							{/if}
						</div>
						<div class="chips" role="toolbar" aria-label="Filter the discography">
							<Chip label="All" selected={shelfPick === 'all'} onclick={() => (shelfPick = 'all')} />
							{#each kinds as k (k.shelf)}<Chip label={k.title} selected={shelfPick === k.shelf} onclick={() => (shelfPick = k.shelf)} />{/each}
							<span class="sep" aria-hidden="true"></span>
							<Chip label="Have" icon="owned" selected={havePick === 'have'} onclick={() => (havePick = havePick === 'have' ? 'all' : 'have')} />
							<Chip label="Missing" icon="dash" selected={havePick === 'missing'} onclick={() => (havePick = havePick === 'missing' ? 'all' : 'missing')} />
						</div>
						{#if disco.length}
							<ul class="grid">{#each shown as a, i (a.mbid)}<li>{@render albumCard(a, i < 6)}</li>{/each}</ul>
							{#if disco.length > shown.length}<div><Button onclick={() => (discoAll = true)}>Show all {disco.length}</Button></div>{/if}
						{:else}
							<p class="quiet">Nothing here with these filters.</p>
						{/if}
					</section>
				{/if}

				{#if soon.length}
					<section class="block">
						<h2>Coming soon</h2>
						<ul class="soon">
							{#each soon as a (a.mbid)}
								<li>
									<a href={albumHref(a.mbid, mbid)}><span class="cov" style="width:72px;height:72px">{#if a.image && !badArt.has(a.mbid)}<img src={coverFor(appPicture(a.image), 72).src} alt="" width="72" height="72" loading="lazy" decoding="async" onerror={() => (badArt = new Set([...badArt, a.mbid]))} />{:else}<ArtPlaceholder name={a.name} shape="square" />{/if}</span></a>
									<span class="soon-words"><a class="s-name" href={albumHref(a.mbid, mbid)}>{a.name}</a><span class="s-album">{kindWord(a)} · {releaseWords(a.release_date ?? '')}</span></span>
									{#if !data.kid}<IconButton icon={follows.get(a.mbid) ? 'bellOn' : 'bell'} label={follows.get(a.mbid) ? `Stop following ${a.name}` : `Tell me when ${a.name} is out`} aria-pressed={!!follows.get(a.mbid)} onclick={() => followAlbum(a)} />{/if}
								</li>
							{/each}
						</ul>
					</section>
				{/if}
			{/if}

			{#if similar === null}
				<SkRail kind="person" item="104px" titleWidth="150px" />
			{:else if similar.length}
				<section class="block">
					<h2>Fans also like</h2>
					<ul class="circles" use:dragScroll>{#each similar as p (p.mbid)}<li><PersonCircle name={p.name} image={appPicture(p.image)} href={artistHref(p.mbid)} size={152} /></li>{/each}</ul>
				</section>
			{/if}

			{#if more?.appears_on.length}
				<Shelf title="Appears on" item="var(--music-item)" items={more.appears_on} key={(a) => a.mbid} placeholder="square" lines={2}>
					{#snippet card(a: PageCredit)}{@render creditCard(a)}{/snippet}
				</Shelf>
			{/if}

			{#if data?.related.length}
				<section class="block">
					<h2>Bands and collaborators</h2>
					<ul class="circles" use:dragScroll>{#each data.related as p (p.mbid)}<li><PersonCircle name={p.name} role={p.note} image={appPicture(p.image)} href={artistHref(p.mbid)} size={152} /></li>{/each}</ul>
				</section>
			{/if}

			<section class="block about" bind:this={aboutEl} data-testid="artist-about">
				<h2>About {hero.name}</h2>
				<div class="about-in">
					{#if hero.bio}
						<div>
							<p class="bio" class:open={bioOpen}>{hero.bio.extract}</p>
							<div class="bio-foot">
								<button class="more" type="button" onclick={() => (bioOpen = !bioOpen)} aria-expanded={bioOpen}>{bioOpen ? 'Less' : 'More'}</button>
							</div>
						</div>
					{:else if !data}
						<span class="bio-sk" aria-hidden="true"><span class="sk sk-line"></span><span class="sk sk-line"></span><span class="sk sk-line short"></span></span>
					{:else}
						<p class="quiet">No write-up for this artist yet.</p>
					{/if}
					<dl class="kv">
						{#if hero.type}<div><dt>Kind</dt><dd>{artistKind(hero.type)}</dd></div>{/if}
						{#if hero.country}<div><dt>From</dt><dd>{countryName(hero.country)}</dd></div>{/if}
						{#if data?.genres.length}<div><dt>Genres</dt><dd>{data.genres.slice(0, 4).join(', ')}</dd></div>{/if}
						{#if data?.total}<div><dt>Releases</dt><dd>{data.total}</dd></div>{/if}
						{#if hero.status}<div><dt>Active</dt><dd>{hero.status === 'ended' ? 'No longer active' : 'Still active'}</dd></div>{/if}
						{#if lib?.nextRelease}<div><dt>Next album</dt><dd>{lib.nextRelease}</dd></div>{/if}
					</dl>
				</div>
			</section>
			{/if}

			{#if below}{@render below()}{/if}

		{/if}
	</PageLoader>
</div>

{#if manage}
	<AddArtistDialog bind:this={addDialog} {folders} {rules} {defaults} onadded={(_a, t) => { toasts.push(`${t.name} is in your Library. Albums and tracks are being filled in.`, 'ok'); void load(); }} />
{:else if mayAsk}
	<AskArtistDialog bind:this={askDialog} onasked={onAsked} />
{/if}

<style>
	.artist { --music-item: max(168px, calc((100vw - 96px) / 7.2)); display: flex; flex-direction: column; gap: 40px; padding-bottom: 48px; min-width: 0; }
	/* The hero stays dark in both themes: a centred photo on a slowly panning wash. */
	.hero { --hero-ink: #07080a; position: relative; isolation: isolate; min-height: clamp(460px, 52vw, 640px); display: flex; align-items: flex-end; justify-content: center; overflow: hidden; margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) 0; background: var(--hero-ink); color: #fff; }
	.stage { position: absolute; inset: -6%; z-index: -2; animation: pan 48s ease-in-out infinite alternate; }
	.art { width: 100%; height: 100%; object-fit: cover; object-position: center 22%; opacity: 0; transition: opacity 600ms ease-out; }
	.art.in { opacity: .45; }
	.art.soft { filter: blur(56px) saturate(150%); transform: scale(1.3); }
	.art.soft.in { opacity: .6; }
	@keyframes pan { from { transform: scale(1.04) translate3d(-2%, -1%, 0); } to { transform: scale(1.12) translate3d(2%, 1.5%, 0); } }
	@media (prefers-reduced-motion: reduce) { .stage { animation: none; } }
	/* The music video sits over the photo and fades in once it plays. */
	.clip { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); pointer-events: none; }
	@media (prefers-reduced-motion: reduce) { .clip { transition: none; } }
	.clip.in { opacity: 1; }
	.hero { transition: min-height 900ms var(--ease-out); }
	.hero.vid { min-height: min(56.25vw, 86vh); }
	/* No photo and no video: a compact header (name and initials), never a tall empty block. */
	.hero.bare { min-height: 0; background: linear-gradient(180deg, color-mix(in srgb, var(--hero-ink) 60%, var(--surface-2)), var(--hero-ink)); }
	.hero.bare .hero-in { padding-top: 56px; }
	.hero.vid .stage { inset: 0; animation: none; }
	/* Video on: photo and name settle smaller at the bottom so the video leads. */
	.hero-in { transform-origin: 50% 100%; transition: transform 1100ms cubic-bezier(.22, 1, .36, 1) 250ms, opacity 600ms ease 250ms; will-change: transform; }
	.hero.vid .hero-in { transform: scale(.58); opacity: .92; }
	@media (max-width: 600px) { .hero.vid .hero-in { transform: scale(.7); } }
	@media (prefers-reduced-motion: reduce) { .hero-in { transition: none; } }
	.vctl { position: absolute; right: clamp(16px, 4vw, 48px); bottom: clamp(16px, 2.5vw, 28px); z-index: 2; display: flex; gap: 8px; }
	.vctl button { display: grid; place-items: center; width: 38px; height: 38px; border-radius: 50%; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 18%); backdrop-filter: blur(18px) saturate(160%); -webkit-backdrop-filter: blur(18px) saturate(160%); color: #fff; cursor: pointer; transition: background .2s; }
	.vctl button:hover { background: rgb(255 255 255 / 26%); }
	.vctl button:focus-visible { outline: 2px solid #fff; outline-offset: 2px; }
	@media (max-width: 600px) { .vctl { bottom: 12px; right: 12px; gap: 6px; } .vctl button { width: 34px; height: 34px; } }
	.art.under, .art.soft.under { opacity: 0; transition-duration: 900ms; }
	.shade { position: absolute; inset: 0; z-index: -1; background: radial-gradient(ellipse 60% 55% at 50% 42%, rgb(7 8 10 / 0) 0%, rgb(7 8 10 / .55) 70%, var(--hero-ink) 100%), linear-gradient(to top, var(--hero-ink) 0%, rgb(7 8 10 / 0) 45%); }
	.crumbs { position: absolute; top: clamp(14px, 2vw, 22px); left: clamp(20px, 4vw, 48px); z-index: 1; }
	.chip { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 12px 0 10px; border-radius: 999px; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 18%); backdrop-filter: blur(18px) saturate(160%); -webkit-backdrop-filter: blur(18px) saturate(160%); color: #fff; font: 600 13px/1.2 var(--font-ui); }
	.chip:hover { background: rgb(255 255 255 / 24%); }
	.hero-in { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 72px 20px clamp(16px, 2vw, 24px); text-align: center; max-width: 900px; }
	.photo { width: 184px; height: 184px; border-radius: 50%; object-fit: cover; box-shadow: 0 30px 80px rgb(0 0 0 / .6), 0 0 0 1px rgb(255 255 255 / .14); margin-bottom: 6px; }
	.photo.crop { object-position: 50% 22%; }
	.photo.initials { display: grid; place-items: center; background: rgb(255 255 255 / .08); backdrop-filter: blur(12px); color: rgb(255 255 255 / .9); font-size: 64px; font-weight: 700; letter-spacing: .02em; }
	h1 { margin: 0; font-family: var(--font-display); font-weight: 700; font-size: clamp(34px, 4.6vw, 56px); line-height: 1.05; letter-spacing: -.02em; text-wrap: balance; }
	h1.logo img { display: block; max-width: min(440px, 80vw); max-height: 120px; object-fit: contain; filter: drop-shadow(0 4px 18px rgb(0 0 0 / .45)); }
	.under-hero { --hero-ink: #07080a; display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 2px 20px 0; margin-bottom: calc(var(--s-6) * -.5); text-align: center; }
	.under-hero .round { margin-top: 2px; }
	.bioline { margin: 2px 0 0; max-width: 64ch; color: rgb(255 255 255 / .78); font: var(--t-callout); line-height: 1.5; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.bioline .more { color: #fff; font-weight: 600; text-decoration: underline; text-underline-offset: 3px; }
	.stats { display: flex; flex-wrap: wrap; justify-content: center; gap: 0; margin: 10px 0 4px; padding: 12px 0; border-block: 1px solid rgb(255 255 255 / .12); width: min(720px, 100%); }
	.stats div { display: flex; flex-direction: column-reverse; align-items: center; gap: 2px; padding: 0 22px; border-left: 1px solid rgb(255 255 255 / .1); }
	.stats div:first-child { border-left: 0; }
	.stats dt { color: rgb(255 255 255 / .55); font-size: 11.5px; font-weight: 600; letter-spacing: .06em; text-transform: uppercase; }
	.stats dd { margin: 0; color: #fff; font: 700 22px/1.1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.rb.main.pill { display: inline-flex; align-items: center; gap: 8px; width: auto; height: 48px; padding: 0 22px; border-radius: 999px; font: 600 15px/1 var(--font-ui); white-space: nowrap; }
	.libline { display: inline-flex; align-items: center; gap: 10px; flex-wrap: wrap; justify-content: center; color: rgb(255 255 255 / .75); font: var(--t-footnote); }
	.libword { color: #fff; font-weight: 600; }
	.bar { width: 120px; height: 4px; border-radius: 99px; background: rgb(255 255 255 / .15); overflow: hidden; }
	.bar i { display: block; height: 100%; background: #fff; border-radius: inherit; }
	@media (max-width: 600px) { .stats div { padding: 4px 12px; } .stats dd { font-size: 18px; } }
	.facts { margin: 0; color: rgb(255 255 255 / .7); font: var(--t-subhead); }
	.round { display: flex; align-items: center; gap: 18px; margin-top: 8px; }
	.rb { display: grid; place-items: center; width: 44px; height: 44px; border-radius: 50%; border: 1px solid rgb(255 255 255 / .2); background: rgb(255 255 255 / .12); color: #fff; backdrop-filter: blur(14px); -webkit-backdrop-filter: blur(14px); cursor: pointer; transition: transform 160ms var(--ease-out), background 160ms; }
	.rb:hover { background: rgb(255 255 255 / .22); transform: scale(1.05); }
	.rb:active { transform: scale(.96); }
	.rb.on { background: #fff; color: var(--hero-ink); }
	.rb.main { width: 60px; height: 60px; background: #fff; color: var(--hero-ink); border: 0; box-shadow: 0 10px 30px rgb(0 0 0 / .4); }
	.rb.main:disabled { background: rgb(255 255 255 / .3); color: #fff; cursor: default; transform: none; }
	.rb:focus-visible { outline: 2px solid #fff; outline-offset: 3px; }
	.cta-word { margin: -4px 0 0; font: var(--t-footnote); color: rgb(255 255 255 / .75); }
	.have { display: inline-flex; align-items: center; gap: 6px; margin: 0; padding: 5px 12px; border-radius: 999px; background: rgb(255 255 255 / .12); font: var(--t-footnote); font-weight: 600; font-variant-numeric: tabular-nums; }
	h2 { margin: 0; font: var(--t-title2); }
	.lead { display: grid; grid-template-columns: minmax(240px, 300px) minmax(0, 1fr); gap: clamp(20px, 4vw, 56px); align-items: start; }
	.latest, .top { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.latest-card { display: grid; grid-template-columns: 160px minmax(0, 1fr); gap: 16px; align-items: center; color: var(--text); border-radius: var(--r-card); }
	.cov { position: relative; display: block; flex: none; overflow: hidden; border-radius: var(--r-card); background: var(--surface-2); }
	.cov img { width: 100%; height: 100%; }
	.latest-card img { width: 160px; height: 160px; border-radius: var(--r-card); object-fit: cover; box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out); background: var(--surface-2); }
	.latest-card:hover img { transform: translateY(var(--lift)); }
	.l-meta { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.l-date { color: var(--text-muted); font: var(--t-footnote); text-transform: uppercase; letter-spacing: .04em; }
	.l-name { font: var(--t-headline); overflow-wrap: anywhere; }
	.l-kind { color: var(--text-muted); font: var(--t-subhead); }
	.pill { align-self: flex-start; margin-top: 4px; padding: 3px 10px; border-radius: 999px; background: var(--surface-2); font: var(--t-footnote); font-weight: 600; }
	.pill.available { color: var(--ok); }
	.pill.downloading { color: var(--accent); }
	/* Top songs: three columns of three rows, hairlines between rows. */
	.songs { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); grid-auto-flow: column; grid-template-rows: repeat(3, auto); gap: 0 24px; margin: 0; padding: 0; list-style: none; }
	.songs li { display: grid; grid-template-columns: 40px minmax(0, 1fr) 32px; align-items: center; gap: 12px; min-height: 60px; border-bottom: 1px solid var(--line); }
	.songs img, .songs .cov { width: 40px; height: 40px; border-radius: 6px; object-fit: cover; background: var(--surface-2); }
	.s-words, .soon-words { display: flex; flex-direction: column; min-width: 0; }
	.s-name { font: var(--t-callout); font-weight: 600; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.s-album { color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	a.s-album:hover { text-decoration: underline; }
	.s-done { display: grid; place-items: center; color: var(--text-muted); }
	.e { display: inline-grid; place-items: center; width: 15px; height: 15px; margin-right: 6px; border-radius: 3px; background: var(--text-muted); color: var(--bg); font: 700 10px/1 var(--font-ui); vertical-align: 1px; }
	.block { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.row-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
	.confirm { display: inline-flex; align-items: center; gap: 10px; flex-wrap: wrap; font: var(--t-subhead); }
	.chips { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
	.sep { width: 1px; height: 20px; background: var(--line); margin: 0 4px; }
	.essential, .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--music-item), 1fr)); gap: 24px 16px; margin: 0; padding: 0; list-style: none; --poster-ratio: 1; }
	.quiet { margin: 0; color: var(--text-muted); font: var(--t-subhead); }
	.soon { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr)); gap: 12px; margin: 0; padding: 0; list-style: none; }
	.soon li { display: grid; grid-template-columns: 72px minmax(0, 1fr) auto; gap: 14px; align-items: center; padding: 10px; border-radius: var(--r-card); background: var(--surface-1, var(--surface-2)); }
	.soon img { width: 72px; height: 72px; border-radius: 8px; object-fit: cover; background: var(--surface-2); }
	.circles { display: flex; gap: 18px; overflow-x: auto; margin: 0; padding: 4px 2px 8px; list-style: none; scrollbar-width: none; }
	.about { scroll-margin-top: 80px; }
	.about-in { display: grid; grid-template-columns: minmax(0, 1fr) minmax(200px, 280px); gap: clamp(20px, 4vw, 56px); align-items: start; }
	.bio { margin: 0; max-width: 72ch; font: var(--t-body); line-height: 1.6; display: -webkit-box; -webkit-line-clamp: 5; line-clamp: 5; -webkit-box-orient: vertical; overflow: hidden; }
	.bio.open { display: block; }
	.bio-foot { display: flex; align-items: baseline; gap: 14px; flex-wrap: wrap; margin-top: 8px; }
	.more { padding: 0; border: 0; background: none; color: var(--text); font: var(--t-subhead); font-weight: 600; cursor: pointer; text-decoration: underline; }
	.kv { margin: 0; display: flex; flex-direction: column; gap: 12px; }
	.kv dt { color: var(--text-muted); font: var(--t-footnote); text-transform: uppercase; letter-spacing: .05em; }
	.kv dd { margin: 2px 0 0; font: var(--t-callout); }
	.bio-sk { display: flex; flex-direction: column; gap: 8px; max-width: 72ch; }
	.bio-sk .sk-line { height: 14px; width: 100%; }
	.bio-sk .short { width: 55%; }
	.sk-latest .sq { width: 160px; height: 160px; border-radius: var(--r-card); }
	.sk-lines { display: flex; flex-direction: column; gap: 8px; }
	.sk-lines .sk-line { height: 13px; width: 80%; }
	.sk-lines .short { width: 45%; }
	.sk-songs { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 0 24px; }
	.sq-rail { --poster-ratio: 1; }
	@media (max-width: 1280px) { .songs { grid-template-columns: repeat(2, minmax(0, 1fr)); grid-auto-flow: row; grid-template-rows: none; } .sk-songs { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
	@media (max-width: 1000px) { .lead { grid-template-columns: minmax(0, 1fr); } }
	@media (max-width: 767px) {
		.artist { --music-item: calc((100cqi - var(--shelf-gap, 12px)) / 2.3); gap: 32px; }
		.hero { min-height: 440px; }
		.photo { width: 136px; height: 136px; }
		.songs, .sk-songs { grid-template-columns: minmax(0, 1fr); }
		.grid, .essential { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.about-in { grid-template-columns: minmax(0, 1fr); }
	}
	/* Overview / Library tabs for an artist in the Library: the same underline tabs as a title page. */
	.ar-tabs { display: flex; gap: 6px; margin: 8px 0 4px; border-bottom: 1px solid var(--line); }
	.ar-tabs button { height: 40px; padding: 0 14px; border: 0; border-bottom: 2px solid transparent; background: none; color: var(--text-muted); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	.ar-tabs button.on { color: var(--text); border-bottom-color: var(--text); }
</style>

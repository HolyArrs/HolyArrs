<script module lang="ts">
	/** The Library's own parts when this artist is in the Library (/library/{id}). */
	export type ArtistLibrary = {
		titleId: number;
		staff: boolean;
		progress: string;
		status: string;
		note: string;
		nextRelease?: string;
		/** stamp changes when the page hears the artist changed; the Library tab reads again. */
		stamp?: number;
		onAsk?: () => void;
	};
</script>

<script lang="ts">
	import { dragScroll } from '#lib/ui/drag-scroll.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { onMount, tick } from 'svelte';
	import { beforeNavigate, goto } from '$app/navigation';
	import { page } from '$app/state';
	import { paintWindow, sampleCover, tintOf, unpaintWindow } from '#lib/music/album-tint.ts';
	import { api, message } from '#lib/core/api.ts';
	import { gapWaits, keepShown, reaskWhileIncomplete } from '#lib/music/reask.ts';
	import GapNotice from '#lib/music/GapNotice.svelte';
	import { session } from '#lib/core/session.svelte.ts';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import PersonCircle from '#lib/ui/PersonCircle.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { pictureFor, pictureOf } from '#lib/core/pictures.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import AskArtistDialog from '#lib/AskArtistDialog.svelte';
	import AddArtistDialog from '#lib/AddArtistDialog.svelte';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { refreshInbox } from '#lib/requests.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import { albumYear } from '#lib/music/music.ts';
	import { releaseWords } from '#lib/music/music-trending.ts';
	import { SkRail, SkRowTitle, SkTrackRow } from '#lib/ui/sk/index.ts';
	import { attachHLS, holdStart, screenTarget } from '#lib/core/trailer/hls-player.ts';
	import { HERO_DELAY, setSoundOn, soundOn } from '#lib/core/trailer/trailer.ts';
	import { artistVideo, autoplayHere, loadUISettings, nativeHLS } from '#lib/core/trailer/autoplay.svelte.ts';
	import { heroFullscreen, heroVideoStart, type HeroVideo } from '#lib/core/trailer/hero-video.ts';
	import { aboutSize, musicLoadError, albumHref, appPicture, artistHref, artistKind, countryName, coverFor, kindWord, length, owned, listening, changeWords, fullCount, shortCount, type Catalog, askAgainOnce, coreIfSlow, withCover, shelves as shelvesOf, type ArtistCoreData, type ArtistMoreData, type ArtistPageData, type PageAlbum, type PageArtist, type PageCredit, type PageSong, type Shelf as ShelfKind } from '#lib/music/music-pages.ts';
	import type { Ask, LibraryFolder, LookupArtist, MusicSettings, PickRulesView } from '#lib/core/types.ts';
	import ArtistVideos from '#lib/ArtistVideos.svelte';
	import ArtistLibraryTab from '#lib/music/ArtistLibrary.svelte';
	import type { Snippet } from 'svelte';

	// The one artist page: any MusicBrainz artist, in the Library or not. The
	// Library route passes `library` (its menu, progress and ask) and `below`
	// (albums to keep, how HolyArrs keeps it, history, wishlist). HolyArrs asks
	// for music; it does not play it, so songs lead to their album and to asking for it.
	// The Compact design (design/artist-page-study-20261027.md): a short banner, the round photo over its edge.
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
	const related = $derived(withCover(data?.related));
	let confirmAll = $state(false);
	let askingAll = $state(false);
	let aboutOpen = $state(false);
	let error = $state('');
	let errorNet = $state(false);
	let busy = $state(false);
	// The answer was already kept by the app (fetched ahead or seen within a minute): no loader hold.
	let fromCache = $state(false);
	let leave: AbortController | null = null;
	let lookup = $state<LookupArtist | null>(null);
	let art = $state<{ logo?: string; backgrounds?: string[]; thumb?: string }>({});
	let following = $state<boolean | null>(null);
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
	// Videos that would not play here (twice: as picked, then at the tallest
	// picture): never attached again on this page, so a failing stream drops
	// back to the photo instead of being attached over and over.
	let heroBad = new Set<string>();
	// The key of the video being tried once more at the tallest picture.
	let heroTall = $state('');
	$effect(() => {
		void mbid;
		playedVideos = [];
		heroBad = new Set();
		heroTall = '';
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
	// Full screen is the browser's own, on the hero: the video fit whole (black
	// bars, never stretched or cut) with exit, pause and sound. iPhone Safari
	// lets only the video itself go full screen, in its own player. Esc or the
	// exit button leave it.
	let heroBox: HTMLElement | undefined = $state();
	let heroFull = $state(false);
	function fullScreen() {
		const el = videoEl as (HTMLVideoElement & { webkitEnterFullscreen?: () => void }) | undefined;
		if (!el) return;
		if (document.fullscreenElement) void document.exitFullscreen().catch(() => {});
		else if (heroFullscreen(document.fullscreenEnabled, heroBox, el) === 'page') void heroBox?.requestFullscreen().catch(() => {});
		else el.webkitEnterFullscreen?.();
	}
	$effect(() => {
		if (heroFull && !heroVideo) void document.exitFullscreen().catch(() => {});
	});
	// The video keeps the stage while it is paused or between two videos (a
	// video that ends also fires pause): the banner keeps its size either way.
	let videoIn = $state(false);
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
	/** toLibraryTab is the hero's In Library button: the Library tab, scrolled to. */
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
		// The full answer holds the core: the quick core part is asked only when the full one is slow.
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
		// The server may still be getting the similar artists (incomplete): the row keeps its placeholder and is asked again, then hides if none came.
		const simPath = `/music/artists/${want}?part=similar`;
		void api.get<{ similar: PageArtist[]; incomplete?: boolean }>(simPath).then((r) => {
			if (want !== mbid) return;
			void reaskWhileIncomplete(r, () => api.get<{ similar: PageArtist[]; incomplete?: boolean }>(simPath), (v, pending) => {
				if (want !== mbid || stop.signal.aborted) return;
				similar = pending && !v.similar.length ? null : withCover(v.similar);
			}, stop.signal);
		}).catch(() => want === mbid && (similar = []));
		void api.get<ArtistMoreData>(`/music/artists/${want}?part=more`).then((r) => want === mbid && (more = { ...r, appears_on: withCover(r.appears_on) })).catch(() => want === mbid && (more = { essential: [], appears_on: [] }));
		// A cold artist's art may still be on its way (the server fetches it
		// right then): an answer with no wide photo is asked again over about
		// 20 s, and the photo fades in when it lands.
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
			// The shelves failing shows the error; the core failing alone is quiet (the full answer has the hero too).
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
		heroIn = false;
		heroVideo = null;
		videoIn = false;
		void load();
		return () => leave?.abort();
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
			void artistVideo(want, ctl.signal).then((v) => !ctl.signal.aborted && want === mbid && v?.proxy_url && !heroBad.has(v.key) && (heroVideo = v));
		});
		return () => ctl.abort();
	});

	// Play it on the banner; an error drops back to the photo.
	let attached = $state(false);
	$effect(() => {
		const v = heroVideo;
		const el = videoEl;
		if (!v || !el) return;
		// One more try at the tallest picture: a stream can carry one rendition
		// the browser will not decode (one music video's 720p did, its 1080p played).
		const tall = heroTall === v.key;
		let detach: (() => void) | null = null;
		let stopped = false;
		const fail = () => {
			if (stopped) return;
			if (!tall) {
				heroTall = v.key;
				return;
			}
			heroBad.add(v.key);
			videoIn = false;
			heroVideo = null;
		};
		el.muted = true;
		videoMuted = true;
		el.addEventListener('error', fail);
		// Never the opening: a start the browser dropped (Safari's own HLS) jumps back to it.
		const unhold = holdStart(el, heroVideoStart(v));
		void attachHLS(el, v.proxy_url, nativeHLS(), heroVideoStart(v), tall ? 1080 : screenTarget('cover'), tall ? 1080 : 0)
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
			unhold();
			detach?.();
		};
	});

	// The photo and name hold for the beat; then the video takes over.
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
			.catch((e: unknown) => {
				// A play the page itself cut short (the stream was swapped or
				// detached) is not the video failing.
				if (e instanceof DOMException && e.name === 'AbortError') return;
				if (heroVideo) heroBad.add(heroVideo.key);
				videoIn = false;
				heroVideo = null;
			});
	});

	// Phones get their own sizes.
	let phone = $state(false);
	// How far the banner has scrolled away (0…1), and whether the name has left the screen.
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

	// The artist's photo. One the server has not stored yet is asked for
	// again while the page is open (it fetches it right away, a user is
	// waiting), and swaps in once stored: no reload needed.
	let photoTry = $state(0);
	const PHOTO_RETRY = [20000, 70000, 150000];
	const picture = $derived.by(() => {
		const p = appPicture(hero?.image);
		return p && photoTry ? `${p}${p.includes('?') ? '&' : '?'}fresh=${photoTry}` : p;
	});
	// The banner never stays empty: each wide photo in turn, then a soft crop of the artist's photo.
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
	// The banner's main colour tints the slim bar.
	const tint = $derived(pictureFor('wide', mbid)?.color || pictureFor('artist', mbid)?.color || pictureOf(hero?.image)?.color || '');
	// The whole window takes the artist's colour (as an album does): the stored
	// banner sampled once it shows (truer than the stored colour), else the stored one. The banner
	// fades into it at the bottom, so page and picture read as one.
	let sampledTint = $state<string | null>(null);
	// The colour worked out for an artist is kept (per browser), so a return
	// visit opens in it at once.
	const tintKey = (id: string) => `holyarrs.artist-tint.${id}`;
	$effect(() => {
		const id = mbid;
		let kept: string | null = null;
		try { kept = localStorage.getItem(tintKey(id)); } catch {}
		sampledTint = kept;
	});
	const pageTint = $derived(sampledTint || tint ? tintOf(sampledTint || tint) : null);
	$effect(() => {
		if (pageTint) paintWindow(pageTint);
	});
	let toTinted = false;
	beforeNavigate((nav) => (toTinted = nav.to?.route.id === page.route.id || !!nav.to?.route.id?.startsWith('/music/album')));
	onMount(() => () => {
		if (!toTinted) unpaintWindow();
	});
	// Some music videos carry black bars in the picture itself (an old 4:3
	// video in a wide frame). The frame is read a few times as it plays; when
	// both sides are black the video is scaled up just enough to hide them.
	let crop = $state(1);
	let barTimers: ReturnType<typeof setTimeout>[] = [];
	function sideBars(v: HTMLVideoElement): number {
		try {
			const w = 64, h = 36, c = document.createElement('canvas');
			c.width = w; c.height = h;
			const g = c.getContext('2d', { willReadFrequently: true });
			if (!g || !v.videoWidth) return 0;
			g.drawImage(v, 0, 0, w, h);
			const d = g.getImageData(0, 0, w, h).data;
			const lit = (x: number) => {
				let l = 0;
				for (let y = 0; y < h; y++) { const k = (y * w + x) * 4; l += d[k] + d[k + 1] + d[k + 2]; }
				return l / (h * 3 * 255) > 0.05;
			};
			let left = 0, right = 0;
			while (left < w / 3 && !lit(left)) left++;
			while (right < w / 3 && !lit(w - 1 - right)) right++;
			// Only bars on both sides count (a dark scene on one side is not a bar).
			return left >= 2 && right >= 2 ? Math.min(left, right) / w : 0;
		} catch {
			return 0;
		}
	}
	function findBars() {
		barTimers.forEach(clearTimeout);
		const v = videoEl;
		if (!v) return;
		let least = Infinity;
		barTimers = [1500, 4000, 8000].map((ms) => setTimeout(() => {
			// The smallest bar seen wins, so a dark moment never over-crops.
			least = Math.min(least, sideBars(v));
			crop = least > 0 && least < 0.3 ? Math.min(1 / (1 - 2 * least), 1.5) : 1;
		}, ms));
	}
	$effect(() => {
		void heroVideo;
		crop = 1;
		return () => barTimers.forEach(clearTimeout);
	});
	// The colour is read from the artist's round photo (one picture per
	// artist, so every visitor gets the same colour); the banner only when
	// there is no photo.
	$effect(() => {
		const src = picture, id = mbid;
		if (!src || sampledTint) return;
		const img = new Image();
		img.onload = () => {
			if (id !== mbid || sampledTint) return;
			const c = sampleCover(img);
			if (c) { sampledTint = c; try { localStorage.setItem(tintKey(id), c); } catch {} }
		};
		img.src = src;
	});
	function heroLoaded(e: Event) {
		heroIn = true;
		if (sampledTint || picture) return;
		sampledTint = sampleCover(e.currentTarget as HTMLImageElement);
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
	const smallCount = $derived(catalog ? catalog.eps + catalog.singles : 0);
	const listenWords = $derived(listen ? `${shortCount(listen.n)} ${listen.unit}` : '');
	// One sentence or two of the bio for the About card; the full text is in its sheet.
	const bioLine = $derived(hero?.bio?.extract ? hero.bio.extract.split(/(?<=[.!?])\s+/).slice(0, 2).join(' ') : '');
	// About stills: the wide photo and the square photo take turns with a slow zoom, like the people page.
	const aboutStills = $derived([wideFace && !wideFailed ? wideFace : '', picture && !photoFailed ? picture : ''].filter((x, i, a): x is string => !!x && a.indexOf(x) === i));
	let still = $state(0);
	// The About photos: the wide stills, else the one portrait.
	const shots = $derived(aboutStills.length ? aboutStills : picture ? [picture] : []);
	$effect(() => {
		const n = aboutStills.length;
		if (n < 2 || matchMedia('(prefers-reduced-motion: reduce)').matches) return;
		const t = setInterval(() => (still = (still + 1) % n), 7000);
		return () => clearInterval(t);
	});
	const genres = $derived((data?.genres ?? []).slice(0, 4).join(', '));
	// "0 of 113 tracks · 48 missing" said once, even when progress and status repeat a part.
	const libLine = $derived(lib?.staff ? [...new Set([lib.progress, lib.status].filter(Boolean).flatMap((x) => String(x).split(' · ')))].join(' · ') : '');
	const haveShare = $derived(data?.total ? Math.round((100 * (data.have ?? 0)) / data.total) : 0);
	// The discography: one row per kind of release.
	const ROW_WORDS: Record<ShelfKind, string> = { albums: 'Albums', singles: 'Singles & EPs', compilations: 'Compilations', live: 'Live', other: 'Soundtracks, remixes and more' };
	const rows = $derived(shelvesOf(released).map((s) => ({ ...s, title: ROW_WORDS[s.shelf] ?? s.title })));
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
	// Similar artists: the 200 px artist circle; smaller on phones so more than one shows.
	const circle = $derived(phone ? 136 : 200);

	/** The person's ask, or what they may do: ask (members) or add (managers). */
	function askAlbum(a?: PageAlbum) {
		if (!lookup) return void toasts.push('Still reading artist', 'fail', { body: 'Try again in a moment' });
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
		toasts.push(r.joined ? `${r.ask.name} already requested` : `${r.ask.name} requested`, 'ok', { body: r.joined ? "You're on the request too" : 'Waiting for approval' });
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
			toasts.push("Couldn't send request", 'fail', { body: message(e) });
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
			toasts.push("Couldn't update follow", 'fail', { body: message(e) });
		}
	}

	async function followAlbum(a: PageAlbum) {
		const on = !follows.get(a.mbid);
		follows = new Map([...follows, [a.mbid, on]]);
		try {
			if (on) await api.put(`/metadata/album/${a.mbid}/follow`, { name: a.name, artist: hero?.name ?? '', release_date: a.release_date ?? '' });
			else await api.del(`/metadata/album/${a.mbid}/follow`);
			// The browser keeps the page answer a minute (max-age=60): fetch it
			// afresh so a reload or a return visit shows the bell as it is now.
			await fetch(to(`/api/v1/music/artists/${mbid}?similar=0`), { cache: 'reload', headers: { Accept: 'application/json' }, credentials: 'same-origin' }).catch(() => {});
		} catch (e) {
			follows = new Map([...follows, [a.mbid, !on]]);
			toasts.push("Couldn't update follow", 'fail', { body: message(e) });
		}
	}

	const albumAsked = (a: PageAlbum) => asked.has(a.mbid) || (!!lookup?.ask && !lookup.ask.albums?.length);
	const cta = $derived(titleId ? 'Open in Library' : lookup?.ask ? 'Asked' : manage ? 'Add artist' : 'Ask for discography');

	async function copyLink(href: string) {
		try {
			await navigator.clipboard.writeText(new URL(href, location.href).href);
			toasts.push('Link copied', 'ok');
		} catch {
			toasts.push("Couldn't copy link", 'fail', { body: 'Select it instead' });
		}
	}
	/** The "…" menu of a top song: open its album, ask for it (add, for managers), copy its link. */
	function songItems(s: PageSong): MenuItem[] {
		if (!s.album_mbid) return [];
		const href = albumHref(s.album_mbid, mbid);
		const al = data?.albums.find((x) => x.mbid === s.album_mbid);
		const items: MenuItem[] = [{ label: 'Open album', icon: 'music', href }];
		if (titleId) items.push({ label: 'Open in Library', icon: 'library', onselect: () => askAlbum(al) });
		else if (al && manage) items.push({ label: `Add ${hero?.name ?? 'artist'}`, icon: 'plus', onselect: () => askAlbum(al) });
		else if (al && mayAsk) items.push(albumAsked(al) ? { label: 'Asked', icon: 'check', disabled: true } : { label: 'Ask for the album', icon: 'plus', onselect: () => askAlbum(al) });
		items.push({ label: 'Copy link', icon: 'copy', onselect: () => void copyLink(href) });
		return items;
	}
	const yearOf = (s: PageSong) => {
		const a = s.album_mbid ? data?.albums.find((x) => x.mbid === s.album_mbid) : undefined;
		return a ? albumYear(a.release_date) : '';
	};
</script>

<svelte:head><title>{hero?.name ?? 'Artist'} · HolyArrs</title></svelte:head>

{#snippet albumCard(a: PageAlbum, eager = false, row = false)}
	{@const c = coverFor(appPicture(a.image), 200)}
	<PosterCard title={a.name} href={albumHref(a.mbid, mbid)} image={c.src} {eager} kind="artist" shape="square" music needsArt={row} actionOnHover
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
	<PosterCard title={a.name} href={albumHref(a.mbid, a.artist_mbid || mbid)} image={c.src} kind="artist" shape="square" music needsArt
		subtitle={a.artist_name || undefined} badges={a.explicit ? ['E'] : undefined} />
{/snippet}

{#snippet mainAct()}
	<!-- The main button sits where a player puts play: In Library, Ask or Add. -->
	{#if lib?.staff}
		<ActionButton icon="library" label="In Library: open the Library tab" caption="In Library" primary onclick={toLibraryTab} data-testid="hero-library" />
	{:else if lib?.onAsk}
		<ActionButton icon="plus" label="Ask for albums" primary onclick={lib.onAsk} data-testid="ask-albums" />
	{:else if lib}
		<!-- In the Library, nothing to ask: no main button. -->
	{:else if titleId}
		<!-- Everyone goes to the one artist page in the Library, like a title page. -->
		<ActionButton icon="library" label="Open in Library" primary href={to(`/library/${titleId}`)} />

	{:else if manage || mayAsk}
		<ActionButton icon={lookup?.ask ? 'check' : 'plus'} label={cta} primary disabled={!!lookup?.ask || !lookup} onclick={() => askAlbum()} data-testid="artist-ask" />
	{/if}
{/snippet}

{#snippet aboutAct()}
	<ActionButton icon="info" label="About {hero?.name ?? 'this artist'}" caption="About" aria-haspopup="dialog" onclick={() => (aboutOpen = true)} data-testid="artist-about-open" />
{/snippet}

{#snippet followAct()}
	{#if titleId && following !== null}
		<ActionButton icon={following ? 'bellOn' : 'bell'} label={following ? 'Following' : 'Follow'} pressed={!!following} onclick={toggleFollow} />
	{/if}
{/snippet}

{#snippet acts()}
	<!-- The app's own hero buttons (ActionButton): the main one first, where a player puts play. -->
	<div class="acts">
		{@render mainAct()}{@render aboutAct()}
		{@render followAct()}
	</div>
{/snippet}

{#snippet photo(size: number)}
	{#if picture && !photoFailed}<img class="photo" src={picture} alt="" width={size} height={size} fetchpriority="high" onerror={() => (photoFailed = true)} />{:else if wideFace && !wideFailed}<img class="photo crop" src={wideFace} alt="" width={size} height={size} onerror={() => (wideFailed = true)} />{:else}<div class="photo none" aria-hidden="true" data-testid="artist-photo-placeholder"><ArtPlaceholder name={hero?.name ?? ''} shape="circle" /></div>{/if}
{/snippet}

{#snippet stageArt()}
	{#if heroSrc}
		{#key heroSrc}
			<img class="art" class:soft={heroSoft} class:in={heroIn} class:under={videoIn} src={heroSrc} alt="" decoding="async" fetchpriority="high" onload={heroLoaded} onerror={() => { heroIn = false; heroTry++; }} />
		{/key}
	{/if}
{/snippet}

{#snippet bannerHero()}
	<!-- The banner: the wide photo (or the music video) across the content width; it zooms out and darkens as it scrolls away. -->
	<header class="hero" class:paused={videoIn && videoPaused && !heroFull} class:vid={videoIn} class:bare={!heroSrc && !heroVideo} class:full={heroFull} style:--gone={gone.toFixed(3)} bind:this={heroBox} data-hero data-testid="artist-hero">
		<div class="stage" aria-hidden="true">
			{@render stageArt()}
			{#if heroVideo}
				<video class="clip" class:in={videoIn || heroFull} style:--crop={heroFull ? 1 : crop} bind:this={videoEl} muted playsinline disablepictureinpicture preload="metadata" onplaying={() => { videoIn = true; videoPaused = false; findBars(); }} onpause={() => (videoPaused = true)} onended={() => void nextVideo()} onplay={() => (videoPaused = false)} onvolumechange={() => videoEl && (videoMuted = videoEl.muted)} data-testid="artist-hero-video"></video>
			{/if}
		</div>
		<div class="shade" aria-hidden="true"></div>
		<div class="dim" aria-hidden="true"></div>
		{#if heroVideo && videoIn}
			<div class="vctl" data-testid="artist-hero-video-controls">
				<IconButton icon={videoPaused ? 'play' : 'pause'} label={videoPaused ? 'Play the video' : 'Pause the video'} variant="glass" size="md" tipSide="below" onclick={togglePause} />
				<IconButton icon={videoMuted ? 'soundOff' : 'sound'} label={videoMuted ? 'Turn sound on' : 'Turn sound off'} variant="glass" size="md" tipSide="below" onclick={toggleMute} />
				{#if !heroFull}<IconButton icon="expand" label="Play full screen" variant="glass" size="md" tipSide="below" onclick={fullScreen} />{/if}
			</div>
			{#if heroFull}<button type="button" class="vexit" onclick={fullScreen} aria-label="Exit full screen" data-testid="artist-hero-video-exit" {@attach (n) => n.focus()}><Icon name="x" size={20} /></button>{/if}
		{/if}
	</header>
{/snippet}


{#snippet compactLine()}
	<!-- Under the short banner: the round photo over its edge, the name beside it, the buttons at the end of the same line. -->
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
		<div class="slim" class:on={slimOn} style:--tint={tint || undefined} data-testid="artist-slim-bar" aria-hidden={!slimOn} inert={!slimOn}>
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
					<span class="libword">Your Library</span>
					<span class="bar" aria-hidden="true"><i style="width: {haveShare}%"></i></span>
					<span>{libLine}</span>
				</div>
			{/if}
			{#if lib?.nextRelease}<p class="quiet" data-testid="artist-next">Next album: {lib.nextRelease}</p>{/if}
		</div>
	{/if}
{/snippet}

{#snippet topSongs()}
	{#if data?.top.length}
		<section class="block top" data-testid="top-songs">
			<h2>Top songs</h2>
			<ol class="songs">
				{#each songs as s, i (s.title + i)}
					{@const year = yearOf(s)}
					<li class="song">
						{#if s.album_mbid}
							<a class="num" href={albumHref(s.album_mbid, mbid)} aria-label="Open {s.album}"><span class="n">{i + 1}</span><span class="go" aria-hidden="true"><Icon name="arrow" size={16} /></span></a>
						{:else}
							<span class="num"><span class="n">{i + 1}</span></span>
						{/if}
						<span class="cov">{#if s.image && !badArt.has('top:' + s.title)}<img src={coverFor(appPicture(s.image), 48).src} alt="" width="48" height="48" loading={i < 5 ? 'eager' : 'lazy'} decoding="async" onerror={() => (badArt = new Set([...badArt, 'top:' + s.title]))} />{:else}<ArtPlaceholder name={s.title} shape="square" />{/if}</span>
						<span class="s-words">
							<span class="s-name">{#if s.explicit}<span class="e" aria-label="Explicit">E</span>{/if}{s.title}</span>
							<span class="s-album">{[s.album, year].filter(Boolean).join(' · ')}</span>
						</span>
						<span class="s-len">{length(s.length_ms)}</span>
						{#if s.album_mbid}<span class="s-more"><Menu label="More for {s.title}" items={songItems(s)} width={240} /></span>{:else}<span></span>{/if}
					</li>
				{/each}
			</ol>
		</section>
	{/if}
{/snippet}




{#snippet kindRows()}
	<!-- The discography: one row per kind of release. -->
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
		<!-- MusicBrainz lists nothing for this artist: what a store knows, so the page is not empty. No ask control (a request needs a MusicBrainz id). -->
		<section class="block" data-testid="store-discography">
			<h2>Discography</h2>
			<ul class="grid">
				{#each data.store_albums as a, i (a.title + i)}
					<li>
						<PosterCard title={a.title} image={a.image ? coverFor(appPicture(a.image), 200).src : null} eager={i < 6} kind="artist" shape="square" music
							subtitle={[albumYear(a.release_date), a.type === 'album' ? '' : ({ single: 'Single', ep: 'EP', compilation: 'Compilation' } as Record<string, string>)[a.type] ?? '', a.track_count ? `${a.track_count} ${a.track_count === 1 ? 'track' : 'tracks'}` : ''].filter(Boolean).join(' · ') || undefined}
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
			<h2>Essential albums</h2>
			<ul class="grid">{#each essential as a (a.mbid)}<li>{@render albumCard(a, false, true)}</li>{/each}</ul>
		</section>
	{/if}
{/snippet}

{#snippet soonRow()}
	{#if soon.length}
		<section class="block">
			<h2>Coming soon</h2>
			<ul class="soon">
				{#each soon as a (a.mbid)}
					<li>
						<!-- The cover repeats the name link beside it, so screen readers and Tab skip it. -->
						<a href={albumHref(a.mbid, mbid)} tabindex="-1" aria-hidden="true"><span class="cov small">{#if a.image && !badArt.has(a.mbid)}<img src={coverFor(appPicture(a.image), 72).src} alt="" width="72" height="72" loading="lazy" decoding="async" onerror={() => (badArt = new Set([...badArt, a.mbid]))} />{:else}<ArtPlaceholder name={a.name} shape="square" />{/if}</span></a>
						<span class="soon-words"><a class="s-name" href={albumHref(a.mbid, mbid)}>{a.name}</a><span class="s-album">{kindWord(a)} · {releaseWords(a.release_date ?? '')}</span></span>
						{#if !data?.kid}<!-- The bell sits at the row's right edge: its tip lines up with it, so a long name never pushes the page sideways. --><IconButton icon={follows.get(a.mbid) ? 'bellOn' : 'bell'} label={follows.get(a.mbid) ? `Stop following ${a.name}` : `Tell me when ${a.name} is out`} tipSide="below-end" aria-pressed={!!follows.get(a.mbid)} onclick={() => followAlbum(a)} />{/if}
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
		<Shelf title="Appears on" item="var(--music-item)" items={more.appears_on} key={(a) => a.mbid} placeholder="square" lines={2}>
			{#snippet card(a: PageCredit)}{@render creditCard(a)}{/snippet}
		</Shelf>
	{/if}
{/snippet}

{#snippet relatedRow()}
	{#if related.length}
		<section class="block">
			<h2>Bands and collaborators</h2>
			<ul class="circles" use:dragScroll>{#each related as p (p.mbid)}<li><PersonCircle name={p.name} role={p.note} image={appPicture(p.image)} href={artistHref(p.mbid)} size={circle} needsArt /></li>{/each}</ul>
		</section>
	{/if}
{/snippet}

{#snippet statsBand()}
	<!-- Where concerts would be: listening, and the ask-for-missing button. -->
	{#if data && (listen || (missing?.count && mayAsk && !manage && !titleId && !allAsked))}
		{@const up = (listen?.today ?? 0) > 0}
		<section class="stats" data-testid="artist-stats" aria-label="Listening and releases">
			{#if listen}
				<p class="st1">
					<span><b>{shortCount(listen.n)}</b> {listen.unit}</span>
					{#if listen.rank}<span><b>#{listen.rank}</b> in the world</span>{/if}
					{#if listen.today}<span class="chg" class:up class:down={!up}>{changeWords(listen.today)}</span>{/if}
				</p>
			{/if}
			{#if missing?.count && mayAsk && !manage && !titleId && !allAsked}
				<div class="st-ask">
					{#if confirmAll}
						<span class="confirm" role="group" aria-label="Confirm asking for all missing">
							<span>Ask for {missing.count} {missing.count === 1 ? 'release' : 'releases'}, {aboutSize(missing.size_bytes)}?</span>
							<Button variant="primary" onclick={askAllMissing} disabled={askingAll}>Ask</Button>
							<Button onclick={() => (confirmAll = false)}>Cancel</Button>
						</span>
					{:else}
						<Button icon="plus" onclick={() => (confirmAll = true)} data-testid="ask-all-missing">Ask for all missing ({missing.count})</Button>
					{/if}
				</div>
			{/if}
		</section>
	{/if}
{/snippet}

{#snippet aboutCard()}
	{@const img = wideFace && !wideFailed ? wideFace : picture}
	<section class="block about">
		<h2>About</h2>
		<button type="button" class="acard" aria-haspopup="dialog" aria-label="About {hero?.name}: read more" onclick={() => (aboutOpen = true)} onpointermove={(e) => { const b = e.currentTarget.getBoundingClientRect(); e.currentTarget.style.setProperty('--mx', `${((e.clientX - b.left) / b.width) * 100}%`); e.currentTarget.style.setProperty('--my', `${((e.clientY - b.top) / b.height) * 100}%`); }} data-testid="artist-about">
			{#if aboutStills.length}{#each aboutStills as src, i (src)}<img class="ab-frame" class:on={i === still % aboutStills.length} {src} alt="" loading="lazy" decoding="async" />{/each}<span class="a-glow" aria-hidden="true"></span>{:else if img}<img class="ab-frame on" src={img} alt="" loading="lazy" decoding="async" />{:else}<span class="a-none" aria-hidden="true"><ArtPlaceholder name={hero?.name ?? ''} shape="wide" /></span>{/if}
			{#if listen?.rank}<span class="disc" aria-hidden="true"><b>#{listen.rank}</b><small>in the world</small></span>{/if}
			<span class="a-foot">
				{#if listenWords || catalog?.span || genres}<span class="a-lis">{[listenWords, catalog?.span, genres.split(', ').slice(0, 2).join(', ')].filter(Boolean).join(' · ')}</span>{/if}
				{#if bioLine}<span class="a-bio">{bioLine}</span>{:else}<span class="a-bio">{hero?.name}</span>{/if}
				<span class="a-more">More about {hero?.name}</span>
			</span>
		</button>
	</section>
{/snippet}

{#snippet similarRow()}
	{#if similar === null}
		<SkRail kind="person" item="{circle + 16}px" titleWidth="150px" />
	{:else if similar.length}
		<section class="block" data-testid="similar-artists">
			<h2>Similar artists</h2>
			<ul class="circles" use:dragScroll>{#each similar as p (p.mbid)}<li><PersonCircle name={p.name} image={appPicture(p.image)} href={artistHref(p.mbid)} size={circle} needsArt /></li>{/each}</ul>
		</section>
	{/if}
{/snippet}


{#snippet loading()}
	{#if !error}
		<section class="block" aria-busy="true" aria-label="Loading albums and songs">
			<SkRowTitle width="120px" /><div class="sk-songs">{#each Array.from({ length: 10 }, (_, i) => i) as i (i)}<SkTrackRow height={64} />{/each}</div>
		</section>
		<div class="sq-rail"><SkRail item="var(--music-item)" titleWidth="160px" /></div>
	{:else}
		<LoadError hasData={true} message={error} network={errorNet} onretry={load} {busy} />
	{/if}
{/snippet}

{#snippet sections()}
	<!-- The page under the hero. -->
	{#if !data}
		{@render loading()}
	{:else}
		{@render topSongs()}
		{@render essentialRow()}
		{@render kindRows()}
		{@render storeDisco()}
		{@render soonRow()}
		{@render videos()}
		{@render statsBand()}
		{@render appearsOn()}
		{@render relatedRow()}
		{@render aboutCard()}
	{/if}
	{@render similarRow()}
{/snippet}

<svelte:document onfullscreenchange={() => (heroFull = !!heroBox && document.fullscreenElement === heroBox)} />

<div class="artist" class:tinted={!!pageTint} style={pageTint ? `--ar-bg:${pageTint.bg};--ar-soft:${pageTint.soft}` : undefined}>
	{#if error && !hero}<LoadError hasData={false} message={error} network={errorNet} onretry={load} {busy} />{/if}
	<PageLoader loading={!hero && !error} cached={fromCache} label="Loading this artist" onretry={load}>
		{#if hero}
			{@render bannerHero()}
			{@render slimBar()}
			{@render compactLine()}
			{@render libBits()}

			{#if titleId && manage}
				<!-- An artist in the Library: Overview (this page) and Library (releases, tracks, settings). -->
				<div class="ar-tabs" role="tablist" data-testid="artist-tabs">
					<button type="button" role="tab" class:on={artistTab === 'overview'} aria-selected={artistTab === 'overview'} onclick={() => (artistTab = 'overview')}>Overview</button>
					<button type="button" role="tab" class:on={artistTab === 'library'} aria-selected={artistTab === 'library'} onclick={() => (artistTab = 'library')}>Library</button>
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

{#if hero}
	<Sheet bind:open={aboutOpen} title="About {hero.name}" size="lg">
		<div class="sheet" data-testid="artist-about-sheet">
				{#if shots.length}
					<figure class="col-photo">
						{#each shots as src, i (src)}<img class:on={i === still % shots.length} {src} alt="" decoding="async" />{/each}
						{#if shots.length > 1}
							<button type="button" class="col-nav prev" aria-label="Previous photo" onclick={() => (still = (still - 1 + shots.length) % shots.length)}><Icon name="chevronLeft" size={20} /></button>
							<button type="button" class="col-nav next" aria-label="Next photo" onclick={() => (still = (still + 1) % shots.length)}><Icon name="chevronRight" size={20} /></button>
						{/if}
					</figure>
				{/if}
				<div class="col">
					<aside class="col-side">
						{#if listen?.rank}<span class="col-disc"><b>#{listen.rank}</b><small>in the world</small></span>{/if}
						{#if listen}<p class="col-big"><b>{fullCount(listen.n)}</b><span>{listen.unit}</span></p>{/if}
						{#if listen?.today}<p class="col-big"><b class="chg" class:up={listen.today > 0} class:down={listen.today < 0}>{changeWords(listen.today, '')}</b><span>today</span></p>{/if}
						<dl class="col-list">
							{#if catalog?.albums}<div><dt>{catalog.albums === 1 ? 'Album' : 'Albums'}</dt><dd>{catalog.albums}</dd></div>{/if}
							{#if smallCount}<div><dt>Singles & EPs</dt><dd>{smallCount}</dd></div>{/if}
							{#if hero.country}<div><dt>From</dt><dd>{countryName(hero.country)}</dd></div>{/if}
							{#if catalog?.since}<div><dt>Active</dt><dd>{catalog.since} – {hero.status === 'ended' ? (catalog.span?.split('–')[1] ?? '') : 'now'}</dd></div>{/if}
							{#if hero.type}<div><dt>Kind</dt><dd>{artistKind(hero.type)}</dd></div>{/if}
						</dl>
					</aside>
					<div class="col-main">
						{#if hero.bio?.extract}<div class="col-bio">{#each hero.bio.extract.split(/\n+/).filter(Boolean) as t, i (i)}<p>{t}</p>{/each}</div>{/if}
						<p class="col-by">{#if picture && !photoFailed}<img src={picture} alt="" />{/if}{hero.name}</p>
						{#if genres}<div class="chips">{#each genres.split(', ') as g (g)}<span>{g}</span>{/each}</div>{/if}
					</div>
				</div>
		</div>
	</Sheet>
{/if}

{#if manage}
	<AddArtistDialog bind:this={addDialog} {folders} {rules} {defaults} onadded={(_a, t) => { toasts.push(`${t.name} added`, 'ok', { body: 'Filling in albums and tracks' }); void load(); }} />
{:else if mayAsk}
	<AskArtistDialog bind:this={askDialog} onasked={onAsked} />
{/if}

<style>
	/* Windows opened on a tinted artist page get a solid tinted background; the page's see-through surfaces stay inside them. */
	.artist.tinted :global(.ui-sheet) { background: radial-gradient(120% 70% at 50% 0%, color-mix(in srgb, var(--ar-bg, #2a2c33) 85%, #fff 6%), color-mix(in srgb, var(--ar-bg, #1b1c20) 55%, #050506) 70%); }
	.artist.tinted { --ok: #30d158; --fail: #ff6961; --fail-text: #ff6961; --accent-text: #fff; --text: #fff; --bg: var(--ar-bg); --text-muted: var(--ar-soft); --surface-1: rgb(255 255 255 / 8%); --surface-2: rgb(255 255 255 / 12%); --border: rgb(255 255 255 / 14%); --line: rgb(255 255 255 / 14%); color: #fff; color-scheme: dark; }
	.artist.tinted .hero { --hero-ink: var(--ar-bg); }
	.artist { --music-item: max(168px, calc((100vw - 96px) / 7.2)); --gap: 40px; display: flex; flex-direction: column; gap: var(--gap); padding-bottom: 48px; min-width: 0; }
	h1 { margin: 0; font-family: var(--font-display); font-weight: 800; line-height: 1; letter-spacing: -.03em; text-wrap: balance; overflow-wrap: anywhere; }
	h2 { margin: 0; font: var(--t-title2); }
	.quiet { margin: 0; color: var(--text-muted); font: var(--t-subhead); }

	/* The short banner (dark in both themes): the wide photo or the music video. It zooms out from 1.1 and darkens as it leaves. */
	.hero { --hero-ink: #07080a; position: relative; isolation: isolate; overflow: hidden; display: flex; align-items: flex-end; height: clamp(280px, 44vh, 520px); transition: background-color var(--dur-slow) var(--ease-out); -webkit-mask-image: linear-gradient(to bottom, #000 55%, rgb(0 0 0 / .82) 68%, rgb(0 0 0 / .5) 80%, rgb(0 0 0 / .18) 91%, transparent 100%); mask-image: linear-gradient(to bottom, #000 55%, rgb(0 0 0 / .82) 68%, rgb(0 0 0 / .5) 80%, rgb(0 0 0 / .18) 91%, transparent 100%); margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) 0; background: var(--hero-ink); color: #fff; }
	.stage { position: absolute; inset: 0; z-index: -2; scale: calc(1.1 - .1 * var(--gone, 0)); transform-origin: 50% 40%; will-change: scale; }
	.art { width: 100%; height: 100%; object-fit: cover; object-position: center 22%; opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); }
	.art.in { opacity: .82; }
	.art.soft { filter: blur(56px) saturate(150%); transform: scale(1.3); }
	.art.soft.in { opacity: .6; }
	.shade { position: absolute; inset: 0; z-index: -1; pointer-events: none; background: linear-gradient(to top, color-mix(in srgb, var(--hero-ink) 60%, transparent) 0%, color-mix(in srgb, var(--hero-ink) 25%, transparent) 35%, transparent 60%), linear-gradient(to bottom, rgb(7 8 10 / .4), rgb(7 8 10 / 0) 30%); }
	.dim { position: absolute; inset: 0; z-index: -1; background: var(--hero-ink); opacity: calc(var(--gone, 0) * .85); pointer-events: none; }
	.hero.bare { background: linear-gradient(180deg, color-mix(in srgb, var(--hero-ink) 60%, #3a3d44), var(--hero-ink)); }
	@media (prefers-reduced-motion: reduce) { .stage { scale: 1; } }
	/* The video covers the banner at the same size; paused, it hides and the banner shows again. */
	.clip { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); pointer-events: none; }
	.clip.in { opacity: 1; }
	.clip { scale: var(--crop, 1); transition: opacity var(--dur-slow) var(--ease-out), scale 600ms var(--ease-out); }
	.art.under { opacity: 0; transition-duration: 900ms; }
	.hero.paused .clip { opacity: 0; }
	.hero.paused .art.in.under { opacity: .82; }
	.hero.paused .art.soft.in.under { opacity: .6; }
	@media (prefers-reduced-motion: reduce) { .clip, .art { transition: none; } }
	.vctl { position: absolute; top: 16px; right: var(--gutter); z-index: 4; display: flex; gap: 8px; }
	@media (max-width: 767px) { .vctl { top: 12px; right: 16px; } }
	.vexit { display: grid; place-items: center; width: 38px; height: 38px; border-radius: 50%; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 18%); backdrop-filter: blur(18px) saturate(160%); -webkit-backdrop-filter: blur(18px) saturate(160%); color: #fff; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.vexit:hover { background: rgb(255 255 255 / 26%); }
	.vexit:focus-visible { outline: 2px solid #fff; outline-offset: 2px; }
	/* Full screen: the video alone, fit whole on black, with exit, pause and sound. */
	.hero.full { height: auto; background: #000; -webkit-mask-image: none; mask-image: none; }
	.hero.full .stage { scale: 1; }
	.hero.full .art, .hero.full .shade, .hero.full .dim { display: none; }
	.hero.full .clip { object-fit: contain; background: #000; }
	.hero.full .vctl { right: max(24px, env(safe-area-inset-right)); bottom: max(24px, env(safe-area-inset-bottom)); top: auto; gap: 10px; }
	.hero.full .vctl button, .vexit { width: 44px; height: 44px; }
	.vexit { position: absolute; top: max(20px, env(safe-area-inset-top)); right: max(24px, env(safe-area-inset-right)); z-index: 2; }

	/* Under the banner: the round photo over its edge, the name beside it, the buttons at the end of the same line. */
	.cline { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: end; gap: 20px; margin-top: calc(var(--gap) * -1 - 52px); position: relative; z-index: 1; }
	.c-photo { --photo: 136px; display: block; }
	.photo { width: var(--photo); height: var(--photo); border-radius: 50%; object-fit: cover; flex: none; background: #1c1d21; box-shadow: 0 12px 32px rgb(0 0 0 / .35), 0 0 0 4px var(--bg); }
	.photo.crop { object-position: 50% 22%; }
	/* No photo after every source: the shared placeholder, as on every other picture slot. */
	.photo.none { position: relative; overflow: hidden; }
	.c-words { display: flex; flex-direction: column; gap: 4px; min-width: 0; padding-bottom: 8px; }
	.c-words h1 { font-size: clamp(30px, 3vw, 40px); color: var(--text); }
	.c-line { margin: 0; color: var(--text-muted); font: 600 15px/1.3 var(--font-ui); font-variant-numeric: tabular-nums; }
	.cline .acts { padding-bottom: 8px; }

	/* The buttons sit on the page: in light mode they take the page's ink. */
	.acts { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
	:global(html:not([data-theme='dark'])) .cline .acts :global(.ui-act), :global(html:not([data-theme='dark'])) .cline .acts :global(.ui-pop > .trigger) { background: color-mix(in srgb, var(--text) 7%, transparent); border-color: var(--line); color: var(--text); backdrop-filter: none; -webkit-backdrop-filter: none; }
	:global(html:not([data-theme='dark'])) .cline .acts :global(.ui-act.primary), :global(html:not([data-theme='dark'])) .cline .acts :global(.ui-act[aria-pressed='true']) { background: var(--text); border-color: var(--text); color: var(--bg); }
	:global(html:not([data-theme='dark'])) .cline .acts :global(.ui-act:focus-visible) { box-shadow: 0 0 0 2px var(--bg), 0 0 0 4px var(--text); }

	/* The slim bar: sticky under the top bar (in its space when it hides), tinted from the banner, dark in both themes. */
	.slimwrap { position: sticky; top: var(--topbar); z-index: 25; height: 0; margin-bottom: calc(var(--gap) * -1); transition: transform var(--bar-glide); }
	:global(html.bar-hidden) .slimwrap { transform: translateY(calc(-1 * var(--topbar))); }
	.slim { --tint: #3a3d44; position: absolute; top: 0; left: calc(var(--gutter) * -1); right: calc(var(--gutter) * -1); display: flex; align-items: center; gap: 14px; height: 64px; padding: 0 var(--gutter); box-sizing: border-box; background: color-mix(in srgb, var(--tint) 45%, #0b0c0f); color: #fff; box-shadow: 0 8px 24px rgb(0 0 0 / .22); opacity: 0; translate: 0 -8px; pointer-events: none; transition: opacity var(--dur) var(--ease-out), translate var(--dur) var(--ease-out); }
	.slim.on { opacity: 1; translate: 0 0; pointer-events: auto; }
	.slim-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 700 22px/1.1 var(--font-display); letter-spacing: -.01em; }
	@media (prefers-reduced-motion: reduce) { .slim, .slimwrap { transition: none; translate: none; } }

	.libbits { display: flex; flex-direction: column; gap: 8px; margin-top: calc(var(--gap) * -1 + 16px); }
	.libline { display: inline-flex; align-items: center; gap: 10px; flex-wrap: wrap; color: var(--text-muted); font: var(--t-footnote); }
	.libword { color: var(--text); font-weight: 600; }
	.bar { width: 120px; height: 4px; border-radius: 99px; background: var(--line); overflow: hidden; }
	.bar i { display: block; height: 100%; background: var(--text); border-radius: inherit; }

	/* Top songs: two columns of five; each row has its number, cover, words, length and … menu. */
	.block { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
	.songs { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); grid-template-rows: repeat(5, auto); grid-auto-flow: column; gap: 0 24px; margin: 0; padding: 0; list-style: none; }
	.song { display: grid; grid-template-columns: 32px 48px minmax(0, 1fr) auto 36px; align-items: center; gap: 12px; height: 64px; padding: 0 8px; border-radius: 10px; transition: background var(--dur-fast) var(--ease-out), translate var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	.song:hover, .song:focus-within { background: var(--surface-2); translate: 0 -1px; box-shadow: var(--shadow-card); }
	.num { position: relative; display: grid; place-items: center; width: 32px; height: 32px; border-radius: 50%; color: var(--text-muted); font: 600 15px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.num .go { position: absolute; inset: 0; display: grid; place-items: center; opacity: 0; color: var(--text); transition: opacity var(--dur-fast) var(--ease-out); }
	.song:hover a.num .n, .song:focus-within a.num .n { opacity: 0; }
	.song:hover a.num .go, .song:focus-within a.num .go { opacity: 1; }
	a.num:hover { background: var(--surface-3); }
	a.num:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Touch phones: a centred 44px hit area on the song-number link. */
	@media (max-width: 767px) and (pointer: coarse) { a.num::after { content: ''; position: absolute; left: 50%; top: 50%; width: 44px; height: 44px; translate: -50% -50%; } }
	.cov { position: relative; display: block; flex: none; width: 48px; height: 48px; overflow: hidden; border-radius: 6px; background: var(--surface-2); }
	.cov img { width: 100%; height: 100%; object-fit: cover; }
	.cov.small { width: 72px; height: 72px; border-radius: 8px; }
	.s-words, .soon-words { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.s-name { font: var(--t-callout); font-weight: 600; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.s-album { color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.s-len { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.s-more { display: grid; place-items: center; opacity: .6; transition: opacity var(--dur-fast) var(--ease-out); }
	.song:hover .s-more, .song:focus-within .s-more { opacity: 1; }
	@media (hover: none) { .s-more { opacity: 1; } }
	.e { display: inline-grid; place-items: center; width: 15px; height: 15px; margin-right: 6px; border-radius: 3px; background: var(--text-muted); color: var(--bg); font: 700 10px/1 var(--font-ui); vertical-align: 1px; }

	/* Releases: one row per kind; a grid for essential albums and a store's list. */
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--music-item), 1fr)); gap: 24px 16px; margin: 0; padding: 0; list-style: none; --poster-ratio: 1; }
	.rows { display: flex; flex-direction: column; gap: var(--gap); }
	.soon { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr)); gap: 12px; margin: 0; padding: 0; list-style: none; }
	.soon li { display: grid; grid-template-columns: 72px minmax(0, 1fr) auto; gap: 14px; align-items: center; padding: 10px; border-radius: var(--r-card); background: var(--surface-1, var(--surface-2)); }
	.circles { display: flex; gap: 20px; overflow-x: auto; margin: 0; padding: 4px 2px 8px; list-style: none; scrollbar-width: none; }

	/* The stats band, where concerts would be: two centred lines between two hairlines. */
	.stats { display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 28px 16px; border-block: 1px solid var(--line); text-align: center; font-variant-numeric: tabular-nums; }
	.stats p { display: flex; flex-wrap: wrap; justify-content: center; align-items: baseline; margin: 0; }
	.stats p > span { display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
	.stats p > span + span::before { content: '·'; margin: 0 14px; color: var(--text-faint); }
	.st1 { font: 500 20px/1.35 var(--font-ui); color: var(--text-muted); }
	.st1 b { color: var(--text); font-weight: 700; }
	.chg.up { color: var(--ok-text); font-weight: 600; }
	.chg.down { color: var(--fail-text); font-weight: 600; }
	/* On the artist colour, light tones that read on any deep page colour. */
	.artist.tinted .chg.up { color: #9df5b4; }
	.artist.tinted .chg.down { color: #ffb3b3; }
	.st-ask { margin-top: 6px; }
	.confirm { display: inline-flex; align-items: center; gap: 10px; flex-wrap: wrap; justify-content: center; font: var(--t-subhead); }

	/* About: the photo card with its "#N in the world" disc and two lines of the bio; it opens the sheet. */
	.about { scroll-margin-top: 80px; }
	.acard { position: relative; display: block; width: 100%; height: clamp(300px, 32vw, 440px); padding: 0; border: 0; border-radius: var(--r-card); overflow: hidden; background: #1c1d21; color: #fff; text-align: left; cursor: pointer; isolation: isolate; box-shadow: 0 0 0 1px rgb(255 255 255 / 8%), 0 24px 60px rgb(0 0 0 / .4); transition: translate var(--dur-slow) var(--ease-out), box-shadow var(--dur-slow) var(--ease-out); }
	.acard:hover { translate: 0 var(--lift); }
	.acard:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.ab-frame { position: absolute; inset: 0; z-index: -2; width: 100%; height: 100%; object-fit: cover; object-position: center 25%; opacity: 0; scale: 1.02; transition: opacity 1.2s var(--ease-out), scale 8s linear; }
	.ab-frame.on { opacity: 1; scale: 1.12; }
	.acard::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(to top, color-mix(in srgb, var(--ar-bg, #000) 70%, #000) 0%, color-mix(in srgb, var(--ar-bg, #000) 35%, transparent) 45%, transparent 75%), linear-gradient(to right, rgb(0 0 0 / .35), transparent 60%); }
	.a-glow { position: absolute; inset: 0; z-index: -1; pointer-events: none; background: radial-gradient(60% 50% at var(--mx, 70%) var(--my, 20%), rgb(255 255 255 / .1), transparent 70%); opacity: 0; transition: opacity var(--dur) var(--ease-out); }
	.acard:hover .a-glow { opacity: 1; }
	.a-none { position: absolute; inset: 0; z-index: -2; overflow: hidden; }
	.disc { position: absolute; top: 20px; left: 20px; display: grid; place-content: center; justify-items: center; width: 92px; height: 92px; border-radius: 50%; background: var(--accent-fill); color: var(--text-on-red); box-shadow: 0 8px 24px rgb(0 0 0 / .3); text-align: center; }
	.disc b { font: 800 26px/1 var(--font-display); }
	.disc small { margin-top: 3px; font: 600 11px/1.1 var(--font-ui); }
	.a-foot { position: absolute; left: 28px; right: 28px; bottom: 26px; display: flex; flex-direction: column; gap: 8px; }
	.a-lis { font: 700 12px/1.3 var(--font-ui); text-transform: uppercase; letter-spacing: .08em; opacity: .8; font-variant-numeric: tabular-nums; }
	.a-bio { max-width: 64ch; color: rgb(255 255 255 / .9); font: 500 19px/1.45 var(--font-ui); text-shadow: 0 1px 12px rgb(0 0 0 / .35); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.a-more { align-self: flex-start; margin-top: 6px; padding: 9px 16px; border-radius: 999px; background: rgb(255 255 255 / 14%); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 16%); backdrop-filter: blur(20px) saturate(160%); font: 600 14px/1 var(--font-ui); transition: background var(--dur-fast) var(--ease-out); }
	.acard:hover .a-more { background: rgb(255 255 255 / 24%); }
	@media (prefers-reduced-motion: reduce) { .acard:hover, .song:hover { translate: none; } .ab-frame { transition: opacity 1.2s; scale: 1; } }

	/* The About sheet: an Apple TV-style dark window in the artist's colour, a zooming still with the face, glass tiles, the story. */
	:global(.ui-sheet:has(.sheet[data-testid='artist-about-sheet'])) { --w: 1040px; max-height: min(92dvh, 980px); --text: #fff; --text-muted: rgb(235 235 245 / .62); --surface-1: rgb(255 255 255 / 7%); --surface-2: rgb(255 255 255 / 12%); --line: rgb(255 255 255 / 10%); --border: rgb(255 255 255 / 10%); color-scheme: dark; color: #fff; background: radial-gradient(120% 70% at 50% 0%, color-mix(in srgb, var(--tint-bg, #2a2c33) 85%, #fff 6%), color-mix(in srgb, var(--tint-bg, #1b1c20) 55%, #050506) 70%); box-shadow: 0 0 0 1px rgb(255 255 255 / 8%), 0 40px 120px rgb(0 0 0 / .6); }
	.sheet { display: flex; flex-direction: column; gap: 24px; }
	.chg { font-style: normal; }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; }
	.chips span { padding: 8px 14px; border-radius: 999px; background: rgb(255 255 255 / 9%); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 8%); font: 600 13px/1 var(--font-ui); text-transform: capitalize; }
	/* The photo is the centrepiece: the listener count is its title, the name sits small above it. */
	/* Small glass cards, one figure each. */
	/* Trial side-column About: the photo whole on black, a figures column at left, the story at right. */
	.col-photo { position: relative; margin: 0; height: clamp(260px, 44vh, 420px); border-radius: 18px; overflow: hidden; background: #000; }
	.col-photo img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: contain; opacity: 0; transition: opacity .6s var(--ease-out); }
	.col-photo img.on { opacity: 1; }
	.col-nav { position: absolute; top: 50%; translate: 0 -50%; width: 40px; height: 40px; border: 0; border-radius: 50%; display: grid; place-items: center; background: rgb(0 0 0 / .45); color: #fff; cursor: pointer; backdrop-filter: blur(12px); }
	.col-nav:hover { background: rgb(0 0 0 / .7); }
	.col-nav.prev { left: 14px; } .col-nav.next { right: 14px; }
	.col { display: grid; grid-template-columns: 230px minmax(0, 1fr); gap: 40px; }
	.col-side { display: flex; flex-direction: column; gap: 22px; min-width: 0; }
	.col-disc { display: grid; place-content: center; justify-items: center; width: 84px; height: 84px; border-radius: 50%; background: var(--accent-fill); color: var(--text-on-red); text-align: center; }
	.col-disc b { font: 800 24px/1 var(--font-display); }
	.col-disc small { font: 600 10px/1.1 var(--font-ui); margin-top: 2px; }
	.col-big { margin: 0; display: flex; flex-direction: column; gap: 4px; }
	.col-big b { font: 800 30px/1 var(--font-display); letter-spacing: -.02em; font-variant-numeric: tabular-nums; }
	.col-big span { color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); text-transform: capitalize; }
	.col-list { margin: 6px 0 0; display: flex; flex-direction: column; gap: 14px; }
	.col-list div { display: flex; flex-direction: column; gap: 2px; }
	.col-list dt { order: 2; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); }
	.col-list dd { order: 1; margin: 0; font: 700 14px/1.3 var(--font-ui); }
	.col-main { display: flex; flex-direction: column; gap: 22px; min-width: 0; }
	.col-bio { display: flex; flex-direction: column; gap: 14px; }
	.col-bio p { margin: 0; color: rgb(235 235 245 / .8); font: 500 16px/1.65 var(--font-ui); max-width: 68ch; }
	.col-by { margin: 0; display: flex; align-items: center; gap: 10px; color: var(--text-muted); font: 500 13px/1 var(--font-ui); }
	.col-by img { width: 28px; height: 28px; border-radius: 50%; object-fit: cover; }
	@media (max-width: 767px) {
		.col { grid-template-columns: minmax(0, 1fr); gap: 28px; }
		.col-list { flex-direction: row; flex-wrap: wrap; gap: 14px 28px; }
		.a-bio { font-size: 16px; }
		.a-foot { left: 18px; right: 18px; bottom: 18px; }
	}

	/* Loading. */
	.sk-songs { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 24px; }
	.sq-rail { --poster-ratio: 1; }

	/* Overview / Library tabs for an artist in the Library: the same underline tabs as a title page. */
	.ar-tabs { display: flex; gap: 6px; margin-top: calc(var(--gap) * -1 + 16px); border-bottom: 1px solid var(--line); scroll-margin-top: calc(var(--topbar, 56px) + 16px); }
	.ar-tabs button { height: 40px; padding: 0 14px; border: 0; border-bottom: 2px solid transparent; background: none; color: var(--text-muted); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	@media (max-width: 767px) and (pointer: coarse) { .ar-tabs button { height: 44px; } }
	.ar-tabs button.on { color: var(--text); border-bottom-color: var(--text); }

	@media (max-width: 1000px) {
		.songs { grid-template-columns: minmax(0, 1fr); grid-template-rows: none; grid-auto-flow: row; }
	}
	/* Phones: designed for 390 and 360, not squeezed. */
	@media (max-width: 767px) {
		.artist { --music-item: calc((100cqi - var(--shelf-gap, 12px)) / 2.3); --gap: 32px; }
		.hero { height: 180px; margin-top: calc(var(--s-4) * -1); }
		.vctl { top: 12px; right: 12px; gap: 6px; }
		.vctl button { width: 34px; height: 34px; }
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
		.song { grid-template-columns: 24px 48px minmax(0, 1fr) 36px; gap: 10px; padding: 0 4px; }
		.s-len { display: none; }
		.num { width: 24px; }
		.grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.stats { padding: 22px 8px; }
		.st1 { font-size: 17px; }
		.stats p > span + span::before { margin: 0 8px; }
		.acard { height: 360px; }
		.disc { width: 76px; height: 76px; top: 14px; left: 14px; }
		.disc b { font-size: 21px; }
		.disc small { font-size: 10px; }
		.a-foot { left: 16px; right: 16px; bottom: 16px; }
		.sk-songs { grid-template-columns: minmax(0, 1fr); }
	}
</style>

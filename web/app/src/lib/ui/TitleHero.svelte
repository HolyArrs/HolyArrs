<script lang="ts" module>
	// Shared helpers for title pages.

	/** "2 hr 36 min" from minutes. */
	export function runtimeWords(min?: number | null): string {
		if (!min || min <= 0) return '';
		const h = Math.floor(min / 60);
		const m = Math.round(min % 60);
		return [h ? `${h} hr` : '', m ? `${m} min` : ''].filter(Boolean).join(' ');
	}

	/** The last part of a path ("Dune (2021)" from C:\...\Movies\Dune (2021)), never the full path. */
	export function shortFolder(path?: string | null): string {
		const parts = (path ?? '').split(/[\\/]+/).filter(Boolean);
		return parts[parts.length - 1] ?? '';
	}

	export type RichMeta = { backdrop?: string; logo?: string; poster?: string; genres: string[]; runtime?: number; certification?: string; score?: number; overview?: string; year?: number; backdrops: string[]; videos: Video[]; moods?: string[]; quality?: string; seasons?: number };
	import type { Video } from '../trailer.ts';
	import { shownScore } from '../rating.ts';
	type Raw = Record<string, unknown>;
	const s = (v: unknown) => (typeof v === 'string' && v.trim() ? v.trim() : undefined);
	const n = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) && v > 0 ? v : undefined);
	const metaCache = new Map<string, Promise<RichMeta | null>>();

	/**
	 * richMetadata asks GET /metadata/{kind}/{tmdb} for art and facts the
	 * Library title lacks. A missing endpoint or title answers null.
	 */
	export function richMetadata(get: <T>(path: string) => Promise<T>, kind: 'movie' | 'show', tmdb: string | undefined | null): Promise<RichMeta | null> {
		if (!tmdb) return Promise.resolve(null);
		const key = `${kind}:${tmdb}`;
		let p = metaCache.get(key);
		if (!p) {
			p = get<Raw>(`/metadata/${kind}/${encodeURIComponent(tmdb)}`)
				.then((raw) => {
					const genres = Array.isArray(raw.genres) ? raw.genres.map((g) => (typeof g === 'string' ? g : s((g as Raw)?.name))).filter((g): g is string => !!g) : [];
					const rating = s(raw.certification) ?? s(raw.content_rating) ?? (s(raw.rating)?.includes('/') ? undefined : s(raw.rating));
					const backdrops = Array.isArray(raw.backdrops) ? raw.backdrops.filter((b): b is string => typeof b === 'string') : [];
					const moods = Array.isArray(raw.moods) ? raw.moods.filter((m): m is string => typeof m === 'string' && !!m.trim()) : [];
					const videos = Array.isArray(raw.videos) ? (raw.videos as Video[]).filter((v) => v && typeof v.key === 'string') : [];
					return { backdrop: s(raw.backdrop), logo: s(raw.logo), poster: s(raw.poster), genres, runtime: n(raw.runtime_minutes) ?? n(raw.runtime), certification: rating, score: shownScore(n(raw.vote_average) ?? n(raw.score), n(raw.vote_count) ?? n(raw.votes)), overview: s(raw.overview), year: n(raw.year), backdrops, videos, moods, quality: s(raw.quality), seasons: Array.isArray(raw.seasons) ? raw.seasons.filter((x) => n((x as Raw)?.number)).length || undefined : n(raw.number_of_seasons) };
				})
				.catch(() => {
					// A failed read is not remembered: the next page visit asks again.
					metaCache.delete(key);
					return null;
				});
			metaCache.set(key, p);
		}
		return p;
	}

</script>

<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { setTopCrumbs } from '../top-crumbs.svelte.ts';
	import Icon, { type IconName } from '../Icon.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import { viewTransitionName } from '../motion.ts';
	import { proxied, sized } from '../tmdb-image.ts';
	import { awardsTip, loadWikiFacts, type WikiFacts } from '../wiki-facts.ts';
	import Billboard from './Billboard.svelte';
	import ActionButton from './ActionButton.svelte';
	import { heroPictures } from '../trailer.ts';
	import { autoplayHere, loadUISettings, trailerSources } from '../autoplay.svelte.ts';
	import { openTheater } from '../theater.svelte.ts';
	import { api } from '../api.ts';
	import { loadExtras, type Extras } from '../outside-ratings.ts';
	import RatingsStrip from './RatingsStrip.svelte';
	import { ratingPicks, shownRatings } from '../rating-picks.svelte.ts';

	// The title page hero (movie, show, artist, album, not-yet-added titles):
	// the Explore billboard (Billboard.svelte), so moving from Explore to a
	// title keeps one look. Breadcrumb pill top-left, logo or title in the
	// left 40 %, one facts line with the status, a short synopsis with Show
	// more, then the action row: the page's main button (Ask, Add or In
	// Library), a Trailer button, and the page's ⋯ menu. Movies and shows
	// play the muted trailer after the dwell (desktop); the Trailer button
	// opens the theater everywhere. With no backdrop the poster fills the
	// frame, darkened; an album shows its cover square over a blurred copy.
	let {
		title,
		kind,
		tmdb,
		mbid,
		backdrop,
		backdrops = [],
		poster,
		logo,
		meta: facts = [],
		certification,
		score,
		badges = [],
		status,
		percent,
		statusLabel,
		description,
		breadcrumb = [],
		crumbIcon = 'film',
		vtName,
		square = false,
		cover,
		settled = true,
		byline,
		actions: actionRow,
		ratings = true,
		showFacts = true,
		tagline,
		heroStory,
		heroCaps
	}: {
		title: string;
		/** A movie or show: the muted trailer and the Trailer button. */
		kind?: 'movie' | 'show';
		tmdb?: string | null;
		/** An artist's MusicBrainz id: fanart.tv pictures when the artist has none. */
		mbid?: string | null;
		backdrop?: string | null;
		/** More backdrops for the slow crossfade (Layer 1). */
		backdrops?: string[];
		poster?: string | null;
		logo?: string | null;
		meta?: string[];
		/** Age rating, shown in a bordered box (PG-13, TV-MA). */
		certification?: string | null;
		/** Score out of 10. */
		score?: number | null;
		/** Quality badges: 4K, Dolby Vision, Atmos, CC. */
		badges?: string[];
		status?: StatusKind;
		percent?: number;
		statusLabel?: string;
		description?: string;
		breadcrumb?: { href: string; label: string }[];
		crumbIcon?: IconName;
		vtName?: string;
		/** Square art (music). */
		square?: boolean;
		/** An album cover, shown square bottom-left over its own blurred copy. */
		cover?: string | null;
		settled?: boolean;
		/** The status line under the hero, such as who asked. */
		byline?: Snippet;
		actions?: Snippet;
		/** The score badge and outside ratings in the facts line (off when the page shows them below). */
		ratings?: boolean;
		/** The facts line, age rating and award badges (off when the page shows them under the hero). */
		showFacts?: boolean;
		/** A tagline under the logo or title. */
		tagline?: string | null;
		/** The story inside the hero (three lines at most), when the page shows it there. */
		heroStory?: string | null;
		/** A small-caps line above the story in the hero (director · country · minutes). */
		heroCaps?: string | null;
	} = $props();

	let backdropFailed = $state(false);
	let logoFailed = $state(false);
	// A dark wordmark (black letters made for light posters) vanishes on the
	// dark picture: it is drawn white instead, its shape unchanged.
	let darkLogo = $state(false);
	// The shown logo may come from another site; a CORS copy is read for the check.
	function checkLogo(src: string) {
		darkLogo = false;
		const probe = new Image();
		probe.crossOrigin = 'anonymous';
		probe.onload = () => src === shownLogo && (darkLogo = logoIsDark(probe));
		probe.src = src;
	}
	function logoIsDark(img: HTMLImageElement): boolean {
		try {
			const c = document.createElement('canvas');
			c.width = 48; c.height = 24;
			const g = c.getContext('2d', { willReadFrequently: true });
			if (!g) return false;
			g.drawImage(img, 0, 0, 48, 24);
			const d = g.getImageData(0, 0, 48, 24).data;
			let n = 0, l = 0;
			for (let k = 0; k < d.length; k += 4) {
				if (d[k + 3] < 96) continue;
				n++; l += (0.2126 * d[k] + 0.7152 * d[k + 1] + 0.0722 * d[k + 2]) / 255;
			}
			return n > 20 && l / n < 0.3;
		} catch {
			return false;
		}
	}
	// A very wide wordmark (over 6:1) would come out a thin line in the logo box: it gets a floor height and more width.
	let wideLogo = $state(false);
	let posterFailed = $state(false);

	// Awards from Wikidata (metadata server): a badge like "Oscar winner" or
	// "3 Emmys", the awards listed on hover.
	let wiki = $state<WikiFacts>({ badges: [], awards: [], bio: null });
	$effect(() => {
		const k = kind;
		const t = tmdb;
		wiki = { badges: [], awards: [], bio: null };
		if (!k || !t) return;
		let live = true;
		void loadWikiFacts(api.get, k, t).then((f) => live && (wiki = f));
		return () => (live = false);
	});

	// fanart.tv art and outside ratings (metadata server, when its keys are set):
	// the fanart logo only when TMDB has none or its logo fails to load.
	let extras = $state<Extras>({ backgrounds: [], scores: [] });
	let fanartLogoFailed = $state(false);
	$effect(() => {
		const k = kind ?? (mbid ? 'artist' : undefined);
		const id = kind ? tmdb : mbid;
		extras = { backgrounds: [], scores: [] };
		fanartLogoFailed = false;
		if (!k || !id) return;
		let live = true;
		void loadExtras(api.get, k, id).then((e) => live && (extras = e));
		return () => (live = false);
	});
	const shownLogo = $derived(logo && !logoFailed ? proxied(logo) : extras.logo && !fanartLogoFailed ? extras.logo : null);
	const heroBackdrop = $derived(backdrop || (mbid ? extras.backgrounds[0] : null));
	const heroPoster = $derived(poster || (mbid ? extras.thumb : null));

	const wide = $derived(heroBackdrop && !backdropFailed && !cover ? heroBackdrop : null);
	const pictures = $derived(wide ? heroPictures(wide, backdrops).map((u) => sized(u, 'backdrop')) : []);
	// No backdrop: the poster (or the cover) itself, cropped and darkened.
	const stand = $derived(cover ?? (!wide && heroPoster && !posterFailed ? heroPoster : null));

	// Only the ratings this browser picked (Settings › Appearance).
	const shown = $derived(shownRatings(extras.scores, score, ratingPicks.list, !ratingPicks.custom));
	const scoreText = $derived(shown.tmdb ? (Math.round(shown.tmdb * 10) / 10).toFixed(1) : '');
	// The breadcrumb lives in the top bar (one pattern on every page): the page's parent for a fresh visit, and its name.
	$effect(() => {
		setTopCrumbs(breadcrumb[0] ?? null, breadcrumb.length > 1 ? (breadcrumb.at(-1)?.label ?? title) : title);
		return () => setTopCrumbs(null);
	});

	// The Trailer button shows once the title has a trailer; it opens the theater.
	let videos = $state<Video[] | undefined>();
	$effect(() => {
		const k = kind;
		const t = tmdb;
		videos = undefined;
		if (!k || !t) return;
		let live = true;
		void richMetadata(api.get, k, t).then((m) => live && (videos = m?.videos ?? []));
		return () => (live = false);
	});
	let opening = $state(false);
	/** heroStill draws the hero's picture as it shows (cover, the hero's shape). */
	function heroStill(hero: HTMLElement | null | undefined): HTMLCanvasElement | null {
		const img = hero?.querySelector<HTMLImageElement>('.art img');
		const r = hero?.getBoundingClientRect();
		if (!img?.complete || !img.naturalWidth || !r?.width || !r.height) return null;
		const c = document.createElement('canvas');
		c.width = Math.round(r.width);
		c.height = Math.round(r.height);
		const k = Math.max(c.width / img.naturalWidth, c.height / img.naturalHeight);
		const w = img.naturalWidth * k;
		const h = img.naturalHeight * k;
		try {
			c.getContext('2d')?.drawImage(img, (c.width - w) * 0.7, (c.height - h) * 0.3, w, h);
		} catch {
			return null;
		}
		return c;
	}
	async function playTrailer(e: MouseEvent) {
		if (!kind || !tmdb || opening) return;
		const btn = e.currentTarget as HTMLElement;
		const hero = btn.closest('section');
		// The hero's trailer is playing: hand off from it, as a click on the picture does (same second, sound, frame, rect).
		const playing = hero?.querySelector<HTMLButtonElement>('.art button.expand');
		if (playing) {
			playing.click();
			return;
		}
		opening = true;
		const r = hero?.getBoundingClientRect();
		// The hero's picture is held while the big player loads (no black frame).
		const still = heroStill(hero);
		try {
			const s = await trailerSources(kind, tmdb, videos);
			if (s[0]) openTheater({ kind, tmdb, title, source: s[0], fallbacks: s.slice(1), time: 0, rect: r ? { left: r.left, top: r.top, width: r.width, height: r.height } : null, from: 'hero', still, returnTo: btn });
		} finally {
			opening = false;
		}
	}
	// No autoplay on this device (phone, reduced motion, data saver, setting off): the Trailer button shows its word.
	let still = $state(true);
	onMount(() => void loadUISettings().then(() => (still = !autoplayHere())));
</script>

<div class="ui-thero" class:square>
	<Billboard label={title} testid="title-hero" {kind} {tmdb} {title} {videos} {pictures} {vtName} {settled} once position="70% 30%" onpictureerror={() => (backdropFailed = true)} art={stand ? standArt : undefined}>
		{#snippet heading()}
			{#if cover}
				<img class="cover" src={cover} alt="" decoding="async" use:viewTransitionName={vtName} />
			{/if}
			{#if shownLogo}
				<h1 class="bb-logo" class:wide={wideLogo} class:dark={darkLogo}><img src={shownLogo} alt={title} onload={(e) => { const i = e.currentTarget as HTMLImageElement; wideLogo = i.naturalHeight > 0 && i.naturalWidth / i.naturalHeight > 6; checkLogo(i.src); }} onerror={() => (logo && shownLogo === proxied(logo) ? (logoFailed = true) : (fanartLogoFailed = true))} /></h1>
			{:else}
				<h1 class="bb-name" class:short={!!cover}>{title}</h1>
			{/if}
			{#if tagline}<p class="hero-tagline" data-testid="hero-tagline">{tagline}</p>{/if}
			{#if heroCaps}<p class="hero-caps" data-testid="hero-caps">{heroCaps}</p>{/if}
			{#if heroStory}<p class="hero-story" data-testid="hero-story">{heroStory}</p>{/if}
		{/snippet}
		{#snippet meta()}
			{#if showFacts && facts.length}<p>{facts.join(' · ')}</p>{/if}
			{#if showFacts && certification}<span class="b cert" title="Age rating">{certification}</span>{/if}
			{#if ratings && scoreText}<span class="b score" title="Score out of 10"><svg width="11" height="11" viewBox="0 0 12 12" aria-hidden="true"><path d="M6 .8l1.6 3.3 3.6.5-2.6 2.5.6 3.6L6 9l-3.2 1.7.6-3.6L.8 4.6l3.6-.5z" fill="currentColor" /></svg>{scoreText}</span>{/if}
			{#if ratings}<RatingsStrip scores={shown.scores} />{/if}
			{#each showFacts ? wiki.badges.slice(0, 2) : [] as b (b)}<span class="b award" title={awardsTip(wiki.awards)} data-testid="award-badge"><svg width="11" height="11" viewBox="0 0 12 12" aria-hidden="true"><path d="M3 1h6v2.4a3 3 0 0 1-6 0zM3 2H1.2v.8A2 2 0 0 0 3 4.8M9 2h1.8v.8A2 2 0 0 1 9 4.8M5.3 6.3h1.4v2.2H5.3zM3.6 8.5h4.8V11H3.6z" fill="currentColor" /></svg>{b}</span>{/each}
			{#each badges as b (b)}<span class="b">{b}</span>{/each}
			{#if status}<StatusPill kind={status} {percent} label={statusLabel} on="art" />{/if}
		{/snippet}
		{#snippet actions()}
			{@render actionRow?.()}
			{#if kind && tmdb && videos?.length}
				<ActionButton icon="play" label="Play the trailer" caption="Trailer" pill={still} onclick={playTrailer} busy={opening} data-testid="hero-trailer" data-off-while-playing />
			{/if}
		{/snippet}
	</Billboard>
	<!-- A title page: the story sits under the hero, in full, and never pops over the art. -->
	{#if description}<p class="story" data-testid="title-story">{description}</p>{/if}
	{#if byline}<div class="status-line">{@render byline()}</div>{/if}
</div>

{#snippet standArt()}
	<img class="bd" class:blur={!!cover} class:square src={sized(stand!, 'poster')} alt="" decoding="async" fetchpriority="high" use:viewTransitionName={cover ? undefined : vtName} onerror={() => (posterFailed = true)} />
{/snippet}

<style>
	.bb-logo.dark :global(img) { filter: brightness(0) invert(1) drop-shadow(0 2px 10px rgb(0 0 0 / .45)); }
	.bb-logo.wide :global(img) { min-height: clamp(40px, 3.4vw, 64px); max-width: min(92vw, 760px) !important; }
	.bd { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 20%; filter: saturate(1.1) brightness(.75); }
	.bd.square { object-position: center 35%; }
	.bd.blur { filter: blur(40px) saturate(1.3) brightness(.6); transform: scale(1.2); }
	.cover { display: block; width: clamp(120px, 14vw, 220px); aspect-ratio: 1; object-fit: cover; border-radius: 12px; box-shadow: 0 12px 40px rgb(0 0 0 / 50%); margin-bottom: 12px; }
	.ui-thero :global(.bb-name.short) { min-height: 0; }
	/* The billboard isolates its layers, so an open menu's panel can't rise past the sections below (the About band
	   covered its last items on phones): lift the whole hero while a panel is open, still under the top bar. */
	.ui-thero:has(:global(.ui-pop > .panel)) { position: relative; z-index: 10; }
	.b { display: inline-flex; align-items: center; gap: 4px; height: 22px; padding: 0 7px; border-radius: 5px; border: 1px solid rgb(255 255 255 / 55%); color: rgb(255 255 255 / 92%); font: 700 11.5px/1 var(--font-ui); letter-spacing: .03em; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.b.award { border-color: rgb(245 197 24 / 75%); color: #f5c518; }
	.cert { border-width: 1.5px; border-color: rgb(255 255 255 / 80%); }
	.score { border-radius: 999px; padding: 0 9px 0 7px; background: rgb(255 255 255 / 12%); border-color: rgb(255 255 255 / 30%); }
	.score svg { color: #f5c518; }
	.hero-caps { margin: 12px 0 0; color: rgb(255 255 255 / 92%); font: 600 13px/1.3 var(--font-ui); letter-spacing: .08em; text-transform: uppercase; text-shadow: 0 1px 3px rgb(0 0 0 / 55%); }
	.hero-story { margin: 12px 0 0; max-width: 58ch; color: rgb(255 255 255 / 90%); font: 500 clamp(14px, 1.1vw, 16px)/1.5 var(--font-ui); text-shadow: 0 1px 3px rgb(0 0 0 / 55%); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
	.hero-tagline { margin: 10px 0 0; max-width: 40ch; color: rgb(255 255 255 / 88%); font: italic 500 clamp(15px, 1.3vw, 18px)/1.35 var(--font-ui); text-shadow: 0 1px 3px rgb(0 0 0 / 55%), 0 0 18px rgb(0 0 0 / 45%); }
	.story { max-width: 72ch; margin: var(--s-5) 0 0; font: 400 16px/1.55 var(--font-ui); color: var(--text); }
	.ui-thero :global(.tp-member) { min-height: 44px; display: flex; align-items: center; }
	.status-line { margin-top: var(--s-4); display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; font: 500 15px/1.4 var(--font-ui); color: var(--text-muted); }
</style>

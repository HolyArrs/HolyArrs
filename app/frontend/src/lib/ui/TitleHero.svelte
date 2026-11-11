<script lang="ts" module>
	// Title page hero: shared metadata helpers and the banner with art, badges and trailer button.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import type { Video } from '../core/trailer/trailer.ts';
	import { shownScore } from '../core/rating.ts';

	// Minutes as "1 hr 5 min"; empty when unknown.
	export function runtimeWords(min?: number | null): string {
		if (!min || min <= 0) return '';
		const h = Math.floor(min / 60);
		const m = Math.round(min % 60);
		if (h && m) return t('ui.titleHero.hoursMinutes', { h, m });
		return h ? t('ui.titleHero.hours', { h }) : t('ui.titleHero.minutes', { m });
	}

	// Last folder name only, for either slash style.
	export function shortFolder(path?: string | null): string {
		const parts = (path ?? '').split(/[\\/]+/).filter(Boolean);
		return parts[parts.length - 1] ?? '';
	}

	export type RichMeta = { backdrop?: string; logo?: string; poster?: string; genres: string[]; runtime?: number; certification?: string; score?: number; overview?: string; year?: number; backdrops: string[]; videos: Video[]; moods?: string[]; quality?: string; seasons?: number };
	type Raw = Record<string, unknown>;
	// s and n pick a clean string or positive number from loose server data
	const s = (v: unknown) => (typeof v === 'string' && v.trim() ? v.trim() : undefined);
	const n = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) && v > 0 ? v : undefined);
	// one request per title, shared by every caller
	const metaCache = new Map<string, Promise<RichMeta | null>>();

	// Failed lookups are dropped from the cache so a later call can retry.
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
	import Icon, { type IconName } from './Icon.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import Billboard from './Billboard.svelte';
	import ActionButton from './ActionButton.svelte';
	import RatingsStrip from './RatingsStrip.svelte';
	import { awardsTip, loadWikiFacts, type WikiFacts } from './wiki-facts.ts';
	import { loadExtras, type Extras } from './outside-ratings.ts';
	import { ratingPicks, shownRatings } from './rating-picks.svelte.ts';
	import { setTopCrumbs } from '../core/top-crumbs.svelte.ts';
	import { viewTransitionName } from '../core/motion.ts';
	import { proxied, sized } from '../core/tmdb-image.ts';
	import { heroPictures } from '../core/trailer/trailer.ts';
	import { autoplayHere, loadUISettings, trailerClickSources } from '../core/trailer/autoplay.svelte.ts';
	import { openTheater } from '../core/trailer/theater.svelte.ts';
	import { api } from '../core/api.ts';
	import { minimalLook } from '../core/look.svelte.ts';

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
		tagline
	}: {
		title: string;
		kind?: 'movie' | 'show';
		tmdb?: string | null;
		mbid?: string | null;
		backdrop?: string | null;
		backdrops?: string[];
		poster?: string | null;
		logo?: string | null;
		meta?: string[];
		certification?: string | null;
		score?: number | null;
		badges?: string[];
		status?: StatusKind;
		percent?: number;
		statusLabel?: string;
		description?: string;
		breadcrumb?: { href: string; label: string }[];
		crumbIcon?: IconName;
		vtName?: string;
		square?: boolean;
		cover?: string | null;
		settled?: boolean;
		byline?: Snippet;
		actions?: Snippet;
		ratings?: boolean;
		showFacts?: boolean;
		tagline?: string | null;
	} = $props();

	let backdropFailed = $state(false);
	let logoFailed = $state(false);
	let darkLogo = $state(false);
	let wideLogo = $state(false);
	let posterFailed = $state(false);
	let wiki = $state<WikiFacts>({ badges: [], awards: [], bio: null });
	let extras = $state<Extras>({ backgrounds: [], scores: [] });
	let fanartLogoFailed = $state(false);
	let videos = $state<Video[] | undefined>();
	let opening = $state(false);
	let still = $state(true);

	// the title logo wins; fan art logo is the fallback when it fails to load
	const shownLogo = $derived(logo && !logoFailed ? proxied(logo) : extras.logo && !fanartLogoFailed ? extras.logo : null);
	const heroBackdrop = $derived(backdrop || (mbid ? extras.backgrounds[0] : null));
	const heroPoster = $derived(poster || (mbid ? extras.thumb : null));

	// a cover replaces the wide backdrop; a poster fills in only when no backdrop exists
	const wide = $derived(heroBackdrop && !backdropFailed && !cover ? heroBackdrop : null);
	const pictures = $derived(wide ? heroPictures(wide, backdrops).map((u) => sized(u, 'backdrop')) : []);
	// The minimal look shows only a small picture: the cover, else the poster.
	const thumb = $derived(cover ?? (heroPoster ? sized(heroPoster, 'poster', 148) : null));
	const stand = $derived(cover ?? (!wide && heroPoster && !posterFailed ? heroPoster : null));

	const shown = $derived(shownRatings(extras.scores, score, ratingPicks.list, !ratingPicks.custom));
	// one decimal, like 7.4
	const scoreText = $derived(shown.tmdb ? (Math.round(shown.tmdb * 10) / 10).toFixed(1) : '');

	// each effect below drops late answers once its inputs change
	$effect(() => {
		const k = kind;
		const t = tmdb;
		wiki = { badges: [], awards: [], bio: null };
		if (!k || !t) return;
		let live = true;
		void loadWikiFacts(api.get, k, t).then((f) => live && (wiki = f));
		return () => (live = false);
	});
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
	// crumbs live in the top bar, so clear them when the hero leaves
	$effect(() => {
		setTopCrumbs(breadcrumb[0] ?? null, breadcrumb.length > 1 ? (breadcrumb.at(-1)?.label ?? title) : title);
		return () => setTopCrumbs(null);
	});
	$effect(() => {
		const k = kind;
		const t = tmdb;
		videos = undefined;
		if (!k || !t) return;
		let live = true;
		void richMetadata(api.get, k, t).then((m) => live && (videos = m?.videos ?? []));
		return () => (live = false);
	});

	// the still poster pill shows unless autoplay is already running here
	onMount(() => void loadUISettings().then(() => (still = !autoplayHere())));

	// A running hero trailer is expanded instead of opening a second player.
	async function playTrailer(e: MouseEvent) {
		if (!kind || !tmdb || opening) return;
		const btn = e.currentTarget as HTMLElement;
		const hero = btn.closest('section');
		const playing = hero?.querySelector<HTMLButtonElement>('.art button.expand');
		if (playing) {
			playing.click();
			return;
		}
		opening = true;
		const r = hero?.getBoundingClientRect();
		const still = heroStill(hero);
		try {
			const s = await trailerClickSources(kind, tmdb, videos);
			if (s[0]) openTheater({ kind, tmdb, title, source: s[0], fallbacks: s.slice(1), time: 0, rect: r ? { left: r.left, top: r.top, width: r.width, height: r.height } : null, from: 'hero', still, returnTo: btn });
		} finally {
			opening = false;
		}
	}

	// Dark logos vanish on the dark hero, so they get inverted to white.
	function checkLogo(src: string) {
		darkLogo = false;
		const probe = new Image();
		probe.crossOrigin = 'anonymous';
		probe.onload = () => src === shownLogo && (darkLogo = logoIsDark(probe));
		probe.src = src;
	}
	// Samples a tiny canvas; the size, alpha and brightness cutoffs just need to tell black from light.
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
	// Copies the visible hero image so the theater can grow from the same frame.
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
</script>

<!-- description and byline sit under the banner, outside the art -->
<div class="ui-thero" class:square>
	<Billboard label={title} testid="title-hero" {kind} {tmdb} {title} {videos} {pictures} {vtName} {settled} once position="70% 30%" onpictureerror={() => (backdropFailed = true)} art={stand ? standArt : undefined} {thumb} thumbSquare={square || !!cover}>
		{#snippet heading()}
			{#if cover && !minimalLook()}
				<img class="cover" src={cover} alt="" decoding="async" use:viewTransitionName={vtName} />
			{/if}
			{#if shownLogo}
				<h1 class="bb-logo" class:wide={wideLogo} class:dark={darkLogo}><img src={shownLogo} alt={title} onload={(e) => { const i = e.currentTarget as HTMLImageElement; wideLogo = i.naturalHeight > 0 && i.naturalWidth / i.naturalHeight > 6; checkLogo(i.src); }} onerror={() => (logo && shownLogo === proxied(logo) ? (logoFailed = true) : (fanartLogoFailed = true))} /></h1>
			{:else}
				<h1 class="bb-name" class:short={!!cover}>{title}</h1>
			{/if}
			{#if tagline}<p class="hero-tagline" data-testid="hero-tagline">{tagline}</p>{/if}
		{/snippet}
		{#snippet meta()}
			{#if showFacts && facts.length}<p>{facts.join(' · ')}</p>{/if}
			{#if showFacts && certification}<span class="b cert" title={t('ui.titleHero.ageRating')}>{certification}</span>{/if}
			{#if ratings && scoreText}<span class="b score" title={t('ui.titleHero.scoreOutOf10')}><svg width="11" height="11" viewBox="0 0 12 12" aria-hidden="true"><path d="M6 .8l1.6 3.3 3.6.5-2.6 2.5.6 3.6L6 9l-3.2 1.7.6-3.6L.8 4.6l3.6-.5z" fill="currentColor" /></svg>{scoreText}</span>{/if}
			{#if ratings}<RatingsStrip scores={shown.scores} />{/if}
			{#each showFacts ? wiki.badges.slice(0, 2) : [] as b (b)}<span class="b award" title={awardsTip(wiki.awards)} data-testid="award-badge"><svg width="11" height="11" viewBox="0 0 12 12" aria-hidden="true"><path d="M3 1h6v2.4a3 3 0 0 1-6 0zM3 2H1.2v.8A2 2 0 0 0 3 4.8M9 2h1.8v.8A2 2 0 0 1 9 4.8M5.3 6.3h1.4v2.2H5.3zM3.6 8.5h4.8V11H3.6z" fill="currentColor" /></svg>{b}</span>{/each}
			{#each badges as b (b)}<span class="b">{serverWords(b)}</span>{/each}
			{#if status}<StatusPill kind={status} {percent} label={statusLabel} on="art" />{/if}
		{/snippet}
		{#snippet actions()}
			{@render actionRow?.()}
			{#if kind && tmdb && videos?.length}
				<ActionButton icon="play" label={t('ui.titleHero.playTheTrailer')} caption={t('ui.titleHero.trailer')} pill={still} onclick={playTrailer} busy={opening} data-testid="hero-trailer" data-off-while-playing />
			{/if}
		{/snippet}
	</Billboard>
	{#if description}<p class="story" data-testid="title-story">{description}</p>{/if}
	{#if byline}<div class="status-line">{@render byline()}</div>{/if}
</div>

{#snippet standArt()}
	<img class="bd" class:blur={!!cover} class:square src={sized(stand!, 'poster')} alt="" decoding="async" fetchpriority="high" use:viewTransitionName={cover ? undefined : vtName} onerror={() => (posterFailed = true)} />
{/snippet}

<style>
	/* dark logos turn white, wide ones get a size floor so they stay legible */
	.bb-logo.dark :global(img) { filter: brightness(0) invert(1) drop-shadow(0 2px 10px rgb(0 0 0 / .45)); }
	.bb-logo.wide :global(img) { min-height: clamp(40px, 3.4vw, 64px); max-width: min(92vw, 760px) !important; }
	/* backdrop is dimmed so the text on top stays readable */
	.bd { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 20%; filter: saturate(1.1) brightness(.75); }
	.bd.square { object-position: center 35%; }
	.bd.blur { filter: blur(40px) saturate(1.3) brightness(.6); transform: scale(1.2); }
	.cover { display: block; width: clamp(120px, 14vw, 220px); aspect-ratio: 1; object-fit: cover; border-radius: 12px; box-shadow: 0 12px 40px rgb(0 0 0 / 50%); margin-bottom: 12px; }
	.ui-thero :global(.bb-name.short) { min-height: 0; }
	/* lifts the hero above later sections while a popover is open */
	.ui-thero:has(:global(.ui-pop > .panel)) { position: relative; z-index: 10; }
	.b { display: inline-flex; align-items: center; gap: 4px; height: 22px; padding: 0 7px; border-radius: 5px; border: 1px solid rgb(255 255 255 / 55%); color: rgb(255 255 255 / 92%); font: 700 11.5px/1 var(--font-ui); letter-spacing: .03em; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.b.award { border-color: rgb(245 197 24 / 75%); color: #f5c518; }
	.cert { border-width: 1.5px; border-color: rgb(255 255 255 / 80%); }
	.score { border-radius: 999px; padding: 0 9px 0 7px; background: rgb(255 255 255 / 12%); border-color: rgb(255 255 255 / 30%); }
	.score svg { color: #f5c518; }
	.hero-tagline { margin: 10px 0 0; max-width: 40ch; color: rgb(255 255 255 / 88%); font: italic 500 clamp(15px, 1.3vw, 18px)/1.35 var(--font-ui); text-shadow: 0 1px 3px rgb(0 0 0 / 55%), 0 0 18px rgb(0 0 0 / 45%); }
	.story { max-width: 72ch; margin: var(--s-5) 0 0; font: 400 16px/1.55 var(--font-ui); color: var(--text); }
	/* 44px keeps the member control a comfortable touch target */
	.status-line { margin-top: var(--s-4); display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; font: 500 15px/1.4 var(--font-ui); color: var(--text-muted); }
</style>

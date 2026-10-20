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

	export type RichMeta = { backdrop?: string; logo?: string; poster?: string; genres: string[]; runtime?: number; certification?: string; score?: number; overview?: string; year?: number; backdrops: string[]; videos: Video[]; moods?: string[]; quality?: string };
	import type { Video } from '../trailer.ts';
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
					return { backdrop: s(raw.backdrop), logo: s(raw.logo), poster: s(raw.poster), genres, runtime: n(raw.runtime_minutes) ?? n(raw.runtime), certification: rating, score: n(raw.vote_average) ?? n(raw.score), overview: s(raw.overview), year: n(raw.year), backdrops, videos, moods, quality: s(raw.quality) };
				})
				.catch(() => null);
			metaCache.set(key, p);
		}
		return p;
	}

</script>

<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import { viewTransitionName } from '../motion.ts';
	import { sized } from '../tmdb-image.ts';
	import Billboard from './Billboard.svelte';
	import ActionButton from './ActionButton.svelte';
	import { heroPictures } from '../trailer.ts';
	import { autoplayHere, loadUISettings, trailerSources } from '../autoplay.svelte.ts';
	import { openTheater } from '../theater.svelte.ts';
	import { api } from '../api.ts';

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
		actions: actionRow
	}: {
		title: string;
		/** A movie or show: the muted trailer and the Trailer button. */
		kind?: 'movie' | 'show';
		tmdb?: string | null;
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
	} = $props();

	let more = $state(false);
	let text = $state<HTMLElement>();
	let clamped = $state(false);
	let backdropFailed = $state(false);
	let logoFailed = $state(false);
	let posterFailed = $state(false);

	$effect(() => {
		void description;
		if (!text) return;
		const check = () => text && (clamped = more || text.scrollHeight > text.clientHeight + 1);
		check();
		const ro = new ResizeObserver(check);
		ro.observe(text);
		return () => ro.disconnect();
	});

	const wide = $derived(backdrop && !backdropFailed && !cover ? backdrop : null);
	const pictures = $derived(wide ? heroPictures(wide, backdrops).map((u) => sized(u, 'backdrop')) : []);
	// No backdrop: the poster (or the cover) itself, cropped and darkened.
	const stand = $derived(cover ?? (!wide && poster && !posterFailed ? poster : null));

	const scoreText = $derived(score ? (Math.round(score * 10) / 10).toFixed(1) : '');
	const crumbHome = $derived(breadcrumb[0]);
	const crumbRest = $derived(breadcrumb.slice(1));

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
	async function playTrailer(e: MouseEvent) {
		if (!kind || !tmdb || opening) return;
		opening = true;
		const r = (e.currentTarget as HTMLElement).closest('section')?.getBoundingClientRect();
		try {
			const s = await trailerSources(kind, tmdb, videos);
			if (s[0]) openTheater({ kind, tmdb, title, source: s[0], time: 0, rect: r ? { left: r.left, top: r.top, width: r.width, height: r.height } : null, from: 'hero' });
		} finally {
			opening = false;
		}
	}
	// No autoplay on this device (phone, reduced motion, data saver, setting off): the Trailer button shows its word.
	let still = $state(true);
	onMount(() => void loadUISettings().then(() => (still = !autoplayHere())));
</script>

<div class="ui-thero" class:square>
	<Billboard label={title} testid="title-hero" {kind} {tmdb} {title} {videos} {pictures} {vtName} {settled} reveal position="70% 30%" onpictureerror={() => (backdropFailed = true)} art={stand ? standArt : undefined}>
		{#snippet top()}
			{#if crumbHome}
				<nav class="crumbs" aria-label="Breadcrumb">
					<ol>
						<li><a class="chip" href={crumbHome.href} aria-current={crumbRest.length ? undefined : 'page'}><Icon name={crumbIcon} size={14} />{crumbHome.label}</a></li>
						{#each crumbRest as c, i (i)}
							<li>
								<span class="sep" aria-hidden="true">›</span>
								<a class="here" href={c.href} aria-current={i === crumbRest.length - 1 ? 'page' : undefined}>{c.label}</a>
							</li>
						{/each}
					</ol>
				</nav>
			{/if}
		{/snippet}
		{#snippet heading()}
			{#if cover}
				<img class="cover" src={cover} alt="" decoding="async" use:viewTransitionName={vtName} />
			{/if}
			{#if logo && !logoFailed}
				<h1 class="bb-logo"><img src={logo} alt={title} onerror={() => (logoFailed = true)} /></h1>
			{:else}
				<h1 class="bb-name" class:short={!!cover}>{title}</h1>
			{/if}
		{/snippet}
		{#snippet meta()}
			{#if facts.length}<p>{facts.join(' · ')}</p>{/if}
			{#if certification}<span class="b cert" title="Age rating">{certification}</span>{/if}
			{#if scoreText}<span class="b score" title="Score out of 10"><svg width="11" height="11" viewBox="0 0 12 12" aria-hidden="true"><path d="M6 .8l1.6 3.3 3.6.5-2.6 2.5.6 3.6L6 9l-3.2 1.7.6-3.6L.8 4.6l3.6-.5z" fill="currentColor" /></svg>{scoreText}</span>{/if}
			{#each badges as b (b)}<span class="b">{b}</span>{/each}
			{#if status}<StatusPill kind={status} {percent} label={statusLabel} on="art" />{/if}
		{/snippet}
		{#snippet fold()}
			{#if description}
				<div class="about">
					<p class="bb-desc" class:open={more} bind:this={text}>{description}</p>
					{#if clamped}<button class="more" type="button" aria-expanded={more} onclick={() => (more = !more)}>{more ? 'Show less' : 'Show more'}</button>{/if}
				</div>
			{/if}
		{/snippet}
		{#snippet actions()}
			{@render actionRow?.()}
			{#if kind && tmdb && videos?.length}
				<ActionButton icon="play" label="Play the trailer" caption="Trailer" pill={still} onclick={playTrailer} busy={opening} data-testid="hero-trailer" />
			{/if}
		{/snippet}
	</Billboard>
	{#if byline}<div class="status-line">{@render byline()}</div>{/if}
</div>

{#snippet standArt()}
	<img class="bd" class:blur={!!cover} class:square src={sized(stand!, 'poster')} alt="" decoding="async" fetchpriority="high" use:viewTransitionName={cover ? undefined : vtName} onerror={() => (posterFailed = true)} />
{/snippet}

<style>
	.bd { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 20%; filter: saturate(1.1) brightness(.75); }
	.bd.square { object-position: center 35%; }
	.bd.blur { filter: blur(40px) saturate(1.3) brightness(.6); transform: scale(1.2); }
	.cover { display: block; width: clamp(120px, 14vw, 220px); aspect-ratio: 1; object-fit: cover; border-radius: 12px; box-shadow: 0 12px 40px rgb(0 0 0 / 50%); margin-bottom: 12px; }
	.ui-thero :global(.bb-name.short) { min-height: 0; }
	.crumbs ol { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; list-style: none; margin: 0; padding: 0; font: 600 13px/1.2 var(--font-ui); }
	.crumbs li { display: inline-flex; align-items: center; gap: 8px; min-width: 0; }
	.chip { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 12px 0 10px; border-radius: 999px; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 18%); backdrop-filter: blur(18px) saturate(160%); -webkit-backdrop-filter: blur(18px) saturate(160%); color: #fff; transition: background var(--dur-fast) var(--ease-out); }
	.chip:hover { background: rgb(255 255 255 / 24%); }
	.sep { color: rgb(255 255 255 / 60%); }
	.here { color: rgb(255 255 255 / 86%); min-height: 30px; display: inline-flex; align-items: center; overflow-wrap: anywhere; text-shadow: 0 1px 2px rgb(0 0 0 / 60%); }
	.here:hover { color: #fff; text-decoration: underline; text-underline-offset: 3px; }
	.chip:focus-visible, .here:focus-visible, .more:focus-visible { outline: none; box-shadow: 0 0 0 2px #fff; border-radius: 999px; }
	.b { display: inline-flex; align-items: center; gap: 4px; height: 22px; padding: 0 7px; border-radius: 5px; border: 1px solid rgb(255 255 255 / 55%); color: rgb(255 255 255 / 92%); font: 700 11.5px/1 var(--font-ui); letter-spacing: .03em; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.cert { border-width: 1.5px; border-color: rgb(255 255 255 / 80%); }
	.score { border-radius: 999px; padding: 0 9px 0 7px; background: rgb(255 255 255 / 12%); border-color: rgb(255 255 255 / 30%); }
	.score svg { color: #f5c518; }
	.about { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; }
	.more { min-height: 32px; padding: 0 2px; color: #fff; font: 600 14px/1 var(--font-ui); opacity: .85; transition: opacity var(--dur-fast) var(--ease-out); }
	.more:hover { opacity: 1; text-decoration: underline; text-underline-offset: 3px; }
	.ui-thero :global(.tp-member) { min-height: 44px; display: flex; align-items: center; }
	.status-line { margin-top: var(--s-4); display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; font: 500 15px/1.4 var(--font-ui); color: var(--text-muted); }
</style>

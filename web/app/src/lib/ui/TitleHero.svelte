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

	export type RichMeta = { backdrop?: string; logo?: string; poster?: string; genres: string[]; runtime?: number; certification?: string; score?: number; overview?: string };
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
					return { backdrop: s(raw.backdrop), logo: s(raw.logo), poster: s(raw.poster), genres, runtime: n(raw.runtime_minutes) ?? n(raw.runtime), certification: rating, score: n(raw.vote_average) ?? n(raw.score), overview: s(raw.overview) };
				})
				.catch(() => null);
			metaCache.set(key, p);
		}
		return p;
	}

</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import { viewTransitionName } from '../motion.ts';

	// The title page hero (movie, show, artist, not-yet-added titles), in the
	// style of a TV app detail page: full-bleed backdrop with a strong left
	// gradient, breadcrumb chip, logo or large title, meta line with the
	// status, outlined badges, a 3-line description and an action row of
	// pills. With no backdrop the poster itself is cropped
	// full-bleed and darkened. The hero is always dark (it is an image
	// area), in both themes.
	let {
		title,
		backdrop,
		poster,
		logo,
		meta = [],
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
		actions
	}: {
		title: string;
		backdrop?: string | null;
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

	const wide = $derived(backdrop && !backdropFailed ? backdrop : null);
	// No backdrop: the poster itself, cropped full-bleed and darkened.
	const stand = $derived(!wide && poster && !posterFailed ? poster : null);

	const scoreText = $derived(score ? (Math.round(score * 10) / 10).toFixed(1) : '');
	const hasBadges = $derived(!!certification || !!scoreText || badges.length > 0);
	const crumbHome = $derived(breadcrumb[0]);
	const crumbRest = $derived(breadcrumb.slice(1));
</script>

<section class="ui-thero" class:stand={!!stand} class:plain={!wide && !stand} aria-label={title}>
	<div class="art" aria-hidden="true">
		{#if wide}
			<img class="bd" src={wide} alt="" decoding="async" fetchpriority="high" use:viewTransitionName={vtName} onerror={() => (backdropFailed = true)} />
		{:else if stand}
			<img class="bd from-poster" class:square src={stand} alt="" decoding="async" fetchpriority="high" use:viewTransitionName={vtName} onerror={() => (posterFailed = true)} />
		{/if}
		<div class="shade"></div>
	</div>

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

	<div class="content">
		{#if logo && !logoFailed}
			<h1 class="logo"><img src={logo} alt={title} onerror={() => (logoFailed = true)} /></h1>
		{:else}
			<h1 class="name">{title}</h1>
		{/if}
		{#if meta.length || status}
			<div class="meta">
				{#if meta.length}<p>{meta.join(' · ')}</p>{/if}
				{#if status}<StatusPill kind={status} {percent} label={statusLabel} on="art" />{/if}
			</div>
		{/if}
		{#if hasBadges}
			<ul class="badges" aria-label="Rating and quality">
				{#if certification}<li class="b cert" title="Age rating">{certification}</li>{/if}
				{#if scoreText}<li class="b score" title="Score out of 10"><svg width="11" height="11" viewBox="0 0 12 12" aria-hidden="true"><path d="M6 .8l1.6 3.3 3.6.5-2.6 2.5.6 3.6L6 9l-3.2 1.7.6-3.6L.8 4.6l3.6-.5z" fill="currentColor" /></svg>{scoreText}</li>{/if}
				{#each badges as b (b)}<li class="b">{b}</li>{/each}
			</ul>
		{/if}
		{#if description}
			<div class="about">
				<p class="desc" class:open={more} bind:this={text}>{description}</p>
				{#if clamped}<button class="more" type="button" aria-expanded={more} onclick={() => (more = !more)}>{more ? 'Show less' : 'Show more'}</button>{/if}
			</div>
		{/if}
		{#if actions}<div class="actions">{@render actions()}</div>{/if}
	</div>
</section>

<style>
	.ui-thero {
		--hero-ink: #0b0b0c;
		position: relative; isolation: isolate; color: #fff; background: var(--hero-ink);
		margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) 0;
		height: clamp(460px, 62vh, 720px);
		display: flex; flex-direction: column; justify-content: flex-end;
	}
	.art { position: absolute; inset: 0; z-index: -2; overflow: hidden; background: radial-gradient(110% 90% at 80% 20%, #2a2c33 0%, transparent 60%), linear-gradient(160deg, #1d1f25 0%, #0b0b0c 75%); }
	.bd { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: 70% 30%; animation: hero-in var(--dur-page, 420ms) var(--ease-out) both; }
	.shade {
		position: absolute; inset: 0;
		background:
			linear-gradient(to right, rgb(11 11 12 / 92%) 0%, rgb(11 11 12 / 78%) 26%, rgb(11 11 12 / 38%) 52%, rgb(11 11 12 / 0%) 74%),
			linear-gradient(to top, var(--hero-ink) 0%, rgb(11 11 12 / 70%) 16%, rgb(11 11 12 / 0%) 44%),
			linear-gradient(to bottom, rgb(11 11 12 / 45%) 0%, rgb(11 11 12 / 0%) 18%);
	}
	.stand .shade { background: linear-gradient(to right, rgb(11 11 12 / 96%) 0%, rgb(11 11 12 / 86%) 30%, rgb(11 11 12 / 55%) 58%, rgb(11 11 12 / 30%) 100%), linear-gradient(to top, var(--hero-ink) 0%, rgb(11 11 12 / 75%) 22%, rgb(11 11 12 / 0%) 55%), linear-gradient(to bottom, rgb(11 11 12 / 55%) 0%, rgb(11 11 12 / 0%) 22%); }
	.from-poster { object-position: center 20%; filter: saturate(1.1) brightness(.8); }
	.from-poster.square { object-position: center 35%; }

	.crumbs { position: absolute; top: clamp(16px, 2.4vw, 28px); left: clamp(20px, 4vw, 56px); right: 16px; z-index: 1; }
	.crumbs ol { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; list-style: none; margin: 0; padding: 0; font: 600 13px/1.2 var(--font-ui); }
	.crumbs li { display: inline-flex; align-items: center; gap: 8px; min-width: 0; }
	.chip { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 12px 0 10px; border-radius: 999px; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 18%); backdrop-filter: blur(18px) saturate(160%); -webkit-backdrop-filter: blur(18px) saturate(160%); color: #fff; transition: background var(--dur-fast) var(--ease-out); }
	.chip:hover { background: rgb(255 255 255 / 24%); }
	.sep { color: rgb(255 255 255 / 60%); }
	.here { color: rgb(255 255 255 / 86%); min-height: 30px; display: inline-flex; align-items: center; overflow-wrap: anywhere; }
	.here:hover { color: #fff; text-decoration: underline; text-underline-offset: 3px; }
	.chip:focus-visible, .here:focus-visible, .more:focus-visible { outline: none; box-shadow: 0 0 0 2px #fff; border-radius: 999px; }

	.content { position: relative; width: min(600px, 100%); padding: 0 clamp(20px, 4vw, 56px) clamp(28px, 4vw, 52px); display: flex; flex-direction: column; gap: 12px; }
	h1 { margin: 0; }
	.name { font-family: var(--font-display, var(--font-ui)); font-weight: 800; font-size: clamp(44px, 6vw, 84px); line-height: .98; letter-spacing: -0.035em; text-wrap: balance; overflow-wrap: anywhere; text-shadow: 0 2px 30px rgb(0 0 0 / 35%); }
	.logo img { display: block; max-width: min(520px, 100%); max-height: 180px; width: auto; height: auto; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 4px 24px rgb(0 0 0 / 45%)); }
	.meta { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 12px; }
	.meta p { margin: 0; font: 600 15px/1.35 var(--font-ui); color: rgb(255 255 255 / 86%); font-variant-numeric: tabular-nums; }
	.badges { display: flex; flex-wrap: wrap; gap: 6px; list-style: none; margin: 0; padding: 0; }
	.b { display: inline-flex; align-items: center; gap: 4px; height: 22px; padding: 0 7px; border-radius: 5px; border: 1px solid rgb(255 255 255 / 55%); color: rgb(255 255 255 / 92%); font: 700 11.5px/1 var(--font-ui); letter-spacing: .03em; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.cert { border-width: 1.5px; border-color: rgb(255 255 255 / 80%); }
	.score { border-radius: 999px; padding: 0 9px 0 7px; background: rgb(255 255 255 / 12%); border-color: rgb(255 255 255 / 30%); }
	.score svg { color: #f5c518; }
	.about { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; }
	.desc { margin: 0; font: 400 16px/1.5 var(--font-ui); color: rgb(255 255 255 / 88%); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; max-width: 60ch; text-shadow: 0 1px 12px rgb(0 0 0 / 30%); }
	.desc.open { -webkit-line-clamp: unset; line-clamp: unset; display: block; }
	.more { min-height: 32px; padding: 0 2px; color: #fff; font: 600 14px/1 var(--font-ui); opacity: .85; transition: opacity var(--dur-fast) var(--ease-out); }
	.more:hover { opacity: 1; text-decoration: underline; text-underline-offset: 3px; }
	.actions { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-top: 6px; }
	.actions :global(.tp-member) { min-height: 44px; display: flex; align-items: center; }

	@keyframes hero-in { from { opacity: 0; scale: 1.03; } }
	@media (prefers-reduced-motion: reduce) { .bd { animation: none; } }

	@media (max-width: 1023px) {
		.ui-thero { height: clamp(440px, 52vh, 620px); }
	}
	@media (max-width: 767px) {
		.ui-thero { margin-top: calc(var(--s-4) * -1); height: auto; display: block; padding-bottom: 28px; }
		.art { position: relative; inset: auto; aspect-ratio: 16 / 9; }
		.plain .art { aspect-ratio: auto; height: 72px; }
		.shade { background: linear-gradient(to top, var(--hero-ink) 0%, rgb(11 11 12 / 55%) 26%, rgb(11 11 12 / 0%) 60%), linear-gradient(to bottom, rgb(11 11 12 / 70%) 0%, rgb(11 11 12 / 0%) 34%); }
		.stand .shade { background: linear-gradient(to top, var(--hero-ink) 0%, rgb(11 11 12 / 70%) 30%, rgb(11 11 12 / 25%) 65%), linear-gradient(to bottom, rgb(11 11 12 / 60%) 0%, rgb(11 11 12 / 0%) 30%); }
		.crumbs { left: var(--gutter); top: 14px; }
		.content { width: 100%; padding: 0 var(--gutter); margin-top: -28px; gap: 10px; }
		.plain .content { margin-top: 0; }
		.name { font-size: clamp(34px, 10vw, 48px); }
		.logo img { max-width: min(320px, 85%); max-height: 110px; }
		.actions { gap: 8px; }
		.actions :global(.ui-act.primary) { flex: 1 1 100%; }
		.about { order: 10; }
	}
</style>

<!-- Poster card for a library title: lazy fade-in, status bar, hover details and tilt. -->
<script lang="ts" module>
	// One observer shared by every poster; each card registers its own reveal callback.
	let io: IntersectionObserver | undefined;
	const pending = new Map<Element, () => void>();
	// Staggers cards that appear together, capped so a big batch does not drag on.
	function watch(node: Element, show: () => void): () => void {
		if (typeof IntersectionObserver === 'undefined') return void show(), () => {};
		io ??= new IntersectionObserver((entries) => {
			let n = 0;
			for (const e of entries) {
				if (!e.isIntersecting) continue;
				const run = pending.get(e.target);
				pending.delete(e.target);
				io?.unobserve(e.target);
				(e.target as HTMLElement).style.setProperty('--rise-delay', `${Math.min(n++, 11) * 30}ms`);
				run?.();
			}
		}, { rootMargin: '0px 0px -4% 0px' });
		pending.set(node, show);
		io.observe(node);
		return () => {
			pending.delete(node);
			io?.unobserve(node);
		};
	}
</script>

<script lang="ts">
	import { onMount } from 'svelte';
	import Icon, { type IconName } from '#lib/ui/Icon.svelte';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import { whenLoaded } from '#lib/ui/lazy-art.ts';
	import { tiltFor, type BarTone, type PosterDetail } from './poster.ts';
	import type { LibState } from './library-state.ts';
	import { reducedMotion, viewTransitionName } from '../core/motion.ts';
	import { sized } from '../core/tmdb-image.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let {
		title,
		year,
		href,
		image,
		kind = 'movie',
		vtName,
		status: ls,
		attention = false,
		bar,
		detail = 'status',
		extra = '',
		facts = [],
		score = '',
		kept,
		canManage = false,
		selecting = false,
		selected = false,
		onsearch,
		onkeep
	}: {
		title: string;
		year?: number | string;
		href: string;
		image?: string | null;
		kind?: 'movie' | 'show' | 'artist';
		vtName?: string;
		status: LibState | null;

		attention?: boolean;
		bar: { tone: BarTone; fill: number };
		detail?: PosterDetail;

		extra?: string;

		facts?: string[];
		// Shown on the art while the Library is sorted by score.
		score?: string;
		kept: boolean;
		canManage?: boolean;
		selecting?: boolean;
		selected?: boolean;
		onsearch?: () => void;
		onkeep?: (keep: boolean) => void;
	} = $props();

	let root = $state<HTMLElement>();
	let frame = $state<HTMLElement>();
	let shown = $state(false);
	let loaded = $state(false);
	let failed = $state(false);
	let cw = $state(0);
	let touch = $state(false);
	let raf = 0;
	const pic = $derived(sized(image, 'poster', cw || 200));
	const kindIcon = $derived<IconName>(kind === 'show' ? 'tv' : kind === 'artist' ? 'music' : 'film');
	const words = $derived(ls?.label ?? (bar.tone === 'ok' ? t('mediafiles.poster.inLibrary') : ''));
	const name = $derived([title, year, words, score && `${t('mediafiles.col.score')} ${score}`].filter(Boolean).join(', '));

	$effect(() => {
		if (!root) return;
		if (reducedMotion()) return void (shown = true);
		return watch(root, () => (shown = true));
	});
	$effect(() => () => cancelAnimationFrame(raf));

	// Touch screens get no hover panel, so skip rendering it.
	onMount(() => void (touch = matchMedia('(hover: none)').matches));

	// Tilt only for a real mouse and when motion is welcome.
	const canTilt = () => typeof matchMedia !== 'undefined' && matchMedia('(hover: hover) and (pointer: fine)').matches && !reducedMotion();
	// Redraw once per frame so pointer events do not queue style writes.
	function move(e: PointerEvent) {
		if (!frame || e.pointerType !== 'mouse' || selecting || !canTilt()) return;
		const r = frame.getBoundingClientRect();
		const { rx, ry } = tiltFor(e.clientX - r.left, e.clientY - r.top, r.width, r.height, 4);
		cancelAnimationFrame(raf);
		raf = requestAnimationFrame(() => {
			frame?.style.setProperty('--rx', `${rx}deg`);
			frame?.style.setProperty('--ry', `${ry}deg`);
		});
	}
	function reset() {
		cancelAnimationFrame(raf);
		frame?.style.setProperty('--rx', '0deg');
		frame?.style.setProperty('--ry', '0deg');
	}
</script>

<div class="lp" class:square={kind === 'artist'} class:shown class:selecting class:selected class:attention bind:this={root} bind:clientWidth={cw}>
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="frame" bind:this={frame} onpointermove={move} onpointerleave={reset}>
		<a class="art" {href} aria-label={name} use:viewTransitionName={vtName}>
			{#if !image || failed || !loaded}
				<ArtPlaceholder name={title} shape={kind === 'artist' ? 'square' : 'poster'} loading={!!image && !failed && !loaded} />
			{/if}
			{#if image && !failed}
				<img src={pic} class:in={loaded} alt="" loading="lazy" decoding="async" use:whenLoaded={() => (loaded = true)} onerror={() => (failed = true)} />
			{/if}
			{#if attention && ls}
				<span class="badge {bar.tone}" aria-hidden="true"><Icon name={ls.icon} size={12} stroke={2.25} />{#if ls.percent !== undefined && bar.tone === 'busy'}<b>{ls.percent}%</b>{/if}</span>
			{/if}
			<!-- While picking, the pick dot takes this corner. -->
			{#if score && !selecting}
				<span class="badge score" aria-hidden="true"><svg width="10" height="10" viewBox="0 0 12 12"><path d="M6 .8l1.6 3.3 3.6.5-2.6 2.5.6 3.6L6 9l-3.2 1.7.6-3.6L.8 4.6l3.6-.5z" fill="currentColor" /></svg><b>{score}</b></span>
			{/if}
			<span class="line {bar.tone}" aria-hidden="true"><i style="transform: scaleX({bar.fill})"></i></span>
		</a>
		{#if !touch}
		<div class="glass" aria-label={t('mediafiles.poster.details', { title })} role="group">
			<p class="g-state {bar.tone}">{words || t('mediafiles.poster.inLibrary')}</p>
			{#each facts.filter(Boolean) as f (f)}<p class="g-fact">{f}</p>{/each}
			<div class="g-acts">
				{#if canManage}
					<button type="button" class="g-btn" onclick={() => onsearch?.()} aria-label={t('mediafiles.poster.searchFor', { title })} title={t('mediafiles.select.searchNow')}><Icon name="search" size={15} /></button>
					<button type="button" class="g-btn" class:on={kept} aria-pressed={kept} onclick={() => onkeep?.(!kept)} aria-label={t('mediafiles.poster.keepFor', { title })} title={kept ? t('mediafiles.poster.kept') : t('mediafiles.poster.notKept')}><Icon name={kept ? 'bellOn' : 'bell'} size={15} /></button>
				{/if}
				<a class="g-open" {href} tabindex="-1">{t('mediafiles.poster.open')}<Icon name="chevronRight" size={14} /></a>
			</div>
		</div>
		{/if}
	</div>
	<p class="cap"><span class="t" title={title}>{title}</span>{#if year}<span class="y">{year}</span>{/if}</p>
	{#if detail !== 'title' && attention && ls}<p class="sub {bar.tone}">{ls.label}</p>{/if}
	{#if detail === 'full' && extra}<p class="sub">{extra}</p>{/if}
</div>

<style>
	.lp { --rx: 0deg; --ry: 0deg; position: relative; min-width: 0; display: flex; flex-direction: column; gap: 3px; opacity: 0; transform: translateY(12px); }
	.lp.shown { opacity: 1; transform: none; transition: opacity var(--dur) var(--ease-out) var(--rise-delay, 0ms), transform var(--dur) var(--ease-out) var(--rise-delay, 0ms); }


	.lp.square { --poster-ratio: 1 / 1; }
	.frame { container-type: inline-size; position: relative; aspect-ratio: var(--poster-ratio); margin-bottom: 6px; border-radius: var(--r-card); transform: perspective(800px) translateY(var(--ty, 0)) scale(var(--s, 1)) rotateX(var(--rx)) rotateY(var(--ry)); transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); box-shadow: var(--shadow-card); will-change: transform; }
	.art { position: absolute; inset: 0; display: block; overflow: hidden; border-radius: inherit; background: var(--surface-2); color: var(--text); }
	.art:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity var(--dur) var(--ease-out); }
	img.in { opacity: 1; }


	.badge { position: absolute; top: 7px; right: 7px; display: inline-flex; align-items: center; gap: 3px; height: 22px; min-width: 22px; padding: 0 6px; box-sizing: border-box; justify-content: center; border-radius: 99px; font-weight: 600; font-size: 11px; line-height: 1; font-family: var(--font-text, inherit); font-variant-numeric: tabular-nums; color: #fff; background: rgb(20 20 22 / 72%); -webkit-backdrop-filter: blur(8px) saturate(160%); backdrop-filter: blur(8px) saturate(160%); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 14%); }
	.badge.fail { color: var(--fail-on-art); }
	.badge.busy { color: var(--info-on-art); }
	.badge.gap { color: var(--wait-on-art, #ffb340); }
	.badge.idle { color: rgb(255 255 255 / 82%); }
	.badge b { color: #fff; font-weight: 600; }
	.badge.score { right: auto; left: 7px; padding: 0 7px 0 6px; color: #f5c518; }


	.line { position: absolute; left: 0; right: 0; bottom: 0; height: 3px; background: rgb(0 0 0 / 38%); }
	.line i { position: absolute; inset: 0; transform-origin: left; background: var(--ok-on-art); transition: transform var(--dur) var(--ease-out); }
	.line.busy i { background: var(--info-on-art); }
	.line.gap i { background: var(--wait-on-art, #ffb340); }
	.line.gap { background: color-mix(in srgb, var(--fail-on-art) 45%, rgb(0 0 0 / 40%)); }
	.line.fail i { background: var(--fail-on-art); }
	.line.idle i { background: rgb(255 255 255 / 55%); }


	/* Frosted hover panel; the --gp vars carry the light and dark colours. */
	.glass { --gp-bg: rgb(244 244 246 / 86%); --gp-fg: #1d1d1f; --gp-fg2: rgb(60 60 67); --gp-btn: rgb(0 0 0 / 8%); --gp-btn-hi: rgb(0 0 0 / 14%); --gp-edge: rgb(255 255 255 / 60%); position: absolute; left: 0; right: 0; bottom: 0; min-height: 34%; display: flex; flex-direction: column; justify-content: flex-end; gap: 2px; padding: clamp(7px, 6cqi, 10px) clamp(7px, 6cqi, 10px) clamp(6px, 5.5cqi, 9px); box-sizing: border-box; border-radius: 0 0 var(--r-card) var(--r-card); color: var(--gp-fg); background: var(--gp-bg); -webkit-backdrop-filter: blur(30px) saturate(180%); backdrop-filter: blur(30px) saturate(180%); box-shadow: inset 0 1px 0 var(--gp-edge); opacity: 0; pointer-events: none; transition: opacity var(--dur-fast) var(--ease-in); }
	@media (prefers-color-scheme: dark) { :global(:root:not([data-theme='light'])) .glass { --gp-bg: rgb(28 28 30 / 80%); --gp-fg: #fff; --gp-fg2: rgb(235 235 245 / 82%); --gp-btn: rgb(255 255 255 / 14%); --gp-btn-hi: rgb(255 255 255 / 24%); --gp-edge: rgb(255 255 255 / 12%); } }
	:global([data-theme='dark']) .glass { --gp-bg: rgb(28 28 30 / 80%); --gp-fg: #fff; --gp-fg2: rgb(235 235 245 / 82%); --gp-btn: rgb(255 255 255 / 14%); --gp-btn-hi: rgb(255 255 255 / 24%); --gp-edge: rgb(255 255 255 / 12%); }


	.g-state { display: flex; align-items: center; gap: 5px; margin: 0; font-weight: 600; font-size: clamp(10px, 7.4cqi, 12px); line-height: 1.3; font-family: var(--font-text, inherit); color: var(--gp-fg); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.g-state::before { content: ''; flex: none; width: 6px; height: 6px; border-radius: 99px; background: var(--ok-text); }
	.g-state.busy::before { background: var(--info-text); }
	.g-state.gap::before { background: var(--wait-text); }
	.g-state.fail::before { background: var(--fail-text); }
	.g-state.idle::before { background: var(--gp-fg2); }
	.g-fact { margin: 0; font-weight: 400; font-size: clamp(9.5px, 6.8cqi, 11px); line-height: 1.35; font-family: var(--font-text, inherit); color: var(--gp-fg2); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

	@container (max-width: 200px) { .lp.square .g-fact:nth-child(n + 4) { display: none; } }
	.g-acts { display: flex; align-items: center; gap: clamp(4px, 3.6cqi, 6px); margin-top: clamp(4px, 3.6cqi, 6px); min-width: 0; }
	.g-btn, .g-open { display: inline-grid; place-items: center; height: clamp(24px, 18cqi, 28px); border: 0; border-radius: 99px; color: var(--gp-fg); background: var(--gp-btn); cursor: pointer; transition: background var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.g-btn { width: clamp(24px, 18cqi, 28px); flex: none; padding: 0; }
	.g-btn.on { color: var(--ok-text); }
	.g-open { margin-left: auto; display: inline-flex; gap: 2px; padding: 0 clamp(5px, 4.5cqi, 8px) 0 clamp(7px, 6.5cqi, 11px); font-weight: 600; font-size: clamp(10px, 7.4cqi, 12px); line-height: 1; font-family: var(--font-text, inherit); text-decoration: none; flex: none; }
	.g-btn:hover, .g-open:hover { background: var(--gp-btn-hi); }
	.g-btn:active, .g-open:active { transform: scale(0.94); }
	.g-btn:focus-visible, .g-open:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	@media (hover: hover) and (pointer: fine) {
		.frame:hover, .frame:focus-within { --ty: -4px; --s: 1.03; box-shadow: var(--shadow-lift); z-index: 2; }
		.frame:hover .glass, .frame:focus-within .glass { opacity: 1; pointer-events: auto; transition: opacity var(--dur) var(--ease-out); }
		.frame:hover .badge, .frame:focus-within .badge { opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); }
	}

	/* Keyboard focus should not leave the card tilted. */
	.frame:focus-within:not(:hover) { --rx: 0deg !important; --ry: 0deg !important; }
	.badge { transition: opacity var(--dur-fast) var(--ease-out); }


	.lp.selecting .frame { --s: 0.95; }
	.lp.selecting .glass { display: none; }
	.lp.selected .frame { --s: 0.92; }

	.cap { margin: 0; display: flex; align-items: baseline; gap: 6px; min-width: 0; font: var(--t-subhead); font-size: 14px; }
	.t { min-width: 0; font-weight: 600; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.y { flex: none; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.sub { margin: 0; color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sub.fail { color: var(--fail-text); }
	.sub.busy { color: var(--info-text); }
	.sub.gap { color: var(--wait-text); }

	@media (prefers-reduced-transparency: reduce) {
		.badge { backdrop-filter: none; -webkit-backdrop-filter: none; background: rgb(24 24 26 / 94%); }
		.glass { backdrop-filter: none; -webkit-backdrop-filter: none; }
	}
	@media (prefers-reduced-motion: reduce) {
		.lp, .lp.shown { opacity: 1; transform: none; transition: none; }
		.frame, .glass, .line i, .badge, img { transition: none !important; }
		.frame { --rx: 0deg !important; --ry: 0deg !important; }
	}
</style>

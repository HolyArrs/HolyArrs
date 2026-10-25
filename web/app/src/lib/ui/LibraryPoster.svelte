<script lang="ts" module>
	// One observer for every Library poster: posters that enter the screen in the
	// same frame rise in together, 30 ms apart, once each.
	let io: IntersectionObserver | undefined;
	const pending = new Map<Element, () => void>();
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
	import Icon, { type IconName } from '../Icon.svelte';
	import ArtPlaceholder from './ArtPlaceholder.svelte';
	import { reducedMotion, viewTransitionName } from '../motion.ts';
	import { sized } from '../tmdb-image.ts';
	import { whenLoaded } from '../lazy-art.ts';
	import { tiltFor, type BarTone, type PosterDetail } from '../library-poster.ts';
	import type { LibState } from '../library-state.ts';

	// The Library's own poster: the art, one caption line, and a health line on
	// the art's foot. A corner badge and words only when the title needs a look.
	// Hover or keyboard focus lifts it and slides a glass panel over the lower third.
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
		/** The state earns a badge and words at rest. */
		attention?: boolean;
		bar: { tone: BarTone; fill: number };
		detail?: PosterDetail;
		/** The "Everything" line: pick rules and quality, or the next airing. */
		extra?: string;
		/** Hover panel lines, in order: quality and rules, added, asked by. */
		facts?: string[];
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
	const pic = $derived(sized(image, 'poster', cw || 200));
	const kindIcon = $derived<IconName>(kind === 'show' ? 'tv' : kind === 'artist' ? 'music' : 'film');
	const words = $derived(ls?.label ?? (bar.tone === 'ok' ? 'In the Library' : ''));
	const name = $derived([title, year, words].filter(Boolean).join(', '));

	$effect(() => {
		if (!root) return;
		if (reducedMotion()) return void (shown = true);
		return watch(root, () => (shown = true));
	});

	// Tilt toward the pointer: fine pointers only, never for keyboard focus or
	// reduced motion. One write per frame, straight to the style (no re-render).
	let raf = 0;
	const canTilt = () => typeof matchMedia !== 'undefined' && matchMedia('(hover: hover) and (pointer: fine)').matches && !reducedMotion();
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
	$effect(() => () => cancelAnimationFrame(raf));
</script>

<div class="lp" class:shown class:selecting class:selected class:attention bind:this={root} bind:clientWidth={cw}>
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="frame" bind:this={frame} onpointermove={move} onpointerleave={reset}>
		<a class="art" {href} aria-label={name} use:viewTransitionName={vtName}>
			{#if !image || failed || !loaded}
				<ArtPlaceholder name={title} shape="poster" loading={!!image && !failed && !loaded} />
			{/if}
			{#if image && !failed}
				<img src={pic} class:in={loaded} alt="" loading="lazy" decoding="async" use:whenLoaded={() => (loaded = true)} onerror={() => (failed = true)} />
			{/if}
			{#if attention && ls}
				<span class="badge {bar.tone}" aria-hidden="true"><Icon name={ls.icon} size={12} stroke={2.25} />{#if ls.percent !== undefined && bar.tone === 'busy'}<b>{ls.percent}%</b>{/if}</span>
			{/if}
			<span class="line {bar.tone}" aria-hidden="true"><i style="transform: scaleX({bar.fill})"></i></span>
		</a>
		<div class="glass" aria-label="{title} details" role="group">
			<p class="g-state {bar.tone}">{words || 'In the Library'}</p>
			{#each facts.filter(Boolean) as f (f)}<p class="g-fact">{f}</p>{/each}
			<div class="g-acts">
				{#if canManage}
					<button type="button" class="g-btn" onclick={() => onsearch?.()} aria-label="Search now for {title}" title="Search now"><Icon name="search" size={15} /></button>
					<button type="button" class="g-btn" class:on={kept} aria-pressed={kept} onclick={() => onkeep?.(!kept)} aria-label="Keep {title} up to date" title={kept ? 'Kept up to date' : 'Not kept up to date'}><Icon name={kept ? 'bellOn' : 'bell'} size={15} /></button>
				{/if}
				<a class="g-open" {href} tabindex="-1">Open<Icon name="chevronRight" size={14} /></a>
			</div>
		</div>
	</div>
	<p class="cap"><span class="t" title={title}>{title}</span>{#if year}<span class="y">{year}</span>{/if}</p>
	{#if detail !== 'title' && attention && ls}<p class="sub {bar.tone}">{ls.label}</p>{/if}
	{#if detail === 'full' && extra}<p class="sub">{extra}</p>{/if}
</div>

<style>
	.lp { --rx: 0deg; --ry: 0deg; position: relative; min-width: 0; display: flex; flex-direction: column; gap: 3px; opacity: 0; transform: translateY(12px); }
	.lp.shown { opacity: 1; transform: none; transition: opacity 220ms var(--ease-out) var(--rise-delay, 0ms), transform 220ms var(--ease-out) var(--rise-delay, 0ms); }

	.frame { position: relative; aspect-ratio: var(--poster-ratio); margin-bottom: 6px; border-radius: var(--r-card); transform: perspective(800px) translateY(var(--ty, 0)) scale(var(--s, 1)) rotateX(var(--rx)) rotateY(var(--ry)); transition: transform 200ms var(--ease-out), box-shadow 200ms var(--ease-out); box-shadow: var(--shadow-card); will-change: transform; }
	.art { position: absolute; inset: 0; display: block; overflow: hidden; border-radius: inherit; background: var(--surface-2); color: var(--text); }
	.art:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity 220ms var(--ease-out); }
	img.in { opacity: 1; }

	/* Corner badge: only when the title needs a look. */
	.badge { position: absolute; top: 7px; right: 7px; display: inline-flex; align-items: center; gap: 3px; height: 22px; min-width: 22px; padding: 0 6px; box-sizing: border-box; justify-content: center; border-radius: 99px; font: 600 11px/1 var(--font-text, inherit); font-variant-numeric: tabular-nums; color: #fff; background: rgb(20 20 22 / 72%); backdrop-filter: blur(8px) saturate(160%); -webkit-backdrop-filter: blur(8px) saturate(160%); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 14%); }
	.badge.fail { color: var(--fail-on-art); }
	.badge.busy { color: var(--info-on-art); }
	.badge.gap { color: var(--wait-on-art, #ffb340); }
	.badge.idle { color: rgb(255 255 255 / 82%); }
	.badge b { color: #fff; font-weight: 600; }

	/* Health line on every poster's foot (scaled, never resized). */
	.line { position: absolute; left: 0; right: 0; bottom: 0; height: 3px; background: rgb(0 0 0 / 38%); }
	.line i { position: absolute; inset: 0; transform-origin: left; background: var(--ok-on-art); transition: transform 240ms var(--ease-out); }
	.line.busy i { background: var(--info-on-art); }
	.line.gap i { background: var(--wait-on-art, #ffb340); }
	.line.gap { background: color-mix(in srgb, var(--fail-on-art) 45%, rgb(0 0 0 / 40%)); }
	.line.fail i { background: var(--fail-on-art); }
	.line.idle i { background: rgb(255 255 255 / 55%); }

	/* Glass panel over the lower third. Enter 220 ms, leave 150 ms. */
	.glass { position: absolute; left: 0; right: 0; bottom: 0; min-height: 34%; display: flex; flex-direction: column; justify-content: flex-end; gap: 2px; padding: 10px 10px 9px; box-sizing: border-box; border-radius: 0 0 var(--r-card) var(--r-card); color: #fff; background: linear-gradient(to bottom, rgb(18 18 20 / 42%), rgb(18 18 20 / 70%)); backdrop-filter: blur(18px) saturate(170%); -webkit-backdrop-filter: blur(18px) saturate(170%); box-shadow: inset 0 1px 0 rgb(255 255 255 / 16%); opacity: 0; transform: translateY(14px); pointer-events: none; transition: opacity 150ms var(--ease-in), transform 150ms var(--ease-in); }
	.g-state { margin: 0; font: 600 12px/1.3 var(--font-text, inherit); color: var(--ok-on-art); }
	.g-state.busy { color: var(--info-on-art); }
	.g-state.gap { color: var(--wait-on-art, #ffb340); }
	.g-state.fail { color: var(--fail-on-art); }
	.g-state.idle { color: rgb(255 255 255 / 88%); }
	.g-fact { margin: 0; font: 400 11px/1.35 var(--font-text, inherit); color: rgb(255 255 255 / 80%); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.g-acts { display: flex; align-items: center; gap: 6px; margin-top: 6px; }
	.g-btn, .g-open { display: inline-grid; place-items: center; height: 28px; border: 0; border-radius: 99px; color: #fff; background: rgb(255 255 255 / 16%); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 18%); cursor: pointer; transition: background 150ms var(--ease-out), transform 150ms var(--ease-out); }
	.g-btn { width: 28px; padding: 0; }
	.g-btn.on { color: var(--ok-on-art); }
	.g-open { margin-left: auto; display: inline-flex; gap: 2px; padding: 0 8px 0 11px; font: 600 12px/1 var(--font-text, inherit); text-decoration: none; }
	.g-btn:hover, .g-open:hover { background: rgb(255 255 255 / 26%); }
	.g-btn:active, .g-open:active { transform: scale(0.94); }
	.g-btn:focus-visible, .g-open:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	@media (hover: hover) and (pointer: fine) {
		.frame:hover, .frame:focus-within { --ty: -4px; --s: 1.03; box-shadow: var(--shadow-lift); z-index: 2; }
		.frame:hover .glass, .frame:focus-within .glass { opacity: 1; transform: none; pointer-events: auto; transition: opacity 220ms var(--ease-out), transform 220ms var(--ease-out); }
		.frame:hover .badge, .frame:focus-within .badge { opacity: 0; transition: opacity 150ms var(--ease-out); }
	}
	/* Keyboard focus shows the panel with no tilt. */
	.frame:focus-within:not(:hover) { --rx: 0deg !important; --ry: 0deg !important; }
	.badge { transition: opacity 200ms var(--ease-out); }

	/* Select mode: posters step back a little; the page's check springs in. */
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
		.glass, .badge { backdrop-filter: none; -webkit-backdrop-filter: none; background: rgb(24 24 26 / 94%); }
	}
	@media (prefers-reduced-motion: reduce) {
		.lp, .lp.shown { opacity: 1; transform: none; transition: none; }
		.frame, .glass, .line i, .badge, img { transition: none !important; }
		.frame { --rx: 0deg !important; --ry: 0deg !important; }
	}
</style>

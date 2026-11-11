<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Hover card that grows out of a poster or tile and plays a trailer after a pause.
	import IconButton from './IconButton.svelte';
	import Icon, { type IconName } from './Icon.svelte';
	import TrailerPlayer from './TrailerPlayer.svelte';
	import { richMetadata, type RichMeta } from './TitleHero.svelte';
	import type { CardOwned } from './CardBadges.svelte';
	import { followHint, followLabel, followPath, movieFollowHint, setFollow, type FollowTarget } from './follow.ts';
	import { fitTags, growFacts } from './grow-lines.ts';
	import { flipTransform, growPlacement, GROW_MS, GROW_SCALE, GROW_WIDTH, SHRINK_MS, TRAILER_AFTER, type Growth, type Rect } from './grow.ts';
	import { onDestroy, onMount, type Snippet } from 'svelte';
	import { api, message } from '../core/api.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { proxied, sized } from '../core/tmdb-image.ts';
	import { minimalLook } from '../core/look.svelte.ts';
	import { autoplayHere, loadUISettings, trailerSources, type TrailerSource } from '../core/trailer/autoplay.svelte.ts';
	import { fromHandoff, openTheater } from '../core/trailer/theater.svelte.ts';
	import { THEATER_GROW_MS } from '../core/motion.ts';

	// How long the trailer stays faded out so the card can settle first.
	const HOLD_MS = 1200;
	const STATE_ICON: Partial<Record<CardOwned['state'], IconName>> = { available: 'check', partial: 'dash', downloading: 'download', requested: 'clock' };

	let {
		anchor,
		kind,
		tmdb,
		title,
		href,
		year,
		image,
		logo,
		lifted = false,
		leaving = false,
		badge,
		action,
		onenter,
		onleave,
		onclose,
		ongone,
		onmore,
		focusIn = false,
		ontab,
		returnTo,
		followId,
		followTmdb = false,
		context = null,
		service = null,
		owned = null
	}: {
		anchor: HTMLElement;
		kind: 'movie' | 'show';
		tmdb: string;
		title: string;
		href?: string;
		year?: number;
		image?: string | null;
		logo?: string | null;
		lifted?: boolean;
		leaving?: boolean;
		badge?: Snippet;
		action?: Snippet;
		onenter?: () => void;
		onleave?: () => void;
		onclose: (back?: boolean) => void;
		focusIn?: boolean;
		ontab?: (back: boolean) => void;
		returnTo?: HTMLElement;
		ongone: () => void;
		onmore?: (from: DOMRect) => void;
		followId?: number;
		followTmdb?: boolean;
		context?: { text: string; kind: 'house' | 'why' } | null;
		service?: { name: string; logo?: string } | null;
		owned?: CardOwned | null;
	} = $props();

	let following = $state(false);
	let followBusy = $state(false);
	let card = $state<HTMLElement>();
	let g = $state<Growth | null>(null);
	let meta = $state<RichMeta | null>(null);
	let sources = $state<TrailerSource[]>([]);
	let trailerOk = $state(false);
	let playing = $state(false);
	let held = $state(true);
	let shown = $state(false);
	let logoFailed = $state(false);
	let bigIn = $state(false);
	let anim: Animation | null = null;
	const ctl = new AbortController();

	const followTarget = $derived<FollowTarget | null>(followId && kind === 'show' ? { titleId: followId } : followTmdb ? { tmdb, kind } : null);
	const showing = $derived(playing && !held);
	const pic = $derived(image ?? (meta?.backdrop ? sized(meta.backdrop, 'backdrop') || null : null));
	// Only TMDB sizes have a bigger variant to swap in. The minimal look keeps the card's own picture.
	const big = $derived(!minimalLook() && pic && /\/artwork\/tmdb\/w(?:300|780)\//.test(pic) ? pic.replace(/\/w(?:300|780)\//, '/w1280/') : null);
	const shownLogo = $derived(logoFailed ? null : logo ? logo : meta?.logo ? sized(meta.logo, 'poster') : null);
	const facts = $derived(growFacts({ kind, year: meta?.year ?? year, seasons: meta?.seasons, runtime: meta?.runtime, certification: meta?.certification, score: meta?.score }));
	const tags = $derived(fitTags(meta?.moods, meta?.genres));
	const bellHint = $derived(kind === 'movie' ? movieFollowHint(title, following) : followHint('show', following));
	const stateIcon = $derived(owned ? STATE_ICON[owned.state] : undefined);

	$effect(() => {
		const t = followTarget;
		if (!t) return;
		api.get<{ following: boolean }>(followPath(t)).then((r) => (following = r.following), () => {});
	});
	$effect(() => {
		if (leaving) shrink();
	});

	onMount(() => {
		const t = setTimeout(() => (held = false), HOLD_MS);
		return () => clearTimeout(t);
	});
	onMount(() => {
		grow();
		if (focusIn) requestAnimationFrame(() => focusables()[0]?.focus({ preventScroll: true }));
		const close = () => onclose();
		// The card is fixed over the page, so any scroll or resize closes it.
		window.addEventListener('scroll', close, { passive: true, capture: true });
		window.addEventListener('resize', close);
		const later = setTimeout(() => (trailerOk = true), TRAILER_AFTER);
		const metaP = richMetadata(api.get, kind, tmdb).then((m) => (meta = m));
		void (async () => {
			await loadUISettings();
			if (!autoplayHere() || ctl.signal.aborted) return;
			const first = await trailerSources(kind, tmdb, undefined, ctl.signal);
			if (ctl.signal.aborted) return;
			if (first.some((x) => x.kind !== 'embed')) return void (sources = first);
			const m = await metaP;
			if (!ctl.signal.aborted) sources = await trailerSources(kind, tmdb, m?.videos, ctl.signal);
		})();
		return () => {
			clearTimeout(later);
			window.removeEventListener('scroll', close, { capture: true });
			window.removeEventListener('resize', close);
		};
	});
	onDestroy(() => {
		ctl.abort();
		anim?.cancel();
	});

	async function flipFollow() {
		const target = followTarget;
		if (!target) return;
		followBusy = true;
		try {
			following = await setFollow(api, target, !following);
		} catch (err) {
			toasts.push(t('ui.growCard.couldntUpdateFollow'), 'fail', { body: message(err) });
		} finally {
			followBusy = false;
		}
	}

	function grow() {
		const next = measure();
		if (!next || !card) return;
		g = next;
		shown = true;
		if (reduce()) return;
		anim?.cancel();
		card.style.willChange = 'transform';
		anim = card.animate(
			[
				{ transform: flipTransform(next.from), clipPath: clipTo(next) },
				{ transform: 'none', clipPath: 'inset(0 0 0 0 round var(--r-card))' }
			],
			{ duration: GROW_MS, easing: 'cubic-bezier(.2, .8, .2, 1)', fill: 'both' }
		);
		settle(anim);
	}

	// Plays the grow in reverse onto the anchor, then tells the parent to drop the card.
	function shrink() {
		if (card?.style.visibility === 'hidden') return ongone();
		if (!card || !g || reduce()) return ongone();
		const a = from();
		const back = { x: a.left - g.left, y: a.top - g.top, scale: a.width / g.width };
		// Start from the live transform so a shrink mid-grow does not jump.
		const now = anim && anim.playState === 'running' ? getComputedStyle(card).transform : 'none';
		anim?.cancel();
		card.style.willChange = 'transform';
		anim = card.animate([{ transform: now, clipPath: 'inset(0 0 0 0 round var(--r-card))' }, { transform: flipTransform(back), clipPath: clipTo(g) }], { duration: SHRINK_MS, easing: 'cubic-bezier(.4, 0, .6, 1)', fill: 'both' });
		anim.onfinish = () => ongone();
	}

	function hideNow() {
		anim?.cancel();
		anim = null;
		if (card) card.style.visibility = 'hidden';
	}

	// Arrows go to the anchor's row; Tab at either end leaves the card.
	function keydown(e: KeyboardEvent) {
		if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) {
			e.preventDefault();
			anchor.dispatchEvent(new KeyboardEvent('keydown', { key: e.key, bubbles: true, cancelable: true }));
			return;
		}
		if (e.key !== 'Tab' || !ontab) return;
		const list = focusables();
		const i = list.indexOf(document.activeElement as HTMLElement);
		if (e.shiftKey ? i <= 0 : i === list.length - 1) {
			e.preventDefault();
			ontab(e.shiftKey);
		}
	}
	function focusout(e: FocusEvent) {
		const to = e.relatedTarget as Node | null;
		if (to && (card?.contains(to) || anchor.closest('.ui-wide, .ui-poster')?.contains(to))) return;
		onleave?.();
	}

	const reduce = () => window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
	const focusables = () => (card ? [...card.querySelectorAll<HTMLElement>('a[href]:not([tabindex="-1"]), button:not([disabled])')].filter((e) => e.getClientRects().length > 0) : []);

	// Start box for the grow. A non 16:9 anchor gets a 16:9 box centred on it so the card lines up.
	function from(): Rect {
		const a = anchor.getBoundingClientRect();
		if (a.height <= 0 || Math.abs(a.width / a.height - 16 / 9) < 0.05) return a;
		const width = (a.height * 16) / 9 / GROW_SCALE;
		const height = (width * 9) / 16;
		return { left: a.left + a.width / 2 - width / 2, top: a.top + a.height / 2 - height / 2, width, height };
	}

	function measure(): Growth | null {
		if (!card) return null;
		const a = from();
		const panel = card.querySelector<HTMLElement>('.panel')?.offsetHeight ?? 0;
		const row = (anchor.closest('.row') ?? anchor.closest('main'))?.getBoundingClientRect();
		return growPlacement({ left: a.left, top: a.top, width: a.width, height: a.height }, panel, { width: document.documentElement.clientWidth, height: window.innerHeight }, row ? { left: row.left, right: row.right } : undefined, { min: GROW_WIDTH, max: GROW_WIDTH });
	}

	// Cuts off the info panel so only the media shows while the card is small.
	function clipTo(at: Growth) {
		const panel = Math.max(0, (card?.offsetHeight ?? 0) - at.mediaHeight);
		return `inset(0 0 ${panel}px 0 round var(--r-card))`;
	}

	function settle(a: Animation, done?: () => void) {
		a.onfinish = () => {
			if (anim === a) {
				a.cancel();
				anim = null;
				if (card) card.style.willChange = '';
			}
			done?.();
		};
	}

	// Moves the card to body so page transforms and overflow cannot clip it.
	function portal(node: HTMLElement) {
		document.body.appendChild(node);
		return { destroy: () => node.remove() };
	}
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose(!!card?.contains(document.activeElement))} />

<!-- svelte-ignore a11y_no_static_element_interactions, a11y_no_noninteractive_element_interactions -->
<div
	class="ui-grow"
	class:shown
	bind:this={card}
	use:portal
	role="group"
	aria-label={t('ui.growCard.preview', { title })}
	data-testid="grow-card"
	style:left="{g?.left ?? -9999}px"
	style:top="{g?.top ?? -9999}px"
	style:width="{g?.width ?? GROW_WIDTH}px"
	onpointerenter={onenter}
	onpointerleave={onleave}
	onfocusin={onenter}
	onfocusout={focusout}
	onkeydown={keydown}
>
	<a class="media" href={href} aria-label={href ? t('ui.growCard.open', { title }) : undefined}>
		{#if pic}<img src={pic} alt="" decoding="async" />{/if}
		{#if big}<img src={big} alt="" decoding="async" class="big" class:in={bigIn} onload={() => (bigIn = true)} />{/if}
		{#if trailerOk && sources.length}
			<span class="tp" class:held={!showing}><TrailerPlayer {sources} {title} repeat corner onplaying={() => (playing = true)} onfail={() => (playing = false)} onended={() => (playing = false)} onexpand={(o) => { openTheater({ kind, tmdb, title, ...fromHandoff(o), from: 'hover', returnTo }); hideNow(); setTimeout(onclose, THEATER_GROW_MS + 50); }} /></span>
		{/if}
		<span class="scrim" class:playing={showing} aria-hidden="true"></span>
		{#if shownLogo}<span class="name" class:lifted class:playing={showing} aria-hidden="true"><img class="logo" src={shownLogo} alt="" onerror={() => (logoFailed = true)} /></span>
		{:else}<span class="name" class:lifted class:playing={showing} aria-hidden="true"><span class="word">{title}</span></span>{/if}
		{#if badge && g}<span class="badge" class:playing={showing}>{@render badge()}</span>{/if}
	</a>
	<div class="panel">
		<div class="buttons">
			{#if action}{@render action()}{/if}
			{#if href && stateIcon && owned}<IconButton class="state" icon={stateIcon} label={owned.label} variant="glass" size="lg" {href} />
			{:else if href && !followTarget}<IconButton class="state" icon="arrow" label={t('ui.growCard.goTo', { title })} variant="glass" size="lg" {href} />{/if}
			{#if followTarget}<button type="button" class="more bell" class:on={following} aria-pressed={following} aria-label="{followLabel(following)} {title}" title={bellHint} disabled={followBusy} onclick={flipFollow} data-testid="grow-follow"><Icon name={following ? 'bellOn' : 'bell'} size={18} /></button>{/if}
			{#if onmore}<button type="button" class="more info" aria-label={t('ui.growCard.moreInfoAbout', { title })} title={t('ui.growCard.moreInfo')} aria-haspopup="dialog" onclick={() => card && onmore(card.getBoundingClientRect())}><Icon name="chevronDown" size={20} /></button>
			{:else if href}<a class="more info" {href} aria-label={t('ui.growCard.moreAbout', { title })} title={t('ui.growCard.moreInfo')}><Icon name="chevronDown" size={20} /></a>{/if}
		</div>
		<p class="title" title={title}>{title}</p>
		<p class="facts" data-testid="grow-facts">
			{#each facts as f, i (f.part)}{#if i}<i aria-hidden="true">·</i>{/if}<span class:rating={f.part === 'rating'} class:stars={f.part === 'stars'}>{f.text}</span>{/each}
			{#if service}<span class="svc" title={t('ui.growCard.on', { name: service.name })}>{#if service.logo}<img src={proxied(service.logo)} alt="" />{/if}<span class={service.logo ? 'sr' : 'svc-word'}>{t('ui.growCard.on', { name: service.name })}</span></span>{/if}
		</p>
		{#if context}<p class="ctx" class:why={context.kind === 'why'} data-testid="grow-context">{context.text}</p>
		{:else}<p class="tags">{#each tags as t, i (t)}{#if i}<i aria-hidden="true">·</i>{/if}<span>{t}</span>{/each}</p>{/if}
	</div>
</div>

<style>
	/* Fixed and hidden until placed so it never flashes at the wrong spot. */
	.ui-grow {
		--grow-shadow: 0 18px 48px rgb(0 0 0 / 0.42), 0 4px 14px rgb(0 0 0 / 0.28);
		position: fixed; z-index: 60;
		border-radius: var(--r-card); overflow: hidden;
		background: var(--surface-1); color: var(--text);
		box-shadow: var(--grow-shadow), 0 0 0 1px var(--line);
		transform-origin: 0 0;
		visibility: hidden;
	}
	/* Minimal: a lighter shadow. */
	:global(html[data-look='minimal']) .ui-grow { --grow-shadow: 0 8px 24px rgb(0 0 0 / 0.22); }
	.ui-grow.shown { visibility: visible; }
	.media { position: relative; display: block; container-type: inline-size; aspect-ratio: var(--wide-ratio, 16 / 9); background: var(--surface-2); overflow: hidden; }
	.media > img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
	.media > img.big { opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); }
	.media > img.big.in { opacity: 1; }
	.scrim { position: absolute; inset: auto 0 0; height: 55%; background: linear-gradient(to top, rgb(0 0 0 / 0.62), transparent); pointer-events: none; transition: opacity var(--dur-slow) var(--ease-out); }
	/* Logo box height tracks card width through container units. */
	.name { position: absolute; left: 6%; bottom: 9%; z-index: 2; width: 40%; height: 13cqw; display: flex; align-items: flex-end; justify-content: flex-start; pointer-events: none; transform-origin: left bottom; transition: opacity var(--dur) var(--ease-out), transform var(--dur-slow) var(--ease-out); }
	.name.lifted { bottom: 9%; }
	.name.playing { transform: scale(.8); }
	.badge.playing { opacity: 0; }
	.tp { position: absolute; inset: 0; transition: opacity var(--dur-slow) var(--ease-out); }
	.tp.held { opacity: 0; }
	.ui-grow :global(.corner) { opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); }
	.ui-grow:hover :global(.corner), .ui-grow:focus-within :global(.corner) { opacity: 1; }
	.word { font: 700 clamp(14px, 4.4cqw, 26px)/1.05 var(--font-display); letter-spacing: -0.02em; color: #fff; text-shadow: 0 2px 10px rgb(0 0 0 / 0.55); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	.badge { transition: opacity var(--dur) var(--ease-out); }
	.scrim.playing { opacity: 0.5; }
	.logo { display: block; max-width: 100%; max-height: 100%; width: auto; height: auto; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 2px 8px rgb(0 0 0 / 0.5)); }
	.badge { position: absolute; inset: 0; z-index: 3; container: art / inline-size; pointer-events: none; }
	/* Info panel fades in after the card has grown. */
	.panel { display: flex; flex-direction: column; gap: 8px; padding: 12px 14px 14px; opacity: 0; transform: translateY(-6px); }
	.shown .panel { animation: panel-in var(--dur-fast) var(--ease-out) 120ms both; }
	@keyframes panel-in { to { opacity: 1; transform: none; } }
	.buttons { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.buttons :global(.ui-ibtn.glass) { background: var(--text); color: var(--surface-1); box-shadow: none; }
	.buttons :global(.ui-ibtn.glass:hover) { background: color-mix(in srgb, var(--text) 85%, var(--surface-1)); }
	.more { flex: none; width: 40px; height: 40px; display: grid; place-items: center; border-radius: 50%; color: var(--text); box-shadow: inset 0 0 0 2px color-mix(in srgb, var(--text) 40%, transparent); transition: box-shadow var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out); }
	.more:hover { box-shadow: inset 0 0 0 2px var(--text); background: var(--surface-2); }
	.more:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.more.bell.on { background: var(--text); color: var(--bg); box-shadow: none; }
	.more.info { margin-left: auto; }
	/* Only one state button shows even if the action slot adds its own. */
	.buttons :global(.state:not(:first-child)) { display: none; }
	.buttons :global(.ui-ibtn) { width: 40px; height: 40px; flex: none; }
	.buttons :global(.ui-ibtn:focus-visible) { outline: none; box-shadow: var(--focus-ring); }
	.title { margin: 2px 0 -4px; font: var(--t-headline); letter-spacing: -0.01em; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.facts, .tags, .ctx { min-height: 18px; }
	.ctx { margin: 0; font: var(--t-footnote); color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ctx.why { color: var(--text-muted); }
	.svc { display: inline-flex; align-items: center; margin-left: 4px; flex: none; }
	.svc img { display: block; width: 18px; height: 18px; border-radius: 4px; object-fit: cover; box-shadow: 0 0 0 1px var(--line); }
	.svc-word { color: var(--text); }
	/* Visually hidden but still read by screen readers. */
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	.facts { display: flex; align-items: center; gap: 6px; margin: 0; white-space: nowrap; overflow: hidden; font: var(--t-footnote); color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.rating { padding: 0 5px; border: 1px solid color-mix(in srgb, var(--text) 45%, transparent); border-radius: 3px; color: var(--text); font-size: 11px; line-height: 16px; }
	.stars { color: var(--text); font-weight: 600; font-variant-numeric: tabular-nums; }
	.tags { display: flex; flex-wrap: nowrap; overflow: hidden; white-space: nowrap; align-items: center; gap: 6px; font: var(--t-footnote); color: var(--text); }
	.tags i { color: var(--text-muted); font-style: normal; font-size: 9px; }
	@media (prefers-reduced-motion: reduce) {
		.shown .panel { animation: none; opacity: 1; transform: none; }
		.name, .scrim { transition: opacity var(--dur-fast) linear; }
			}
</style>

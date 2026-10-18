<script lang="ts">
	import type { Snippet } from 'svelte';
	import Badge from './Badge.svelte';
	import StatusPill, { type StatusKind } from './StatusPill.svelte';
	import { viewTransitionName } from '../motion.ts';

	// The title page hero (movie, show, artist). Like Hero, but only the art
	// is clipped, so menus in the action row can open past its edge. With no
	// backdrop the poster is blurred behind and shown sharp beside the title;
	// the sharp art carries the view-transition-name so a poster grows into it.
	let {
		title,
		backdrop,
		poster,
		logo,
		meta = [],
		badges = [],
		status,
		percent,
		statusLabel,
		description,
		breadcrumb = [],
		vtName,
		square = false,
		actions,
		extra
	}: {
		title: string;
		backdrop?: string | null;
		poster?: string | null;
		logo?: string | null;
		meta?: string[];
		badges?: string[];
		status?: StatusKind;
		percent?: number;
		statusLabel?: string;
		description?: string;
		breadcrumb?: { href: string; label: string }[];
		vtName?: string;
		/** Square art (music). */
		square?: boolean;
		actions?: Snippet;
		/** Extra lines under the action row (download progress, notes). */
		extra?: Snippet;
	} = $props();

	let more = $state(false);
	let text = $state<HTMLElement>();
	let clamped = $state(false);
	let failed = $state(false);
	$effect(() => {
		void description;
		if (!text) return;
		const check = () => text && (clamped = more || text.scrollHeight > text.clientHeight + 1);
		check();
		const ro = new ResizeObserver(check);
		ro.observe(text);
		return () => ro.disconnect();
	});
	const art = $derived(backdrop || poster || null);
	const thumb = $derived(!backdrop && poster && !failed ? poster : null);
</script>

<section class="ui-thero" aria-label={title}>
	<div class="bg" class:blur={!backdrop} aria-hidden="true">
		{#if art}<img src={art} alt="" decoding="async" use:viewTransitionName={backdrop ? vtName : undefined} />{/if}
		<div class="shade"></div>
	</div>
	<div class="inner" class:has-thumb={!!thumb}>
		{#if thumb}
			<div class="thumb" class:square use:viewTransitionName={vtName}><img src={thumb} alt="" decoding="async" onerror={() => (failed = true)} /></div>
		{/if}
		<div class="content">
			{#if breadcrumb.length}
				<nav class="crumbs" aria-label="Breadcrumb">
					<ol>
						{#each breadcrumb as c, i (i)}
							<li>
								{#if i > 0}<span class="sep" aria-hidden="true">›</span>{/if}
								<a href={c.href} aria-current={i === breadcrumb.length - 1 ? 'page' : undefined}>{c.label}</a>
							</li>
						{/each}
					</ol>
				</nav>
			{/if}
			{#if logo}<h1 class="logo"><img src={logo} alt={title} /></h1>{:else}<h1>{title}</h1>{/if}
			{#if meta.length}<p class="meta">{meta.join(' · ')}</p>{/if}
			{#if badges.length || status}
				<div class="badges">
					{#if status}<StatusPill kind={status} {percent} label={statusLabel} on="art" />{/if}
					{#each badges as b (b)}<Badge label={b} on="art" />{/each}
				</div>
			{/if}
			{#if description}
				<p class="desc" class:open={more} bind:this={text}>{description}</p>
				{#if clamped}<button class="more" type="button" aria-expanded={more} onclick={() => (more = !more)}>{more ? 'Show less' : 'Show more'}</button>{/if}
			{/if}
			{#if actions}<div class="actions">{@render actions()}</div>{/if}
			{#if extra}<div class="extra">{@render extra()}</div>{/if}
		</div>
	</div>
</section>

<style>
	.ui-thero { position: relative; isolation: isolate; color: var(--on-art); border-radius: var(--r-panel); min-height: clamp(380px, 42vw, 620px); display: flex; align-items: flex-end; }
	.bg { position: absolute; inset: 0; z-index: -1; overflow: hidden; border-radius: inherit; background: #15151a; }
	.bg img { width: 100%; height: 100%; object-fit: cover; object-position: center 25%; }
	.bg.blur img { filter: blur(48px) saturate(150%) brightness(.9); transform: scale(1.25); }
	.shade { position: absolute; inset: 0; background: linear-gradient(to top, rgb(0 0 0 / 88%) 0%, rgb(0 0 0 / 58%) 40%, rgb(0 0 0 / 12%) 78%), linear-gradient(to right, rgb(0 0 0 / 55%) 0%, rgb(0 0 0 / 0%) 65%); }
	.inner { width: 100%; display: flex; align-items: flex-end; gap: clamp(20px, 3vw, 40px); padding: clamp(20px, 4vw, 48px); }
	.thumb { flex: none; width: clamp(150px, 17vw, 230px); aspect-ratio: 2 / 3; border-radius: 12px; overflow: hidden; background: rgb(255 255 255 / 8%); box-shadow: 0 18px 48px rgb(0 0 0 / 45%), 0 0 0 1px rgb(255 255 255 / 10%); }
	.thumb.square { aspect-ratio: 1; width: clamp(160px, 19vw, 260px); }
	.thumb img { width: 100%; height: 100%; object-fit: cover; }
	.content { min-width: 0; width: min(680px, 100%); display: flex; flex-direction: column; gap: 10px; }
	.crumbs ol { display: flex; flex-wrap: wrap; gap: 6px; list-style: none; margin: 0; padding: 0; font: var(--t-subhead); color: rgb(255 255 255 / 80%); }
	.crumbs li { display: inline-flex; gap: 6px; min-width: 0; }
	.crumbs a { display: inline-flex; align-items: center; min-height: 32px; border-radius: 4px; overflow-wrap: anywhere; }
	.crumbs a:hover { color: var(--on-art); text-decoration: underline; }
	.crumbs a[aria-current='page'] { color: var(--on-art); }
	.crumbs a:focus-visible, .more:focus-visible { outline: none; box-shadow: 0 0 0 2px var(--on-art); }
	.sep { display: inline-flex; align-items: center; }
	h1 { font: var(--t-large-title); font-size: clamp(30px, 4.4vw, 52px); line-height: 1.05; letter-spacing: var(--t-large-title-track); text-wrap: balance; overflow-wrap: anywhere; text-shadow: 0 1px 24px rgb(0 0 0 / 30%); }
	h1.logo img { max-width: min(420px, 80%); max-height: 140px; object-fit: contain; object-position: left bottom; }
	.meta { font: var(--t-callout); color: rgb(255 255 255 / 88%); font-variant-numeric: tabular-nums; }
	.badges { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
	.desc { font: var(--t-body); color: rgb(255 255 255 / 92%); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; max-width: 62ch; }
	.desc.open { -webkit-line-clamp: unset; line-clamp: unset; display: block; }
	.more { align-self: flex-start; min-height: 32px; padding: 0 2px; color: var(--on-art); font: var(--t-subhead); font-weight: 600; text-decoration: underline; text-underline-offset: 3px; border-radius: 4px; }
	.actions { display: flex; flex-wrap: wrap; align-items: flex-start; gap: 8px 12px; margin-top: 8px; }
	.extra { display: flex; flex-direction: column; gap: 8px; }
	@media (max-width: 833px) {
		.inner { flex-direction: column; align-items: flex-start; }
		.thumb { width: 132px; }
		.thumb.square { width: 150px; }
	}
	@media (max-width: 767px) {
		.ui-thero { border-radius: 0; margin-inline: calc(var(--gutter) * -1); min-height: 0; }
		.inner { padding: 24px var(--gutter) 24px; padding-top: 32px; }
		.actions { gap: 8px 4px; }
	}
</style>

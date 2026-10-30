<script lang="ts">
	// Approvals as Reels (a trial next to the Stage, on the pill bar): one ask per
	// full-height card that snaps as you scroll. Story bars on top, the asker as
	// the post's header, answers down the right side, the title and their note as
	// the caption. Double-tap the art to approve. ↑ ↓ move between reels.
	import { tick } from 'svelte';
	import { fade, scale } from 'svelte/transition';
	import { cubicOut, backOut } from 'svelte/easing';
	import Poster from '../Poster.svelte';
	import Icon from '../Icon.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import KidMark from './KidMark.svelte';
	import { ago, dateTime } from '../format.ts';
	import { to } from '../paths.ts';
	import type { ApprovalsCtx } from './approvals.ts';
	import { artistPhoto, askArtist } from './details.svelte.ts';
	import { askSummary, inboxTabs } from './inbox.ts';
	import { lifecycle } from './lifecycle.ts';
	import type { Ask } from '../types.ts';

	let { ctx, open }: { ctx: ApprovalsCtx; open: (a: Ask, el?: HTMLElement | null) => void } = $props();

	const waiting = (a: Ask) => ctx.approver && a.state === 'asked' && !ctx.isPending(a.id);
	const list = $derived([...ctx.shown].sort((x, y) => Date.parse(y.asked_at) - Date.parse(x.asked_at)));
	$effect(() => {
		for (const a of list.slice(0, 12)) ctx.want(a);
	});
	const art = (a: Ask) => {
		if (a.kind === 'artist') return askArtist(a)?.backgrounds[0] ?? artistPhoto(a) ?? '';
		const d = ctx.details(a);
		return d?.backdrop ? to(d.backdrop) : a.poster ? to(a.poster) : '';
	};
	const kindOf = (a: Ask) => (a.kind === 'show' ? 'TV show' : a.kind === 'artist' ? 'Music' : 'Movie');
	const count = (a: Ask) => (a.kind === 'movie' ? '' : (askSummary(a, ctx.info(a)).line.split(' · ').at(-1) ?? ''));
	const tabs = $derived(inboxTabs.filter((t) => ctx.approver || t.tab !== 'failed'));

	// Which reel is on screen: the one most in view.
	let feed = $state<HTMLElement>();
	let at = $state(0);
	$effect(() => {
		if (!feed) return;
		const io = new IntersectionObserver(
			(es) => {
				for (const e of es) if (e.isIntersecting && e.intersectionRatio > 0.6) at = Number((e.target as HTMLElement).dataset.i);
			},
			{ root: feed, threshold: [0.6] }
		);
		for (const el of feed.querySelectorAll('[data-i]')) io.observe(el);
		return () => io.disconnect();
	});
	function go(i: number) {
		const el = feed?.querySelector<HTMLElement>(`[data-i="${Math.min(Math.max(i, 0), list.length - 1)}"]`);
		el?.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	// Double-tap approves, with a burst.
	let burst = $state(0);
	let lastTap = 0;
	function tap(a: Ask) {
		const now = Date.now();
		if (now - lastTap < 320 && waiting(a)) {
			burst = a.id;
			ctx.approve([a]);
			setTimeout(() => (burst = 0), 900);
		}
		lastTap = now;
	}
	function keys(e: KeyboardEvent) {
		if (document.querySelector('dialog[open]') || e.metaKey || e.ctrlKey || e.altKey) return;
		if (e.target instanceof Element && e.target.closest('input, textarea, select, [contenteditable]')) return;
		if (e.key === 'ArrowDown' || e.key === 'j') { e.preventDefault(); go(at + 1); }
		else if (e.key === 'ArrowUp' || e.key === 'k') { e.preventDefault(); go(at - 1); }
	}
	void tick;
</script>

<svelte:window onkeydown={keys} />

<div class="reels" data-testid="approvals-reels">
	<div class="tabs" role="tablist" aria-label="Show asks that are">
		{#each tabs as t (t.tab)}
			<button type="button" role="tab" class="tab" aria-selected={ctx.tab === t.tab} onclick={() => ctx.setTab(t.tab)}>{t.label}{#if ctx.counts[t.tab]}<span class="n">{ctx.counts[t.tab]}</span>{/if}</button>
		{/each}
	</div>

	<div class="phone">
		<div class="feed" bind:this={feed} aria-label="Asks, one at a time">
			{#each list as a, i (a.id)}
				{@const w = ctx.who(a)}
				{@const pic = art(a)}
				{@const life = lifecycle(a)}
				<article class="reel" data-i={i} aria-label="{a.name}, asked by {w.name}">
					<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
					<div class="media" onclick={() => tap(a)}>
						{#if pic}<div class="bg" class:live={at === i} style:background-image={`url("${pic}")`}></div>{:else}<div class="bg poster"><Poster src={a.poster} name={a.name} /></div>{/if}
						<div class="shade"></div>
						{#if burst === a.id}
							<span class="burst" in:scale={{ start: 0.3, duration: 420, easing: backOut }} out:fade={{ duration: 300 }}><Icon name="check" size={84} /></span>
						{/if}
					</div>

					<div class="stories" aria-hidden="true">
						{#each list as b, j (b.id)}<span class="seg" class:done={j < i} class:now={j === i}><span class="segfill"></span></span>{/each}
					</div>

					<header class="who">
						<span class="ring" class:wait={waiting(a)}><Avatar name={w.name} src={w.avatar} size={36} /></span>
						<div><strong>{w.name}{#if w.kid}<KidMark label="Kid" size="sm" />{/if}</strong><time datetime={a.asked_at} title={dateTime(a.asked_at)}>asked {ago(a.asked_at)}</time></div>
					</header>

					<div class="side">
						{#if waiting(a)}
							<button type="button" class="sb yes" aria-label="Approve {a.name}" onclick={() => ctx.approve([a])}><span class="sbi"><Icon name="check" size={26} /></span><span>Approve</span></button>
							<button type="button" class="sb" aria-label="Turn down {a.name}" onclick={() => ctx.turnDown([a])}><span class="sbi"><Icon name="x" size={26} /></span><span>Turn down</span></button>
						{/if}
						<button type="button" class="sb" aria-label="Details for {a.name}" onclick={(e) => open(a, e.currentTarget)}><span class="sbi"><Icon name="info" size={24} /></span><span>Details</span></button>
						<span class="mini">{#if a.kind === 'artist' && artistPhoto(a)}<img src={artistPhoto(a)} alt="" />{:else}<Poster src={a.poster} name={a.name} />{/if}</span>
					</div>

					<div class="caption">
						<h2>{a.name}</h2>
						<p class="facts"><span>{kindOf(a)}</span>{#if a.year}<span>{a.year}</span>{/if}{#if count(a)}<span>{count(a)}</span>{/if}</p>
						{#if a.note}<p class="note">&ldquo;{a.note}&rdquo;</p>{/if}
						<p class="status tone-{life.tone}"><span class="dot"></span>{ctx.decided(a) || life.label}</p>
						{#if waiting(a)}<p class="hint">Double-tap to approve</p>{/if}
					</div>
					<span class="life tone-{life.tone}"><span class="fill" style:width="{life.pct}%"></span></span>
				</article>
			{:else}
				<div class="empty" in:fade><Icon name="check" size={28} /><h2>You’re all caught up</h2><p>New asks land here the moment someone asks.</p></div>
			{/each}
		</div>
	</div>

	<div class="nav" aria-hidden={list.length < 2}>
		<button type="button" class="nb" aria-label="The ask before" disabled={at === 0} onclick={() => go(at - 1)}><Icon name="chevronDown" size={20} /></button>
		<span class="pos">{list.length ? at + 1 : 0} / {list.length}</span>
		<button type="button" class="nb down" aria-label="The next ask" disabled={at >= list.length - 1} onclick={() => go(at + 1)}><Icon name="chevronDown" size={20} /></button>
	</div>
</div>

<style>
	.reels { --ph: clamp(560px, calc(100dvh - var(--topbar, 56px) - 170px), 920px); display: flex; flex-direction: column; align-items: center; gap: 14px; position: relative; }
	button { font: inherit; color: inherit; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tabs { display: inline-flex; gap: 2px; padding: 3px; border-radius: var(--r-pill); background: var(--surface-2); }
	.tab { display: inline-flex; align-items: baseline; gap: 6px; min-height: 32px; padding: 0 14px; border: 0; border-radius: var(--r-pill); background: none; color: var(--text-muted); font: 600 13px/1 var(--font-ui); cursor: pointer; transition: background 220ms var(--ease-out), color 220ms var(--ease-out); }
	.tab[aria-selected='true'] { background: var(--surface-1); color: var(--text); box-shadow: 0 1px 3px rgb(0 0 0 / 0.2); }
	.n { font-size: 11px; opacity: 0.7; }

	/* The phone-shaped frame, 9:16. */
	.phone { position: relative; height: var(--ph); aspect-ratio: 9 / 16; border-radius: 34px; overflow: hidden; background: #000; box-shadow: 0 0 0 1px var(--line), 0 0 0 10px color-mix(in srgb, var(--surface-1) 70%, transparent), 0 50px 100px -40px rgb(0 0 0 / 0.8); }
	.feed { height: 100%; overflow-y: auto; scroll-snap-type: y mandatory; overscroll-behavior: contain; scrollbar-width: none; }
	.feed::-webkit-scrollbar { display: none; }
	.reel { position: relative; height: 100%; scroll-snap-align: start; scroll-snap-stop: always; overflow: hidden; color: #fff; isolation: isolate; }
	.media { position: absolute; inset: 0; z-index: -1; cursor: pointer; }
	.bg { position: absolute; inset: -4%; background-size: cover; background-position: 50% 30%; transform: scale(1.05); transition: transform 8s linear; }
	.bg.live { transform: scale(1.18) translateY(-2%); }
	.bg.poster { inset: 0; display: grid; --poster-ratio: auto; }
	.shade { position: absolute; inset: 0; background: linear-gradient(to bottom, rgb(0 0 0 / 0.55), transparent 22%, transparent 48%, rgb(0 0 0 / 0.85)); }
	.burst { position: absolute; left: 50%; top: 45%; translate: -50% -50%; display: grid; place-items: center; width: 140px; height: 140px; border-radius: 50%; background: rgb(255 255 255 / 0.18); color: #fff; -webkit-backdrop-filter: blur(10px); backdrop-filter: blur(10px); filter: drop-shadow(0 10px 30px rgb(0 0 0 / 0.5)); }

	.stories { position: absolute; top: 12px; left: 12px; right: 12px; display: flex; gap: 3px; }
	.seg { flex: 1; height: 3px; border-radius: 3px; background: rgb(255 255 255 / 0.3); overflow: hidden; }
	.segfill { display: block; height: 100%; width: 0; background: #fff; }
	.seg.done .segfill { width: 100%; }
	.seg.now .segfill { width: 100%; animation: grow 5s linear both; }
	@keyframes grow { from { width: 0; } }

	.who { position: absolute; top: 26px; left: 14px; right: 14px; display: flex; align-items: center; gap: 10px; text-shadow: 0 1px 8px rgb(0 0 0 / 0.6); }
	.who div { display: flex; flex-direction: column; font: var(--t-subhead); }
	.who strong { display: inline-flex; align-items: center; gap: 6px; font-weight: 700; }
	.who time { font: var(--t-caption); opacity: 0.8; }
	.ring { display: inline-flex; padding: 2px; border-radius: 50%; background: rgb(255 255 255 / 0.5); }
	.ring.wait { background: conic-gradient(from 210deg, var(--wait), var(--accent), var(--wait)); }

	.side { position: absolute; right: 10px; bottom: 120px; display: flex; flex-direction: column; align-items: center; gap: 16px; }
	.sb { display: flex; flex-direction: column; align-items: center; gap: 4px; padding: 0; border: 0; background: none; font: 600 11px/1 var(--font-ui); text-shadow: 0 1px 6px rgb(0 0 0 / 0.7); cursor: pointer; }
	.sbi { display: grid; place-items: center; width: 50px; height: 50px; border-radius: 50%; background: rgb(255 255 255 / 0.16); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.15); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); transition: transform 160ms var(--ease-out), background 200ms var(--ease-out); }
	.sb:hover .sbi { background: rgb(255 255 255 / 0.26); }
	.sb:active .sbi { transform: scale(0.88); }
	.sb.yes .sbi { background: #fff; color: #0b0b0c; }
	.mini { width: 40px; aspect-ratio: 2 / 3; --poster-ratio: 2 / 3; border-radius: 8px; overflow: hidden; box-shadow: 0 0 0 2px #fff; animation: spin-in 600ms var(--ease-out) both; }
	.mini img { width: 100%; height: 100%; object-fit: cover; }
	@keyframes spin-in { from { transform: rotate(-12deg) scale(0.8); opacity: 0; } }

	.caption { position: absolute; left: 16px; right: 80px; bottom: 26px; display: flex; flex-direction: column; gap: 6px; text-shadow: 0 1px 10px rgb(0 0 0 / 0.6); }
	.caption h2 { font: 800 clamp(24px, 2.4vw, 32px)/1.05 var(--font-display, var(--font-ui)); letter-spacing: -0.03em; overflow-wrap: anywhere; }
	.facts { display: flex; flex-wrap: wrap; font: 600 13px/1.3 var(--font-ui); opacity: 0.92; }
	.facts span + span::before { content: '·'; margin: 0 7px; opacity: 0.6; }
	.note { font: var(--t-callout); }
	.status { display: inline-flex; align-items: center; gap: 6px; font: var(--t-caption); font-weight: 600; opacity: 0.9; }
	.dot { width: 7px; height: 7px; border-radius: 50%; background: currentColor; }
	.status.tone-wait .dot { background: var(--wait); }
	.status.tone-go .dot { background: var(--accent); }
	.status.tone-ok .dot { background: var(--ok); }
	.hint { font: var(--t-caption); opacity: 0.6; }
	.life { position: absolute; left: 0; right: 0; bottom: 0; height: 3px; background: rgb(255 255 255 / 0.12); }
	.fill { display: block; height: 100%; transition: width 900ms cubic-bezier(0.22, 1, 0.36, 1); }
	.life.tone-wait .fill { background: var(--wait); }
	.life.tone-go .fill { background: var(--accent); }
	.life.tone-ok .fill { background: var(--ok); }
	.life.tone-off .fill { background: rgb(255 255 255 / 0.35); }
	.life.tone-bad .fill { background: var(--fail, #e5484d); }

	.empty { display: grid; place-content: center; justify-items: center; gap: 8px; height: 100%; color: rgb(255 255 255 / 0.7); text-align: center; padding: 24px; }
	.empty h2 { color: #fff; font: var(--t-title3); }

	.nav { position: absolute; right: calc(50% - var(--ph) * 9 / 32 - 74px); top: 50%; translate: 0 -50%; display: flex; flex-direction: column; align-items: center; gap: 10px; }
	.nb { display: grid; place-items: center; width: 44px; height: 44px; border: 0; border-radius: 50%; background: var(--surface-2); color: var(--text); cursor: pointer; transform: rotate(180deg); transition: background 200ms var(--ease-out); }
	.nb.down { transform: none; }
	.nb:hover:not(:disabled) { background: var(--surface-3); }
	.nb:disabled { opacity: 0.35; cursor: default; }
	.pos { color: var(--text-muted); font: var(--t-caption); font-variant-numeric: tabular-nums; }

	@media (prefers-reduced-motion: reduce) {
		.bg, .fill, .sbi { transition: none; }
		.seg.now .segfill, .mini { animation: none; }
	}
</style>

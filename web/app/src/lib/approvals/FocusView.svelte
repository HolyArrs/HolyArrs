<script lang="ts">
	// 3 Focus: one ask at a time, big. Backdrop, poster, story, facts and what
	// was asked, with Approve / Approve with options / Turn down. The rest wait
	// in a strip below. Keys: ← → move, A approves, D turns down.
	import Poster from '../Poster.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import Icon from '../Icon.svelte';
	import { glideScroll } from '../ui/transitions.ts';
	import StatusPill from '../ui/StatusPill.svelte';
	import KidMark from './KidMark.svelte';
	import { to } from '../paths.ts';
	import { ago, dateTime } from '../format.ts';
	import { kindWord, runtimeLine, stepIndex, type ApprovalsCtx } from './approvals.ts';
	import { askTabs } from '../wishlist.ts';

	let { ctx }: { ctx: ApprovalsCtx } = $props();
	let at = $state(0);
	let focusId = $state(0);
	// The ask in focus stays in focus while the list changes around it; once it
	// leaves (approved, turned down), the one that took its place comes up.
	const idx = $derived.by(() => {
		const i = ctx.shown.findIndex((a) => a.id === focusId);
		return i >= 0 ? i : Math.min(at, Math.max(ctx.shown.length - 1, 0));
	});
	const a = $derived(ctx.shown[idx] ?? null);
	$effect(() => {
		if (a) {
			ctx.want(a);
			const next = ctx.shown[idx + 1];
			if (next) ctx.want(next);
		}
	});
	const d = $derived(a ? ctx.details(a) : undefined);
	const who = $derived(a ? ctx.who(a) : null);
	const art = $derived(d?.backdrop ?? a?.poster ?? '');
	const facts = $derived(a ? [a.year ? String(a.year) : '', kindWord(a.kind), runtimeLine(d?.runtime), d?.rating ?? '', ...(d?.genres ?? []).slice(0, 3)].filter(Boolean) : []);
	const can = $derived(!!a && ctx.approver && a.state === 'asked' && !ctx.isPending(a.id));

	function go(i: number) {
		at = i;
		focusId = ctx.shown[i]?.id ?? 0;
	}
	function step(s: number) {
		if (ctx.shown.length) go(stepIndex(idx, s, ctx.shown.length));
	}
	function approve() {
		if (!a || !can) return;
		at = idx;
		focusId = 0;
		ctx.approve([a]);
	}
	function turnDown() {
		if (!a || !can) return;
		at = idx;
		ctx.turnDown([a]);
	}
	function keys(e: KeyboardEvent) {
		if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey) return;
		const t = e.target as HTMLElement | null;
		if (t?.closest('input, textarea, select, [contenteditable], dialog')) return;
		if (document.querySelector('dialog[open]')) return;
		if (e.key === 'ArrowRight') step(1);
		else if (e.key === 'ArrowLeft') step(-1);
		else if (e.key === 'a' || e.key === 'A') approve();
		else if (e.key === 'd' || e.key === 'D') turnDown();
		else return;
		e.preventDefault();
	}
	let strip: HTMLElement | undefined = $state();
	$effect(() => {
		void idx;
		// Only the strip scrolls (scrollIntoView would also slide the page's clipping boxes).
		const on = strip?.querySelector<HTMLElement>('[aria-current="true"]')?.parentElement;
		if (strip && on) strip.scrollTo({ left: on.offsetLeft - (strip.clientWidth - on.offsetWidth) / 2, behavior: glideScroll() });
	});
	const decidedLately = $derived(ctx.viewed.filter((x) => x.state !== 'asked').slice(0, 6));
</script>

<svelte:window onkeydown={keys} />

{#if !a}
	<section class="calm" data-testid="calm">
		<div class="calm-glow" aria-hidden="true"></div>
		<span class="calm-ic"><Icon name="check" size={28} /></span>
		<h2>{ctx.tab === 'waiting' ? 'All caught up' : `Nothing ${askTabs.find((t) => t.tab === ctx.tab)?.label.toLowerCase()} here`}</h2>
		<p>{ctx.tab === 'waiting' ? 'Every ask has an answer. New ones come up here one at a time.' : 'Pick another tab above to look through it.'}</p>
		{#if ctx.tab === 'waiting' && decidedLately.length}
			<p class="calm-sub">Lately decided</p>
			<ul class="calm-row">
				{#each decidedLately as x (x.id)}
					<li><div class="mini"><Poster src={x.poster} name={x.name} /></div><StatusPill kind={ctx.pill(x).kind} label={ctx.pill(x).label} /></li>
				{/each}
			</ul>
		{/if}
	</section>
{:else}
	<section class="stage" aria-roledescription="carousel" aria-label="One ask at a time">
		{#key a.id}
			<article class="hero" class:music={a.kind === 'artist' || !d?.backdrop} class:sending={ctx.isPending(a.id)} data-testid="ask" aria-label="{a.name}, {idx + 1} of {ctx.shown.length}">
				{#if art}<div class="bd" style:background-image={`url("${to(art)}")`} aria-hidden="true"></div>{/if}
				<div class="count" aria-hidden="true">{idx + 1} <span>of {ctx.shown.length}</span></div>
				<div class="hero-in">
					<a class="poster" href={ctx.href(a) ? to(ctx.href(a)) : undefined} tabindex="-1" aria-hidden="true"><Poster src={a.poster} name={a.name} /></a>
					<div class="copy">
						<p class="facts">{facts.join(' · ')}</p>
						<h2 class="name">{#if ctx.href(a)}<a href={to(ctx.href(a))}>{a.name}</a>{:else}{a.name}{/if}</h2>
						<div class="ask-line">
							<Avatar name={who?.name ?? ''} src={who?.avatar} size={26} />
							<span data-testid="asked-by"><b>{who?.name}</b> asked <time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time></span>
							<KidMark label={who?.kid ?? ''} size="sm" />
						</div>
						{#if ctx.what(a)}<p class="part" data-testid={a.kind === 'artist' ? 'ask-albums' : 'ask-seasons'}><Icon name={a.kind === 'artist' ? 'music' : 'tv'} size={15} />For {ctx.what(a)}</p>{/if}
						{#if d?.overview}<p class="story">{d.overview}</p>{/if}
						{#if a.note}<p class="story">“{a.note}”</p>{/if}
						{#if a.state !== 'asked'}<div class="pill"><StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} testid="ask-state" /></div>{/if}
					</div>
				</div>
				<div class="decide">
					{#if can}
						<button class="act yes" type="button" data-testid="ask-approve" onclick={approve}><Icon name="check" size={20} />Approve<kbd>A</kbd></button>
						<button class="act opt" type="button" onclick={() => a && ctx.withOptions(a)}><Icon name="sparkle" size={18} />Approve with options</button>
						<button class="act no" type="button" data-testid="ask-decline" onclick={turnDown}><Icon name="x" size={20} />Turn down<kbd>D</kbd></button>
					{:else if ctx.isPending(a.id)}
						<span class="bar-line">Sending in a moment…</span>
					{:else}
						<button class="act opt" type="button" onclick={(e) => a && ctx.review(a, e.currentTarget)}>Review</button>
					{/if}
					<span class="nav">
						<button class="arrow" type="button" aria-label="Previous ask" disabled={ctx.shown.length < 2} onclick={() => step(-1)}><Icon name="back" size={20} /></button>
						<button class="arrow" type="button" aria-label="Next ask" disabled={ctx.shown.length < 2} onclick={() => step(1)}><span class="flip"><Icon name="back" size={20} /></span></button>
					</span>
				</div>
			</article>
		{/key}
		<p class="keys muted" aria-hidden="true"><kbd>←</kbd><kbd>→</kbd> move · <kbd>A</kbd> approve · <kbd>D</kbd> turn down</p>
	</section>

	{#if ctx.shown.length > 1 || ctx.choosable}
		<div class="queue-head">
			<h2 class="queue-title">Up next</h2>
			{#if ctx.choosable}<button class="choose" type="button" onclick={() => ctx.chooseAll(ctx.shown)}>{ctx.shown.every((x) => ctx.isChosen(x.id)) ? 'Clear choice' : 'Choose all'}</button>{/if}
		</div>
		<ol class="queue" bind:this={strip} aria-label="Asks in this list">
			{#each ctx.shown as x, i (x.id)}
				<li class="q" class:chosen={ctx.isChosen(x.id)}>
					<button class="q-btn" type="button" aria-current={i === idx} aria-label="{x.name}, {i + 1} of {ctx.shown.length}" onclick={() => go(i)}>
						<span class="q-art"><Poster src={x.poster} name={x.name} /></span>
						<span class="q-name">{x.name}</span>
					</button>
					{#if ctx.choosable}<input class="tick" type="checkbox" aria-label="Choose {x.name}" checked={ctx.isChosen(x.id)} onchange={() => ctx.toggleChosen(x.id)} />{/if}
				</li>
			{/each}
		</ol>
	{/if}
{/if}

<style>
	.stage { display: flex; flex-direction: column; gap: 10px; }
	.hero { position: relative; isolation: isolate; overflow: hidden; min-height: 460px; display: flex; flex-direction: column; justify-content: flex-end; border-radius: 22px; background: #111214; color: var(--on-art); box-shadow: var(--shadow-lift); animation: arrive var(--dur-slow) var(--ease-out) both; }
	.hero.sending { opacity: 0.65; }
	@keyframes arrive { from { opacity: 0; transform: translateY(10px) scale(0.99); } }
	.bd { position: absolute; inset: 0; z-index: -2; background-size: cover; background-position: center 22%; animation: drift 14s var(--ease-out) both; }
	.music .bd { filter: blur(34px) saturate(1.35); transform: scale(1.3); animation: none; }
	@keyframes drift { from { transform: scale(1.08); } to { transform: scale(1); } }
	.hero::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(90deg, rgb(8 8 10 / 92%) 0%, rgb(8 8 10 / 70%) 42%, rgb(8 8 10 / 10%) 80%), linear-gradient(0deg, rgb(8 8 10 / 92%) 0%, transparent 50%); }
	.count { position: absolute; top: 18px; right: 22px; font: var(--t-title2); font-variant-numeric: tabular-nums; }
	.count span { color: var(--on-art-muted); font: var(--t-subhead); }
	.hero-in { display: flex; align-items: flex-end; gap: 28px; padding: 40px 32px 20px; max-width: 920px; }
	.poster { width: 168px; flex: none; border-radius: 10px; overflow: hidden; box-shadow: 0 20px 50px rgb(0 0 0 / 55%); }
	.copy { min-width: 0; display: flex; flex-direction: column; gap: 10px; }
	.facts { color: var(--on-art-muted); font: var(--t-subhead); overflow-wrap: anywhere; }
	.name { font: var(--t-large-title); letter-spacing: var(--t-large-title-track); overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
	.name a:hover { text-decoration: underline; text-underline-offset: 5px; text-decoration-thickness: 2px; }
	.ask-line { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; font: var(--t-callout); color: var(--on-art-muted); }
	.ask-line b { color: var(--on-art); }
	.part { display: inline-flex; align-items: center; gap: 6px; align-self: flex-start; padding: 5px 12px; border-radius: var(--r-pill); background: rgb(255 255 255 / 14%); font: var(--t-subhead); font-weight: 600; overflow-wrap: anywhere; -webkit-backdrop-filter: blur(10px); backdrop-filter: blur(10px); }
	.story { max-width: 62ch; color: rgb(255 255 255 / 80%); font: var(--t-callout); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
	.pill { align-self: flex-start; }
	.decide { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; padding: 12px 32px 26px; }
	.act { min-height: 50px; display: inline-flex; align-items: center; justify-content: center; gap: 8px; padding: 0 22px; border-radius: 14px; font: var(--t-headline); font-size: 16px; white-space: nowrap; transition: transform var(--dur-fast) var(--ease-out), background var(--dur-fast) var(--ease-out); }
	.act:active { transform: scale(0.97); }
	.yes { background: #fff; color: #111; }
	.yes:hover { background: rgb(255 255 255 / 88%); }
	.opt { background: rgb(255 255 255 / 16%); color: #fff; -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); }
	.opt:hover { background: rgb(255 255 255 / 26%); }
	.no { background: rgb(255 255 255 / 10%); color: #fff; }
	.no:hover { background: var(--ha-red); }
	kbd { margin-left: 4px; padding: 1px 6px; border-radius: 5px; background: rgb(0 0 0 / 12%); font: 600 11px/1.4 var(--font-mono); opacity: 0.75; }
	.no kbd { background: rgb(255 255 255 / 16%); }
	.bar-line { color: var(--on-art-muted); font: var(--t-callout); }
	.nav { display: inline-flex; gap: 8px; margin-left: auto; }
	.arrow { width: 46px; height: 46px; display: grid; place-items: center; border-radius: 50%; background: rgb(255 255 255 / 14%); color: #fff; }
	.arrow:hover:not(:disabled) { background: rgb(255 255 255 / 26%); }
	.arrow:disabled { opacity: 0.35; }
	.flip { display: grid; transform: scaleX(-1); }
	.keys { font: var(--t-footnote); text-align: center; }
	.keys kbd { background: var(--surface-2); opacity: 1; color: var(--text); }
	.queue-head { display: flex; align-items: center; justify-content: space-between; margin: 22px 0 10px; }
	.queue-title { font: var(--t-headline); }
	.choose { min-height: 36px; padding: 0 12px; border-radius: var(--r-pill); color: var(--accent-text); font: var(--t-subhead); font-weight: 600; }
	.choose:hover { background: var(--accent-soft); }
	.queue { position: relative; display: flex; gap: 12px; overflow-x: auto; padding: 6px 4px 12px; scroll-snap-type: x proximity; scrollbar-width: thin; }
	.q { position: relative; flex: none; width: 104px; scroll-snap-align: center; }
	.q-btn { display: flex; flex-direction: column; gap: 6px; width: 100%; text-align: left; border-radius: 10px; }
	.q-art { display: block; border-radius: 10px; overflow: hidden; opacity: 0.6; transition: opacity var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.q-btn:hover .q-art { opacity: 0.9; }
	.q-btn[aria-current='true'] .q-art { opacity: 1; transform: translateY(-3px); box-shadow: 0 0 0 3px var(--text), var(--shadow-lift); }
	.q-name { font: var(--t-footnote); color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.q-btn[aria-current='true'] .q-name { color: var(--text); font-weight: 600; }
	.chosen .q-art { opacity: 1; box-shadow: 0 0 0 3px var(--accent); }
	.tick { position: absolute; top: 6px; left: 6px; width: 18px; height: 18px; accent-color: var(--accent-fill); }
	.calm { position: relative; isolation: isolate; overflow: hidden; display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 72px 20px; border-radius: 22px; text-align: center; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.calm-glow { position: absolute; inset: -40% -10% auto; height: 120%; z-index: -1; background: radial-gradient(closest-side, color-mix(in srgb, var(--ok) 22%, transparent), transparent); }
	.calm-ic { width: 60px; height: 60px; display: grid; place-items: center; border-radius: 50%; background: var(--ok); color: #fff; }
	.calm h2 { font: var(--t-title1); letter-spacing: var(--t-title1-track); }
	.calm p { color: var(--text-muted); max-width: 44ch; }
	.calm .calm-sub { margin-top: 18px; font: var(--t-footnote); font-weight: 600; text-transform: uppercase; letter-spacing: 0.08em; }
	.calm-row { display: flex; flex-wrap: wrap; justify-content: center; gap: 12px; }
	.calm-row li { display: flex; flex-direction: column; align-items: center; gap: 6px; }
	.mini { width: 64px; border-radius: 6px; overflow: hidden; opacity: 0.85; }
	@media (max-width: 900px) {
		.hero { min-height: 0; }
		.hero-in { padding: 64px 22px 14px; gap: 18px; }
		.poster { width: 120px; }
		.name { font: var(--t-title1); letter-spacing: var(--t-title1-track); }
		.decide { padding: 10px 22px 22px; }
	}
	@media (max-width: 600px) {
		.hero { border-radius: 18px; }
		.hero::after { background: linear-gradient(0deg, rgb(8 8 10 / 96%) 0%, rgb(8 8 10 / 80%) 50%, rgb(8 8 10 / 25%) 100%); }
		.hero-in { flex-direction: column; align-items: flex-start; padding: 56px 16px 10px; gap: 14px; }
		.count { top: 14px; right: 16px; font: var(--t-title3); }
		.poster { width: 104px; }
		.name { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
		.decide { display: grid; grid-template-columns: 1fr 1fr; padding: 8px 16px 16px; gap: 8px; }
		.act { padding: 0 12px; min-height: 48px; font-size: 15px; }
		.yes { grid-column: 1 / -1; }
		.act kbd { display: none; }
		.nav { grid-column: 1 / -1; justify-content: space-between; margin: 0; }
		.arrow { width: 44px; height: 44px; }
		.keys { display: none; }
		.queue { margin: 0 calc(-1 * var(--gutter)); padding: 6px var(--gutter) 12px; }
		.q { width: 86px; }
		.calm { padding: 56px 16px; }
	}
	@media (pointer: coarse) { .keys { display: none; } .act kbd { display: none; } }
	@media (prefers-reduced-motion: reduce) { .hero, .bd { animation: none; } }
</style>

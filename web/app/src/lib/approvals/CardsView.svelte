<script lang="ts">
	import { glide } from '../ui/transitions.ts';
	// 1 Cards: a grid of backdrop cards. Poster, title, who asked and when, the
	// part asked for, and big Approve / Turn down right on the card.
	import Poster from '../Poster.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import Icon from '../Icon.svelte';
	import StatusPill from '../ui/StatusPill.svelte';
	import KidMark from './KidMark.svelte';
	import { to } from '../paths.ts';
	import { ago, dateTime } from '../format.ts';
	import { kindWord, type ApprovalsCtx } from './approvals.ts';
	import { askTabs } from '../wishlist.ts';

	let { ctx }: { ctx: ApprovalsCtx } = $props();
	$effect(() => ctx.shown.slice(0, 24).forEach((a) => ctx.want(a)));
	const others = $derived(askTabs.filter((t) => t.tab !== ctx.tab && ctx.counts[t.tab] > 0));
</script>

{#if ctx.shown.length === 0}
	<section class="calm" data-testid="calm">
		<div class="ghosts" aria-hidden="true"><span></span><span></span><span></span></div>
		<div class="calm-text">
			<span class="calm-ic"><Icon name="check" size={22} /></span>
			<h2>{ctx.tab === 'waiting' ? 'Nothing waits for you' : `Nothing ${askTabs.find((t) => t.tab === ctx.tab)?.label.toLowerCase()} here`}</h2>
			<p class="muted">{ctx.tab === 'waiting' ? 'New asks land here as cards the moment someone asks.' : 'Asks show here once they get to this point.'}</p>
			{#if others.length}
				<div class="jump">
					{#each others as t (t.tab)}<button type="button" onclick={() => ctx.setTab(t.tab)}>{t.label} <b>{ctx.counts[t.tab]}</b></button>{/each}
				</div>
			{/if}
		</div>
	</section>
{:else}
	<ul class="grid" aria-label="Asks">
		{#each ctx.shown as a, i (a.id)}
			{@const who = ctx.who(a)}
			{@const d = ctx.details(a)}
			{@const art = d?.backdrop ?? a.poster ?? ''}
			<li animate:glide class="rcard rise" class:chosen={ctx.isChosen(a.id)} class:sending={ctx.isPending(a.id)} class:music={a.kind === 'artist' || !d?.backdrop} style="--i: {i}" data-testid="ask">
				{#if art}<div class="bd" style:background-image={`url("${to(art)}")`} aria-hidden="true"></div>{/if}
				<div class="top">
					{#if ctx.choosable}
						<label class="tick"><input type="checkbox" aria-label="Choose {a.name}" checked={ctx.isChosen(a.id)} onchange={() => ctx.toggleChosen(a.id)} /></label>
					{/if}
					<span class="tag">{kindWord(a.kind)}</span>
					{#if a.state !== 'asked'}<StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} testid="ask-state" />{/if}
				</div>
				<div class="main">
					<a class="poster" href={ctx.href(a) ? to(ctx.href(a)) : undefined} tabindex="-1" aria-hidden="true"><Poster src={a.poster} name={a.name} /></a>
					<div class="info">
						<p class="year">{a.year ?? ''}</p>
						<h2 class="name">{#if ctx.href(a)}<a href={to(ctx.href(a))}>{a.name}</a>{:else}{a.name}{/if}</h2>
						{#if ctx.what(a)}<p class="part" data-testid={a.kind === 'artist' ? 'ask-albums' : 'ask-seasons'}><Icon name={a.kind === 'artist' ? 'music' : 'tv'} size={14} />{ctx.what(a)}</p>{/if}
						<p class="who" data-testid="asked-by">
							<Avatar name={who.name} src={who.avatar} size={22} />
							<span class="who-name">{who.name}</span>
							<KidMark label={who.kid} size="sm" />
							<time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time>
						</p>
						{#if a.note}<p class="note">“{a.note}”</p>{/if}
					</div>
				</div>
				<div class="acts">
					{#if ctx.approver && a.state === 'asked' && !ctx.isPending(a.id)}
						<button class="big yes" type="button" data-testid="ask-approve" aria-label="Approve {a.name}" onclick={() => ctx.approve([a])}><Icon name="check" size={18} />Approve</button>
						<button class="big no" type="button" data-testid="ask-decline" aria-label="Turn down {a.name}" onclick={() => ctx.turnDown([a])}><Icon name="x" size={18} />Turn down</button>
						<button class="opts" type="button" aria-label="Approve {a.name} with options" title="Approve with options" onclick={() => ctx.withOptions(a)}><Icon name="sparkle" size={18} /><span class="opts-word">Options</span></button>
					{:else if ctx.isPending(a.id)}
						<span class="sending-line">Sending in a moment…</span>
					{/if}
					<button class="review" type="button" aria-label="Review {a.name}" onclick={(e) => ctx.review(a, e.currentTarget)}>Review</button>
				</div>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr)); gap: 16px; }
	.rcard { position: relative; isolation: isolate; overflow: hidden; display: flex; flex-direction: column; gap: 12px; padding: 14px; border-radius: var(--r-panel); background: #16171a; color: var(--on-art); box-shadow: var(--shadow-card); transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out), opacity var(--dur-fast) var(--ease-out); }
	.rcard:hover { box-shadow: var(--shadow-lift); }
	.rcard.chosen { box-shadow: 0 0 0 2px var(--accent), var(--shadow-lift); }
	.rcard.sending { opacity: 0.6; }
	.bd { position: absolute; inset: 0; z-index: -2; background-size: cover; background-position: center 30%; transition: transform var(--dur-slow) var(--ease-out); }
	.music .bd { filter: blur(22px) saturate(1.3); transform: scale(1.25); }
	.rcard:hover .bd { transform: scale(1.04); }
	.music:hover .bd { transform: scale(1.3); }
	.rcard::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(100deg, rgb(10 10 12 / 92%) 0%, rgb(10 10 12 / 78%) 48%, rgb(10 10 12 / 45%) 100%), linear-gradient(0deg, rgb(10 10 12 / 85%) 0%, transparent 45%); }
	.top { display: flex; align-items: center; gap: 8px; min-height: 26px; }
	.tick input { width: 18px; height: 18px; accent-color: var(--accent-fill); display: block; }
	.tag { padding: 2px 8px; border-radius: var(--r-sm); background: rgb(255 255 255 / 14%); color: var(--on-art); font: var(--t-footnote); font-weight: 600; letter-spacing: 0.02em; -webkit-backdrop-filter: blur(8px); backdrop-filter: blur(8px); }
	.main { display: flex; gap: 14px; align-items: flex-start; }
	.poster { width: 84px; flex: none; border-radius: 8px; overflow: hidden; box-shadow: 0 8px 24px rgb(0 0 0 / 45%); }
	.info { min-width: 0; display: flex; flex-direction: column; gap: 6px; }
	.year { color: var(--on-art-muted); font: var(--t-footnote); min-height: 1em; }
	.name { font: var(--t-title3); letter-spacing: var(--t-title3-track); overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
	.name a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.part { display: inline-flex; align-items: center; gap: 6px; align-self: flex-start; padding: 3px 10px; border-radius: var(--r-pill); background: rgb(255 255 255 / 12%); font: var(--t-footnote); font-weight: 600; overflow-wrap: anywhere; }
	.who { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; color: var(--on-art-muted); font: var(--t-footnote); }
	.who-name { color: var(--on-art); font-weight: 600; }
	.who time::before { content: '· '; }
	.note { font: var(--t-footnote); color: var(--on-art-muted); overflow-wrap: anywhere; }
	.acts { display: flex; align-items: center; gap: 8px; margin-top: auto; }
	.big { flex: 1 1 0; min-width: 0; min-height: 44px; display: inline-flex; align-items: center; justify-content: center; gap: 6px; border-radius: var(--r-btn); font: var(--t-headline); font-size: 15px; white-space: nowrap; transition: background var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.big:active { transform: scale(0.97); }
	.yes { background: var(--ok); color: #fff; }
	.yes:hover { background: color-mix(in srgb, var(--ok) 86%, #fff); }
	.no { background: rgb(255 255 255 / 12%); color: var(--on-art); }
	.no:hover { background: color-mix(in srgb, var(--ha-red) 70%, transparent); }
	.opts { width: 44px; height: 44px; flex: none; display: grid; place-items: center; border-radius: var(--r-btn); background: rgb(255 255 255 / 12%); color: var(--on-art); }
	.opts-word { display: none; }
	.opts:hover { background: rgb(255 255 255 / 22%); }
	.review { min-height: 44px; padding: 0 12px; border-radius: var(--r-btn); color: var(--on-art); font: var(--t-subhead); font-weight: 600; }
	.review:hover { background: rgb(255 255 255 / 12%); }
	.sending-line { flex: 1; font: var(--t-footnote); color: var(--on-art-muted); }
	.acts .review:only-child { margin-left: auto; }
	.calm { position: relative; display: grid; place-items: center; min-height: 360px; padding: 32px 16px; border-radius: var(--r-panel); overflow: hidden; }
	.ghosts { position: absolute; inset: 0; display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; padding: 16px; opacity: 0.55; }
	.ghosts span { border-radius: var(--r-panel); border: 1.5px dashed var(--line-strong); }
	.calm-text { position: relative; display: flex; flex-direction: column; align-items: center; gap: 8px; max-width: 380px; padding: 24px; text-align: center; border-radius: var(--r-panel); background: var(--bg); box-shadow: 0 0 0 1px var(--line); }
	.calm-ic { width: 44px; height: 44px; display: grid; place-items: center; border-radius: 50%; background: color-mix(in srgb, var(--ok) 18%, transparent); color: var(--ok-text); }
	.calm h2 { font: var(--t-title3); }
	.jump { display: flex; flex-wrap: wrap; justify-content: center; gap: 8px; margin-top: 6px; }
	.jump button { min-height: 36px; padding: 0 14px; border-radius: var(--r-pill); background: var(--surface-2); font: var(--t-subhead); }
	.jump button:hover { background: var(--surface-3); }
	.jump b { margin-left: 4px; color: var(--text-muted); font-weight: 600; }
	@media (max-width: 600px) {
		.grid { gap: 12px; }
		.rcard { padding: 12px; }
		.poster { width: 72px; }
		.ghosts { display: none; }
		.calm { min-height: 0; padding: 8px 0; }
		.acts { display: grid; grid-template-columns: 1fr 1fr; }
		.opts { width: auto; gap: 6px; display: inline-flex; align-items: center; justify-content: center; font: var(--t-subhead); font-weight: 600; }
		.opts-word { display: inline; }
		.review { background: rgb(255 255 255 / 8%); }
	}
	@media (prefers-reduced-motion: reduce) { .bd, .rcard { transition: none; } }
</style>

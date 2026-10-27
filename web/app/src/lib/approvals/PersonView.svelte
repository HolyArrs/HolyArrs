<script lang="ts">
	// 2 By person: one row per person who asked, a sideways shelf of posters.
	// Hover (or tap on a phone) a poster for Approve / Turn down; a whole row
	// can be approved at once.
	import Poster from '../Poster.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import Icon from '../Icon.svelte';
	import Button from '../ui/Button.svelte';
	import StatusPill from '../ui/StatusPill.svelte';
	import KidMark from './KidMark.svelte';
	import { to } from '../paths.ts';
	import { ago } from '../format.ts';
	import { personGroups, kindWord, type ApprovalsCtx } from './approvals.ts';
	import { askTabs } from '../wishlist.ts';

	let { ctx }: { ctx: ApprovalsCtx } = $props();
	const groups = $derived(personGroups(ctx.shown));
	// Everyone who has asked for anything, for the calm state.
	const everyone = $derived(personGroups(ctx.viewed));
	let armed = $state(0);
	const waitingRow = $derived(ctx.approver && ctx.tab === 'waiting');
</script>

{#if ctx.shown.length === 0}
	<section class="calm" data-testid="calm">
		<span class="calm-ic"><Icon name="check" size={24} /></span>
		<h2>{ctx.tab === 'waiting' ? 'Everyone has an answer' : `Nothing ${askTabs.find((t) => t.tab === ctx.tab)?.label.toLowerCase()} here`}</h2>
		<p class="muted">{ctx.tab === 'waiting' ? 'When someone asks, they get their own row here.' : 'Rows show here per person once asks get to this point.'}</p>
		{#if everyone.length}
			<ul class="faces" aria-label="People who asked">
				{#each everyone.slice(0, 8) as g (g.id)}
					<li><Avatar name={g.name} src={ctx.who(g.asks[0]).avatar} size={44} /><span>{g.name}</span><small>{g.asks.length} {g.asks.length === 1 ? 'ask' : 'asks'}</small></li>
				{/each}
			</ul>
		{/if}
	</section>
{:else}
	<div class="people">
		{#each groups as g (g.id)}
			{@const who = ctx.who(g.asks[0])}
			{@const open = g.asks.filter((a) => a.state === 'asked' && !ctx.isPending(a.id))}
			<section class="person" aria-labelledby="p-{g.id}">
				<header class="person-head">
					<Avatar name={who.name} src={who.avatar} size={32} />
					<h2 id="p-{g.id}" class="person-name">{g.name}<span class="n">{" · "}{g.asks.length}</span></h2>
					<KidMark label={who.kid} size="sm" />
					<span class="grow"></span>
					{#if ctx.choosable}
						<label class="pick-row"><input type="checkbox" checked={g.asks.every((a) => ctx.isChosen(a.id))} onchange={() => ctx.chooseAll(g.asks)} />Choose {g.asks.length === 1 ? 'it' : `${g.name}’s`}</label>
					{/if}
					{#if waitingRow && open.length > 1}
						<Button size="sm" variant="primary" icon="check" type="button" onclick={() => ctx.approve(open)}>Approve all from {g.name}</Button>
						<Button size="sm" icon="x" type="button" aria-label="Turn down all from {g.name}" onclick={() => ctx.turnDown(open)}>Turn down all</Button>
					{/if}
				</header>
				<ul class="shelf" aria-label="Asks from {g.name}">
					{#each g.asks as a (a.id)}
						<li class="tile" class:armed={armed === a.id} class:chosen={ctx.isChosen(a.id)} class:sending={ctx.isPending(a.id)} data-testid="ask">
							<div class="art" class:square={a.kind === 'artist'}>
								<button class="art-btn" type="button" aria-label="Show actions for {a.name}" aria-expanded={armed === a.id} onclick={() => (armed = armed === a.id ? 0 : a.id)}><Poster src={a.poster} name={a.name} /></button>
								{#if ctx.choosable}<input class="tick" type="checkbox" aria-label="Choose {a.name}" checked={ctx.isChosen(a.id)} onchange={() => ctx.toggleChosen(a.id)} />{/if}
								{#if ctx.what(a)}<span class="part">{ctx.what(a)}</span>{/if}
								<div class="over">
									{#if ctx.approver && a.state === 'asked' && !ctx.isPending(a.id)}
										<button class="o yes" type="button" data-testid="ask-approve" onclick={() => ((armed = 0), ctx.approve([a]))}><Icon name="check" size={18} />Approve</button>
										<button class="o no" type="button" data-testid="ask-decline" onclick={() => ((armed = 0), ctx.turnDown([a]))}><Icon name="x" size={18} />Turn down</button>
										<button class="o plain" type="button" onclick={() => ((armed = 0), ctx.withOptions(a))}>Options…</button>
									{:else if ctx.isPending(a.id)}
										<span class="o-line">Sending in a moment…</span>
									{:else}
										<StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} testid="ask-state" />
									{/if}
									<button class="o plain" type="button" onclick={(e) => ((armed = 0), ctx.review(a, e.currentTarget))}>Review</button>
								</div>
							</div>
							<p class="t-name">{#if ctx.href(a)}<a href={to(ctx.href(a))}>{a.name}</a>{:else}{a.name}{/if}</p>
							<p class="t-meta">{kindWord(a.kind)}{a.year ? ` · ${a.year}` : ''} · {ago(a.asked_at)}</p>
						</li>
					{/each}
				</ul>
			</section>
		{/each}
	</div>
{/if}

<style>
	.people { display: flex; flex-direction: column; gap: 28px; }
	.person-head { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 10px; margin-bottom: 12px; }
	.person-name { font: var(--t-title3); letter-spacing: var(--t-title3-track); overflow-wrap: anywhere; }
	.n { color: var(--text-muted); font-weight: 500; }
	.grow { flex: 1; }
	.pick-row { display: inline-flex; align-items: center; gap: 6px; min-height: 36px; color: var(--text-muted); font: var(--t-subhead); }
	.pick-row input, .tick { width: 18px; height: 18px; accent-color: var(--accent-fill); }
	.shelf { display: flex; gap: 14px; overflow-x: auto; scroll-snap-type: x mandatory; padding: 4px 4px 12px; scrollbar-width: thin; overscroll-behavior-x: contain; }
	.tile { flex: none; width: 168px; scroll-snap-align: start; }
	.art { position: relative; border-radius: var(--r-card); overflow: hidden; box-shadow: var(--shadow-card); transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out); }
	.art-btn { display: block; width: 100%; }
	.art-btn :global(.poster) { border-radius: 0; }
	.armed .art { transform: translateY(-3px); box-shadow: var(--shadow-lift); }
	.chosen .art { box-shadow: 0 0 0 3px var(--accent), var(--shadow-lift); }
	.sending { opacity: 0.6; }
	.tick { position: absolute; top: 8px; left: 8px; z-index: 2; }
	.part { position: absolute; left: 8px; right: 8px; bottom: 8px; padding: 3px 8px; border-radius: var(--r-pill); background: rgb(0 0 0 / 62%); color: #fff; font: var(--t-caption); font-weight: 600; text-align: center; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; -webkit-backdrop-filter: blur(8px); backdrop-filter: blur(8px); pointer-events: none; transition: opacity var(--dur-fast); }
	.over { position: absolute; inset: 0; z-index: 3; display: flex; flex-direction: column; justify-content: flex-end; gap: 6px; padding: 10px; background: linear-gradient(180deg, rgb(0 0 0 / 10%) 0%, rgb(0 0 0 / 82%) 55%); opacity: 0; visibility: hidden; transition: opacity var(--dur-fast) var(--ease-out), visibility 0s linear var(--dur-fast); }
	/* Mouse: hover shows the actions. Touch: only a tap (armed) does, so a first tap never lands on a button. */
	.tile:has(:focus-visible) .over, .armed .over { opacity: 1; visibility: visible; transition: opacity var(--dur-fast) var(--ease-out); }
	.armed .part, .tile:has(:focus-visible) .part { opacity: 0; }
	@media (hover: hover) {
		.tile:hover .art { transform: translateY(-3px); box-shadow: var(--shadow-lift); }
		.tile:hover .over { opacity: 1; visibility: visible; transition: opacity var(--dur-fast) var(--ease-out); }
		.tile:hover .part { opacity: 0; }
	}
	.o { min-height: 36px; display: inline-flex; align-items: center; justify-content: center; gap: 6px; border-radius: var(--r-btn); font: var(--t-subhead); font-weight: 600; color: #fff; }
	.o.yes { background: var(--ok); }
	.o.no { background: rgb(255 255 255 / 18%); }
	.o.no:hover { background: var(--ha-red); }
	.o.plain { min-height: 30px; background: transparent; color: rgb(255 255 255 / 86%); }
	.o.plain:hover { color: #fff; text-decoration: underline; }
	.o-line { color: #fff; font: var(--t-footnote); text-align: center; }
	.over :global(.ui-status) { align-self: center; }
	.t-name { margin-top: 8px; font: var(--t-subhead); font-weight: 600; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow-wrap: anywhere; }
	.t-name a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.t-meta { color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.calm { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 48px 16px; text-align: center; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.calm-ic { width: 48px; height: 48px; display: grid; place-items: center; border-radius: 50%; background: color-mix(in srgb, var(--ok) 18%, transparent); color: var(--ok-text); }
	.calm h2 { font: var(--t-title3); }
	.faces { display: flex; flex-wrap: wrap; justify-content: center; gap: 18px; margin-top: 14px; }
	.faces li { display: flex; flex-direction: column; align-items: center; gap: 4px; font: var(--t-footnote); opacity: 0.85; }
	.faces small { color: var(--text-muted); }
	@media (max-width: 600px) {
		.people { gap: 22px; }
		.tile { width: 132px; }
		.shelf { gap: 10px; margin: 0 calc(-1 * var(--gutter)); padding: 4px var(--gutter) 12px; scroll-padding: 0 var(--gutter); }
		.person-head :global(.ui-btn) { flex: 1 1 auto; }
		.o { min-height: 40px; }
	}
	@media (prefers-reduced-motion: reduce) { .art, .over { transition: none; } }
</style>
